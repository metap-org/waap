// Layer: APP · declared-action logic. A business function behind a declared action; the zones package wires it (changeactions.go).

// Package changes is the life of a change request (UC-40, 42 to 46): draft, check, submit, then
// approve and publish, reject or cancel.
//
// A change is a list of operations on one zone's policy, or on the tenant-wide records (zone
// uuid.Nil). A tenant-wide change reaches every zone, so only an admin decides it, and it records no
// policy version: versions belong to a zone.
//
// Checking it is applying it: the operations
// run through the guarded backend, as the caller, inside a transaction that is then rolled back, so
// the check sees exactly the validation, plan limits, permissions and constraints publishing will
// meet, and the diff is of what really happened. Publishing runs them again and commits, but only if
// the zone's policy is still the one the diff was made against.
package changes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/metap-org/metap/pkg/action"
	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/kit/errcode"
	"github.com/metap-org/metap/pkg/permission"
	"github.com/metap-org/waap/m-management/zones/guard"
	"github.com/metap-org/waap/m-management/zones/internal/jx"
	"github.com/metap-org/waap/m-management/zones/policy"
	"github.com/metap-org/waap/m-management/zones/versions"
	"github.com/metap-org/waap/model"
)

// Operation kinds.
const (
	OpCreate = "create"
	OpUpdate = "update"
	OpDelete = "delete"
)

// MaxOperations bounds one change: it is applied in one transaction.
const MaxOperations = 200

// Operation is one write the change makes.
type Operation struct {
	Op     string `json:"op"`
	Entity string `json:"entity"`
	// ID is the record an update or delete is of.
	ID string `json:"id,omitempty"`
	// Data is the new record of a create, or the fields an update sets.
	Data crud.JsonObject `json:"data,omitempty"`
}

// DiffEntry is one record the change adds, alters or removes. Before and After are absent for a
// record that did not exist before or does not exist after.
type DiffEntry struct {
	Kind   string          `json:"kind"`
	Entity string          `json:"entity"`
	ID     string          `json:"id,omitempty"`
	Before crud.JsonObject `json:"before,omitempty"`
	After  crud.JsonObject `json:"after,omitempty"`
}

// CheckError is why the operations could not be applied.
type CheckError struct {
	// Operation is the index of the one that failed.
	Operation   int                 `json:"operation"`
	Code        string              `json:"code"`
	Message     string              `json:"message,omitempty"`
	FieldErrors map[string][]string `json:"fieldErrors,omitempty"`
}

// CheckResult is what the last check found. OK means the change can be submitted.
type CheckResult struct {
	OK       bool             `json:"ok"`
	Error    *CheckError      `json:"error,omitempty"`
	Findings []policy.Finding `json:"findings"`
}

// Outcome is what every change action answers.
type Outcome struct {
	ID               string      `json:"id"`
	Status           string      `json:"status"`
	Check            CheckResult `json:"check"`
	Diff             []DiffEntry `json:"diff"`
	PublishedVersion int         `json:"publishedVersion,omitempty"`
}

// Service runs the change actions.
type Service struct {
	// Records is the service's own backend, under the guards: what a change request row is read and
	// written through, since clients may not.
	Records crud.RecordBackend
	// Policy is the guarded backend a change's operations go through, as a person's own edits do.
	Policy      crud.RecordBackend
	Permissions *permission.PermissionService
	Locks       guard.Locker
}

func invalid(format string, args ...any) error {
	return crud.NewServiceErrorMsg(http.StatusUnprocessableEntity, errcode.ValidationFailed, fmt.Sprintf(format, args...))
}

func conflict(code, message string) error {
	return crud.NewServiceErrorMsg(http.StatusConflict, code, message)
}

func forbidden(code, message string) error {
	return crud.NewServiceErrorMsg(http.StatusForbidden, code, message)
}

