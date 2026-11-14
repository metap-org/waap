/**
 * Module 2 — the zone list. Hand-built rather than `GeneratedList` because a zone row is the
 * product's primary object: it carries derived state (verification, routing, whether the zone has
 * any protection at all) that no single metadata field holds, and each row is a link into the
 * zone-centric IA the rest of the portal is organised around.
 */
import { Link } from "react-router-dom";
import {
  Button,
  EmptyState,
  Input,
  PageHeader,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@metap/ui";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { ApiErrorMessage } from "@metap/platform-ui";
import { ENTITIES, useRecords, type ZoneData } from "../api/waf";
import { StatusBadge } from "../components/primitives";

export function ZonesPage() {
  const { t } = useTranslation();
  const [search, setSearch] = useState("");
  const [status, setStatus] = useState<string>("");
  // `hostname` is `searchable` in metadata, so the generic list route turns this filter into a
  // substring match server-side rather than something this component has to do client-side.
  const zones = useRecords<ZoneData>(
    ENTITIES.zones,
    { hostname: search || undefined, status: status || undefined },
    50,
  );

  return (
    <div>
      <PageHeader
        title={t("waf.zones.title")}
        description={t("waf.zones.description")}
        actions={
          <Button asChild>
            <Link to="/onboarding">{t("waf.common.addZone")}</Link>
          </Button>
        }
      />

      <div className="mb-3 flex flex-wrap gap-2">
        <Input
          className="max-w-xs"
          placeholder={t("waf.zones.searchPlaceholder")}
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />
        <div className="flex gap-1">
          {["", "active", "pending", "paused", "suspended"].map((value) => (
            <Button
              key={value || "all"}
              size="sm"
              variant={status === value ? "default" : "outline"}
              onClick={() => setStatus(value)}
            >
              {value ? t(`waf.status.${value}`) : t("waf.common.all")}
            </Button>
          ))}
        </div>
      </div>

      {zones.isLoading ? (
        <p className="text-sm text-muted-foreground">
          {t("waf.common.loading")}
        </p>
      ) : zones.error ? (
        <ApiErrorMessage error={zones.error} />
      ) : (zones.data ?? []).length === 0 ? (
        <EmptyState
          title={t("waf.zones.noZonesYet")}
          description={t("waf.zones.noZonesYetDescription")}
          action={
            <Button asChild>
              <Link to="/onboarding">{t("waf.common.addZone")}</Link>
            </Button>
          }
        />
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t("waf.zones.colHostname")}</TableHead>
              <TableHead>{t("waf.zones.colStatus")}</TableHead>
              <TableHead>{t("waf.zones.colMode")}</TableHead>
              <TableHead>{t("waf.zones.colDomain")}</TableHead>
              <TableHead>{t("waf.zones.colDnsRouting")}</TableHead>
              <TableHead>{t("waf.zones.colProtection")}</TableHead>
              <TableHead className="text-right">
                {t("waf.zones.colConfigVersion")}
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {(zones.data ?? []).map((zone) => (
              <TableRow key={zone.id}>
                <TableCell>
                  <Link
                    className="font-medium hover:underline"
                    to={`/zones/${zone.id}`}
                  >
                    {zone.data.hostname}
                  </Link>
                  <div className="text-xs text-muted-foreground">
                    {zone.data.originAddress}
                  </div>
                </TableCell>
                <TableCell>
                  <StatusBadge value={zone.data.status ?? zone.status} />
                </TableCell>
                <TableCell>
                  <StatusBadge value={zone.data.protectionMode} />
                </TableCell>
                <TableCell>
                  <StatusBadge value={zone.data.verificationStatus} />
                </TableCell>
                <TableCell>
                  <StatusBadge value={zone.data.dnsRoutingStatus} />
                </TableCell>
                <TableCell>
                  {zone.data.hasConfig ? (
                    <span className="text-sm text-muted-foreground">
                      {t("waf.common.configured")}
                    </span>
                  ) : (
                    <span className="text-sm text-amber-600 dark:text-amber-500">
                      {t("waf.common.none")}
                    </span>
                  )}
                </TableCell>
                <TableCell className="text-right tabular-nums">
                  {zone.data.configVersion ?? 0}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
    </div>
  );
}
