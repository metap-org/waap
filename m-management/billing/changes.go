// Layer: APP · declared-action logic. metap serves POST /api/{entity}[/{id}]/actions/{action} and calls the handler registered in routes.go.

// Changing plan (UC-71, UC-73). A customer cannot edit its own subscription, so it asks: a
// plan change request in the operator's tenant. The operator's admin decides. Approving an upgrade
// applies at once; approving a downgrade applies from the end of the billing period
// (billingPeriod after approval: there is no payment, so no real period), by recording the move on
// the subscription (pendingPlanId, pendingFrom) and letting the plan resolution switch at that time.
//
// A downgrade never removes what the customer already has: limits only stop new creates, so a
// customer over the smaller plan keeps its zones and simply cannot add more (the grace UC-73 asks
// for, without a timer).
package billing

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/httpserver"
	"github.com/metap-org/metap/pkg/kit/errcode"
	"github.com/metap-org/metap/pkg/permission"
	"github.com/metap-org/metap/pkg/query"
	"github.com/metap-org/waap/model"
)

// billingPeriod is how long after approval a downgrade waits before it applies.
const billingPeriod = 30 * 24 * time.Hour

// Request statuses (waf.plan_change_requests.status).
const (
	requestPending  = "pending"
	requestApproved = "approved"
	requestRejected = "rejected"
)

// effectivePlanID is the plan a subscription gives right now: a pending move once its time has come.
func effectivePlanID(sub crud.JsonObject, now time.Time) json.RawMessage {
	if pending, ok := jsonAsString(sub[model.FieldPendingPlanID]); ok && pending != "" {
		if from, ok := jsonAsString(sub[model.FieldPendingFrom]); ok {
			if at, err := time.Parse(time.RFC3339, from); err == nil && !at.After(now) {
				return sub[model.FieldPendingPlanID]
			}
		}
	}
	return sub[model.FieldPlanID]
}

// allowance orders plans for "is this an upgrade": unlimited is the largest. Two numbers, zones
// first, then rules per zone.
func allowance(p Plan) [2]int {
	rank := func(n int) int {
		if n == Unlimited {
			return int(^uint(0) >> 1)
		}
		return n
	}
	return [2]int{rank(p.MaxZones), rank(p.MaxRulesPerZone)}
}

// isUpgrade reports whether moving from -> to gives at least as much of everything and more of
// something. Any plan that gives less of something is a downgrade, so a customer is never moved to
// less before its period ends.
func isUpgrade(from, to Plan) bool {
	a, b := allowance(from), allowance(to)
	return b[0] >= a[0] && b[1] >= a[1] && b != a
}

func forbidden(msg string) error {
	return crud.NewServiceErrorMsg(http.StatusForbidden, errcode.Forbidden, msg)
}

func invalid(field, msg string) error {
	return crud.NewServiceErrorFields(http.StatusUnprocessableEntity, errcode.ValidationFailed, map[string][]string{field: {msg}})
}

func (p *PlanSource) planByID(ctx context.Context, id uuid.UUID) (Plan, bool) {
	got, err := p.Backend.Get(ctx, model.Plans, id, p.operatorRC())
	if err != nil {
		return Plan{}, false
	}
	return planFromRecord(got.Record), true
}

// RequestPlanChange records that the caller's workspace wants another plan. Only that workspace's
// admin may ask, and only for an enabled plan other than the one it has.
func (p *PlanSource) RequestPlanChange(ctx context.Context, rc *permission.RequestContext, planID, note, customerName string) (crud.JsonObject, error) {
	if !p.enforced() || rc.TenantID == p.Operator.String() {
		return nil, forbidden("This workspace is not on a plan.")
	}
	if !rc.IsAdmin() {
		return nil, forbidden("Only an admin can ask for a plan change.")
	}
	tenant, err := uuid.Parse(rc.TenantID)
	if err != nil {
		return nil, err
	}
	id, err := uuid.Parse(planID)
	if err != nil {
		return nil, invalid(model.FieldPlanID, "not a valid id")
	}
	target, ok := p.planByID(ctx, id)
	if !ok {
		return nil, invalid(model.FieldPlanID, "no such plan")
	}
	enabled, err := p.Enabled(ctx)
	if err != nil {
		return nil, err
	}
	offered := false
	for _, e := range enabled {
		offered = offered || e.ID == target.ID
	}
	if !offered {
		return nil, invalid(model.FieldPlanID, "this plan is not offered")
	}
	current, _, err := p.For(ctx, tenant)
	if err != nil {
		return nil, err
	}
	if current.ID == target.ID {
		return nil, invalid(model.FieldPlanID, "already on this plan")
	}
	rc0 := p.operatorRC()
	open, err := p.Backend.List(ctx, model.PlanChanges, query.ListInput{Limit: existenceCheckLimit, Filters: [][2]string{
		{model.FieldCustomerTenantID, tenant.String()}, {model.FieldStatus, requestPending}}}, rc0)
	if err != nil {
		return nil, err
	}
	if len(open.Data) > 0 {
		return nil, crud.NewServiceErrorMsg(http.StatusConflict, errcode.ValidationFailed, "There is already a request waiting for an answer.")
	}
	created, err := p.Backend.Create(ctx, model.PlanChanges, crud.JsonObject{
		model.FieldCustomerTenantID: jsonStr(tenant.String()),
		model.FieldCustomerName:     jsonStr(customerName),
		model.FieldRequestedPlanID:  jsonStr(target.ID),
		model.FieldStatus:           jsonStr(requestPending),
		model.FieldNote:             jsonStr(note),
		model.FieldOpenRequestKey:   jsonStr(tenant.String()),
	}, rc0, nil)
	if err != nil {
		// Two requests at once both pass the check above; the unique key lets only one in.
		if se, ok := crud.AsServiceError(err); ok && se.Code == errcode.UniqueViolation {
			return nil, crud.NewServiceErrorMsg(http.StatusConflict, errcode.ValidationFailed, "There is already a request waiting for an answer.")
		}
		return nil, err
	}
	return crud.ToObject(map[string]any{"id": created.ID.String(), "status": requestPending}, nil)
}

