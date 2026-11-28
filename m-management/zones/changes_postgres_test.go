//go:build integration

package zones_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/permission"
	"github.com/metap-org/metap/pkg/query"
	"github.com/metap-org/metap/pkg/web"
	"github.com/metap-org/waap/m-management/zones"
	"github.com/metap-org/waap/m-management/zones/changes"
	"github.com/metap-org/waap/m-management/zones/guard"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type changeFixture struct {
	t       *testing.T
	ctx     context.Context
	backend *guard.Backend
	actions *crud.ActionRegistry
	state   *web.State
	// admin and second are two tenant admins; viewer holds a role with no rights on the policy.
	admin, second, viewer *permission.RequestContext
	zone                  crud.RecordDto
	account               uuid.UUID
}

func newChangeFixture(t *testing.T) *changeFixture {
	t.Helper()
	pool := testPool(t)
	registry := setupRegistry(t, pool)
	tenantID, userID, secondID := uuid.New(), uuid.New(), uuid.New()
	seedAdmin(t, pool, tenantID, userID)
	cleanupEntityRows(t, pool, tenantID)
	state, _ := newTestState(t, pool, registry)

	f := &changeFixture{
		t: t, state: state, ctx: context.Background(), backend: zones.Guarded(state.Crud, nil, state.Router), actions: crud.NewActionRegistry(),
		admin: adminContext(tenantID, userID), second: adminContext(tenantID, secondID),
		viewer: &permission.RequestContext{TenantID: tenantID.String(), UserID: uuid.NewString(), Roles: []string{"viewer"}, HasRoles: true},
	}
	require.NoError(t, zones.RegisterActions(zones.Config{}, nil)(state, f.actions))
	apex := freshApex(t)
	t.Cleanup(func() {
		pool.Exec(context.Background(), "DELETE FROM waf.waf_domains WHERE apex_domain = $1", apex)
	})
	var err error
	f.zone, err = f.backend.Create(f.ctx, "waf.zones", crud.JsonObject{
		"hostname": jsonMarshal(t, "shop."+apex), "originAddress": jsonMarshal(t, "1.2.3.4"), "protectionMode": jsonMarshal(t, "monitor"),
	}, f.admin, nil)
	require.NoError(t, err)
	f.account = uuid.MustParse(jsonString0(f.zone.Data["accountId"]))
	return f
}

func (f *changeFixture) control(mode string) {
	f.t.Helper()
	got, err := f.backend.Get(f.ctx, "waf.accounts", f.account, f.admin)
	require.NoError(f.t, err)
	_, err = f.backend.Update(f.ctx, "waf.accounts", f.account, got.Record.Version, crud.JsonObject{"changeControl": jsonMarshal(f.t, mode)}, f.admin, nil)
	require.NoError(f.t, err)
}

func (f *changeFixture) invoke(entity, name string, id uuid.UUID, rc *permission.RequestContext, input any) (changes.Outcome, error) {
	f.t.Helper()
	in := crud.JsonObject{}
	if input != nil {
		require.NoError(f.t, json.Unmarshal(jsonMarshal(f.t, input), &in))
	}
	var out changes.Outcome
	res, err := f.actions.Invoke(f.ctx, crud.ActionCall{Entity: entity, Name: name, ID: id, RC: rc, Input: in})
	if err == nil {
		require.NoError(f.t, json.Unmarshal(jsonMarshal(f.t, res), &out))
	}
	return out, err
}

func (f *changeFixture) propose(rc *permission.RequestContext, ops ...map[string]any) (changes.Outcome, error) {
	return f.invoke("waf.zones", "createChangeRequest", f.zone.ID, rc, map[string]any{"title": "Block a source", "operations": ops})
}

func (f *changeFixture) act(name, id string, rc *permission.RequestContext, input any) (changes.Outcome, error) {
	return f.invoke("waf.change_requests", name, uuid.MustParse(id), rc, input)
}