// authorize refuses a caller who may not make every kind of write the operations make. Applying them
// checks each record as well; this is what stops someone with no right to the policy at all, whose
// operations would otherwise fail on something else first.
func (s Service) authorize(ctx context.Context, ops []Operation, rc *permission.RequestContext) error {
	checks := map[string]func(context.Context, *permission.RequestContext, string) (permission.PermissionDecision, error){
		OpCreate: s.Permissions.CanCreateEntity,
		OpUpdate: s.Permissions.CanUpdateEntity,
		OpDelete: s.Permissions.CanDeleteEntity,
	}
	for _, op := range ops {
		decision, err := checks[op.Op](ctx, rc, op.Entity)
		if err != nil {
			return err
		}
		if !decision.Allowed {
			return forbidden(errcode.Forbidden, "You may not "+op.Op+" "+op.Entity+".")
		}
	}
	return nil
}

// validateShape checks what can be checked without the zone's records.
func validateShape(ops []Operation) error {
	if len(ops) == 0 {
		return invalid("a change needs at least one operation")
	}
	if len(ops) > MaxOperations {
		return invalid("a change takes at most %d operations", MaxOperations)
	}
	seen := map[string]bool{}
	for i, op := range ops {
		if !versions.IsVersioned(op.Entity) {
			return invalid("operation %d: %q is not part of a zone's policy", i, op.Entity)
		}
		switch op.Op {
		case OpCreate:
			if op.ID != "" || len(op.Data) == 0 {
				return invalid("operation %d: a create takes data and no id", i)
			}
			continue
		case OpUpdate:
			if len(op.Data) == 0 {
				return invalid("operation %d: an update takes data", i)
			}
		case OpDelete:
			if len(op.Data) != 0 {
				return invalid("operation %d: a delete takes no data", i)
			}
		default:
			return invalid("operation %d: op must be create, update or delete", i)
		}
		if _, err := uuid.Parse(op.ID); err != nil {
			return invalid("operation %d: id is required", i)
		}
		if seen[op.ID] {
			return invalid("operation %d: one change touches a record once", i)
		}
		seen[op.ID] = true
	}
	return nil
}

// operationError carries which operation failed.
type operationError struct {
	index int
	err   error
}

func (e *operationError) Error() string { return fmt.Sprintf("operation %d: %v", e.index, e.err) }
func (e *operationError) Unwrap() error { return e.err }

// apply runs ops on the zone as rc. ctx must be inside a Step. It returns the zone's policy before
// and after.
func (s Service) apply(ctx context.Context, zoneID uuid.UUID, ops []Operation, rc *permission.RequestContext) (before, after versions.Snapshot, err error) {
	before, lockVersions, err := versions.ReadZoneState(ctx, s.Records, zoneID, rc)
	if err != nil {
		return nil, nil, err
	}
	mine := map[string]string{}
	for entity, items := range before {
		for _, it := range items {
			mine[it.ID] = entity
		}
	}
	// One version for the whole change, recorded by the caller of apply, not one per operation.
	writes := guard.Publishing(versions.WithoutVersions(ctx))
	for i, op := range ops {
		fail := func(err error) (versions.Snapshot, versions.Snapshot, error) {
			return nil, nil, &operationError{index: i, err: err}
		}
		if raw, ok := op.Data[model.FieldZoneID]; ok && !sameZone(raw, zoneID) {
			return fail(invalid("a change stays in its own zone"))
		}
		if op.Op == OpCreate {
			data := jx.Clone(op.Data)
			if zoneID != uuid.Nil {
				data[model.FieldZoneID] = jx.String(zoneID.String())
			}
			if _, err := s.Policy.Create(writes, op.Entity, data, rc, nil); err != nil {
				return fail(err)
			}
			continue
		}
		if mine[op.ID] != op.Entity {
			return fail(crud.NewServiceErrorMsg(http.StatusNotFound, errcode.RecordNotFound, "This zone has no such record."))
		}
		id := uuid.MustParse(op.ID)
		if op.Op == OpUpdate {
			_, err = s.Policy.Update(writes, op.Entity, id, lockVersions[op.ID], op.Data, rc, nil)
		} else {
			_, err = s.Policy.Delete(writes, op.Entity, id, lockVersions[op.ID], rc, nil)
		}
		if err != nil {
			return fail(err)
		}
	}
	after, _, err = versions.ReadZoneState(ctx, s.Records, zoneID, rc)
	return before, after, err
}