// MyPlanRequests lists the caller's own workspace's requests, newest first.
func (p *PlanSource) MyPlanRequests(ctx context.Context, rc *permission.RequestContext) ([]map[string]any, error) {
	out := []map[string]any{}
	if !p.enforced() {
		return out, nil
	}
	sortExpr := "-createdAt"
	res, err := p.Backend.List(ctx, model.PlanChanges, query.ListInput{Limit: 20, Sort: &sortExpr,
		Filters: [][2]string{{model.FieldCustomerTenantID, rc.TenantID}}}, p.operatorRC())
	if err != nil {
		return nil, err
	}
	for _, r := range res.Data {
		status, _ := jsonAsString(r.Data[model.FieldStatus])
		reply, _ := jsonAsString(r.Data[model.FieldDecisionNote])
		planID, _ := jsonAsString(r.Data[model.FieldRequestedPlanID])
		name := ""
		if id, err := uuid.Parse(planID); err == nil {
			if pl, ok := p.planByID(ctx, id); ok {
				name = pl.Name
			}
		}
		out = append(out, map[string]any{"id": r.ID.String(), "status": status, "planId": planID, "planName": name, "reply": reply, "createdAt": r.CreatedAt})
	}
	return out, nil
}

// DecidePlanChange approves or rejects a request. Only the operator's admin may.
func (p *PlanSource) DecidePlanChange(ctx context.Context, rc *permission.RequestContext, requestID uuid.UUID, approve bool, note string, now time.Time) (map[string]any, error) {
	if !p.enforced() || rc.TenantID != p.Operator.String() || !rc.IsAdmin() {
		return nil, forbidden("Only the service provider's admin can decide a plan change.")
	}
	op := p.operatorRC()
	req, err := p.Backend.Get(ctx, model.PlanChanges, requestID, op)
	if err != nil {
		return nil, err
	}
	if status, _ := jsonAsString(req.Record.Data[model.FieldStatus]); status != requestPending {
		return nil, crud.NewServiceErrorMsg(http.StatusConflict, errcode.ValidationFailed, "This request was already decided.")
	}
	status := requestRejected
	if approve {
		status = requestApproved
	}
	// Claim the request first, on the version just read: of two deciders only one wins (the other
	// gets version_conflict), and nothing is applied for a request somebody else already decided.
	patch := crud.JsonObject{model.FieldStatus: jsonStr(status), model.FieldDecisionNote: jsonStr(note), model.FieldOpenRequestKey: json.RawMessage("null")}
	claimed, err := p.Backend.Update(ctx, model.PlanChanges, requestID, req.Record.Version, patch, op, nil)
	if err != nil {
		return nil, err
	}
	result := map[string]any{"status": status, "applied": "none"}
	if approve {
		applied, err := p.applyPlanChange(ctx, req.Record.Data, now)
		if err != nil {
			// Not applied: hand the request back so it can be decided again, instead of leaving an
			// "approved" that never happened.
			reopen := crud.JsonObject{model.FieldStatus: jsonStr(requestPending), model.FieldDecisionNote: json.RawMessage("null"),
				model.FieldOpenRequestKey: req.Record.Data[model.FieldCustomerTenantID]}
			if _, rerr := p.Backend.Update(ctx, model.PlanChanges, requestID, claimed.Version, reopen, op, nil); rerr != nil {
				slog.Error("plan change could not be applied and the request could not be reopened", "request", requestID, "error", err, "reopen_error", rerr)
			}
			return nil, err
		}
		result["applied"] = applied
	}
	return result, nil
}