func (f *changeFixture) records(entity string) []crud.RecordDto {
	f.t.Helper()
	res, err := f.backend.List(f.ctx, entity, query.ListInput{Limit: 50, Filters: [][2]string{{"zoneId", f.zone.ID.String()}}}, f.admin)
	require.NoError(f.t, err)
	return res.Data
}

func (f *changeFixture) changeRequest(id string) crud.RecordDto {
	f.t.Helper()
	got, err := f.backend.Get(f.ctx, "waf.change_requests", uuid.MustParse(id), f.admin)
	require.NoError(f.t, err)
	return got.Record
}

func blockOp(ip string) map[string]any {
	return map[string]any{"op": "create", "entity": "waf.ip_access_lists", "data": map[string]any{"type": "blacklist", "value": ip, "enabled": true}}
}

func code(t *testing.T, err error) string {
	t.Helper()
	se, ok := crud.AsServiceError(err)
	require.True(t, ok, "expected a ServiceError, got %v", err)
	return se.Code
}

func TestChangeRequestChangesNothingUntilItIsApproved(t *testing.T) {
	f := newChangeFixture(t)

	draft, err := f.propose(f.admin, blockOp("203.0.113.5"))
	require.NoError(t, err)
	assert.Equal(t, "draft", draft.Status)
	assert.True(t, draft.Check.OK, "%+v", draft.Check)
	require.Len(t, draft.Diff, 1)
	assert.Equal(t, "create", draft.Diff[0].Kind)
	assert.Empty(t, f.records("waf.ip_access_lists"), "the check rolls back")
	assert.Empty(t, f.records("waf.policy_versions"), "the check records no version")

	_, err = f.act("approve", draft.ID, f.admin, nil)
	assert.Equal(t, "change_request_state", code(t, err), "a draft is not approvable")

	submitted, err := f.act("submit", draft.ID, f.admin, nil)
	require.NoError(t, err)
	assert.Equal(t, "pendingReview", submitted.Status)
	assert.Empty(t, f.records("waf.ip_access_lists"))

	published, err := f.act("approve", draft.ID, f.second, map[string]any{"comment": "ok"})
	require.NoError(t, err)
	assert.Equal(t, "published", published.Status)
	require.Len(t, f.records("waf.ip_access_lists"), 1)

	versions := f.records("waf.policy_versions")
	require.Len(t, versions, 2, "a baseline and one version for the whole change")
	var named bool
	for _, v := range versions {
		if jsonString0(v.Data["changeRequestId"]) == draft.ID {
			named = true
			assert.Equal(t, f.second.UserID, jsonString0(v.Data["approvedBy"]))
		}
	}
	assert.True(t, named, "the version names its change request")
	cr := f.changeRequest(draft.ID)
	assert.Equal(t, "approved", jsonString0(cr.Data["decision"]))
	zone, err := f.backend.Get(f.ctx, "waf.zones", f.zone.ID, f.admin)
	require.NoError(t, err)
	assert.JSONEq(t, "false", string(orFalse(zone.Record.Data["hasConfig"])), "an IP entry alone is not configuration")

	_, err = f.act("approve", draft.ID, f.second, nil)
	assert.Equal(t, "change_request_state", code(t, err), "published once")
}

func orFalse(raw json.RawMessage) json.RawMessage {
	if raw == nil {
		return json.RawMessage("false")
	}
	return raw
}

func TestFourEyesNeedsSomeoneElse(t *testing.T) {
	f := newChangeFixture(t)
	f.control("fourEyes")
	draft, err := f.propose(f.admin, blockOp("203.0.113.5"))
	require.NoError(t, err)
	_, err = f.act("submit", draft.ID, f.admin, nil)
	require.NoError(t, err)

	_, err = f.act("approve", draft.ID, f.admin, nil)
	assert.Equal(t, "self_approval_refused", code(t, err))
	// Nor by rewriting someone else's change and approving it: only the author may.
	_, err = f.act("revise", draft.ID, f.second, map[string]any{"operations": []any{blockOp("203.0.113.9")}})
	assert.Equal(t, "forbidden", code(t, err))
	assert.Empty(t, f.records("waf.ip_access_lists"))

	_, err = f.act("approve", draft.ID, f.second, nil)
	require.NoError(t, err)
	assert.Len(t, f.records("waf.ip_access_lists"), 1)
}

