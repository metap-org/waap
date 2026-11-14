/**
 * Module 1 — "Add a zone", the flow `docs/06-onboarding-rules-lists.md` and
 * `docs/11-onboarding-dns-resolution.md` describe end to end.
 *
 * This is the screen that most clearly is not generic CRUD: creating the record is one of five
 * steps, and the other four (prove domain ownership over DNS, check the origin answers, give the
 * zone its first protection, activate it) each call something bespoke. It also happens to be the
 * flow that exposes why `Zone.hasConfig` exists — the `activate` guard needs both a verified
 * hostname *and* at least one policy or rule, and a workflow guard cannot count related records.
 */
import { Fragment, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useTranslation } from "react-i18next";
import {
  Alert,
  AlertDescription,
  Button,
  Input,
  Label,
  PageHeader,
  SectionCard,
  Select,
  Stepper,
  StepperConnector,
  StepperGroup,
  StepperItem,
} from "@metap/ui";
import {
  ENTITIES,
  createRecord,
  syncConfigState,
  testOrigin,
  transitionRecord,
  useInvalidateWaf,
  useRecord,
  verifyDns,
  verifyDomainDns,
  type Domain,
  type OriginTestResult,
  type Zone,
} from "../api/waf";
import { ApiErrorMessage } from "@metap/platform-ui";
import { StatusBadge } from "../components/primitives";

type Step = 0 | 1 | 2 | 3;

