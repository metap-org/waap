// GuardedZonesBackend wraps this service's own CrudService so the create/update/delete-time
// guards that used to be axum/chi middleware (zoneDomainGuard/firewallRuleMatchConditionGuard/
// ipAccessListValueGuard/zoneDeleteGuard) still run for the mutation path that reaches this
// service: metap's REST record routes (/api/{entity}, roadmap 116), which the Customer Portal
// reaches through the API gateway.
//
// This wraps crud.RecordBackend (app.Options.WrapBackend: what the REST routes serve) rather than
// reaching back into chi, so the fix lives at the one layer every real mutation path goes through.
//
// zoneDeleteGuard's cross-service reference check calls each sibling's own REST API directly
// (SCANNING_URL/ALERTING_URL, metap's recordclient), with no service-account credential of its own —
// instead of logging in as a fixed user (which would only ever see one tenant's rows in this
// Schema-strategy shared table, wrong for every other tenant), it forwards the caller's own token
// — metap's HTTP auth puts it on RequestContext.ForwardedBearerToken, which recordclient
// prefers over any stored credential. The sibling resolves that token's real
// roles independently, so the check runs with the caller's own permissions, not a blanket service
// identity. (This used to mint a fresh 30s token with the shared RSA key; with a single JWKS
// issuer no service but the issuer holds a signing key, so forwarding is the only option — and
// also keeps the caller's functionId/scope, which the minted token dropped.)

// An unreachable sibling answers 502 upstream_unavailable from recordclient, which
// zoneReferencedBy turns into 503 reference_check_unavailable.
package zonesservice

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/metap-org/metap/pkg/kit/errcode"

	"github.com/google/uuid"

	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/permission"
	"github.com/metap-org/metap/pkg/query"
	"github.com/metap-org/metap/pkg/recordclient"
	"github.com/metap-org/waap/model"
)

func jsonString(s string) json.RawMessage {
	b, _ := json.Marshal(s)
	return b
}

func jsonAsString(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 {
		return "", false
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", false
	}
	return s, true
}

func cloneJsonObject(data crud.JsonObject) crud.JsonObject {
	out := make(crud.JsonObject, len(data))
	for k, v := range data {
		out[k] = v
	}
	return out
}

type GuardedZonesBackend struct {
	inner    crud.RecordBackend
	scanning crud.RecordBackend
	alerting crud.RecordBackend
	// plans resolves what a tenant may use; nil means no quotas (see WithPlanLimits).
	plans *PlanClient
}

// NewGuardedZonesBackend guards inner; scanningURL/alertingURL are the siblings' REST base URLs
// ("http://scanning-service:3010").
func NewGuardedZonesBackend(inner crud.RecordBackend, scanningURL, alertingURL string) *GuardedZonesBackend {
	return &GuardedZonesBackend{
		inner:    inner,
		scanning: recordclient.New(scanningURL, nil), alerting: recordclient.New(alertingURL, nil),
	}
}

var _ crud.RecordBackend = (*GuardedZonesBackend)(nil)

// resolveDomainID ports zoneDomainGuard's logic verbatim — looks up (or lazily creates) the
// Domain row for hostname's apex and returns its id to inject into the create payload, plus
// whether that domain is already verified (a new zone under it must start verified too: the
// verify action only cascades onto zones that exist at the time).
func (b *GuardedZonesBackend) resolveDomainID(ctx context.Context, hostname string, rc *permission.RequestContext) (uuid.UUID, bool, error) {
	apex := ApexDomain(hostname)
	existing, err := b.inner.List(ctx, model.Domains, query.ListInput{Limit: existenceCheckLimit, Filters: [][2]string{{model.FieldApexDomain, apex}}}, rc)
	if err != nil {
		return uuid.Nil, false, err
	}
	if len(existing.Data) > 0 {
		status, _ := jsonAsString(existing.Data[0].Data[model.FieldVerificationStatus])
		return existing.Data[0].ID, status == model.VerificationVerified, nil
	}

	newDomain := crud.JsonObject{
		model.FieldApexDomain:         jsonString(apex),
		model.FieldVerificationToken:  jsonString(fmt.Sprintf("waf-verify-%s", uuid.New())),
		model.FieldVerificationMethod: jsonString(model.VerificationMethodDNSTxt),
		model.FieldVerificationStatus: jsonString(model.VerificationUnverified),
	}
	created, err := b.inner.Create(ctx, model.Domains, newDomain, rc, nil)
	if err != nil {
		return uuid.Nil, false, err
	}
	return created.ID, false, nil
}

