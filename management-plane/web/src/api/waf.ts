/**
 * One typed seam between the portal's screens and the three pillar services.
 *
 * Everything here is either (a) the generic `metap` record API with WAF's entity names filled in,
 * or (b) one of the custom endpoints the services added for the work `docs/13-screen-api-map.md`
 * flagged as "phải tự code" (DNS verification, origin probe, scan dispatch, correlation, alert
 * delivery). Screens import from here rather than calling the transport directly so an entity name
 * or a field name is written once, in one file, instead of spread across a dozen components.
 *
 * **Generic record CRUD (list/get/create/update/delete/transition/aggregate) now comes from
 * `@metap/platform-ui`'s `graphqlRecords.ts`** (2026-09-05,
 * `docs/features/30-graphql-generic-record-hooks.md` in `metap-docs`) — this file used to define
 * `useRecords`/`useRecord`/`useAggregate`/the 4 mutations itself, until it became clear on read
 * that every one of them was already 100% entity-agnostic (took `entity: string`, built its query
 * from that entity's own metadata, no WAF-specific field anywhere) and belonged in the shared
 * library, the same way `useApiQuery`/`useApiMutation` already do for REST. Re-exported here under
 * their original names (`useRecords` etc.) so none of the 130+ call sites across `pages/*.tsx`
 * needed to change — only this file's own definitions moved out. What's left in this file now is
 * genuinely WAF-specific: the `ENTITIES` map, `Zone`'s own data shape, and the 7 custom actions
 * below that have no generic equivalent (DNS verification, origin probe, config-state sync, scan
 * dispatch, alert test/evaluate, incident correlation).
 *
 * Transport stays GraphQL, through `waf-graphql-gateway`, not REST (2026-09-04 — see
 * `../../graphql-gateway/README.md` for the 8 custom fields). `/graphql` is a fixed,
 * entity-agnostic path (the dev proxy and prod nginx both route it straight to the gateway, see
 * `../../vite.config.ts`) — unlike REST's `/api/:entity*`, there's no per-entity routing to do
 * here at all.
 */
import {
  createGraphQLRecord,
  deleteGraphQLRecord,
  graphqlFetch,
  transitionGraphQLRecord,
  updateGraphQLRecord,
  useGraphQLAggregate,
  useGraphQLRecord,
  useGraphQLRecords,
  useInvalidateGraphQLRecords,
  type AggregateRow,
  type AggregateSpec,
  type GraphQLListResponse,
  type GraphQLRecord,
  type GraphQLSingleResponse,
} from "@metap/platform-ui";

const GRAPHQL_PATH = "/graphql";

export const ENTITIES = {
  zones: "waf.zones",
  domains: "waf.domains",
  ddosPolicies: "waf.ddos_policies",
  firewallRules: "waf.firewall_rules",
  ipAccessLists: "waf.ip_access_lists",
  scanJobs: "waf.scan_jobs",
  scanFindings: "waf.scan_findings",
  securityEvents: "waf.security_events",
  incidents: "waf.incidents",
  alertPolicies: "waf.alert_policies",
  alertNotifications: "waf.alert_notifications",
} as const;

/** Alias kept under this file's original name — every call site across `pages/*.tsx` already
 *  spells it this way. Same shape as `@metap/platform-ui`'s `GraphQLRecord<T>`. */
export type WafRecord<TData = Record<string, unknown>> = GraphQLRecord<TData>;
export type ListResponse<T> = GraphQLListResponse<T>;
export type SingleResponse<T> = GraphQLSingleResponse<T>;
export type { AggregateSpec, AggregateRow };

export type ZoneData = {
  hostname?: string;
  originAddress?: string;
  status?: string;
  protectionMode?: string;
  configVersion?: number;
  hasConfig?: boolean;
  /** Set automatically by `routes::zone_domain_guard` from the hostname's apex — never chosen by
   *  hand. Ownership verification itself lives on the `Domain` this points at (`DomainData`
   *  below); `verificationStatus` here is a technical mirror kept in sync by `verifyDomainDns`'s
   *  cascade, not something this app writes directly. */
  domainId?: string;
  verificationStatus?: string;
  dnsRoutingStatus?: string;
  lastDnsCheckAt?: string;
};

export type Zone = WafRecord<ZoneData>;

export type DomainData = {
  apexDomain?: string;
  verificationToken?: string;
  verificationMethod?: string;
  verificationStatus?: string;
};

export type Domain = WafRecord<DomainData>;

/* ------------------------------------------------------------------ generic record CRUD */
// Re-exported under their pre-extraction names — see this file's own doc comment above.

export const useRecords = useGraphQLRecords;
export const useRecord = useGraphQLRecord;
export const useAggregate = useGraphQLAggregate;
export const createRecord = createGraphQLRecord;
export const updateRecord = updateGraphQLRecord;
export const deleteRecord = deleteGraphQLRecord;
export const transitionRecord = transitionGraphQLRecord;

