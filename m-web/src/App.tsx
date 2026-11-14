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
  AppShellLayout,
  AuthProvider,
  Can,
  CronJobsAdminPage,
  GeneratedForm,
  GeneratedList,
  LocaleProvider,
  LowCodeEntitiesAdminPage,
  OidcCallbackPage,
  PoliciesAdminPage,
  RecordDetail,
  UsersAdminPage,
  useAuth,
} from "@metap/platform-ui";
import type { ShellNavItem } from "@metap/platform-ui";
import { LoginPage } from "./demo/LoginPage";
import { EntitiesPage } from "./demo/EntitiesPage";
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

  const navItems: ShellNavItem[] = [
    { to: "/", label: t("waf.nav.dashboard") },
    { to: "/zones", label: t("waf.nav.zones") },
    { to: "/rules/global", label: t("waf.nav.globalRules") },
    { to: "/access-lists/global", label: t("waf.nav.globalAccessLists") },
    { to: "/incidents", label: t("waf.nav.incidents") },
    { to: "/findings", label: t("waf.nav.findings") },
    { to: "/alerting", label: t("waf.nav.alerting") },
    { to: "/analytics", label: t("waf.nav.analytics") },
    { to: "/settings", label: t("waf.nav.settings"), roles: ["admin"] },
    { to: "/admin/users", label: t("shell.navUsers"), roles: ["admin"] },
    { to: "/admin/policies", label: t("shell.navPolicies"), roles: ["admin"] },
    { to: "/admin/cron-jobs", label: t("shell.navCronJobs"), roles: ["admin"] },
    { to: "/admin/lowcode", label: t("shell.navLowCode"), roles: ["admin"] },
    // The old entity-per-nav-item harness, kept behind one link: still the fastest way to inspect
    // raw records when a product screen doesn't show the field you need.
    { to: "/records", label: t("waf.nav.rawRecords"), roles: ["admin"] },
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

            {/* Generic CRUD escape hatch — the whole of the previous app, now one section.
                `/records` itself (the entity picker) is admin-gated below; a specific entity's
                list/new/detail/edit stay reachable by anyone authenticated, same as before this
                route table was restructured. */}
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
              <Route path="/records" element={<EntitiesPage />} />
              <Route path="/admin/users" element={<UsersAdminPage />} />
              <Route path="/admin/policies" element={<PoliciesAdminPage />} />
              <Route path="/admin/cron-jobs" element={<CronJobsAdminPage />} />
              <Route
                path="/admin/lowcode"
                element={<LowCodeEntitiesAdminPage />}
              />
            </Route>
          </Route>
        </Routes>
      </LocaleProvider>
    </AuthProvider>
  );
}