// validateFirewallRule ports firewallRuleMatchConditionGuard's validation verbatim.
func validateFirewallRule(data crud.JsonObject) error {
	condition, ok := data[model.FieldMatchCondition]
	if ok && !IsValidMatchCondition(condition) {
		return crud.NewServiceErrorFields(422, errcode.ValidationFailed,
			map[string][]string{model.FieldMatchCondition: {"not a recognized rule expression"}})
	}
	return validateExpiry(data)
}

// validateIPAccessList ports ipAccessListValueGuard's validation verbatim.
func validateIPAccessList(data crud.JsonObject) error {
	value, ok := jsonAsString(data[model.FieldValue])
	if ok && !IsValidIPOrCIDR(value) {
		return crud.NewServiceErrorFields(422, errcode.ValidationFailed,
			map[string][]string{model.FieldValue: {"not a valid IP address or CIDR"}})
	}
	return validateExpiry(data)
}

// zoneReferencedBy ports zoneDeleteGuard's cross-service check — see this file's own doc
// comment for the identity-forwarding mechanism. Returns the blocking entity name, or "" when
// clear to delete; a non-nil error is one the caller should return as-is.
func (b *GuardedZonesBackend) zoneReferencedBy(ctx context.Context, zoneID uuid.UUID, rc *permission.RequestContext) (string, error) {
	// The sibling services run the check as the caller, with the caller's own token (set on
	// rc by metap's HTTP auth). No token means we cannot ask on the caller's behalf, and an
	// unchecked delete could orphan scan jobs/incidents — refuse rather than guess.
	if rc.ForwardedBearerToken == "" {
		return "", crud.NewServiceErrorMsg(503, model.ErrReferenceCheckUnavailable,
			"Could not verify cross-service references (no caller token to forward); refusing to delete.")
	}
	forwardedRC := *rc

	checks := []struct {
		backend crud.RecordBackend
		entity  string
	}{
		{b.scanning, model.ScanJobs},
		{b.alerting, model.Incidents},
		// waf.security_events is deliberately not here: it is an append-only log in the
		// Analytics plane, and a deleted zone's history ageing out by TTL is correct, not an
		// orphan to block on.
	}
	for _, check := range checks {
		result, err := check.backend.List(ctx, check.entity, query.ListInput{Limit: existenceCheckLimit, Filters: [][2]string{{model.FieldZoneID, zoneID.String()}}}, &forwardedRC)
		if se, ok := crud.AsServiceError(err); ok && se.Status == http.StatusBadGateway {
			return "", crud.NewServiceErrorMsg(503, model.ErrReferenceCheckUnavailable,
				fmt.Sprintf("Could not verify cross-service references (%s unreachable); refusing to delete.", check.entity))
		}
		if err != nil {
			return "", err
		}
		if len(result.Data) > 0 {
			return check.entity, nil
		}
	}
	return "", nil
}

func (b *GuardedZonesBackend) List(ctx context.Context, entity string, input query.ListInput, rc *permission.RequestContext) (crud.ListResult, error) {
	return b.inner.List(ctx, entity, input, rc)
}

func (b *GuardedZonesBackend) Get(ctx context.Context, entity string, id uuid.UUID, rc *permission.RequestContext) (crud.GetResult, error) {
	return b.inner.Get(ctx, entity, id, rc)
}

func (b *GuardedZonesBackend) GetMany(ctx context.Context, entity string, ids []uuid.UUID, rc *permission.RequestContext) ([]crud.GetManyResult, error) {
	return b.inner.GetMany(ctx, entity, ids, rc)
}

func (b *GuardedZonesBackend) Create(ctx context.Context, entity string, data crud.JsonObject, rc *permission.RequestContext, reason *string) (crud.RecordDto, error) {
	if err := b.checkQuota(ctx, entity, data, rc); err != nil {
		return crud.RecordDto{}, err
	}
	switch entity {
	case model.Zones:
		hostname, ok := jsonAsString(data[model.FieldHostname])
		if !ok {
			// No hostname at all — let the real create reject it with its own "required field"
			// validation error rather than this wrapper inventing a different one.
			return b.inner.Create(ctx, entity, data, rc, reason)
		}
		domainID, verified, err := b.resolveDomainID(ctx, hostname, rc)
		if err != nil {
			return crud.RecordDto{}, err
		}
		withDomain := cloneJsonObject(data)
		accountID, err := b.resolveAccount(ctx, data, rc)
		if err != nil {
			return crud.RecordDto{}, err
		}
		withDomain[model.FieldAccountID] = jsonString(accountID.String())
		withDomain[model.FieldDomainID] = jsonString(domainID.String())
		if verified {
			withDomain[model.FieldVerificationStatus] = jsonString(model.VerificationVerified)
		}
		return b.inner.Create(ctx, entity, withDomain, rc, reason)
	case model.FirewallRules:
		if err := validateFirewallRule(data); err != nil {
			return crud.RecordDto{}, err
		}
	case model.IPAccessLists:
		if err := validateIPAccessList(data); err != nil {
			return crud.RecordDto{}, err
		}
	}
	if !isVersioned(entity) {
		return b.inner.Create(ctx, entity, data, rc, reason)
	}
	zoneID := zoneOfRule(data)
	ensureBaseline(ctx, b.inner, zoneID, rc)
	data = b.withZoneAccount(ctx, zoneID, data, rc)
	created, err := b.inner.Create(ctx, entity, data, rc, reason)
	if err == nil {
		recordChange(ctx, b.inner, zoneID, rc, "Created", entity)
	}
	return created, err
}

