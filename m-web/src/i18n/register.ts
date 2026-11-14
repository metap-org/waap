import { i18n } from "@metap/platform-ui";
import { wafEn, wafVi } from "./resources";

/** Merges this app's own `waf.*` keys into `platform-ui`'s single shared `i18n` instance — that
 *  instance (not a separate one per app) is what `LocaleProvider`/`useTranslation()` bind to
 *  everywhere under `<LocaleProvider>` (`platform-ui/src/i18n/LocaleProvider.tsx`), so adding keys
 *  here rather than creating a second i18next instance is what makes `t("waf.xxx")` actually
 *  resolve. `addResourceBundle(lng, ns, resources, deep, overwrite)` — `deep: true` merges instead
 *  of replacing the "translation" namespace's existing `common`/`workflow`/`shell`/etc. keys,
 *  `overwrite: true` matters only if this module is ever imported twice (safe no-op otherwise).
 *
 *  Import this once, for its side effect only, before the app renders (`main.tsx`) — importing it
 *  from a component would re-run `addResourceBundle` on every render for no reason. */
i18n.addResourceBundle("en", "translation", wafEn, true, true);
i18n.addResourceBundle("vi", "translation", wafVi, true, true);
