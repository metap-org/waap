import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { TooltipProvider, ToastProvider } from "@metap/ui";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter } from "react-router-dom";
import "@metap/ui/style.css";

// Order matters — the theme's overrides must load after the base stylesheet so its
// `[data-theme="waap"]` rules win the cascade over @metap/ui's `:root` defaults (see
// ../../../../metap-themes/packages/enterprise/src/theme.css's own doc comment). The WAAP
// "control room" theme is this product's identity (2026-11-21), a fixed choice, not a switcher; swap the
// import + the `data-theme` value below to try a different one from `../../../../metap-themes/packages/*`.
import "@metap/theme-waap/theme.css";
import "./index.css";

document.documentElement.setAttribute("data-theme", "waap");
import { ApiError, ReactRouterNavigationProvider } from "@metap/platform-ui";
// Side-effect only — merges this app's `waf.*` translation keys into `platform-ui`'s shared
// `i18n` instance. Must run before anything calls `useTranslation()`/`t("waf....")`.
import "./i18n/register";
import App from "./App";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: (failureCount, error) => {
        // A 4xx (permission denied, entity not found, a business validation failure) is a
        // definitive rejection, not a transient failure: retrying just re-asks the same denied
        // question 3 times with backoff before the UI can show anything (found live: a forbidden
        // `waf.zones` read kept "loading" for several seconds while this retried).
        if (error instanceof ApiError && error.status < 500) {
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
