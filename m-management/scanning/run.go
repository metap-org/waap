// Layer: APP · declared-action logic. Hands a job to the scanner (action run).

package scanning

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/httpserver"
	"github.com/metap-org/metap/pkg/kit/httpx"
	"github.com/metap-org/metap/pkg/kit/standard"
	"github.com/metap-org/metap/pkg/permission"
	"github.com/metap-org/waap/model"
)

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

// runScanJob is the "Run scan now" button: the declared action waf.scan_jobs.run.
func runScanJob(ctx context.Context, cfg Config, state *httpserver.AppState, rc *permission.RequestContext, jobID uuid.UUID) (map[string]any, error) {
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

	scanner := cfg.ScannerURL
	if scanner == "" {
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
	resp, err := httpx.NewClient(scannerTimeout).Post(scanner+"/scans", standard.MediaTypeJSON, bytes.NewReader(body))
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