// applyPlanChange puts an approved request on the customer's subscription and says how it applied:
// "now" for an upgrade, "scheduled" for a downgrade.
func (p *PlanSource) applyPlanChange(ctx context.Context, request crud.JsonObject, now time.Time) (string, error) {
	op := p.operatorRC()
	tenantRaw, _ := jsonAsString(request[model.FieldCustomerTenantID])
	tenant, err := uuid.Parse(tenantRaw)
	if err != nil {
		return "", err
	}
	targetRaw, _ := jsonAsString(request[model.FieldRequestedPlanID])
	targetID, err := uuid.Parse(targetRaw)
	if err != nil {
		return "", err
	}
	target, ok := p.planByID(ctx, targetID)
	if !ok {
		return "", invalid(model.FieldPlanID, "the requested plan no longer exists")
	}
	current, _, err := p.For(ctx, tenant)
	if err != nil {
		return "", err
	}
	subs, err := p.Backend.List(ctx, model.Subscriptions, query.ListInput{Limit: existenceCheckLimit,
		Filters: [][2]string{{model.FieldCustomerTenantID, tenant.String()}}}, op)
	if err != nil {
		return "", err
	}
	immediate := current.ID == "" || isUpgrade(current, target) // the fallback plan has no id to wait on
	if len(subs.Data) == 0 {
		data := crud.JsonObject{model.FieldCustomerTenantID: jsonStr(tenant.String()), model.FieldPlanID: jsonStr(current.ID)}
		if immediate {
			data[model.FieldPlanID] = jsonStr(target.ID)
		} else {
			data[model.FieldPendingPlanID] = jsonStr(target.ID)
			data[model.FieldPendingFrom] = jsonStr(now.Add(billingPeriod).UTC().Format(time.RFC3339))
		}
		_, err := p.Backend.Create(ctx, model.Subscriptions, data, op, nil)
		return appliedWord(immediate), err
	}
	sub := subs.Data[0]
	var patch crud.JsonObject
	if immediate {
		// Moving up replaces any move that was waiting.
		patch = crud.JsonObject{model.FieldPlanID: jsonStr(target.ID), model.FieldPendingPlanID: json.RawMessage("null"), model.FieldPendingFrom: json.RawMessage("null")}
	} else {
		patch = crud.JsonObject{model.FieldPendingPlanID: jsonStr(target.ID), model.FieldPendingFrom: jsonStr(now.Add(billingPeriod).UTC().Format(time.RFC3339))}
	}
	_, err = p.Backend.Update(ctx, model.Subscriptions, sub.ID, sub.Version, patch, op, nil)
	return appliedWord(immediate), err
}

func appliedWord(immediate bool) string {
	if immediate {
		return "now"
	}
	return "scheduled"
}

func jsonStr(s string) json.RawMessage {
	b, _ := json.Marshal(s)
	return b
}

// registerChangeActions wires the three actions of waf.plan_change_requests.
func registerChangeActions(cfg Config, state *httpserver.AppState, reg *crud.ActionRegistry) error {
	for _, a := range []struct {
		name string
		run  crud.ActionHandler
	}{
		{model.ActionRequestPlan, func(ctx context.Context, c crud.ActionCall) (crud.JsonObject, error) {
			planID, _ := jsonAsString(c.Input["planId"])
			note, _ := jsonAsString(c.Input["note"])
			// The name is read from the tenant registry, not taken from the caller.
			var name string
			_ = state.Router.SharedPool().QueryRow(ctx, "SELECT name FROM control.tenants WHERE id::text = $1", c.RC.TenantID).Scan(&name)
			return cfg.PlanSource(state.Crud).RequestPlanChange(ctx, c.RC, planID, note, name)
		}},
		{model.ActionMyPlanRequests, func(ctx context.Context, c crud.ActionCall) (crud.JsonObject, error) {
			list, err := cfg.PlanSource(state.Crud).MyPlanRequests(ctx, c.RC)
			return crud.ToObject(map[string]any{"requests": list}, err)
		}},
		{model.ActionDecidePlan, func(ctx context.Context, c crud.ActionCall) (crud.JsonObject, error) {
			approve, _ := jsonAsBool(c.Input["approve"])
			note, _ := jsonAsString(c.Input["note"])
			res, err := cfg.PlanSource(state.Crud).DecidePlanChange(ctx, c.RC, c.ID, approve, note, time.Now())
			return crud.ToObject(res, err)
		}},
	} {
		if err := reg.Register(model.PlanChanges, a.name, a.run); err != nil {
			return err
		}
	}
	return nil
}