func TestWithoutFourEyesTheAuthorConfirms(t *testing.T) {
	f := newChangeFixture(t)
	f.control("review")
	draft, err := f.propose(f.admin, blockOp("203.0.113.5"))
	require.NoError(t, err)
	_, err = f.act("submit", draft.ID, f.admin, nil)
	require.NoError(t, err)
	_, err = f.act("approve", draft.ID, f.admin, nil)
	require.NoError(t, err)
	assert.Equal(t, "selfConfirmed", jsonString0(f.changeRequest(draft.ID).Data["decision"]))
}

func TestAChangeApprovedAgainstAnOldPolicyIsRefused(t *testing.T) {
	f := newChangeFixture(t)
	first, err := f.propose(f.admin, blockOp("203.0.113.5"))
	require.NoError(t, err)
	second, err := f.propose(f.admin, blockOp("203.0.113.6"))
	require.NoError(t, err)
	for _, id := range []string{first.ID, second.ID} {
		_, err = f.act("submit", id, f.admin, nil)
		require.NoError(t, err)
	}
	_, err = f.act("approve", first.ID, f.second, nil)
	require.NoError(t, err)

	_, err = f.act("approve", second.ID, f.second, nil)
	assert.Equal(t, "change_request_stale", code(t, err))
	assert.Len(t, f.records("waf.ip_access_lists"), 1, "a refused publish leaves nothing behind")
	assert.Equal(t, "pendingReview", jsonString0(f.changeRequest(second.ID).Data["status"]))

	_, err = f.act("submit", second.ID, f.admin, nil)
	require.NoError(t, err)
	_, err = f.act("approve", second.ID, f.second, nil)
	require.NoError(t, err)
	assert.Len(t, f.records("waf.ip_access_lists"), 2)
}

func TestABadChangeStaysADraftWithTheReason(t *testing.T) {
	f := newChangeFixture(t)
	draft, err := f.propose(f.admin, blockOp("203.0.113.5"), blockOp("not-an-ip"))
	require.NoError(t, err)
	assert.False(t, draft.Check.OK)
	require.NotNil(t, draft.Check.Error)
	assert.Equal(t, 1, draft.Check.Error.Operation)
	assert.Empty(t, f.records("waf.ip_access_lists"), "the operation before the bad one is rolled back too")

	_, err = f.act("submit", draft.ID, f.admin, nil)
	assert.Equal(t, "change_request_not_ready", code(t, err))

	_, err = f.act("revise", draft.ID, f.admin, map[string]any{"operations": []any{blockOp("203.0.113.5")}})
	require.NoError(t, err)
	_, err = f.act("submit", draft.ID, f.admin, nil)
	require.NoError(t, err)

	_, err = f.act("reject", draft.ID, f.second, map[string]any{"comment": "wrong source"})
	require.NoError(t, err)
	assert.Equal(t, "rejected", jsonString0(f.changeRequest(draft.ID).Data["status"]))
}

func TestWhoMayNotChangeThePolicyCannotProposeOrDecide(t *testing.T) {
	f := newChangeFixture(t)
	_, err := f.propose(f.viewer, blockOp("203.0.113.5"))
	require.Error(t, err)
	_, err = f.propose(f.viewer, blockOp("not-an-ip"))
	require.Error(t, err, "an invalid operation is no way around the permission check")

	draft, err := f.propose(f.admin, blockOp("203.0.113.5"))
	require.NoError(t, err)
	_, err = f.act("submit", draft.ID, f.admin, nil)
	require.NoError(t, err)
	_, err = f.act("approve", draft.ID, f.viewer, nil)
	require.Error(t, err)
	assert.Empty(t, f.records("waf.ip_access_lists"))
}

