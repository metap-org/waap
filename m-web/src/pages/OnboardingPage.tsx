/**
 * Module 1 — "Add a zone" (UC-20 to UC-28): the flow `docs/06-onboarding-rules-lists.md` and
 * `docs/11-onboarding-dns-resolution.md` describe, and the screen that most clearly is not generic
 * CRUD: creating the record is one of several steps, and the others each call something bespoke
 * (resolve the hostname, probe the origin, prove domain ownership by DNS or by a file, apply a
 * starting set of protections, activate).
 *
 * Where a person has got to is read from the zone itself (verification, protection, origin check),
 * not kept in this component, so the flow can be left and resumed: `/onboarding?zone=<id>` opens at
 * the first step that is not done. Verification and protection are independent, as the spec says:
 * only activating needs both.
 */
import { Fragment, useEffect, useMemo, useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
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
  toast,
} from "@metap/ui";
import { ApiErrorMessage, useAsyncAction } from "@metap/platform-ui";
import {
  ENTITIES,
  PRESETS,
  applyPreset,
  createRecord,
  probeOrigin,
  resolveHostname,
  testOrigin,
  transitionRecord,
  useInvalidateWaf,
  useRecord,
  useRecords,
  verifyDns,
  verifyDomainDns,
  type Domain,
  type HostnameView,
  type OriginTestResult,
  type PresetKey,
  type Zone,
} from "../api/waf";
import { StatusBadge } from "../components/primitives";

type Step = 0 | 1 | 2 | 3;

/** The first step a zone has not finished: what resuming opens at. */
function stepFor(zone: Zone): Step {
  if (zone.data.verificationStatus !== "verified") return 1;
  if (!zone.data.hasConfig) return 2;
  return 3;
}

function copy(text: string, done: string) {
  void navigator.clipboard
    ?.writeText(text)
    .then(() => toast(done, { variant: "default" }))
    .catch(() => undefined);
}

