const uiLibPreset = require("@metap/ui/tailwind-preset").default;

/** `@metap/ui/style.css` already ships the compiled Tailwind base/reset layer, so `preflight:
 * false` here — this app's own Tailwind pass only adds `components`/`utilities` for classNames
 * used in this app's own pages and in `@metap/platform-ui`'s (consumed as raw TS source, not
 * pre-bundled — same setup as `metap`'s `apps/crm-fe`). */
module.exports = {
  presets: [uiLibPreset],
  corePlugins: { preflight: false },
  content: ["./index.html", "./src/**/*.{ts,tsx}", "../../../platform-ui/src/**/*.{ts,tsx}"],
};
