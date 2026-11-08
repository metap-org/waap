/**
 * One zone's own whitelist/blacklist entries — thin wrapper over the shared `IpAccessListPanel`,
 * which also backs the tenant-wide `GlobalAccessListPage`. See that component's file header for
 * the shared design; this file only fixes `zoneId`.
 */
import { IpAccessListPanel } from "../../components/IpAccessListPanel";

export function ZoneAccessListTab({ zoneId }: { zoneId: string }) {
  return <IpAccessListPanel zoneId={zoneId} />;
}
