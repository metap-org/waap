// Layer: APP · callback logic. What the scanner posts back.

package scanning

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/httpserver"
	"github.com/metap-org/metap/pkg/kit/errcode"
	"github.com/metap-org/metap/pkg/kit/standard"
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

	if len(body.Findings) > maxFindingsPerCall {
		httpserver.WriteServiceError(w, http.StatusRequestEntityTooLarge, model.ErrTooManyFindings,
			fmt.Sprintf("At most %d findings per call; send the rest in further calls.", maxFindingsPerCall), nil)
		return
	}

	job, ok := loadJob(r.Context(), w, state, jobID, rc)
	if !ok {
		return
	}
	// Only a job that is waiting for or running a scan takes results. Checked before anything is
	// written: a repeated callback, or one for an idle/finished job, must not add findings and then
	// fail on the transition.
	if job.Status == nil || (*job.Status != model.ScanJobQueued && *job.Status != model.ScanJobRunning) {
		httpserver.WriteServiceError(w, http.StatusConflict, model.ErrScanNotRunnable,
			"This scan job is not waiting for results.", nil)
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