// A client, the admin role included, cannot make a change request say what the actions did not.
func TestChangeRequestsAreNotWritableDirectly(t *testing.T) {
	f := newChangeFixture(t)
	draft, err := f.propose(f.admin, blockOp("203.0.113.5"))
	require.NoError(t, err)
	cr := f.changeRequest(draft.ID)
	_, err = f.backend.Update(f.ctx, "waf.change_requests", cr.ID, cr.Version, crud.JsonObject{"status": jsonMarshal(t, "pendingReview")}, f.admin, nil)
	assert.Equal(t, "forbidden", code(t, err))
	_, err = f.backend.Create(f.ctx, "waf.change_requests", cr.Data, f.admin, nil)
	assert.Equal(t, "forbidden", code(t, err))
	_, err = f.backend.Delete(f.ctx, "waf.change_requests", cr.ID, cr.Version, f.admin, nil)
	assert.Equal(t, "forbidden", code(t, err))
}

func TestUnderChangeControlOnlyEmergencyRulesAreWrittenDirectly(t *testing.T) {
	f := newChangeFixture(t)
	zoneID := jsonMarshal(t, f.zone.ID.String())
	entry := func(typ, ip string, life time.Duration) crud.JsonObject {
		data := crud.JsonObject{"zoneId": zoneID, "type": jsonMarshal(t, typ), "value": jsonMarshal(t, ip), "enabled": jsonMarshal(t, true)}
		if life != 0 {
			data["expiresAt"] = jsonMarshal(t, time.Now().Add(life).UTC().Format(time.RFC3339))
		}
		return data
	}
	create := func(data crud.JsonObject) (crud.RecordDto, error) {
		return f.backend.Create(f.ctx, "waf.ip_access_lists", data, f.admin, nil)
	}
	permanent, err := create(entry("blacklist", "203.0.113.1", 0))
	require.NoError(t, err, "off: written directly")

	f.control("review")
	for name, data := range map[string]crud.JsonObject{
		"permanent":       entry("blacklist", "203.0.113.2", 0),
		"too long":        entry("blacklist", "203.0.113.3", 96*time.Hour),
		"lets more in":    entry("whitelist", "203.0.113.4", time.Hour),
		"already expired": entry("blacklist", "203.0.113.7", -time.Hour),
	} {
		_, err = create(data)
		assert.Equal(t, "change_request_required", code(t, err), name)
	}
	temporary, err := create(entry("blacklist", "203.0.113.8", 24*time.Hour))
	require.NoError(t, err)

	_, err = f.backend.Update(f.ctx, "waf.ip_access_lists", temporary.ID, temporary.Version,
		crud.JsonObject{"expiresAt": jsonMarshal(t, time.Now().Add(48*time.Hour).UTC().Format(time.RFC3339))}, f.admin, nil)
	assert.Equal(t, "change_request_required", code(t, err), "extending is not an emergency")
	_, err = f.backend.Delete(f.ctx, "waf.ip_access_lists", permanent.ID, permanent.Version, f.admin, nil)
	assert.Equal(t, "change_request_required", code(t, err))
	_, err = f.backend.Delete(f.ctx, "waf.ip_access_lists", temporary.ID, temporary.Version, f.admin, nil)
	require.NoError(t, err, "a temporary rule may be lifted")

	// Through a change request, an update and a delete of a permanent entry go in together.
	draft, err := f.propose(f.admin,
		map[string]any{"op": "delete", "entity": "waf.ip_access_lists", "id": permanent.ID.String()},
		blockOp("203.0.113.9"))
	require.NoError(t, err)
	require.True(t, draft.Check.OK, "%+v", draft.Check)
	_, err = f.act("submit", draft.ID, f.admin, nil)
	require.NoError(t, err)
	_, err = f.act("approve", draft.ID, f.admin, nil)
	require.NoError(t, err)
	left := f.records("waf.ip_access_lists")
	require.Len(t, left, 1)
	assert.Equal(t, "203.0.113.9", jsonString0(left[0].Data["value"]))
}