/** ISO timestamp `days` ago — the `since` every dashboard window is built from. */
export function daysAgo(days: number): string {
  return new Date(Date.now() - days * 24 * 60 * 60 * 1000).toISOString();
}

/* ------------------------------------------------------------------ custom endpoints */

/** Imperative GraphQL call for the 7 custom actions below (plain async functions, not hooks) —
 *  rides the existing session cookie (`graphqlFetch` with no `token`), same as
 *  `useGraphQLRecords`/etc., no `GET /auth/token` round trip first (removed 2026-09-04). */
async function graphqlAuthed<T>(
  query: string,
  variables?: Record<string, unknown>,
): Promise<T> {
  return graphqlFetch<T>(GRAPHQL_PATH, query, variables);
}

/** Routing-only now — see `DomainVerifyResult`/`verifyDomainDns` for ownership, moved to `Domain`
 *  (Increment 3). */
export type DnsVerifyResult = {
  data: {
    zone: Zone;
    dnsRouted: boolean;
    checked: { cname: string[]; expectedTarget: string };
  };
};

export function verifyDns(zoneId: string) {
  return graphqlAuthed<{ result: DnsVerifyResult }>(
    `mutation VerifyZoneDns($zoneId: ID!) { result: verifyZoneDns(zoneId: $zoneId) }`,
    { zoneId },
  ).then((r) => r.result);
}

export type DomainVerifyResult = {
  data: {
    domain: Domain;
    ownershipVerified: boolean;
    checked: { txt: string[]; expectedToken: string };
  };
};

/** Domain-ownership check (Increment 3) — verified once per apex domain, cascades onto every
 *  `Zone` under it (each zone's own `verificationStatus` mirror gets updated server-side). */
export function verifyDomainDns(domainId: string) {
  return graphqlAuthed<{ result: DomainVerifyResult }>(
    `mutation VerifyDomainDns($domainId: ID!) { result: verifyDomainDns(domainId: $domainId) }`,
    { domainId },
  ).then((r) => r.result);
}

export type OriginTestResult = {
  data: {
    reachable: boolean;
    status?: number;
    latencyMs: number;
    url: string;
    error?: string;
  };
};

export function testOrigin(zoneId: string) {
  return graphqlAuthed<{ result: OriginTestResult }>(
    `mutation TestZoneOrigin($zoneId: ID!) { result: testZoneOrigin(zoneId: $zoneId) }`,
    { zoneId },
  ).then((r) => r.result);
}

/** Recomputes `Zone.hasConfig` from the zone's real policies/rules. Called after any policy/rule
 *  create or delete — the `activate` workflow guard reads that flag, and `PolicyCondition` has no
 *  way to count related records itself. Idempotent, so firing it optimistically is safe. */
export function syncConfigState(zoneId: string) {
  return graphqlAuthed<{
    result: { data: { hasConfig: boolean; changed: boolean } };
  }>(
    `mutation SyncZoneConfigState($zoneId: ID!) { result: syncZoneConfigState(zoneId: $zoneId) }`,
    { zoneId },
  ).then((r) => r.result);
}

export function runScanJob(jobId: string) {
  return graphqlAuthed<{
    result: { data: { dispatched: boolean; detail: string } };
  }>(`mutation RunScanJob($jobId: ID!) { result: runScanJob(jobId: $jobId) }`, {
    jobId,
  }).then((r) => r.result);
}

export function testAlertPolicy(policyId: string) {
  return graphqlAuthed<{
    result: {
      data: { notificationId: string; delivered: boolean; detail: string };
    };
  }>(
    `mutation TestAlertPolicy($policyId: ID!) { result: testAlertPolicy(policyId: $policyId) }`,
    {
      policyId,
    },
  ).then((r) => r.result);
}

export function correlateIncidents(zoneId?: string) {
  return graphqlAuthed<{
    result: {
      data: {
        scannedEvents: number;
        createdIncidents: string[];
        skippedExisting: number;
      };
    };
  }>(
    `mutation CorrelateIncidents($zoneId: String) { result: correlateIncidents(zoneId: $zoneId) }`,
    {
      zoneId: zoneId ?? null,
    },
  ).then((r) => r.result);
}

export function evaluateAlerts() {
  return graphqlAuthed<{
    result: { data: { policiesEvaluated: number; fired: unknown[] } };
  }>(`mutation EvaluateAlerts { result: evaluateAlerts }`).then(
    (r) => r.result,
  );
}

/** Invalidates every WAF query at once. Coarse on purpose: these screens are small and a stale
 *  count on a dashboard is worse than one extra refetch after a mutation. Thin wrapper over
 *  `@metap/platform-ui`'s generic `useInvalidateGraphQLRecords` — kept under this file's original
 *  name/shape (a plain callback, not a re-exported hook alias) since call sites just invoke it. */
export function useInvalidateWaf() {
  return useInvalidateGraphQLRecords();
}
