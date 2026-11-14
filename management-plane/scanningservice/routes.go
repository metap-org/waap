// Custom (non-generic-CRUD) HTTP surface for scanning-service.
//
// Scope boundary first, because it is easy to get wrong: this service does not scan. Running a
// DAST engine against a customer's site is a separate execution concern, the same way edge-plane
// is separate from the portal. What lives here is the portal-side half:
//
//   - POST /api/waf.scan_jobs/{id}/run — hands a queued job to whatever scanner backend is
//     configured (SCANNER_URL) and moves the job's workflow along. With no scanner configured
//     it queues the job and says so, rather than pretending a scan happened.
//   - POST /internal/scan-jobs/{id}/findings — the callback a scanner posts results to. Creates
//     ScanFinding records and completes (or fails) the job in one call, so a scanner integration
//     needs exactly one endpoint and no knowledge of the workflow's state names.
//
// Both go through CrudService (never SQL) so findings get the same validation, permission and
// outbox treatment any portal-created record gets.
package scanningservice

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/httpserver"
	"github.com/metap-org/metap/pkg/permission"
	"github.com/metap-org/metap/pkg/runtime"
)

// scannerURL: where a scan request is handed off. Unset (the default) means "no scanner
// deployed" — the job still queues, which is the honest state: something asked for a scan and
// nothing is going to run it yet.
func scannerURL() (string, bool) { return runtime.Optional("SCANNER_URL") }

func httpClient() *http.Client { return &http.Client{Timeout: 15 * time.Second} }

// doTransition applies one workflow transition, mapping a service-level failure into a plain
// error. Transition names come from entities.ScanJobEntity's workflow (run/start/complete/fail)
// — this module never writes status directly, so guards, audit rows and outbox events all still
// happen.
func doTransition(ctx context.Context, state *httpserver.AppState, job crud.RecordDto, action string, rc *permission.RequestContext) (crud.RecordDto, error) {
	result, err := state.Crud.Transition(ctx, "waf.scan_jobs", job.ID, action, job.Version, nil, rc, nil)
	if err != nil {
		return crud.RecordDto{}, fmt.Errorf("transition %s: %w", action, err)
	}
	return result, nil
}

func loadJob(ctx context.Context, w http.ResponseWriter, state *httpserver.AppState, id uuid.UUID, rc *permission.RequestContext) (crud.RecordDto, bool) {
	result, err := state.Crud.Get(ctx, "waf.scan_jobs", id, rc)
	if err != nil {
		httpserver.WriteCrudError(w, err)
		return crud.RecordDto{}, false
	}
	return result.Record, true
}

// RunScanJob is POST /api/waf.scan_jobs/{id}/run — the portal's "Run scan now" button.
func RunScanJob(w http.ResponseWriter, r *http.Request, state *httpserver.AppState) {
	rc, ok := httpserver.RequireAuth(w, r, state)
	if !ok {
		return
	}
	jobID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpserver.WriteServiceError(w, http.StatusBadRequest, "validation_failed", "Invalid job id.", nil)
		return
	}
	job, ok := loadJob(r.Context(), w, state, jobID, rc)
	if !ok {
		return
	}

	// run is valid from idle/completed/failed — a job already queued or running has nothing to
	// do, and saying so beats a workflow error the UI would have to translate.
	status := ""
	if job.Status != nil {
		status = *job.Status
	}
	if status == "queued" || status == "running" {
		httpserver.WriteServiceError(w, http.StatusConflict, "scan_already_in_progress", "This scan job is already "+status+".", nil)
		return
	}

	queued, err := doTransition(r.Context(), state, job, "run", rc)
	if err != nil {
		httpserver.WriteInternalError(w, err)
		return
	}

	scanner, hasScanner := scannerURL()
	if !hasScanner {
		httpserver.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
			"job": queued, "dispatched": false,
			"detail": "Queued. No scanner backend is configured (SCANNER_URL unset), so nothing will pick this up yet.",
		}})
		return
	}

	// Fire-and-forget by design: the scanner reports back through the findings callback below,
	// so this request doesn't wait out a scan that can take minutes.
	payload := map[string]any{
		"scanJobId": queued.ID, "zoneId": queued.Data["zoneId"], "scanType": queued.Data["scanType"],
		"callbackPath": fmt.Sprintf("/internal/scan-jobs/%s/findings", queued.ID),
	}
	body, _ := json.Marshal(payload)
	resp, err := httpClient().Post(scanner+"/scans", "application/json", bytes.NewReader(body))
	if err != nil {
		httpserver.WriteServiceError(w, http.StatusBadGateway, "scanner_unavailable", "Could not reach the scanner: "+err.Error(), nil)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		httpserver.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"job": queued, "dispatched": true, "detail": "Handed to scanner."}})
		return
	}
	detail := fmt.Sprintf("Scanner rejected the request (%d)", resp.StatusCode)
	running, err := doTransition(r.Context(), state, queued, "start", rc)
	if err != nil {
		httpserver.WriteInternalError(w, err)
		return
	}
	failed, err := doTransition(r.Context(), state, running, "fail", rc)
	if err != nil {
		httpserver.WriteInternalError(w, err)
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"job": failed, "dispatched": false, "detail": detail}})
}