// Two approvals at once of two changes made against the same policy: one is published, the other is
// stale, whichever comes first.
func TestConcurrentApprovalsPublishOne(t *testing.T) {
	f := newChangeFixture(t)
	var ids []uuid.UUID
	for _, ip := range []string{"203.0.113.5", "203.0.113.6"} {
		draft, err := f.propose(f.admin, blockOp(ip))
		require.NoError(t, err)
		_, err = f.act("submit", draft.ID, f.admin, nil)
		require.NoError(t, err)
		ids = append(ids, uuid.MustParse(draft.ID))
	}
	errs := make(chan error, len(ids))
	for _, id := range ids {
		go func() {
			_, err := f.actions.Invoke(f.ctx, crud.ActionCall{Entity: "waf.change_requests", Name: "approve", ID: id, RC: f.second, Input: crud.JsonObject{}})
			errs <- err
		}()
	}
	var failed []error
	for range ids {
		if err := <-errs; err != nil {
			failed = append(failed, err)
		}
	}
	require.Len(t, failed, 1)
	assert.Equal(t, "change_request_stale", code(t, failed[0]))
	assert.Len(t, f.records("waf.ip_access_lists"), 1)
}

// With a role's own grants (as m-bootstrap gives security_analyst: read every waf entity, write the
// policy entities, no write on zones, versions or change requests), two analysts run a four-eyes
// change that also flips the zone's hasConfig.
func TestSecurityAnalystsRunAFourEyesChange(t *testing.T) {
	f := newChangeFixture(t)
	f.control("fourEyes")
	read := []permission.RoleActionGrant{{Role: "security_analyst", HasRole: true, Action: "read"}}
	write := append([]permission.RoleActionGrant{}, read...)
	for _, a := range []string{"create", "update", "delete"} {
		write = append(write, permission.RoleActionGrant{Role: "security_analyst", HasRole: true, Action: a})
	}
	for entity, grants := range map[string][]permission.RoleActionGrant{
		"waf.zones": read, "waf.accounts": read, "waf.policy_versions": read, "waf.change_requests": read,
		"waf.ddos_policies": write, "waf.firewall_rules": write, "waf.ip_access_lists": write,
	} {
		_, err := f.state.Permissions.SyncBasicPolicies(f.ctx, uuid.MustParse(f.admin.TenantID), entity, grants, nil)
		require.NoError(t, err, entity)
	}
	analyst := func() *permission.RequestContext {
		return &permission.RequestContext{TenantID: f.admin.TenantID, UserID: uuid.NewString(), Roles: []string{"security_analyst"}, HasRoles: true}
	}
	author, approver := analyst(), analyst()

	rule := map[string]any{"op": "create", "entity": "waf.firewall_rules", "data": map[string]any{
		"name": "Block admin", "priority": 10, "ruleType": "waf", "action": "block", "enabled": true,
		"matchCondition": map[string]any{"field": "uri.path", "op": "contains", "value": "/admin"},
	}}
	draft, err := f.propose(author, rule)
	require.NoError(t, err)
	require.True(t, draft.Check.OK, "%+v %+v", draft.Check, draft.Check.Error)
	_, err = f.act("submit", draft.ID, author, nil)
	require.NoError(t, err)
	_, err = f.act("approve", draft.ID, approver, nil)
	require.NoError(t, err)

	assert.Len(t, f.records("waf.firewall_rules"), 1)
	zone, err := f.backend.Get(f.ctx, "waf.zones", f.zone.ID, f.admin)
	require.NoError(t, err)
	assert.JSONEq(t, "true", string(zone.Record.Data["hasConfig"]))
}

