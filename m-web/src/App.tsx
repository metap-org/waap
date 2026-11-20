/**
 * WAF Customer Portal — the real product IA, replacing the generic entity harness this app used to
 * be (a nav item per `waf.*` entity, `GeneratedList` behind each one).
 *
 * The organising principle is `docs/07-portal-features.md`'s sitemap: a customer thinks in zones
 * and in work (incidents, findings, alerts), not in tables. So the nav is Dashboard / Zones /
 * Incidents / Findings / Alerting / Analytics, everything about one zone lives behind that zone,
 * and the generic per-entity CRUD screens stay reachable under `/records/*` as an admin/debug
 * escape hatch rather than as the product.
 */
import { ENTITIES } from "./api/waf";
import { navIcons } from "./components/navIcons";
import {
  Navigate,
  Outlet,
  Route,
  Routes,
  useNavigate,
  useParams,
} from "react-router-dom";
import { useTranslation } from "react-i18next";
import {
  AccountSecurityPage,
  AppShellLayout,
  AuthProvider,
  Can,
  CronJobsAdminPage,
  GeneratedForm,
  GeneratedList,
  LocaleProvider,
  OidcCallbackPage,
  PoliciesAdminPage,
  RecordDetail,
  UsersAdminPage,
  WorkspaceAdminPage,
  AcceptInvitePage,
  ForgotPasswordPage,
  ResetPasswordPage,
  RolesAdminPage,
  useAuth,
} from "@metap/platform-ui";
import type { ShellNavItem } from "@metap/platform-ui";
import { LoginPage } from "./demo/LoginPage";
import { DashboardPage } from "./pages/DashboardPage";
import { OnboardingPage } from "./pages/OnboardingPage";
import { ZonesPage } from "./pages/ZonesPage";
import { ZoneDetailPage } from "./pages/ZoneDetailPage";
import { IncidentsPage } from "./pages/IncidentsPage";
import { IncidentDetailPage } from "./pages/IncidentDetailPage";
import { FindingsPage } from "./pages/FindingsPage";
import { AlertingPage } from "./pages/AlertingPage";
import { AnalyticsPage } from "./pages/AnalyticsPage";
import { SettingsPage } from "./pages/SettingsPage";
import { GlobalRulesPage } from "./pages/GlobalRulesPage";
import { GlobalAccessListPage } from "./pages/GlobalAccessListPage";

/** A layout route (rendered once via `<Route element={<RequireAuth />}>`, `<Outlet/>` swapping
 *  only the matched child) — not a per-route wrapper called from the route table anymore
 *  (2026-09-04, found live). It used to be `{ children }: { children: ReactNode }`, called as
 *  `page(<X/>)` at each of ~15 route entries, which meant every one of those was really its own
 *  `<Route path="..." element={<RequireAuth><X/></RequireAuth>} />` — React Router unmounts and
 *  remounts the whole `element` tree on any navigation to a *different* route, `AppShellLayout`
 *  included, so every single in-app navigation tore down and rebuilt the shell from scratch.
 *  Concretely: `AppShellLayout`'s own `useCurrentUser()`/`useCurrentUserEmail()` (`GET /auth/me`,
 *  sometimes `GET /users`) re-fired on every page change, not just once per session, even though
 *  both are cached — a fresh mount is a fresh `useQuery` subscriber, and neither hook overrides
 *  React Query's default `staleTime: 0`, so the *cached* data still gets a background revalidate
 *  on every one of those remounts. The layout-route form below mounts this component exactly once
 *  and keeps it mounted across every child navigation, so those hooks now behave the way their
 *  own doc comments already describe: fetched once, reused. */
