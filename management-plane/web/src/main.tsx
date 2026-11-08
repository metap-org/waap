import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { TooltipProvider, ToastProvider } from "@metap/ui";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter } from "react-router-dom";
import "@metap/ui/style.css";

// Order matters — the theme's overrides must load after the base stylesheet so its
// `[data-theme="enterprise"]` rules win the cascade over @metap/ui's `:root` defaults (see
// ../../../../metap-themes/packages/enterprise/src/theme.css's own doc comment). Trying this
// theme out for the WAF portal (2026-09-03) — a fixed choice for now, not a switcher; swap the
// import + the `data-theme` value below to try a different one from `../../../../metap-themes/packages/*`.
import "@metap/theme-enterprise/theme.css";
import "./index.css";

document.documentElement.setAttribute("data-theme", "enterprise");
import {
  ApiError,
  GraphQLError,
  ReactRouterNavigationProvider,
} from "@metap/platform-ui";
// Side-effect only — merges this app's `waf.*` translation keys into `platform-ui`'s shared
// `i18n` instance. Must run before anything calls `useTranslation()`/`t("waf....")`.
import "./i18n/register";
import App from "./App";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: (failureCount, error) => {
        if (error instanceof ApiError && error.status < 500) {
          return false;
        }
        // This app's own queries (`api/waf.ts`, `SettingsPage.tsx` since 2026-09-26) go through
        // GraphQL, not REST — `ApiError` above only ever covers `platform-ui`'s own REST calls
        // still running under this same `QueryClient` (login, `/metadata/*`, workflow-events/
        // audit-events). A `GraphQLError` means the
        // gateway executed the request and came back with `{errors: [...]}` (permission denied,
        // entity not found, a business validation failure) — a definitive rejection, not a
        // transient failure, so retrying just re-asks the same denied question 3 times with
        // backoff before the UI can show anything. Found live: a forbidden `waf.zones` read kept
        // "loading" for several seconds while this retried, before finally rendering the error.
        if (error instanceof GraphQLError) {
          return false;
        }
        return failureCount < 3;
      },
    },
  },
});

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <TooltipProvider>
      <ToastProvider>
        <QueryClientProvider client={queryClient}>
          <BrowserRouter>
            <ReactRouterNavigationProvider>
              <App />
            </ReactRouterNavigationProvider>
          </BrowserRouter>
        </QueryClientProvider>
      </ToastProvider>
    </TooltipProvider>
  </StrictMode>,
);