// sameZone says whether a zoneId in an operation's data names the change's own zone; for a
// tenant-wide change, whether it names none.
func sameZone(raw json.RawMessage, zoneID uuid.UUID) bool {
	id, isString := jx.AsString(raw)
	if zoneID == uuid.Nil {
		return string(raw) == "null" || (isString && id == "")
	}
	return id == zoneID.String()
}

func diff(before, after versions.Snapshot) []DiffEntry {
	out := []DiffEntry{}
	for _, entity := range []string{model.DdosPolicies, model.FirewallRules, model.IPAccessLists} {
		old := map[string]crud.JsonObject{}
		for _, it := range before[entity] {
			old[it.ID] = it.Data
		}
		kept := map[string]bool{}
		for _, it := range after[entity] {
			was, existed := old[it.ID]
			kept[it.ID] = true
			if !existed {
				// The id a check gave a new record is not the id publishing will give it.
				out = append(out, DiffEntry{Kind: OpCreate, Entity: entity, After: it.Data})
				continue
			}
			a, _ := json.Marshal(was)
			b, _ := json.Marshal(it.Data)
			if string(a) != string(b) {
				out = append(out, DiffEntry{Kind: OpUpdate, Entity: entity, ID: it.ID, Before: was, After: it.Data})
			}
		}
		for _, it := range before[entity] {
			if !kept[it.ID] {
				out = append(out, DiffEntry{Kind: OpDelete, Entity: entity, ID: it.ID, Before: it.Data})
			}
		}
	}
	return out
}

var errRollback = errors.New("checked, rolling back")

type checked struct {
	result   CheckResult
	diff     []DiffEntry
	baseHash string
}

// check applies ops and rolls them back. A mistake in the change is in the result; an error is
// returned only for a refusal of the caller or a failure of the service.
func (s Service) check(ctx context.Context, a *action.Ctx, zoneID uuid.UUID, ops []Operation) (checked, error) {
	out := checked{result: CheckResult{Findings: []policy.Finding{}}, diff: []DiffEntry{}}
	err := a.Step(ctx, "check", func(ctx context.Context) error {
		before, after, err := s.apply(ctx, zoneID, ops, a.RC)
		if err != nil {
			return err
		}
		if out.baseHash, err = versions.Hash(before); err != nil {
			return err
		}
		out.diff = diff(before, after)
		report, err := policy.Validate(ctx, s.Records, zoneID, a.RC)
		if err != nil {
			return err
		}
		out.result.Findings = report.Findings
		out.result.OK = report.Errors == 0 && len(out.diff) > 0
		if len(out.diff) == 0 {
			out.result.Error = &CheckError{Operation: -1, Code: "no_change", Message: "These operations change nothing."}
		}
		return errRollback
	})
	if errors.Is(err, errRollback) {
		return out, nil
	}
	var failed *operationError
	if errors.As(err, &failed) {
		if se, ok := crud.AsServiceError(failed.err); ok && se.Status != http.StatusForbidden && se.Status < http.StatusInternalServerError {
			out.result.Error = &CheckError{Operation: failed.index, Code: se.Code, Message: se.Message, FieldErrors: se.FieldErrors}
			return out, nil
		}
		return out, failed.err
	}
	if err == nil {
		// Step without a transaction would have kept the writes.
		return out, errors.New("changes: a check needs a transaction to roll back")
	}
	return out, err
}