// A tenant-wide rule reaches every zone, so one account with change control puts it under control,
// and only an admin decides its change: someone else under four-eyes.
func TestTenantWideRulesAreUnderTheStrictestAccount(t *testing.T) {
	f := newChangeFixture(t)
	entry := func(ip string) crud.JsonObject {
		return crud.JsonObject{"type": jsonMarshal(t, "blacklist"), "value": jsonMarshal(t, ip), "enabled": jsonMarshal(t, true)}
	}
	tenantWide := func() []crud.RecordDto {
		res, err := f.backend.List(f.ctx, "waf.ip_access_lists", query.ListInput{Limit: 50, Filters: [][2]string{{"zoneId", ""}}}, f.admin)
		require.NoError(t, err)
		return res.Data
	}
	permanent, err := f.backend.Create(f.ctx, "waf.ip_access_lists", entry("198.51.100.1"), f.admin, nil)
	require.NoError(t, err, "off: written directly")
	f.control("fourEyes")

	_, err = f.backend.Create(f.ctx, "waf.ip_access_lists", entry("198.51.100.2"), f.admin, nil)
	assert.Equal(t, "change_request_required", code(t, err))
	_, err = f.backend.Delete(f.ctx, "waf.ip_access_lists", permanent.ID, permanent.Version, f.admin, nil)
	assert.Equal(t, "change_request_required", code(t, err))
	// Nor by taking a zone's rule out of its zone.
	zoned := entry("198.51.100.3")
	zoned["zoneId"] = jsonMarshal(t, f.zone.ID.String())
	zoned["expiresAt"] = jsonMarshal(t, time.Now().Add(time.Hour).UTC().Format(time.RFC3339))
	temporary, err := f.backend.Create(f.ctx, "waf.ip_access_lists", zoned, f.admin, nil)
	require.NoError(t, err)
	_, err = f.backend.Update(f.ctx, "waf.ip_access_lists", temporary.ID, temporary.Version, crud.JsonObject{"zoneId": json.RawMessage("null")}, f.admin, nil)
	assert.Equal(t, "change_request_required", code(t, err))

	propose := func(ops ...map[string]any) (changes.Outcome, error) {
		return f.invoke("waf.change_requests", "createTenantChangeRequest", uuid.Nil, f.admin, map[string]any{"title": "Block everywhere", "operations": ops})
	}
	// A tenant-wide change cannot reach into a zone.
	into := blockOp("198.51.100.4")
	into["data"].(map[string]any)["zoneId"] = f.zone.ID.String()
	bad, err := propose(into)
	require.NoError(t, err)
	assert.False(t, bad.Check.OK)

	draft, err := propose(blockOp("198.51.100.5"), map[string]any{"op": "delete", "entity": "waf.ip_access_lists", "id": permanent.ID.String()})
	require.NoError(t, err)
	require.True(t, draft.Check.OK, "%+v %+v", draft.Check, draft.Check.Error)
	assert.Empty(t, draft.Check.Findings)
	require.Len(t, tenantWide(), 1, "the check rolls back")
	_, err = f.act("submit", draft.ID, f.admin, nil)
	require.NoError(t, err)

	_, err = f.act("approve", draft.ID, f.admin, nil)
	assert.Equal(t, "self_approval_refused", code(t, err))
	analyst := &permission.RequestContext{TenantID: f.admin.TenantID, UserID: uuid.NewString(), Roles: []string{"security_analyst"}, HasRoles: true}
	grants := []permission.RoleActionGrant{}
	for _, a := range []string{"read", "create", "update", "delete"} {
		grants = append(grants, permission.RoleActionGrant{Role: "security_analyst", HasRole: true, Action: a})
	}
	for _, entity := range []string{"waf.change_requests", "waf.ip_access_lists"} {
		_, err = f.state.Permissions.SyncBasicPolicies(f.ctx, uuid.MustParse(f.admin.TenantID), entity, grants, nil)
		require.NoError(t, err)
	}
	_, err = f.act("approve", draft.ID, analyst, nil)
	assert.Equal(t, "forbidden", code(t, err), "write rights are not enough for a tenant-wide change")

	before := len(f.records("waf.policy_versions"))
	published, err := f.act("approve", draft.ID, f.second, nil)
	require.NoError(t, err)
	assert.Zero(t, published.PublishedVersion)
	left := tenantWide()
	require.Len(t, left, 1)
	assert.Equal(t, "198.51.100.5", jsonString0(left[0].Data["value"]))
	assert.Len(t, f.records("waf.policy_versions"), before, "versions belong to a zone")
}