function RequireAuth() {
  const { t } = useTranslation();
  // `useAuth()` dropped `token` for `status` in the cookie-session migration
  // (`docs/roadmap/64-cookie-session-persistence.md` in `../../../metap-docs`) — `status` starts
  // "unknown" until the initial `GET /auth/me` resolves, so redirecting on anything other than a
  // confirmed "anonymous" would bounce a just-logged-in user straight back to `/login`.
  const { status } = useAuth();

  if (status === "unknown") {
    return null;
  }
  if (status === "anonymous") {
    return <Navigate to="/login" replace />;
  }

  // Grouped like a dashboard sidebar: overview, the zone-centric protection config, the work queue
  // (incidents, findings, alerts), then administration.
  const overview = t("waf.navGroup.overview");
  const protection = t("waf.navGroup.protection");
  const operations = t("waf.navGroup.operations");
  const admin = t("waf.navGroup.admin");
  const navItems: ShellNavItem[] = [
    {
      to: "/",
      label: t("waf.nav.dashboard"),
      group: overview,
      icon: navIcons.dashboard,
    },
    {
      to: "/analytics",
      label: t("waf.nav.analytics"),
      group: overview,
      icon: navIcons.analytics,
    },
    {
      to: "/zones",
      label: t("waf.nav.zones"),
      group: protection,
      icon: navIcons.zones,
      entity: ENTITIES.zones,
    },
    {
      to: "/rules/global",
      label: t("waf.nav.globalRules"),
      group: protection,
      icon: navIcons.rules,
      entity: ENTITIES.firewallRules,
    },
    {
      to: "/access-lists/global",
      label: t("waf.nav.globalAccessLists"),
      group: protection,
      icon: navIcons.accessLists,
      entity: ENTITIES.ipAccessLists,
    },
    {
      to: "/incidents",
      label: t("waf.nav.incidents"),
      group: operations,
      icon: navIcons.incidents,
      entity: ENTITIES.incidents,
    },
    {
      to: "/findings",
      label: t("waf.nav.findings"),
      group: operations,
      icon: navIcons.findings,
      entity: ENTITIES.scanFindings,
    },
    {
      to: "/alerting",
      label: t("waf.nav.alerting"),
      group: operations,
      icon: navIcons.alerting,
      entity: ENTITIES.alertPolicies,
    },
    {
      to: "/settings",
      label: t("waf.nav.settings"),
      group: admin,
      icon: navIcons.settings,
      roles: ["admin"],
    },
    {
      to: "/admin/workspace",
      label: t("shell.navWorkspace"),
      group: admin,
      icon: navIcons.workspace,
      roles: ["admin"],
    },
    {
      to: "/admin/users",
      label: t("shell.navUsers"),
      group: admin,
      icon: navIcons.users,
      roles: ["admin"],
    },
    {
      to: "/admin/roles",
      label: t("shell.navRoles"),
      group: admin,
      icon: navIcons.roles,
      roles: ["admin"],
    },
    {
      to: "/admin/policies",
      label: t("shell.navPolicies"),
      group: admin,
      icon: navIcons.policies,
      roles: ["admin"],
    },
    {
      to: "/admin/cron-jobs",
      label: t("shell.navCronJobs"),
      group: admin,
      icon: navIcons.cron,
      roles: ["admin"],
    },
  ];

  return (
    <AppShellLayout brand="WAF Portal" navItems={navItems}>
      <Outlet />
    </AppShellLayout>
  );
}

/** Also a layout route now — see `RequireAuth`'s doc comment for why. Nested one level inside it
 *  in the route table below, so an admin-only page still gets the shell from `RequireAuth`
 *  without a second `AppShellLayout` mount. */
function RequireAdmin() {
  return (
    <Can roles={["admin"]} fallback={<Navigate to="/" replace />}>
      <Outlet />
    </Can>
  );
}

/** Protection config vs. the operational work queue — the same split as the sidebar's groups. */
const PROTECTION_ENTITIES = new Set<string>([
  ENTITIES.zones,
  ENTITIES.domains,
  ENTITIES.ddosPolicies,
  ENTITIES.firewallRules,
  ENTITIES.ipAccessLists,
]);

/** The permission editors group entities into the sidebar's own sections. */
function usePermissionGroups() {
  const { t } = useTranslation();
  return (entity: { name: string }) =>
    PROTECTION_ENTITIES.has(entity.name)
      ? t("waf.navGroup.protection")
      : entity.name.startsWith("waf.")
        ? t("waf.navGroup.operations")
        : entity.name;
}