func raw(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

func (c checked) fields() crud.JsonObject {
	return crud.JsonObject{
		model.FieldCheckResult: raw(c.result),
		model.FieldDiff:        raw(c.diff),
		model.FieldBaseHash:    jx.String(c.baseHash),
	}
}

// ProposeInput is the input of waf.zones.createChangeRequest.
type ProposeInput struct {
	Title       string      `json:"title"`
	Description string      `json:"description"`
	Operations  []Operation `json:"operations"`
}

// Propose drafts a change to zone a.ID. A draft whose check fails is kept, with why.
func (s Service) Propose(ctx context.Context, a *action.Ctx) (any, error) {
	return s.propose(ctx, a, a.ID)
}

// ProposeTenantWide drafts a change to the tenant-wide records.
func (s Service) ProposeTenantWide(ctx context.Context, a *action.Ctx) (any, error) {
	return s.propose(ctx, a, uuid.Nil)
}

func (s Service) propose(ctx context.Context, a *action.Ctx, zoneID uuid.UUID) (any, error) {
	var in ProposeInput
	if err := a.Decode(&in); err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Title) == "" {
		return nil, invalid("title is required")
	}
	if err := validateShape(in.Operations); err != nil {
		return nil, err
	}
	if zoneID != uuid.Nil {
		if err := a.LoadRecord(ctx, nil); err != nil { // the zone exists and the caller may read it
			return nil, err
		}
	}
	if err := s.authorize(ctx, in.Operations, a.RC); err != nil {
		return nil, err
	}
	result, err := s.check(ctx, a, zoneID, in.Operations)
	if err != nil {
		return nil, err
	}
	data := result.fields()
	if zoneID != uuid.Nil {
		data[model.FieldZoneID] = jx.String(zoneID.String())
		if account, ok := a.Current.Data[model.FieldAccountID]; ok {
			data[model.FieldAccountID] = account
		}
	}
	data[model.FieldTitle] = jx.String(strings.TrimSpace(in.Title))
	data[model.FieldDescription] = jx.String(in.Description)
	data[model.FieldStatus] = jx.String(model.ChangeDraft)
	data[model.FieldOperations] = raw(in.Operations)
	data[model.FieldAuthorID] = jx.String(a.RC.UserID)
	var created crud.RecordDto
	err = a.Step(ctx, "draft", func(ctx context.Context) error {
		var err error
		created, err = s.Records.Create(ctx, model.ChangeRequests, data, versions.AsAdmin(a.RC), nil)
		return err
	})
	if err != nil {
		return nil, err
	}
	return Outcome{ID: created.ID.String(), Status: model.ChangeDraft, Check: result.result, Diff: result.diff}, nil
}

// request is a change request as the caller may read it.
type request struct {
	zoneID   uuid.UUID
	status   string
	author   string
	title    string
	baseHash string
	ops      []Operation
}

func load(ctx context.Context, a *action.Ctx) (request, error) {
	var r request
	if err := a.LoadRecord(ctx, nil); err != nil {
		return r, err
	}
	data := a.Current.Data
	r.zoneID = versions.ZoneOfRule(data)
	r.status, _ = jx.AsString(data[model.FieldStatus])
	r.author, _ = jx.AsString(data[model.FieldAuthorID])
	r.title, _ = jx.AsString(data[model.FieldTitle])
	r.baseHash, _ = jx.AsString(data[model.FieldBaseHash])
	if err := json.Unmarshal(data[model.FieldOperations], &r.ops); err != nil {
		return r, fmt.Errorf("changes: unreadable operations: %w", err)
	}
	return r, nil
}

func (r request) mustBe(statuses ...string) error {
	for _, s := range statuses {
		if r.status == s {
			return nil
		}
	}
	return conflict(model.ErrChangeRequestState, fmt.Sprintf("This change request is %s.", r.status))
}

// store writes the change request; it fails if someone else wrote it since it was loaded.
func (s Service) store(ctx context.Context, a *action.Ctx, data crud.JsonObject) error {
	_, err := s.Records.Update(ctx, model.ChangeRequests, a.ID, a.Current.Version, data, versions.AsAdmin(a.RC), nil)
	return err
}

func (s Service) storeStep(ctx context.Context, a *action.Ctx, name string, data crud.JsonObject) error {
	return a.Step(ctx, name, func(ctx context.Context) error {
		if err := s.store(ctx, a, data); err != nil {
			return err
		}
		a.Audit("change_request."+name, a.ID, nil)
		return nil
	})
}

type reviseIn struct {
	Operations []Operation `json:"operations"`
}