export function OnboardingPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [params] = useSearchParams();
  const invalidate = useInvalidateWaf();
  const { busy, run } = useAsyncAction();

  const [zoneId, setZoneId] = useState<string | undefined>(
    params.get("zone") ?? undefined,
  );
  const [step, setStep] = useState<Step>(zoneId ? 1 : 0);
  const [error, setError] = useState<string | null>(null);

  // The zone is always read fresh: a preset or a verification changes it on the server.
  const zoneQuery = useRecord<Zone["data"]>(ENTITIES.zones, zoneId);
  const zone = zoneQuery.data as Zone | undefined;
  const domainQuery = useRecord<Domain["data"]>(
    ENTITIES.domains,
    zone?.data.domainId,
  );
  const domain = domainQuery.data as Domain | undefined;

  // Resuming: once the zone has loaded, open at the first unfinished step (once).
  const [resumed, setResumed] = useState(false);
  useEffect(() => {
    if (zone && params.get("zone") && !resumed) {
      setStep(stepFor(zone));
      setResumed(true);
    }
  }, [zone, params, resumed]);

  async function guard(action: () => Promise<void>) {
    await run(async () => {
      setError(null);
      try {
        await action();
      } catch (e) {
        setError(e instanceof Error ? e.message : String(e));
      }
    });
  }

  const verified = zone?.data.verificationStatus === "verified";
  const labels = [
    "stepDetails",
    "stepVerify",
    "stepProtection",
    "stepActivate",
  ] as const;

  return (
    <div className="mx-auto max-w-3xl">
      <PageHeader
        title={t("waf.onboarding.title")}
        description={t("waf.onboarding.description")}
      />

      <Stepper className="mb-6">
        {labels.map((labelKey, index) => (
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
      {zoneId && zoneQuery.error ? (
        <ApiErrorMessage error={zoneQuery.error} />
      ) : null}

      {step === 0 ? (
        <DetailsStep
          busy={busy}
          guard={guard}
          onCreated={(id) => {
            setZoneId(id);
            setStep(1);
            invalidate();
          }}
        />
      ) : null}

      {step === 1 && zone ? (
        <ConnectStep
          zone={zone}
          domain={domain}
          domainError={domainQuery.error}
          busy={busy}
          guard={guard}
          onChanged={invalidate}
          onContinue={() => setStep(2)}
        />
      ) : null}

      {step === 2 && zone ? (
        <ProtectionStep
          zone={zone}
          busy={busy}
          guard={guard}
          onApplied={() => {
            invalidate();
            setStep(3);
          }}
          onSkip={() => setStep(3)}
          onManual={() => navigate(`/zones/${zone.id}`)}
        />
      ) : null}

      {step === 3 && zone ? (
        <ActivateStep
          zone={zone}
          verified={verified}
          busy={busy}
          onBack={setStep}
          onActivate={() =>
            guard(async () => {
              await transitionRecord<Zone["data"]>(
                ENTITIES.zones,
                zone.id,
                "activate",
                zone.version,
              );
              invalidate();
              navigate(`/zones/${zone.id}`);
            })
          }
        />
      ) : null}
    </div>
  );
}

/* ------------------------------------------------------------------ step 0: details */

function DetailsStep({
  busy,
  guard,
  onCreated,
}: {
  busy: boolean;
  guard: (action: () => Promise<void>) => Promise<void>;
  onCreated: (zoneId: string) => void;
}) {
  const { t } = useTranslation();
  const [hostname, setHostname] = useState("");
  const [originAddress, setOriginAddress] = useState("");
  const [originProtocol, setOriginProtocol] = useState("");
  const [originPort, setOriginPort] = useState("");
  const [originHostHeader, setOriginHostHeader] = useState("");
  const [protectionMode, setProtectionMode] = useState("monitor");
  const [accountId, setAccountId] = useState("");
  const [resolved, setResolved] = useState<HostnameView | null>(null);
  const [hostError, setHostError] = useState<string | null>(null);
  const [probe, setProbe] = useState<OriginTestResult["data"] | null>(null);
  const [probing, setProbing] = useState(false);

  // Only asked when the workspace has more than one open account; otherwise the server files the
  // zone under the default one.
  const accounts = useRecords<{ name?: string }>(
    ENTITIES.accounts,
    { status: "active" },
    100,
  );

  // What the name resolves to now, a moment after typing stops (UC-20, docs/11).
  useEffect(() => {
    if (!hostname.trim()) {
      setResolved(null);
      setHostError(null);
      return;
    }
    const handle = setTimeout(() => {
      resolveHostname(hostname)
        .then((res) => {
          setResolved(res.data);
          setHostError(null);
        })
        .catch((e: unknown) => {
          setResolved(null);
          setHostError(e instanceof Error ? e.message : String(e));
        });
    }, 500);
    return () => clearTimeout(handle);
  }, [hostname]);

  const originSpec = {
    originAddress: originAddress.trim(),
    ...(originProtocol ? { originProtocol } : {}),
    ...(originPort ? { originPort: Number(originPort) } : {}),
    ...(originHostHeader.trim()
      ? { originHostHeader: originHostHeader.trim() }
      : {}),
    hostname: resolved?.hostname ?? hostname.trim(),
  };

  async function runProbe() {
    setProbing(true);
    setProbe(null);
    try {
      setProbe((await probeOrigin(originSpec)).data);
    } catch (e) {
      setProbe({
        reachable: false,
        latencyMs: 0,
        url: "",
        error: e instanceof Error ? e.message : String(e),
      });
    } finally {
      setProbing(false);
    }
  }

  const create = () =>
    guard(async () => {
      // `domainId` is assigned server-side (the hostname's apex), and the hostname is stored in its
      // canonical form: neither is chosen here.
      const response = await createRecord<Zone["data"]>(ENTITIES.zones, {
        hostname: hostname.trim(),
        originAddress: originSpec.originAddress,
        ...(originSpec.originProtocol
          ? { originProtocol: originSpec.originProtocol }
          : {}),
        ...(originSpec.originPort ? { originPort: originSpec.originPort } : {}),
        ...(originSpec.originHostHeader
          ? { originHostHeader: originSpec.originHostHeader }
          : {}),
        protectionMode,
        ...(accountId ? { accountId } : {}),
        dnsRoutingStatus: "unknown",
        hasConfig: false,
        configVersion: 1,
      });
      onCreated(response.data.id);
    });

  return (
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
          {hostError ? (
            <p className="mt-1 text-xs text-destructive">{hostError}</p>
          ) : null}
          {resolved ? <ResolvedHint view={resolved} typed={hostname} /> : null}
          {resolved?.suggestedOrigin && !originAddress ? (
            <Button
              type="button"
              size="sm"
              variant="outline"
              className="mt-2"
              onClick={() => setOriginAddress(resolved.suggestedOrigin ?? "")}
            >
              {t("waf.onboarding.useSuggested", {
                origin: resolved.suggestedOrigin,
              })}
            </Button>
          ) : null}
        </div>

        <div>
          <Label htmlFor="origin">{t("waf.onboarding.originAddress")}</Label>
          <div className="flex gap-2">
            <Input
              id="origin"
              placeholder="203.0.113.10 or origin.example.com"
              value={originAddress}
              onChange={(e) => {
                setOriginAddress(e.target.value);
                setProbe(null);
              }}
            />
            <Button
              type="button"
              variant="outline"
              onClick={() => void runProbe()}
              disabled={probing || !originAddress.trim()}
            >
              {t("waf.onboarding.testOrigin")}
            </Button>
          </div>
          {probe ? (
            <p
              className={`mt-1 text-xs ${probe.reachable ? "text-emerald-600" : "text-amber-600"}`}
            >
              {probe.reachable
                ? t("waf.onboarding.originReachable", {
                    status: probe.status,
                    latency: probe.latencyMs,
                  })
                : t("waf.onboarding.originUnreachableSoft", {
                    error: probe.error ?? t("waf.onboarding.originNoResponse"),
                  })}
            </p>
          ) : (
            <p className="mt-1 text-xs text-muted-foreground">
              {t("waf.onboarding.originHint")}
            </p>
          )}
          <details className="mt-3 rounded-md border p-3 text-sm">
            <summary className="cursor-pointer font-medium">
              {t("waf.onboarding.originAdvanced")}
            </summary>
            <div className="mt-3 grid gap-3 sm:grid-cols-3">
              <div>
                <Label htmlFor="origin-protocol">
                  {t("waf.onboarding.originProtocol")}
                </Label>
                <Select
                  id="origin-protocol"
                  value={originProtocol}
                  onValueChange={(v) => setOriginProtocol(String(v))}
                  options={[
                    { value: "", label: t("waf.onboarding.protocolDefault") },
                    { value: "https", label: "HTTPS" },
                    { value: "http", label: "HTTP" },
                  ]}
                />
              </div>
              <div>
                <Label htmlFor="origin-port">
                  {t("waf.onboarding.originPort")}
                </Label>
                <Input
                  id="origin-port"
                  inputMode="numeric"
                  placeholder="443"
                  value={originPort}
                  onChange={(e) => setOriginPort(e.target.value)}
                />
              </div>
              <div>
                <Label htmlFor="origin-host">
                  {t("waf.onboarding.originHostHeader")}
                </Label>
                <Input
                  id="origin-host"
                  placeholder={resolved?.hostname ?? "shop.example.com"}
                  value={originHostHeader}
                  onChange={(e) => setOriginHostHeader(e.target.value)}
                />
              </div>
            </div>
            <p className="mt-2 text-xs text-muted-foreground">
              {t("waf.onboarding.originAdvancedHint")}
            </p>
          </details>
        </div>

        {(accounts.data ?? []).length > 1 ? (
          <div>
            <Label htmlFor="account">{t("waf.accounts.chooseAccount")}</Label>
            <Select
              id="account"
              value={accountId}
              onValueChange={(value) => setAccountId(String(value))}
              options={[
                { value: "", label: t("waf.accounts.defaultAccount") },
                ...(accounts.data ?? []).map((a) => ({
                  value: a.id,
                  label: a.data.name ?? a.id,
                })),
              ]}
            />
          </div>
        ) : null}

        <div>
          <Label htmlFor="mode">{t("waf.onboarding.protectionMode")}</Label>
          <Select
            id="mode"
            value={protectionMode}
            onValueChange={(value) => setProtectionMode(String(value))}
            options={[
              { value: "monitor", label: t("waf.onboarding.modeMonitor") },
              { value: "enforce", label: t("waf.onboarding.modeEnforce") },
            ]}
          />
          <p className="mt-1 text-xs text-muted-foreground">
            {t("waf.onboarding.modeHint")}
          </p>
        </div>

        <div>
          <Button
            onClick={create}
            disabled={
              busy || !hostname.trim() || !originAddress.trim() || !!hostError
            }
          >
            {t("waf.onboarding.createZone")}
          </Button>
        </div>
      </div>
    </SectionCard>
  );
}

/** What the typed hostname is today, in a line a person can act on. */
function ResolvedHint({ view, typed }: { view: HostnameView; typed: string }) {
  const { t } = useTranslation();
  const where = [...view.a, ...view.aaaa, ...view.cname].join(", ");
  return (
    <div className="mt-1 space-y-1 text-xs">
      {view.hostname !== typed.trim() ? (
        <p className="text-muted-foreground">
          {t("waf.onboarding.willBeStoredAs", { hostname: view.hostname })}
        </p>
      ) : null}
      {view.warnings.includes("unresolved") ? (
        <p className="text-amber-600">{t("waf.onboarding.unresolved")}</p>
      ) : view.pointsToEdge ? (
        <p className="text-emerald-600">{t("waf.onboarding.alreadyAtEdge")}</p>
      ) : (
        <p className="text-muted-foreground">
          {t("waf.onboarding.resolvesTo", { where })}
        </p>
      )}
    </div>
  );
}

/* ------------------------------------------------------------------ step 1: verify and connect */

function ConnectStep({
  zone,
  domain,
  domainError,
  busy,
  guard,
  onChanged,
  onContinue,
}: {
  zone: Zone;
  domain: Domain | undefined;
  domainError: unknown;
  busy: boolean;
  guard: (action: () => Promise<void>) => Promise<void>;
  onChanged: () => void;
  onContinue: () => void;
}) {
  const { t } = useTranslation();
  const [method, setMethod] = useState<"dnsTxt" | "httpFile">("dnsTxt");
  const [detail, setDetail] = useState<string | null>(null);
  const [routing, setRouting] = useState<{
    dnsRouted: boolean;
    target: string;
  } | null>(null);
  const [origin, setOrigin] = useState<OriginTestResult["data"] | null>(null);
  const verified = zone.data.verificationStatus === "verified";
  const apex = domain?.data.apexDomain ?? "";
  const token = domain?.data.verificationToken ?? "";
  const host = zone.data.hostname ?? "";
  const plainHost = host.replace(/^\*\./, "");

  const checkOwnership = () =>
    guard(async () => {
      if (!domain) return;
      const res = await verifyDomainDns(domain.id, { method });
      const checked = res.data.checked;
      setDetail(
        res.data.ownershipVerified
          ? null
          : method === "httpFile"
            ? (checked.file ?? null)
            : (checked.txt ?? []).length > 0
              ? t("waf.onboarding.txtOther", {
                  found: (checked.txt ?? []).join(", "),
                })
              : t("waf.onboarding.txtNone"),
      );
      onChanged();
    });

  const checkRouting = () =>
    guard(async () => {
      const res = await verifyDns(zone.id);
      setRouting({
        dnsRouted: res.data.dnsRouted,
        target: res.data.checked.expectedTarget,
      });
      onChanged();
    });

  const checkOrigin = () =>
    guard(async () => {
      setOrigin((await testOrigin(zone.id)).data);
      onChanged();
    });

  const edgeTarget = routing?.target ?? "";

  return (
    <div className="grid gap-4">
      <SectionCard
        title={t("waf.onboarding.verifyTitle")}
        description={t("waf.onboarding.verifyDescription2", { apex })}
      >
        {domainError ? (
          <ApiErrorMessage error={domainError} />
        ) : domain ? (
          <>
            <div className="mb-3 flex gap-2">
              {(["dnsTxt", "httpFile"] as const).map((m) => (
                <Button
                  key={m}
                  size="sm"
                  variant={method === m ? "default" : "outline"}
                  onClick={() => {
                    setMethod(m);
                    setDetail(null);
                  }}
                >
                  {t(`waf.onboarding.method.${m}`)}
                </Button>
              ))}
            </div>
            {method === "dnsTxt" ? (
              <Record
                rows={[
                  [t("waf.onboarding.recordName"), `_waf-verify.${apex}`],
                  [t("waf.onboarding.recordType"), "TXT"],
                  [t("waf.onboarding.recordValue"), token],
                ]}
                copyValue={token}
              />
            ) : (
              <Record
                rows={[
                  [
                    t("waf.onboarding.filePath"),
                    `http://${apex}/.well-known/waf-verify/${token}`,
                  ],
                  [t("waf.onboarding.fileContent"), token],
                ]}
                copyValue={token}
              />
            )}
            <div className="mt-3 flex items-center gap-3">
              <Button onClick={checkOwnership} disabled={busy}>
                {t("waf.onboarding.checkOwnership")}
              </Button>
              <StatusBadge
                value={
                  verified
                    ? "verified"
                    : (domain.data.verificationStatus ?? "unverified")
                }
              />
            </div>
            {detail && !verified ? (
              <p className="mt-2 text-xs text-amber-600">{detail}</p>
            ) : null}
          </>
        ) : null}
      </SectionCard>

      <SectionCard
        title={t("waf.onboarding.routeTitle")}
        description={t("waf.onboarding.routeDescription")}
      >
        <Record
          rows={[
            [t("waf.onboarding.recordName"), plainHost],
            [t("waf.onboarding.recordType"), "CNAME"],
            [
              t("waf.onboarding.recordValue"),
              edgeTarget || t("waf.onboarding.edgeTargetShown"),
            ],
          ]}
          copyValue={edgeTarget || undefined}
        />
        <div className="mt-3 flex items-center gap-3">
          <Button variant="outline" onClick={checkRouting} disabled={busy}>
            {t("waf.onboarding.checkRouting")}
          </Button>
          {routing ? (
            <StatusBadge value={routing.dnsRouted ? "routed" : "notRouted"} />
          ) : (
            <StatusBadge value={zone.data.dnsRoutingStatus} />
          )}
        </div>
        <p className="mt-2 text-xs text-muted-foreground">
          {t("waf.onboarding.routingInfo", {
            hostname: host,
            target: edgeTarget || "…",
          })}
        </p>
      </SectionCard>

      <SectionCard
        title={t("waf.onboarding.originTitle")}
        description={t("waf.onboarding.originDescription")}
      >
        <div className="flex flex-wrap items-center gap-3">
          <code className="text-xs">{zone.data.originAddress}</code>
          <Button variant="outline" onClick={checkOrigin} disabled={busy}>
            {t("waf.onboarding.testOrigin")}
          </Button>
          {origin ? (
            <span className="text-sm">
              {origin.reachable
                ? t("waf.onboarding.originReachable", {
                    status: origin.status,
                    latency: origin.latencyMs,
                  })
                : t("waf.onboarding.originUnreachableSoft", {
                    error: origin.error ?? t("waf.onboarding.originNoResponse"),
                  })}
            </span>
          ) : zone.data.originCheckedAt ? (
            <span className="text-xs text-muted-foreground">
              {zone.data.originReachable
                ? t("waf.onboarding.originLastOk")
                : t("waf.onboarding.originLastFailed")}
            </span>
          ) : null}
        </div>
      </SectionCard>

      <div className="flex items-center gap-3">
        <Button onClick={onContinue}>{t("waf.onboarding.continue")}</Button>
        <span className="text-xs text-muted-foreground">
          {verified
            ? t("waf.onboarding.verifiedHint")
            : t("waf.onboarding.notVerifiedHint2")}
        </span>
      </div>
    </div>
  );
}

/** A DNS record or file as a small copyable block. */
function Record({
  rows,
  copyValue,
}: {
  rows: [string, string][];
  copyValue?: string;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex items-start gap-2 rounded-md bg-muted/50 p-3 font-mono text-xs">
      <div className="min-w-0 flex-1">
        {rows.map(([label, value]) => (
          <div key={label} className="break-all">
            <span className="text-muted-foreground">{label}</span>
            {value}
          </div>
        ))}
      </div>
      {copyValue ? (
        <Button
          size="sm"
          variant="ghost"
          onClick={() => copy(copyValue, t("waf.onboarding.copied"))}
        >
          {t("waf.onboarding.copy")}
        </Button>
      ) : null}
    </div>
  );
}

/* ------------------------------------------------------------------ step 2: protection */

function ProtectionStep({
  zone,
  busy,
  guard,
  onApplied,
  onSkip,
  onManual,
}: {
  zone: Zone;
  busy: boolean;
  guard: (action: () => Promise<void>) => Promise<void>;
  onApplied: () => void;
  onSkip: () => void;
  onManual: () => void;
}) {
  const { t } = useTranslation();
  const [chosen, setChosen] = useState<PresetKey>("essential");
  const apply = () =>
    guard(async () => {
      const res = await applyPreset(zone.id, chosen);
      toast(
        res.data.added.length > 0
          ? t("waf.onboarding.presetApplied", { count: res.data.added.length })
          : t("waf.onboarding.presetNothingNew"),
        { variant: "default" },
      );
      onApplied();
    });

  return (
    <SectionCard
      title={t("waf.onboarding.protectionTitle")}
      description={t("waf.onboarding.protectionDescription")}
    >
      <div className="grid gap-3 sm:grid-cols-3">
        {PRESETS.map((key) => (
          <button
            key={key}
            type="button"
            onClick={() => setChosen(key)}
            className={`rounded-lg border p-3 text-left text-sm transition-colors ${
              chosen === key
                ? "border-primary bg-primary/5"
                : "hover:border-primary/50"
            }`}
          >
            <div className="font-medium">
              {t(`waf.onboarding.preset.${key}.name`)}
            </div>
            <div className="mt-1 text-xs text-muted-foreground">
              {t(`waf.onboarding.preset.${key}.summary`)}
            </div>
          </button>
        ))}
      </div>
      <p className="mt-3 text-xs text-muted-foreground">
        {t("waf.onboarding.presetExplain")}
      </p>
      <div className="mt-3 flex flex-wrap gap-2">
        <Button onClick={apply} disabled={busy}>
          {t("waf.onboarding.applyPreset")}
        </Button>
        <Button variant="outline" onClick={onManual}>
          {t("waf.onboarding.configureManually")}
        </Button>
        <Button variant="ghost" onClick={onSkip}>
          {t("waf.onboarding.skipProtection")}
        </Button>
      </div>
    </SectionCard>
  );
}

/* ------------------------------------------------------------------ step 3: activate */

function ActivateStep({
  zone,
  verified,
  busy,
  onBack,
  onActivate,
}: {
  zone: Zone;
  verified: boolean;
  busy: boolean;
  onBack: (step: Step) => void;
  onActivate: () => void;
}) {
  const { t } = useTranslation();
  const protectedZone = Boolean(zone.data.hasConfig);
  const checks = useMemo(
    () => [
      {
        key: "verified",
        ok: verified,
        required: true,
        fix: 1 as Step,
      },
      {
        key: "protection",
        ok: protectedZone,
        required: true,
        fix: 2 as Step,
      },
      {
        key: "origin",
        ok: Boolean(zone.data.originReachable),
        required: false,
        fix: 1 as Step,
      },
      {
        key: "routing",
        ok: zone.data.dnsRoutingStatus === "routed",
        required: false,
        fix: 1 as Step,
      },
    ],
    [
      verified,
      protectedZone,
      zone.data.originReachable,
      zone.data.dnsRoutingStatus,
    ],
  );
  const ready = checks.filter((c) => c.required).every((c) => c.ok);

  return (
    <SectionCard
      title={t("waf.onboarding.activateTitle")}
      description={t("waf.onboarding.activateDescription2")}
    >
      <ul className="mb-4 space-y-1 text-sm">
        <li>
          {t("waf.onboarding.activateHostnameLabel")}{" "}
          <code>{zone.data.hostname}</code>
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
      <ul className="mb-4 space-y-2 text-sm">
        {checks.map((c) => (
          <li key={c.key} className="flex items-center gap-2">
            <span
              aria-hidden
              className={
                c.ok
                  ? "text-emerald-600"
                  : c.required
                    ? "text-destructive"
                    : "text-amber-600"
              }
            >
              {c.ok ? "✓" : c.required ? "✕" : "!"}
            </span>
            <span className="flex-1">
              {t(`waf.onboarding.check.${c.key}.${c.ok ? "ok" : "todo"}`)}
              {!c.required && !c.ok ? (
                <span className="ml-1 text-xs text-muted-foreground">
                  {t("waf.onboarding.optional")}
                </span>
              ) : null}
            </span>
            {!c.ok ? (
              <Button size="sm" variant="ghost" onClick={() => onBack(c.fix)}>
                {t("waf.onboarding.goFix")}
              </Button>
            ) : null}
          </li>
        ))}
      </ul>
      <div className="flex items-center gap-3">
        <Button onClick={onActivate} disabled={busy || !ready}>
          {t("waf.onboarding.activateZone")}
        </Button>
        <Link
          to={`/zones/${zone.id}`}
          className="text-sm text-muted-foreground hover:underline"
        >
          {t("waf.onboarding.finishLater")}
        </Link>
      </div>
    </SectionCard>
  );
}