export function OnboardingPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const invalidate = useInvalidateWaf();

  const [step, setStep] = useState<Step>(0);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const [hostname, setHostname] = useState("");
  const [originAddress, setOriginAddress] = useState("");
  const [protectionMode, setProtectionMode] = useState("monitor");
  const [zone, setZone] = useState<Zone | null>(null);
  // Ownership lives on the zone's parent `Domain` now (Increment 3) — `createZone` doesn't know
  // the domain id until the server assigns one (`routes::zone_domain_guard`), so this is fetched
  // separately once `zone` exists.
  const domainQuery = useRecord<Domain["data"]>(
    ENTITIES.domains,
    zone?.data.domainId,
  );
  const domain = domainQuery.data as Domain | undefined;
  const [ownershipVerified, setOwnershipVerified] = useState(false);
  const [routingResult, setRoutingResult] = useState<{
    dnsRouted: boolean;
    target: string;
  } | null>(null);
  const [originResult, setOriginResult] = useState<
    OriginTestResult["data"] | null
  >(null);

  async function guard(action: () => Promise<void>) {
    setBusy(true);
    setError(null);
    try {
      await action();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  }

  const createZone = () =>
    guard(async () => {
      // `domainId` is assigned server-side (`routes::zone_domain_guard`, auto-attached from the
      // hostname's apex) — this app never picks or creates a `Domain` by hand.
      const response = await createRecord<Zone["data"]>(ENTITIES.zones, {
        hostname: hostname.trim(),
        originAddress: originAddress.trim(),
        protectionMode,
        dnsRoutingStatus: "unknown",
        hasConfig: false,
        configVersion: 1,
      });
      setZone(response.data as Zone);
      invalidate();
      setStep(1);
    });

  const runVerifyOwnership = () =>
    guard(async () => {
      if (!domain) return;
      const response = await verifyDomainDns(domain.id);
      setOwnershipVerified(response.data.ownershipVerified);
      // The server cascades `verificationStatus` onto every zone under this domain, but `zone`
      // here is local component state, not a live query — mirror it optimistically rather than
      // waiting on a refetch, since we already know the cascade succeeded server-side.
      if (response.data.ownershipVerified && zone) {
        setZone({
          ...zone,
          data: { ...zone.data, verificationStatus: "verified" },
        });
      }
      invalidate();
    });

  const runVerifyRouting = () =>
    guard(async () => {
      if (!zone) return;
      const response = await verifyDns(zone.id);
      setZone(response.data.zone);
      setRoutingResult({
        dnsRouted: response.data.dnsRouted,
        target: response.data.checked.expectedTarget,
      });
      invalidate();
    });

  const runOriginTest = () =>
    guard(async () => {
      if (!zone) return;
      const response = await testOrigin(zone.id);
      setOriginResult(response.data);
    });

  /** Step 3 gives the zone its first real protection. A sensible default DDoS policy is enough to
   *  satisfy `hasConfig` and means a new zone is never live with nothing in front of it. */
  const addBaselineProtection = () =>
    guard(async () => {
      if (!zone) return;
      await createRecord(ENTITIES.ddosPolicies, {
        zoneId: zone.id,
        sensitivity: "medium",
        action: "challenge",
        requestRateThreshold: 500,
        burstWindow: 60,
        enabled: true,
      });
      // The flag the `activate` guard reads — recomputed from the records that now exist rather
      // than assumed, so a failed create can't leave the zone claiming a config it doesn't have.
      await syncConfigState(zone.id);
      invalidate();
      setStep(3);
    });

  const activate = () =>
    guard(async () => {
      if (!zone) return;
      // Re-read through the sync endpoint first: `zone.version` in this component is from before
      // `sync-config-state` bumped it, and the transition is version-gated.
      const fresh = await transitionRecord<Zone["data"]>(
        ENTITIES.zones,
        zone.id,
        "activate",
        zone.version + 1,
      );
      setZone(fresh.data as Zone);
      invalidate();
      navigate(`/zones/${zone.id}`);
    });

  const verified = zone?.data.verificationStatus === "verified";

  return (
    <div className="mx-auto max-w-3xl">
      <PageHeader
        title={t("waf.onboarding.title")}
        description={t("waf.onboarding.description")}
      />

      <Stepper className="mb-6">
        {(
          [
            "stepDetails",
            "stepVerify",
            "stepProtection",
            "stepActivate",
          ] as const
        ).map((labelKey, index) => (
          <Fragment key={labelKey}>
            {index > 0 ? <StepperConnector /> : null}
            <StepperGroup>
              <StepperItem
                variant={
                  index < step
                    ? "terminal"
                    : index === step
                      ? "current"
                      : "default"
                }
              >
                {t(`waf.onboarding.${labelKey}`)}
              </StepperItem>
            </StepperGroup>
          </Fragment>
        ))}
      </Stepper>

      {error ? (
        <Alert variant="destructive" className="mb-4">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      ) : null}

      {step === 0 ? (
        <SectionCard
          title={t("waf.onboarding.detailsTitle")}
          description={t("waf.onboarding.detailsDescription")}
        >
          <div className="grid gap-4">
            <div>
              <Label htmlFor="hostname">{t("waf.onboarding.hostname")}</Label>
              <Input
                id="hostname"
                placeholder="shop.example.com"
                value={hostname}
                onChange={(e) => setHostname(e.target.value)}
              />
            </div>
            <div>
              <Label htmlFor="origin">
                {t("waf.onboarding.originAddress")}
              </Label>
              <Input
                id="origin"
                placeholder="203.0.113.10 or origin.example.com"
                value={originAddress}
                onChange={(e) => setOriginAddress(e.target.value)}
              />
            </div>
            <div>
              <Label htmlFor="mode">{t("waf.onboarding.protectionMode")}</Label>
              <Select
                id="mode"
                value={protectionMode}
                onValueChange={(value) => setProtectionMode(String(value))}
                options={[
                  {
                    value: "monitor",
                    label: t("waf.onboarding.modeMonitor"),
                  },
                  {
                    value: "enforce",
                    label: t("waf.onboarding.modeEnforce"),
                  },
                ]}
              />
              <p className="mt-1 text-xs text-muted-foreground">
                {t("waf.onboarding.modeHint")}
              </p>
            </div>
            <div>
              <Button
                onClick={createZone}
                disabled={busy || !hostname.trim() || !originAddress.trim()}
              >
                {t("waf.onboarding.createZone")}
              </Button>
            </div>
          </div>
        </SectionCard>
      ) : null}

      {step === 1 && zone ? (
        <div className="grid gap-4">
          <SectionCard
            title={t("waf.onboarding.verifyTitle")}
            description={t("waf.onboarding.verifyDescription")}
          >
            {domainQuery.error ? (
              <ApiErrorMessage error={domainQuery.error} />
            ) : domain ? (
              <>
                <div className="rounded-md bg-muted/50 p-3 font-mono text-xs">
                  <div>
                    <span className="text-muted-foreground">
                      {t("waf.onboarding.recordName")}
                    </span>
                    _waf-verify.{domain.data.apexDomain}
                  </div>
                  <div>
                    <span className="text-muted-foreground">
                      {t("waf.onboarding.recordType")}
                    </span>
                    TXT
                  </div>
                  <div className="break-all">
                    <span className="text-muted-foreground">
                      {t("waf.onboarding.recordValue")}
                    </span>
                    {domain.data.verificationToken}
                  </div>
                </div>
                <div className="mt-3 flex items-center gap-3">
                  <Button onClick={runVerifyOwnership} disabled={busy}>
                    {t("waf.onboarding.checkOwnership")}
                  </Button>
                  <span className="text-sm">
                    {t("waf.onboarding.ownershipLabel")}{" "}
                    <StatusBadge
                      value={
                        ownershipVerified
                          ? "verified"
                          : (domain.data.verificationStatus ?? "unverified")
                      }
                    />
                  </span>
                </div>
              </>
            ) : null}
            <div className="mt-4 flex items-center gap-3">
              <Button
                variant="outline"
                onClick={runVerifyRouting}
                disabled={busy}
              >
                {t("waf.onboarding.checkRouting")}
              </Button>
              {routingResult ? (
                <span className="text-sm">
                  {t("waf.onboarding.routingLabel")}{" "}
                  <StatusBadge
                    value={routingResult.dnsRouted ? "routed" : "notRouted"}
                  />
                </span>
              ) : null}
            </div>
            {routingResult && !routingResult.dnsRouted ? (
              <p className="mt-2 text-xs text-muted-foreground">
                {t("waf.onboarding.routingInfo", {
                  hostname: zone.data.hostname,
                  target: routingResult.target,
                })}
              </p>
            ) : null}
          </SectionCard>

          <SectionCard
            title={t("waf.onboarding.originTitle")}
            description={t("waf.onboarding.originDescription")}
          >
            <div className="flex items-center gap-3">
              <Button variant="outline" onClick={runOriginTest} disabled={busy}>
                {t("waf.onboarding.testOrigin")}
              </Button>
              {originResult ? (
                <span className="text-sm">
                  {originResult.reachable
                    ? t("waf.onboarding.originReachable", {
                        status: originResult.status,
                        latency: originResult.latencyMs,
                      })
                    : t("waf.onboarding.originUnreachable", {
                        error:
                          originResult.error ??
                          t("waf.onboarding.originNoResponse"),
                      })}
                </span>
              ) : null}
            </div>
          </SectionCard>

          <div>
            <Button onClick={() => setStep(2)} disabled={!verified}>
              {t("waf.onboarding.continue")}
            </Button>
            {!verified ? (
              <span className="ml-3 text-xs text-muted-foreground">
                {t("waf.onboarding.notVerifiedHint")}
              </span>
            ) : null}
          </div>
        </div>
      ) : null}

      {step === 2 && zone ? (
        <SectionCard
          title={t("waf.onboarding.protectionTitle")}
          description={t("waf.onboarding.protectionDescription")}
        >
          <p className="text-sm text-muted-foreground">
            {t("waf.onboarding.protectionExplain")}
          </p>
          <div className="mt-3 flex gap-2">
            <Button onClick={addBaselineProtection} disabled={busy}>
              {t("waf.onboarding.addDefaultPolicy")}
            </Button>
            <Button
              variant="outline"
              onClick={() => navigate(`/zones/${zone.id}`)}
            >
              {t("waf.onboarding.configureManually")}
            </Button>
          </div>
        </SectionCard>
      ) : null}

      {step === 3 && zone ? (
        <SectionCard
          title={t("waf.onboarding.activateTitle")}
          description={t("waf.onboarding.activateDescription")}
        >
          <ul className="mb-4 space-y-1 text-sm">
            <li>
              {t("waf.onboarding.activateHostnameLabel")}{" "}
              <code>{zone.data.hostname}</code> ·{" "}
              <StatusBadge value={zone.data.verificationStatus} />
            </li>
            <li>
              {t("waf.onboarding.activateOriginLabel")}{" "}
              <code>{zone.data.originAddress}</code>
            </li>
            <li>
              {t("waf.onboarding.activateModeLabel")}{" "}
              <StatusBadge value={zone.data.protectionMode} />
            </li>
          </ul>
          <Button onClick={activate} disabled={busy}>
            {t("waf.onboarding.activateZone")}
          </Button>
        </SectionCard>
      ) : null}
    </div>
  );
}