// Revise replaces the operations. Only the author: whoever writes what a change does must be the one
// the second approver is checked against.
func (s Service) Revise(ctx context.Context, a *action.Ctx) (any, error) {
	var in reviseIn
	if err := a.Decode(&in); err != nil {
		return nil, err
	}
	if err := validateShape(in.Operations); err != nil {
		return nil, err
	}
	r, err := load(ctx, a)
	if err != nil {
		return nil, err
	}
	if r.author != a.RC.UserID {
		return nil, forbidden(errcode.Forbidden, "Only the author may revise a change.")
	}
	if err := r.mustBe(model.ChangeDraft, model.ChangePendingReview, model.ChangeRejected); err != nil {
		return nil, err
	}
	if err := s.authorize(ctx, in.Operations, a.RC); err != nil {
		return nil, err
	}
	result, err := s.check(ctx, a, r.zoneID, in.Operations)
	if err != nil {
		return nil, err
	}
	data := result.fields()
	data[model.FieldOperations] = raw(in.Operations)
	data[model.FieldStatus] = jx.String(model.ChangeDraft)
	if err := s.storeStep(ctx, a, "revise", data); err != nil {
		return nil, err
	}
	return Outcome{ID: a.ID.String(), Status: model.ChangeDraft, Check: result.result, Diff: result.diff}, nil
}

// Submit checks the change against the zone as it is now and asks for its approval. A change already
// waiting may be submitted again, to take a new diff after the zone moved.
func (s Service) Submit(ctx context.Context, a *action.Ctx) (any, error) {
	r, err := load(ctx, a)
	if err != nil {
		return nil, err
	}
	if r.author != a.RC.UserID {
		return nil, forbidden(errcode.Forbidden, "Only the author may submit a change.")
	}
	if err := r.mustBe(model.ChangeDraft, model.ChangePendingReview); err != nil {
		return nil, err
	}
	if err := s.authorize(ctx, r.ops, a.RC); err != nil {
		return nil, err
	}
	result, err := s.check(ctx, a, r.zoneID, r.ops)
	if err != nil {
		return nil, err
	}
	data := result.fields()
	status := model.ChangeDraft
	if result.result.OK {
		status = model.ChangePendingReview
		data[model.FieldSubmittedAt] = jx.String(time.Now().UTC().Format(time.RFC3339))
	}
	data[model.FieldStatus] = jx.String(status)
	if err := s.storeStep(ctx, a, "submit", data); err != nil {
		return nil, err
	}
	// A check that failed is kept on the draft, and the caller is told.
	if !result.result.OK {
		return nil, conflict(model.ErrChangeRequestNotReady, "The check did not pass; see the draft's check result.")
	}
	return Outcome{ID: a.ID.String(), Status: status, Check: result.result, Diff: result.diff}, nil
}

type decideIn struct {
	Comment string `json:"comment"`
}

func decision(a *action.Ctx, status, kind, comment string) crud.JsonObject {
	return crud.JsonObject{
		model.FieldStatus:          jx.String(status),
		model.FieldDecision:        jx.String(kind),
		model.FieldDecisionComment: jx.String(comment),
		model.FieldDecidedBy:       jx.String(a.RC.UserID),
		model.FieldDecidedAt:       jx.String(time.Now().UTC().Format(time.RFC3339)),
	}
}

// mayDecide refuses a caller who may not approve or reject r: they need the rights to make its
// writes, and for a tenant-wide change they must be an admin.
func (s Service) mayDecide(ctx context.Context, r request, rc *permission.RequestContext) error {
	if r.zoneID == uuid.Nil && !rc.IsAdmin() {
		return forbidden(errcode.Forbidden, "Only an admin decides a tenant-wide change.")
	}
	return s.authorize(ctx, r.ops, rc)
}

