// Layer: APP · callback logic. What the scanner posts back.

package scanning

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/metap-org/metap/pkg/action"
	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/kit/errcode"
	"github.com/metap-org/metap/pkg/kit/standard"
	"github.com/metap-org/metap/pkg/web"
	"github.com/metap-org/waap/model"
)

// maxFindingsPerCall bounds the rows one callback writes (the request body is already capped).
const maxFindingsPerCall = 500

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
func SubmitFindings(w http.ResponseWriter, r *http.Request, state *web.State) {
	rc := web.RC(r) // the route's gate resolved the caller (MountRoutes)
	records := state.Crud
	jobID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		web.WriteServiceError(w, http.StatusBadRequest, errcode.ValidationFailed, "Invalid job id.", nil)
		return
	}
	var body findingsBody
	if !web.DecodeJSONBody(w, r, &body) {
		return
	}
	succeeded := true
	if body.Succeeded != nil {
		succeeded = *body.Succeeded
	}

	if len(body.Findings) > maxFindingsPerCall {
		web.WriteServiceError(w, http.StatusRequestEntityTooLarge, model.ErrTooManyFindings,
			fmt.Sprintf("At most %d findings per call; send the rest in further calls.", maxFindingsPerCall), nil)
		return
	}

	job, ok := loadJob(r.Context(), w, records, jobID, rc)
	if !ok {
		return
	}
	// Only a job that is waiting for or running a scan takes results. Checked before anything is
	// written: a repeated callback, or one for an idle/finished job, must not add findings and then
	// fail on the transition.
	if job.Status == nil || (*job.Status != model.ScanJobQueued && *job.Status != model.ScanJobRunning) {
		web.WriteServiceError(w, http.StatusConflict, model.ErrScanNotRunnable,
			"This scan job is not waiting for results.", nil)
		return
	}

	// The findings, the job's walk through its workflow and lastRunAt are one step: they commit together
	// or not at all. Before this a failure half way (a finding refused, a stale version on the
	// transition) left findings on a job that was still "queued", and a retry then added them again.
	unit, err := action.NewUnit(state.ActionDeps().Sink, rc)
	if err != nil {
		web.WriteCrudError(w, err)
		return
	}
	var created []uuid.UUID
	var finished, jobAfter crud.RecordDto
	err = unit.Step(r.Context(), "submit findings", func(ctx context.Context) error {
		now := standard.NowRFC3339()
		created = make([]uuid.UUID, 0, len(body.Findings))
		for _, finding := range body.Findings {
			data := crud.JsonObject{
				model.FieldScanJobID: jsonStr(job.ID.String()), model.FieldSeverity: jsonStr(finding.Severity),
				model.FieldCategory: jsonStr(finding.Category), model.FieldEndpoint: jsonStr(finding.Endpoint),
				model.FieldFirstSeenAt: jsonStr(now), model.FieldLastSeenAt: jsonStr(now),
			}
			if finding.Description != nil {
				data[model.FieldDescription] = jsonStr(*finding.Description)
			}
			result, err := records.Create(ctx, model.ScanFindings, data, rc, nil)
			if err != nil {
				return err
			}
			created = append(created, result.ID)
		}

		// The job may still be "queued" if the scanner was quick — walk it through start first so
		// the transition graph is respected rather than jumped over.
		current := job
		if current.Status != nil && *current.Status == model.ScanJobQueued {
			var err error
			if current, err = doTransition(ctx, records, current, model.ScanJobStart, rc); err != nil {
				return err
			}
		}
		finish := model.ScanJobComplete
		if !succeeded {
			finish = model.ScanJobFail
		}
		var err error
		if finished, err = doTransition(ctx, records, current, finish, rc); err != nil {
			return err
		}

		// lastRunAt isn't part of any transition's SetFields, so it's written here — the one field
		// this flow owns that the workflow itself doesn't.
		patch := crud.JsonObject{model.FieldLastRunAt: jsonStr(now)}
		jobAfter, err = records.Update(ctx, model.ScanJobs, finished.ID, finished.Version, patch, rc, nil)
		return err
	})
	if err != nil {
		web.WriteCrudError(w, err)
		return
	}
	if err := unit.Close(r.Context()); err != nil {
		web.WriteInternalError(w, err)
		return
	}
	web.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"jobId": finished.ID, "job": jobAfter, "createdFindings": created,
	}})
}

func jsonStr(s string) json.RawMessage {
	b, _ := json.Marshal(s)
	return b
}
