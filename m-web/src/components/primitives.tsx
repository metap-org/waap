/**
 * WAF-specific presentational pieces. `PageHeader`/`EmptyState`/`SectionCard`/`StatTile`/
 * `TimeSeries` and the `shortDate`/`dayLabel` utilities that used to live here moved out
 * (2026-09-05, `docs/features/26-waf-primitives-to-design-system.md`, driven by
 * `platform-ui/docs/audits/03-waf-demo-component-placement-audit.md`) — they were domain-free and
 * are now `@metap/ui`'s `PageHeader`/`EmptyState`/`SectionCard`/`StatTile`/`TimeSeries` and
 * `@metap/platform-ui`'s `shortDate`/`dayLabel`. `StatusBadge`/`TONES` stay here on purpose: the
 * tone-per-value mapping is WAF's own enum vocabulary (zone status, event action, finding
 * severity), exactly the coupling `@metap/ui` — a generic design system — must not take on.
 */
import { Badge } from "@metap/ui";
import { useTranslation } from "react-i18next";

/** Maps a WAF enum value onto a badge variant. One place for the whole product's colour language,
 *  so "blocked" never reads as red on one screen and grey on another. */
const TONES: Record<
  string,
  "default" | "secondary" | "destructive" | "outline"
> = {
  // Zone status
  active: "default",
  pending: "secondary",
  paused: "outline",
  suspended: "destructive",
  // Event action
  blocked: "destructive",
  challenged: "secondary",
  logged: "outline",
  // Severity / incident status
  critical: "destructive",
  high: "destructive",
  medium: "secondary",
  low: "outline",
  info: "outline",
  open: "destructive",
  acknowledged: "secondary",
  mitigating: "secondary",
  resolved: "default",
  // Scan job status
  idle: "outline",
  queued: "secondary",
  running: "secondary",
  completed: "default",
  failed: "destructive",
  // Verification / routing
  verified: "default",
  unverified: "outline",
  routed: "default",
  notRouted: "outline",
  sent: "default",
};

export function StatusBadge({ value }: { value?: string | null }) {
  const { t } = useTranslation();
  if (!value) return <span className="text-muted-foreground">—</span>;
  // Falls back to the raw enum value for anything not in `waf.status` (a value this component
  // hasn't been taught about yet reads better as itself than as a missing-key placeholder).
  const key = `waf.status.${value}`;
  const label = t(key, { defaultValue: value });
  return <Badge variant={TONES[value] ?? "outline"}>{label}</Badge>;
}