// Approve publishes the change: its operations, one new policy version naming who approved it, and
// the change request marked published, all in one transaction. The approver makes the writes, so it
// takes the rights to make them by hand.
func (s Service) Approve(ctx context.Context, a *action.Ctx) (any, error) {
	var in decideIn
	if err := a.Decode(&in); err != nil {
		return nil, err
	}
	r, err := load(ctx, a)
	if err != nil {
		return nil, err
	}
	if err := r.mustBe(model.ChangePendingReview); err != nil {
		return nil, err
	}
	if err := s.mayDecide(ctx, r, a.RC); err != nil {
		return nil, err
	}
	kind := model.DecisionApproved
	if r.author == a.RC.UserID {
		mode, err := guard.ChangeControl(ctx, s.Records, r.zoneID, a.RC)
		if err != nil {
			return nil, err
		}
		if mode == model.ChangeControlFourEyes {
			return nil, forbidden(model.ErrSelfApproval, "This account requires someone else to approve your change.")
		}
		kind = model.DecisionSelfConfirmed
	}
	tenant, err := uuid.Parse(a.RC.TenantID)
	if err != nil {
		return nil, forbidden(errcode.Forbidden, "no tenant")
	}
	var published int
	var applied []DiffEntry
	// Two changes to one zone are published one after the other, so the second sees the first and is
	// stale.
	// The lock is taken inside the step, so it is held until the step's transaction ends.
	err = a.Step(ctx, "publish", func(ctx context.Context) error {
		return s.Locks.WithKeyLock(ctx, tenant, "policy-change:"+r.zoneID.String(), func(ctx context.Context) error {
			versions.EnsureBaseline(ctx, s.Records, r.zoneID, a.RC)
			before, after, err := s.apply(ctx, r.zoneID, r.ops, a.RC)
			if err != nil {
				return err
			}
			hash, err := versions.Hash(before)
			if err != nil {
				return err
			}
			if hash != r.baseHash {
				return conflict(model.ErrChangeRequestStale, "The zone's policy changed since this was submitted. Its author must submit it again.")
			}
			applied = diff(before, after)
			data := decision(a, model.ChangePublished, kind, in.Comment)
			if r.zoneID != uuid.Nil {
				// The zone's mirror of "has any configuration" is the system's to write.
				if err := versions.SyncHasConfig(ctx, s.Records, r.zoneID, versions.AsAdmin(a.RC)); err != nil {
					return err
				}
				published, err = versions.RecordVersion(ctx, s.Records, r.zoneID, a.RC, versions.Published(r.title, a.ID.String(), a.RC.UserID))
				if err != nil {
					return err
				}
				data[model.FieldPublishedVersion] = raw(published)
			}
			if err := s.store(ctx, a, data); err != nil {
				return err
			}
			a.Audit("change_request."+kind, a.ID, applied)
			return nil
		})
	})
	if err != nil {
		var failed *operationError
		if errors.As(err, &failed) {
			return nil, failed.err
		}
		return nil, err
	}
	return Outcome{ID: a.ID.String(), Status: model.ChangePublished, Check: CheckResult{OK: true, Findings: []policy.Finding{}}, Diff: applied, PublishedVersion: published}, nil
}

// Reject sends a waiting change back. It takes the rights approving it would, and the author uses
// cancel instead.
func (s Service) Reject(ctx context.Context, a *action.Ctx) (any, error) {
	var in decideIn
	if err := a.Decode(&in); err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Comment) == "" {
		return nil, invalid("comment is required to reject")
	}
	r, err := load(ctx, a)
	if err != nil {
		return nil, err
	}
	if err := r.mustBe(model.ChangePendingReview); err != nil {
		return nil, err
	}
	if r.author == a.RC.UserID {
		return nil, forbidden(errcode.Forbidden, "Cancel your own change instead of rejecting it.")
	}
	if err := s.mayDecide(ctx, r, a.RC); err != nil {
		return nil, err
	}
	if err := s.storeStep(ctx, a, "reject", decision(a, model.ChangeRejected, model.DecisionRejected, in.Comment)); err != nil {
		return nil, err
	}
	return map[string]string{"id": a.ID.String(), "status": model.ChangeRejected}, nil
}

// Cancel closes a change that has not been published. The author, or an admin.
func (s Service) Cancel(ctx context.Context, a *action.Ctx) (any, error) {
	r, err := load(ctx, a)
	if err != nil {
		return nil, err
	}
	if r.author != a.RC.UserID && !a.RC.IsAdmin() {
		return nil, forbidden(errcode.Forbidden, "Only the author or an admin may cancel a change.")
	}
	if err := r.mustBe(model.ChangeDraft, model.ChangePendingReview, model.ChangeRejected); err != nil {
		return nil, err
	}
	if err := s.storeStep(ctx, a, "cancel", crud.JsonObject{model.FieldStatus: jx.String(model.ChangeCancelled)}); err != nil {
		return nil, err
	}
	return map[string]string{"id": a.ID.String(), "status": model.ChangeCancelled}, nil
}
