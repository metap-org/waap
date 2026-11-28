/** The parts of a change request as people read them: its operations, its diff and its check. */
import { useTranslation } from "react-i18next";
import { Badge } from "@metap/ui";
import { ENTITIES } from "../api/waf";
import {
  subjectOf,
  type ChangeCheck,
  type ChangeDiffEntry,
  type ChangeOperation,
} from "../api/changeRequests";

const ENTITY_LABEL_KEY: Record<string, string> = {
  [ENTITIES.ddosPolicies]: "waf.zoneTabs.versions.entityDdos",
  [ENTITIES.firewallRules]: "waf.zoneTabs.versions.entityRules",
  [ENTITIES.ipAccessLists]: "waf.zoneTabs.versions.entityIpLists",
};

const KIND_VARIANT = {
  create: "default",
  update: "secondary",
  delete: "destructive",
} as const;

/** Fields the server fills in: they say nothing about what a person changed. */
const HIDDEN_FIELDS = new Set(["zoneId", "accountId"]);

function show(value: unknown): string {
  if (value === undefined || value === null || value === "") return "—";
  return typeof value === "object" ? JSON.stringify(value) : String(value);
}

function visibleFields(data: Record<string, unknown> | undefined) {
  return Object.keys(data ?? {}).filter((key) => !HIDDEN_FIELDS.has(key));
}

function Heading({
  kind,
  entity,
  subject,
}: {
  kind: keyof typeof KIND_VARIANT;
  entity: string;
  subject: string;
}) {
  const { t } = useTranslation();
  const labelKey = ENTITY_LABEL_KEY[entity];
  return (
    <div className="flex flex-wrap items-center gap-2">
      <Badge variant={KIND_VARIANT[kind]}>{t(`waf.changes.op.${kind}`)}</Badge>
      <span>{labelKey ? t(labelKey) : entity}</span>
      <span className="font-mono text-xs">{subject}</span>
    </div>
  );
}

export function OperationList({
  operations,
  onRemove,
  busy,
}: {
  operations: ChangeOperation[];
  /** Given, each operation can be taken out of the change. */
  onRemove?: (index: number) => void;
  busy?: boolean;
}) {
  const { t } = useTranslation();
  return (
    <ul className="max-h-48 divide-y overflow-auto rounded-md border text-sm">
      {operations.map((operation, index) => (
        <li
          key={`${operation.op}-${operation.id ?? index}`}
          className="flex items-center justify-between gap-2 px-3 py-2"
        >
          <Heading
            kind={operation.op}
            entity={operation.entity}
            subject={subjectOf(operation.data, operation.id)}
          />
          {onRemove ? (
            <button
              type="button"
              className="text-xs underline disabled:opacity-50"
              disabled={busy}
              onClick={() => onRemove(index)}
            >
              {t("waf.changes.removeOperation")}
            </button>
          ) : null}
        </li>
      ))}
    </ul>
  );
}

export function ChangeDiff({ diff }: { diff: ChangeDiffEntry[] }) {
  const { t } = useTranslation();
  if (diff.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">{t("waf.changes.noDiff")}</p>
    );
  }
  return (
    <ul className="space-y-2 text-sm">
      {diff.map((entry, index) => {
        const fields =
          entry.kind === "update"
            ? visibleFields({ ...entry.before, ...entry.after }).filter(
                (key) => show(entry.before?.[key]) !== show(entry.after?.[key]),
              )
            : visibleFields(entry.after ?? entry.before);
        return (
          <li
            key={`${entry.kind}-${entry.id ?? index}`}
            className="rounded-md border p-3"
          >
            <Heading
              kind={entry.kind}
              entity={entry.entity}
              subject={subjectOf(entry.after ?? entry.before, entry.id)}
            />
            <dl className="mt-2 grid grid-cols-[minmax(6rem,auto)_1fr] gap-x-3 gap-y-1 font-mono text-xs">
              {fields.map((key) => (
                <div key={key} className="contents">
                  <dt className="text-muted-foreground">{key}</dt>
                  <dd className="break-all">
                    {entry.kind === "update" ? (
                      <>
                        <span className="line-through opacity-60">
                          {show(entry.before?.[key])}
                        </span>{" "}
                        → {show(entry.after?.[key])}
                      </>
                    ) : (
                      show((entry.after ?? entry.before)?.[key])
                    )}
                  </dd>
                </div>
              ))}
            </dl>
          </li>
        );
      })}
    </ul>
  );
}

export function ChangeCheckSummary({ check }: { check: ChangeCheck }) {
  const { t } = useTranslation();
  const error = check.error;
  return (
    <div className="text-sm">
      {error ? (
        <div className="text-destructive">
          <p>
            {error.operation >= 0
              ? t("waf.changes.errorAt", { number: error.operation + 1 })
              : null}{" "}
            {error.code === "no_change"
              ? t("waf.changes.noChange")
              : error.message || error.code}
          </p>
          {Object.entries(error.fieldErrors ?? {}).map(([field, messages]) => (
            <p key={field} className="font-mono text-xs">
              {field}: {messages.join(", ")}
            </p>
          ))}
        </div>
      ) : null}
      <ul className="mt-1 space-y-1">
        {check.findings.map((finding, index) => (
          <li key={`${finding.code}-${index}`} className="flex gap-2">
            <Badge
              variant={
                finding.severity === "error"
                  ? "destructive"
                  : finding.severity === "warning"
                    ? "secondary"
                    : "outline"
              }
            >
              {t(`waf.zoneTabs.versions.severity.${finding.severity}`)}
            </Badge>
            <span>
              {t(`waf.zoneTabs.versions.finding.${finding.code}`, {
                subject: finding.subject ?? "",
              })}
            </span>
          </li>
        ))}
      </ul>
    </div>
  );
}
