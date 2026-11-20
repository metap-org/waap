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
	"github.com/metap-org/metap/pkg/kit/env"
	"github.com/metap-org/metap/pkg/kit/errcode"
	"github.com/metap-org/metap/pkg/kit/standard"
	"github.com/metap-org/metap/pkg/permission"
	"github.com/metap-org/waap/model"
)

// scannerURL: where a scan request is handed off. Unset (the default) means "no scanner
// deployed" — the job still queues, which is the honest state: something asked for a scan and
// nothing is going to run it yet.
func scannerURL() (string, bool) { return env.Lookup("SCANNER_URL") }

// scannerTimeout bounds the hand-off request to the scanner backend (the scan itself runs on).
const scannerTimeout = 15 * time.Second

func httpClient() *http.Client { return &http.Client{Timeout: scannerTimeout} }

// doTransition applies one workflow transition, mapping a service-level failure into a plain
// error. Transition names come from entities.ScanJobEntity's workflow (run/start/complete/fail)
// — this module never writes status directly, so guards, audit rows and outbox events all still
// happen.
func doTransition(ctx context.Context, state *httpserver.AppState, job crud.RecordDto, action string, rc *permission.RequestContext) (crud.RecordDto, error) {
	result, err := state.Crud.Transition(ctx, model.ScanJobs, job.ID, action, job.Version, nil, rc, nil)
	if err != nil {
		return crud.RecordDto{}, fmt.Errorf("transition %s: %w", action, err)
	}
	return result, nil
}

func loadJob(ctx context.Context, w http.ResponseWriter, state *httpserver.AppState, id uuid.UUID, rc *permission.RequestContext) (crud.RecordDto, bool) {
	result, err := state.Crud.Get(ctx, model.ScanJobs, id, rc)
	if err != nil {
		httpserver.WriteCrudError(w, err)
		return crud.RecordDto{}, false
	}
	return result.Record, true
}

// runScanJob is the "Run scan now" button — both POST /api/waf.scan_jobs/{id}/run and the declared
// action waf.scan_jobs.run call it.
func runScanJob(ctx context.Context, state *httpserver.AppState, rc *permission.RequestContext, jobID uuid.UUID) (map[string]any, error) {
	got, err := state.Crud.Get(ctx, model.ScanJobs, jobID, rc)
	if err != nil {
		return nil, err
	}
	job := got.Record

	// run is valid from idle/completed/failed — a job already queued or running has nothing to
	// do, and saying so beats a workflow error the UI would have to translate.
	status := ""
	if job.Status != nil {
		status = *job.Status
	}
	if status == model.ScanJobQueued || status == model.ScanJobRunning {
		return nil, crud.NewServiceErrorMsg(http.StatusConflict, model.ErrScanAlreadyInProgress, "This scan job is already "+status+".")
	}

	queued, err := doTransition(ctx, state, job, model.ScanJobRun, rc)
	if err != nil {
		return nil, err
	}

	scanner, hasScanner := scannerURL()
	if !hasScanner {
		return map[string]any{
			"job": queued, "dispatched": false,
			"detail": "Queued. No scanner backend is configured (SCANNER_URL unset), so nothing will pick this up yet.",
		}, nil
	}

	// Fire-and-forget by design: the scanner reports back through the findings callback below,
	// so this request doesn't wait out a scan that can take minutes.
	payload := map[string]any{
		"scanJobId": queued.ID, model.FieldZoneID: queued.Data[model.FieldZoneID], "scanType": queued.Data[model.FieldScanType],
		"callbackPath": fmt.Sprintf("/internal/scan-jobs/%s/findings", queued.ID),
	}
	body, _ := json.Marshal(payload)
	resp, err := httpClient().Post(scanner+"/scans", standard.MediaTypeJSON, bytes.NewReader(body))
	if err != nil {
		return nil, crud.NewServiceErrorMsg(http.StatusBadGateway, model.ErrScannerUnavailable, "Could not reach the scanner: "+err.Error())
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return map[string]any{"job": queued, "dispatched": true, "detail": "Handed to scanner."}, nil
	}
	detail := fmt.Sprintf("Scanner rejected the request (%d)", resp.StatusCode)
	running, err := doTransition(ctx, state, queued, model.ScanJobStart, rc)
	if err != nil {
		return nil, err
	}
	failed, err := doTransition(ctx, state, running, model.ScanJobFail, rc)
	if err != nil {
		return nil, err
	}
	return map[string]any{"job": failed, "dispatched": false, "detail": detail}, nil
}

// RegisterActions registers the handlers for the actions declared on this service's entities.
func RegisterActions(state *httpserver.AppState, reg *crud.ActionRegistry) error {
	return reg.Register(model.ScanJobs, model.ActionRun, func(ctx context.Context, c crud.ActionCall) (crud.JsonObject, error) {
		return crud.ToObject(runScanJob(ctx, state, c.RC, c.ID))
	})
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
		httpserver.WriteServiceError(w, http.StatusBadRequest, errcode.ValidationFailed, "Invalid job id.", nil)
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

	now := standard.NowRFC3339()
	created := make([]uuid.UUID, 0, len(body.Findings))
	for _, finding := range body.Findings {
		data := crud.JsonObject{
			model.FieldScanJobID: jsonStr(job.ID.String()), model.FieldSeverity: jsonStr(finding.Severity),
			model.FieldCategory: jsonStr(finding.Category), model.FieldEndpoint: jsonStr(finding.Endpoint),
			model.FieldFirstSeenAt: jsonStr(now), model.FieldLastSeenAt: jsonStr(now),
		}
		if finding.Description != nil {
			data[model.FieldDescription] = jsonStr(*finding.Description)
		}
		result, err := state.Crud.Create(r.Context(), model.ScanFindings, data, rc, nil)
		if err != nil {
			httpserver.WriteCrudError(w, err)
			return
		}
		created = append(created, result.ID)
	}

	// The job may still be "queued" if the scanner was quick — walk it through start first so
	// the transition graph is respected rather than jumped over.
	current := job
	if current.Status != nil && *current.Status == model.ScanJobQueued {
		current, err = doTransition(r.Context(), state, current, model.ScanJobStart, rc)
		if err != nil {
			httpserver.WriteInternalError(w, err)
			return
		}
	}
	finish := model.ScanJobComplete
	if !succeeded {
		finish = model.ScanJobFail
	}
	finished, err := doTransition(r.Context(), state, current, finish, rc)
	if err != nil {
		httpserver.WriteInternalError(w, err)
		return
	}

	// lastRunAt isn't part of any transition's SetFields, so it's written here — the one field
	// this flow owns that the workflow itself doesn't.
	patch := crud.JsonObject{model.FieldLastRunAt: jsonStr(now)}
	jobAfter, err := state.Crud.Update(r.Context(), model.ScanJobs, finished.ID, finished.Version, patch, rc, nil)
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
	r.Post("/api/waf.scan_jobs/{id}/run", httpserver.RecordActionRoute(state, "job", func(ctx context.Context, rc *permission.RequestContext, id uuid.UUID) (map[string]any, error) {
		return runScanJob(ctx, state, rc, id)
	}))
	r.Post("/internal/scan-jobs/{id}/findings", wrap(SubmitFindings))
}
