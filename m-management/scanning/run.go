// Layer: APP · declared-action logic. Hands a job to the scanner (action run).

package scanning

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/metap-org/metap/pkg/action"
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

// scanRun is what the run action answers.
type scanRun struct {
	Job        crud.RecordDto `json:"job"`
	Dispatched bool           `json:"dispatched"`
	Detail     string         `json:"detail"`
}

// RunScanJob is the "Run scan now" button: the declared action waf.scan_jobs.run.
//
// The job is queued first (one workflow transition), then handed to the scanner outside any
// transaction (the call can be slow), then, only if the scanner refuses it, started and failed as
// one step: a job is never left running because the second of those two moves did not happen.
func RunScanJob(cfg Config, state *httpserver.AppState) action.Action {
	return action.Record(model.ScanJobs, model.ActionRun,
		func(ctx context.Context, a *action.Ctx, _ action.None, _ action.None) (scanRun, error) {
			job := a.Current

			// run is valid from idle/completed/failed — a job already queued or running has nothing to
			// do, and saying so beats a workflow error the UI would have to translate.
			status := ""
			if job.Status != nil {
				status = *job.Status
			}
			if status == model.ScanJobQueued || status == model.ScanJobRunning {
				return scanRun{}, crud.NewServiceErrorMsg(http.StatusConflict, model.ErrScanAlreadyInProgress, "This scan job is already "+status+".")
			}

			queued, err := doTransition(ctx, state, job, model.ScanJobRun, a.RC)
			if err != nil {
				return scanRun{}, err
			}

			scanner := cfg.ScannerURL
			if scanner == "" {
				return scanRun{Job: queued, Detail: "Queued. No scanner backend is configured (SCANNER_URL unset), so nothing will pick this up yet."}, nil
			}

			// Fire-and-forget by design: the scanner reports back through the findings callback, so
			// this request doesn't wait out a scan that can take minutes.
			payload := map[string]any{
				"scanJobId": queued.ID, model.FieldZoneID: queued.Data[model.FieldZoneID], "scanType": queued.Data[model.FieldScanType],
				"callbackPath": fmt.Sprintf("/internal/scan-jobs/%s/findings", queued.ID),
			}
			body, _ := json.Marshal(payload)
			resp, err := httpx.NewClient(scannerTimeout).Post(scanner+"/scans", standard.MediaTypeJSON, bytes.NewReader(body))
			if err != nil {
				return scanRun{}, crud.NewServiceErrorMsg(http.StatusBadGateway, model.ErrScannerUnavailable, "Could not reach the scanner: "+err.Error())
			}
			defer resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return scanRun{Job: queued, Dispatched: true, Detail: "Handed to scanner."}, nil
			}

			detail := fmt.Sprintf("Scanner rejected the request (%d)", resp.StatusCode)
			var failed crud.RecordDto
			if err := a.Step(ctx, "mark failed", func(ctx context.Context) error {
				running, err := doTransition(ctx, state, queued, model.ScanJobStart, a.RC)
				if err != nil {
					return err
				}
				failed, err = doTransition(ctx, state, running, model.ScanJobFail, a.RC)
				return err
			}); err != nil {
				return scanRun{}, err
			}
			return scanRun{Job: failed, Detail: detail}, nil
		})
}