func (b *GuardedZonesBackend) Update(ctx context.Context, entity string, id uuid.UUID, expectedVersion int32, data crud.JsonObject, rc *permission.RequestContext, reason *string) (crud.RecordDto, error) {
	// domainId is create-only (matching the old zoneDomainGuard, which only ever intercepted
	// create) — a Zone's domain never changes once it's assigned.
	switch entity {
	case model.FirewallRules:
		if err := validateFirewallRule(data); err != nil {
			return crud.RecordDto{}, err
		}
	case model.IPAccessLists:
		if err := validateIPAccessList(data); err != nil {
			return crud.RecordDto{}, err
		}
	}
	if !isVersioned(entity) {
		return b.inner.Update(ctx, entity, id, expectedVersion, data, rc, reason)
	}
	zoneID := b.zoneOf(ctx, entity, id, rc)
	ensureBaseline(ctx, b.inner, zoneID, rc)
	updated, err := b.inner.Update(ctx, entity, id, expectedVersion, data, rc, reason)
	if err == nil {
		recordChange(ctx, b.inner, zoneID, rc, "Updated", entity)
	}
	return updated, err
}

// zoneOf is the zone a rule record belongs to (uuid.Nil when tenant-wide or unreadable).
func (b *GuardedZonesBackend) zoneOf(ctx context.Context, entity string, id uuid.UUID, rc *permission.RequestContext) uuid.UUID {
	got, err := b.inner.Get(ctx, entity, id, rc)
	if err != nil {
		return uuid.Nil
	}
	return zoneOfRule(got.Record.Data)
}

func (b *GuardedZonesBackend) Transition(ctx context.Context, entity string, id uuid.UUID, action string, expectedVersion int32, data crud.JsonObject, rc *permission.RequestContext, reason *string) (crud.RecordDto, error) {
	if entity == model.Accounts && action == model.AccountClose {
		if err := b.refuseCloseWhileRunning(ctx, id, rc); err != nil {
			return crud.RecordDto{}, err
		}
	}
	return b.inner.Transition(ctx, entity, id, action, expectedVersion, data, rc, reason)
}

func (b *GuardedZonesBackend) Delete(ctx context.Context, entity string, id uuid.UUID, expectedVersion int32, rc *permission.RequestContext, reason *string) (crud.RecordDto, error) {
	if entity == model.Zones {
		blockingEntity, err := b.zoneReferencedBy(ctx, id, rc)
		if err != nil {
			return crud.RecordDto{}, err
		}
		if blockingEntity != "" {
			return crud.RecordDto{}, crud.NewServiceErrorMsg(409, errcode.RecordReferenced,
				fmt.Sprintf("This zone is still referenced by %q and cannot be deleted.", blockingEntity))
		}
	}
	if !isVersioned(entity) {
		return b.inner.Delete(ctx, entity, id, expectedVersion, rc, reason)
	}
	zoneID := b.zoneOf(ctx, entity, id, rc)
	ensureBaseline(ctx, b.inner, zoneID, rc)
	deleted, err := b.inner.Delete(ctx, entity, id, expectedVersion, rc, reason)
	if err == nil {
		recordChange(ctx, b.inner, zoneID, rc, "Deleted", entity)
	}
	return deleted, err
}

func (b *GuardedZonesBackend) Aggregate(ctx context.Context, entity string, spec query.AggregateSpec, rc *permission.RequestContext) ([]json.RawMessage, error) {
	return b.inner.Aggregate(ctx, entity, spec, rc)
}

// validateExpiry accepts an absent or empty expiresAt, or an RFC 3339 timestamp (a temporary rule or
// IP entry, UC-51); anything else is refused rather than stored as a protection that never lapses.
func validateExpiry(data crud.JsonObject) error {
	if raw, ok := jsonAsString(data[model.FieldExpiresAt]); ok && raw != "" {
		if _, err := time.Parse(time.RFC3339, raw); err != nil {
			return crud.NewServiceErrorFields(422, errcode.ValidationFailed,
				map[string][]string{model.FieldExpiresAt: {"not a valid timestamp"}})
		}
	}
	return nil
}
