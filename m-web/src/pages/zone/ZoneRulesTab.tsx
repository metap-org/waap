/**
 * One zone's own firewall rules — thin wrapper over the shared `FirewallRulesPanel`, which also
 * backs the tenant-wide `GlobalRulesPage`. See that component's file header for the shared
 * design; this file only fixes `zoneId`.
 */
import { FirewallRulesPanel } from "../../components/FirewallRulesPanel";

export function ZoneRulesTab({ zoneId }: { zoneId: string }) {
  return <FirewallRulesPanel zoneId={zoneId} />;
}