function RolesRoute() {
  return <RolesAdminPage permissionGroups={usePermissionGroups()} />;
}

function PoliciesRoute() {
  return <PoliciesAdminPage permissionGroups={usePermissionGroups()} />;
}

function RecordsRoute() {
  const { entityName } = useParams<{ entityName: string }>();
  if (!entityName) return <div>Missing entity name</div>;
  return <GeneratedList entityName={entityName} />;
}

function NewRecordRoute() {
  const { entityName } = useParams<{ entityName: string }>();
  const navigate = useNavigate();
  if (!entityName) return <div>Missing entity name</div>;
  return (
    <GeneratedForm
      entityName={entityName}
      onSaved={() => navigate(`/records/${entityName}`)}
    />
  );
}

function RecordDetailRoute() {
  const { entityName, id } = useParams<{ entityName: string; id: string }>();
  if (!entityName || !id) return <div>Missing entity or id</div>;
  return <RecordDetail entityName={entityName} id={id} />;
}

function EditRecordRoute() {
  const { entityName, id } = useParams<{ entityName: string; id: string }>();
  const navigate = useNavigate();
  if (!entityName || !id) return <div>Missing entity or id</div>;
  return (
    <GeneratedForm
      entityName={entityName}
      recordId={id}
      onSaved={() => navigate(`/records/${entityName}/${id}`)}
    />
  );
}

export default function App() {
  return (
    <AuthProvider>
      <LocaleProvider>
        <Routes>
          <Route path="/login" element={<LoginPage />} />
          {/* Public account pages the invitation and password-reset emails link to. */}
          <Route path="/accept-invite" element={<AcceptInvitePage />} />
          <Route path="/forgot-password" element={<ForgotPasswordPage />} />
          <Route path="/reset-password" element={<ResetPasswordPage />} />
          <Route path="/auth/oidc/callback" element={<OidcCallbackPage />} />

          {/* Layout route — `RequireAuth` mounts `AppShellLayout` once and renders every child
              below through its `<Outlet/>` instead of remounting the shell per navigation (see
              `RequireAuth`'s own doc comment). */}
          <Route element={<RequireAuth />}>
            <Route path="/" element={<DashboardPage />} />
            <Route path="/onboarding" element={<OnboardingPage />} />
            <Route path="/zones" element={<ZonesPage />} />
            <Route path="/zones/:zoneId" element={<ZoneDetailPage />} />
            <Route path="/rules/global" element={<GlobalRulesPage />} />
            <Route
              path="/access-lists/global"
              element={<GlobalAccessListPage />}
            />
            <Route path="/incidents" element={<IncidentsPage />} />
            <Route
              path="/incidents/:incidentId"
              element={<IncidentDetailPage />}
            />
            <Route path="/findings" element={<FindingsPage />} />
            <Route path="/alerting" element={<AlertingPage />} />
            <Route path="/analytics" element={<AnalyticsPage />} />
            <Route path="/account/security" element={<AccountSecurityPage />} />

            {/* Generic record screens, reached from a list row's "view" action and the generic
                create/edit forms. No longer in the nav, and no entity picker at `/records`: the
                product screens above are the portal (2026-11-15). */}
            <Route path="/records/:entityName" element={<RecordsRoute />} />
            <Route
              path="/records/:entityName/new"
              element={<NewRecordRoute />}
            />
            <Route
              path="/records/:entityName/:id"
              element={<RecordDetailRoute />}
            />
            <Route
              path="/records/:entityName/:id/edit"
              element={<EditRecordRoute />}
            />

            <Route element={<RequireAdmin />}>
              <Route path="/settings" element={<SettingsPage />} />
              <Route path="/admin/workspace" element={<WorkspaceAdminPage />} />
              <Route path="/admin/users" element={<UsersAdminPage />} />
              <Route path="/admin/roles" element={<RolesRoute />} />
              <Route path="/admin/policies" element={<PoliciesRoute />} />
              <Route path="/admin/cron-jobs" element={<CronJobsAdminPage />} />
            </Route>
          </Route>
        </Routes>
      </LocaleProvider>
    </AuthProvider>
  );
}
