import { LocaleSwitcher, LoginForm } from "@metap/platform-ui";
import { useTranslation } from "react-i18next";
import { LogoMark } from "../brand/Logo";

/** Sign-in with the product's identity: an ink panel carrying the mark and what the product does,
 *  beside the form. The panel is the same ink as the navigation rail inside the app, so signing in
 *  feels like stepping into the console rather than onto a generic form. Below `lg` the panel is
 *  dropped and only a small mark sits above the form. The form itself (`LoginForm`) is the shared
 *  one: tenant choice, MFA and SSO steps all render in the right-hand column unchanged. */
export function LoginPage() {
  const { t } = useTranslation();
  const pillars = [
    t("waf.brand.pillarDdos"),
    t("waf.brand.pillarWaf"),
    t("waf.brand.pillarScan"),
    t("waf.brand.pillarAlert"),
  ];

  return (
    <div className="grid min-h-screen bg-background lg:grid-cols-[minmax(0,5fr)_minmax(0,6fr)]">
      <aside className="relative hidden overflow-hidden bg-sidebar p-10 text-sidebar-foreground lg:flex lg:flex-col lg:justify-between">
        {/* A faint grid and a cyan glow behind the mark: a console, not a brochure. Pure CSS so the
            panel costs no image request. */}
        <div
          aria-hidden
          className="pointer-events-none absolute inset-0 opacity-[0.35]"
          style={{
            backgroundImage:
              "linear-gradient(hsl(var(--sidebar-border)) 1px, transparent 1px), linear-gradient(90deg, hsl(var(--sidebar-border)) 1px, transparent 1px)",
            backgroundSize: "36px 36px",
            maskImage:
              "radial-gradient(ellipse at 30% 20%, black 20%, transparent 72%)",
            WebkitMaskImage:
              "radial-gradient(ellipse at 30% 20%, black 20%, transparent 72%)",
          }}
        />
        <div
          aria-hidden
          className="pointer-events-none absolute -left-24 -top-24 h-96 w-96 rounded-full opacity-20 blur-3xl"
          style={{ background: "hsl(var(--sidebar-primary))" }}
        />

        <div className="relative flex items-center gap-3 text-sidebar-accent-foreground">
          <LogoMark className="h-9 w-9" />
          <div className="leading-tight">
            <div className="text-base font-bold uppercase tracking-[0.2em]">
              {t("waf.brand.name")}
            </div>
            <div className="text-xs text-sidebar-foreground">
              {t("waf.brand.tagline")}
            </div>
          </div>
        </div>

        <div className="relative max-w-md">
          <h1 className="text-3xl font-semibold leading-tight tracking-tight text-sidebar-accent-foreground">
            {t("waf.brand.headline")}
          </h1>
          <ul className="mt-8 space-y-3 text-sm">
            {pillars.map((pillar) => (
              <li key={pillar} className="flex items-center gap-3">
                <span
                  aria-hidden
                  className="h-1.5 w-1.5 shrink-0 rounded-full"
                  style={{ background: "hsl(var(--sidebar-primary))" }}
                />
                {pillar}
              </li>
            ))}
          </ul>
        </div>

        <p className="relative font-mono text-xs text-sidebar-foreground/70">
          {t("waf.brand.footnote")}
        </p>
      </aside>

      <main className="relative flex items-center justify-center px-6 py-10">
        {/* Signed out, so the choice applies to this browser only; once signed in the saved
            preference (Settings) takes over. */}
        <div className="absolute right-4 top-4 w-36">
          <LocaleSwitcher compact />
        </div>
        <div className="w-full max-w-sm">
          <div className="mb-6 flex items-center gap-2 lg:hidden">
            <span className="text-primary">
              <LogoMark className="h-8 w-8" />
            </span>
            <span className="text-sm font-bold uppercase tracking-[0.2em]">
              {t("waf.brand.name")}
            </span>
          </div>
          <LoginForm />
        </div>
      </main>
    </div>
  );
}
