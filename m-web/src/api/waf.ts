/**
 * One typed seam between the portal's screens and the three pillar services.
 *
 * Everything here is either (a) the generic `metap` record API with WAF's entity names filled in,
 * or (b) one of the custom actions the services added for the work `docs/13-screen-api-map.md`
 * flagged as "phải tự code" (DNS verification, origin probe, scan dispatch, correlation, alert
 * delivery). Screens import from here rather than calling the transport directly so an entity name
 * or a field name is written once, in one file, instead of spread across a dozen components.
 *
 * Transport is REST (roadmap 116): generic record CRUD comes from `@metap/platform-ui`'s
 * `records.ts` (`/api/{entity}*`), the custom actions are each entity's declared actions
 * (`POST /api/{entity}[/{id}]/actions/{action}`). Both go through the API gateway
 * (`../../m-gateway`), which routes every entity to the service that serves it.
 */
import {
  createRecord,
  deleteRecord,
  invokeAction,
  transitionRecord,
  updateRecord,
  useAggregate,
  useInvalidateRecords,
  useRecord,
  useRecords,
  type AggregateRow,
  type AggregateSpec,
  type ApiRecord,
  type SingleResponse,
  useAuth,
} from "@metap/platform-ui";
import { useQuery } from "@tanstack/react-query";

export const ENTITIES = {
  zones: "waf.zones",
  domains: "waf.domains",
  accounts: "waf.accounts",
  ddosPolicies: "waf.ddos_policies",
  firewallRules: "waf.firewall_rules",
  ipAccessLists: "waf.ip_access_lists",
  policyVersions: "waf.policy_versions",
  scanJobs: "waf.scan_jobs",
  scanFindings: "waf.scan_findings",
  securityEvents: "waf.security_events",
  incidents: "waf.incidents",
  falsePositives: "waf.false_positive_reports",
  alertPolicies: "waf.alert_policies",
  alertNotifications: "waf.alert_notifications",
} as const;

/** A WAF record: metap's generic record with the entity's own `data` shape. */
export type WafRecord<TData = Record<string, unknown>> = ApiRecord<TData>;
export type ListResponse<T> = { data: ApiRecord<T>[] };
export type { AggregateSpec, AggregateRow, SingleResponse };

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

export {
  createRecord,
  deleteRecord,
  transitionRecord,
  updateRecord,
  useAggregate,
  useRecord,
  useRecords,
};

/** ISO timestamp `days` ago — the `since` every dashboard window is built from. */
export function daysAgo(days: number): string {
  return new Date(Date.now() - days * 24 * 60 * 60 * 1000).toISOString();
}

/* ------------------------------------------------------------------ custom actions */

/** Runs a declared action and hands back `{ data: <its result> }`, the shape these screens read. */
async function act<T>(
  entity: string,
  action: string,
  options: { id?: string; input?: Record<string, unknown> } = {},
): Promise<{ data: T }> {
  return { data: await invokeAction<T>(entity, action, options) };
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
  return act<DnsVerifyResult["data"]>(ENTITIES.zones, "verifyDns", {
    id: zoneId,
  });
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
  return act<DomainVerifyResult["data"]>(ENTITIES.domains, "verifyDns", {
    id: domainId,
  });
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
  return act<OriginTestResult["data"]>(ENTITIES.zones, "testOrigin", {
    id: zoneId,
  });
}

/** Recomputes `Zone.hasConfig` from the zone's real policies/rules. Called after any policy/rule
 *  create or delete — the `activate` workflow guard reads that flag, and `PolicyCondition` has no
 *  way to count related records itself. Idempotent, so firing it optimistically is safe. */
export function syncConfigState(zoneId: string) {
  return act<{ hasConfig: boolean; changed: boolean }>(
    ENTITIES.zones,
    "syncConfigState",
    { id: zoneId },
  );
}

export type PolicyVersionData = {
  zoneId: string;
  number: number;
  source: "baseline" | "change" | "rollback";
  summary?: string;
  restoredFrom?: number;
  snapshot?: PolicySnapshot;
};
export type PolicyVersion = WafRecord<PolicyVersionData>;
/** One zone's rules as a version holds them: entity name -> its records (id + field values). */
export type PolicySnapshot = Record<
  string,
  { id: string; data: Record<string, unknown> }[]
>;

export type RollbackResult = {
  restoredVersion: number;
  newVersion: number;
  created: number;
  updated: number;
  deleted: number;
};

/** Restores the zone's rules, DDoS policies and IP lists to what a version holds. The server
 *  records the rollback as a new version, so history is never rewritten. */
export function rollbackToVersion(versionId: string) {
  return act<RollbackResult>(ENTITIES.policyVersions, "rollback", {
    id: versionId,
  });
}

export function runScanJob(jobId: string) {
  return act<{ dispatched: boolean; detail: string }>(
    ENTITIES.scanJobs,
    "run",
    { id: jobId },
  );
}

export function testAlertPolicy(policyId: string) {
  return act<{ notificationId: string; delivered: boolean; detail: string }>(
    ENTITIES.alertPolicies,
    "test",
    { id: policyId },
  );
}

export function correlateIncidents(zoneId?: string) {
  return act<{
    scannedEvents: number;
    createdIncidents: string[];
    skippedExisting: number;
  }>(ENTITIES.incidents, "correlate", { input: zoneId ? { zoneId } : {} });
}

export function evaluateAlerts() {
  return act<{ policiesEvaluated: number; fired: unknown[] }>(
    ENTITIES.alertPolicies,
    "evaluate",
  );
}

/** Invalidates every WAF query at once. Coarse on purpose: these screens are small and a stale
 *  count on a dashboard is worse than one extra refetch after a mutation. */
export function useInvalidateWaf() {
  return useInvalidateRecords();
}

/* ------------------------------------------------------------------ plan and usage */

export type PlanLimits = {
  name: string;
  maxZones: number;
  maxRulesPerZone: number;
};
export type PlanUsage = {
  tier: string;
  plan: PlanLimits;
  zones: number;
  plans: PlanLimits[];
};

/** The tenant's plan, what it allows and what is used (0 in a limit means no limit). */
export function usePlanUsage() {
  const { status } = useAuth();
  return useQuery({
    queryKey: ["records", "plan-usage"],
    queryFn: () => invokeAction<PlanUsage>(ENTITIES.zones, "planUsage"),
    enabled: status === "authenticated",
  });
}
