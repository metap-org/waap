// Layer: APP · wiring. Registers the declared action and mounts the scanner callback.

// Custom (non-generic-CRUD) HTTP surface for scanning-service.
//
// Scope boundary first, because it is easy to get wrong: this service does not scan. Running a
// DAST engine against a customer's site is a separate execution concern, the same way edge-plane
// is separate from the portal. What lives here is the portal-side half:
//
//   - the action waf.scan_jobs.run — hands a queued job to whatever scanner backend is
//     configured (SCANNER_URL) and moves the job's workflow along. With no scanner configured
//     it queues the job and says so, rather than pretending a scan happened.
//   - POST /internal/scan-jobs/{id}/findings — the callback a scanner posts results to. Creates
//     ScanFinding records and completes (or fails) the job in one call, so a scanner integration
//     needs exactly one endpoint and no knowledge of the workflow's state names.
//
// Both go through CrudService (never SQL) so findings get the same validation, permission and
// outbox treatment any portal-created record gets.
package scanning

import (
	"net/http"

	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/kit/httpx/router"
	"github.com/metap-org/metap/pkg/web"
)

// RegisterActions registers the actions declared on this service's entities.
func RegisterActions(cfg Config) func(*web.State, *crud.ActionRegistry) error {
	return func(state *web.State, reg *crud.ActionRegistry) error {
		// Reads and writes through the service's own backend, as the scan job always did.
		deps := state.ActionDeps()
		deps.Records = state.Crud
		return RunScanJob(cfg, state).Register(reg, deps)
	}
}

// MountRoutes serves the scanner's callback. Running a scan is a declared action (RegisterActions).
// Every route says who may call it by the gate it is registered through (TestRoutesHaveAnAccessRule).
func MountRoutes(r *router.Router, state *web.State) {
	wrap := func(h func(http.ResponseWriter, *http.Request, *web.State)) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) { h(w, r, state) }
	}
	// The scanner posts as a signed-in caller (a service account); SubmitFindings still reads the job through
	// the caller's own permissions.
	web.Authenticated(r, state).Post("/internal/scan-jobs/{id}/findings", wrap(SubmitFindings))
}