type findingInput struct {
	Severity    string  `json:"severity"`
	Category    string  `json:"category"`
	Endpoint    string  `json:"endpoint"`
	Description *string `json:"description"`
}

type findingsBody struct {
	Findings  []findingInput `json:"findings"`
	Succeeded *bool          `json:"succeeded"`
}

// SubmitFindings is POST /internal/scan-jobs/{id}/findings — scanner callback. Findings are
// created before the job is completed, so a portal user who sees "completed" always sees the
// findings that go with it.
func SubmitFindings(w http.ResponseWriter, r *http.Request, state *httpserver.AppState) {
	rc, ok := httpserver.RequireAuth(w, r, state)
	if !ok {
		return
	}
	jobID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpserver.WriteServiceError(w, http.StatusBadRequest, "validation_failed", "Invalid job id.", nil)
		return
	}
	var body findingsBody
	if !httpserver.DecodeJSONBody(w, r, &body) {
		return
	}
	succeeded := true
	if body.Succeeded != nil {
		succeeded = *body.Succeeded
	}

	job, ok := loadJob(r.Context(), w, state, jobID, rc)
	if !ok {
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	created := make([]uuid.UUID, 0, len(body.Findings))
	for _, finding := range body.Findings {
		data := crud.JsonObject{
			"scanJobId": jsonStr(job.ID.String()), "severity": jsonStr(finding.Severity),
			"category": jsonStr(finding.Category), "endpoint": jsonStr(finding.Endpoint),
			"firstSeenAt": jsonStr(now), "lastSeenAt": jsonStr(now),
		}
		if finding.Description != nil {
			data["description"] = jsonStr(*finding.Description)
		}
		result, err := state.Crud.Create(r.Context(), "waf.scan_findings", data, rc, nil)
		if err != nil {
			httpserver.WriteCrudError(w, err)
			return
		}
		created = append(created, result.ID)
	}

	// The job may still be "queued" if the scanner was quick — walk it through start first so
	// the transition graph is respected rather than jumped over.
	current := job
	if current.Status != nil && *current.Status == "queued" {
		current, err = doTransition(r.Context(), state, current, "start", rc)
		if err != nil {
			httpserver.WriteInternalError(w, err)
			return
		}
	}
	finish := "complete"
	if !succeeded {
		finish = "fail"
	}
	finished, err := doTransition(r.Context(), state, current, finish, rc)
	if err != nil {
		httpserver.WriteInternalError(w, err)
		return
	}

	// lastRunAt isn't part of any transition's SetFields, so it's written here — the one field
	// this flow owns that the workflow itself doesn't.
	patch := crud.JsonObject{"lastRunAt": jsonStr(now)}
	jobAfter, err := state.Crud.Update(r.Context(), "waf.scan_jobs", finished.ID, finished.Version, patch, rc, nil)
	if err != nil {
		httpserver.WriteCrudError(w, err)
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"jobId": finished.ID, "job": jobAfter, "createdFindings": created,
	}})
}

func jsonStr(s string) json.RawMessage {
	b, _ := json.Marshal(s)
	return b
}

// MountRoutes registers this service's custom (non-generic-CRUD) endpoints onto r.
func MountRoutes(r chi.Router, state *httpserver.AppState) {
	wrap := func(h func(http.ResponseWriter, *http.Request, *httpserver.AppState)) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) { h(w, r, state) }
	}
	r.Post("/api/waf.scan_jobs/{id}/run", wrap(RunScanJob))
	r.Post("/internal/scan-jobs/{id}/findings", wrap(SubmitFindings))
}
