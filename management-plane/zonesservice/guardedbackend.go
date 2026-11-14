// GuardedZonesBackend wraps this service's own CrudService so the create/update/delete-time
// guards that used to be axum/chi middleware (zoneDomainGuard/firewallRuleMatchConditionGuard/
// ipAccessListValueGuard/zoneDeleteGuard) still run for the mutation path that reaches this
// service: gRPC, called by waf-graphql-gateway's CompositeBackend on behalf of the real Customer
// Portal.
//
// This wraps crud.RecordBackend (the same seam metap-grpc's GrpcRecordService accepts instead of
// a hardcoded *crud.CrudService) rather than reaching back into chi, so the fix lives at the one
// layer every real mutation path actually goes through.
//
// zoneDeleteGuard's cross-service reference check dials each sibling's own gRPC port directly
// (SCANNING_GRPC_ADDR/ALERTING_GRPC_ADDR), with no service-account credential of its own —
// instead of logging in as a fixed user (which would only ever see one tenant's rows in this
// Schema-strategy shared table, wrong for every other tenant), it forwards the caller's own token
// — metap's HTTP/gRPC auth puts it on RequestContext.ForwardedBearerToken, which GrpcBackend
// prefers over any stored credential. The sibling's GrpcRecordService resolves that token's real
// roles independently, so the check runs with the caller's own permissions, not a blanket service
// identity. (This used to mint a fresh 30s token with the shared RSA key; with a single JWKS
// issuer no service but the issuer holds a signing key, so forwarding is the only option — and
// also keeps the caller's functionId/scope, which the minted token dropped.)
//
// Lazy, cached connection (crossServiceLink): grpc.NewClient never actually dials eagerly in Go
// (unlike Rust's tonic Endpoint::connect), so unlike the Rust original there is no real
// connect-time failure to leave uncached — the constructed *grpcapi.GrpcBackend is always
// cached, and an unreachable sibling surfaces as an error from the real List call instead, which
// zoneReferencedBy already maps to the same 503 reference_check_unavailable response.
package zonesservice

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/google/uuid"

	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/grpcapi"
	"github.com/metap-org/metap/pkg/permission"
	"github.com/metap-org/metap/pkg/query"
	"github.com/metap-org/metap/pkg/runtime"
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

// crossServiceLink is one sibling service's gRPC connection, built lazily and cached — see this
// file's own doc comment for why Go's cache-on-success semantics differ from the Rust original's.
type crossServiceLink struct {
	addr string

	mu     sync.Mutex
	cached crud.RecordBackend
}

func newCrossServiceLink(addr string) *crossServiceLink {
	return &crossServiceLink{addr: addr}
}

func (l *crossServiceLink) get() crud.RecordBackend {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.cached != nil {
		return l.cached
	}
	// The token this placeholder carries is never actually sent — every real call below always
	// sets RequestContext.ForwardedBearerToken, which GrpcBackend prefers unconditionally over
	// this source. It exists only because ConnectGrpcBackend requires a *ServiceTokenSource to
	// construct one at all.
	placeholder := runtime.NewStaticServiceTokenSource("unused-forwarded-token-always-wins")
	backend, err := grpcapi.ConnectGrpcBackend(l.addr, placeholder)
	if err != nil {
		return nil
	}
	l.cached = backend
	return l.cached
}

type GuardedZonesBackend struct {
	inner    crud.RecordBackend
	scanning *crossServiceLink
	alerting *crossServiceLink
}

func NewGuardedZonesBackend(inner crud.RecordBackend, scanningGRPCAddr, alertingGRPCAddr string) *GuardedZonesBackend {
	return &GuardedZonesBackend{
		inner:    inner,
		scanning: newCrossServiceLink(scanningGRPCAddr), alerting: newCrossServiceLink(alertingGRPCAddr),
	}
}

var _ crud.RecordBackend = (*GuardedZonesBackend)(nil)

// resolveDomainID ports zoneDomainGuard's logic verbatim — looks up (or lazily creates) the
// Domain row for hostname's apex and returns its id to inject into the create payload.
func (b *GuardedZonesBackend) resolveDomainID(ctx context.Context, hostname string, rc *permission.RequestContext) (uuid.UUID, error) {
	apex := ApexDomain(hostname)
	existing, err := b.inner.List(ctx, "waf.domains", query.ListInput{Limit: 1, Filters: [][2]string{{"apexDomain", apex}}}, rc)
	if err != nil {
		return uuid.Nil, err
	}
	if len(existing.Data) > 0 {
		return existing.Data[0].ID, nil
	}

	newDomain := crud.JsonObject{
		"apexDomain":         jsonString(apex),
		"verificationToken":  jsonString(fmt.Sprintf("waf-verify-%s", uuid.New())),
		"verificationMethod": jsonString("dnsTxt"),
		"verificationStatus": jsonString("unverified"),
	}
	created, err := b.inner.Create(ctx, "waf.domains", newDomain, rc, nil)
	if err != nil {
		return uuid.Nil, err
	}
	return created.ID, nil
}

// validateFirewallRule ports firewallRuleMatchConditionGuard's validation verbatim.
func validateFirewallRule(data crud.JsonObject) error {
	condition, ok := data["matchCondition"]
	if ok && !IsValidMatchCondition(condition) {
		return crud.NewServiceErrorFields(422, "validation_failed",
			map[string][]string{"matchCondition": {"not a recognized rule expression"}})
	}
	return nil
}

// validateIPAccessList ports ipAccessListValueGuard's validation verbatim.
func validateIPAccessList(data crud.JsonObject) error {
	value, ok := jsonAsString(data["value"])
	if ok && !IsValidIPOrCIDR(value) {
		return crud.NewServiceErrorFields(422, "validation_failed",
			map[string][]string{"value": {"not a valid IP address or CIDR"}})
	}
	return nil
}

// zoneReferencedBy ports zoneDeleteGuard's cross-service check — see this file's own doc
// comment for the identity-forwarding mechanism. Returns the blocking entity name, or "" when
// clear to delete; a non-nil error is one the caller should return as-is.
func (b *GuardedZonesBackend) zoneReferencedBy(ctx context.Context, zoneID uuid.UUID, rc *permission.RequestContext) (string, error) {
	// The sibling services run the check as the caller, with the caller's own token (set on
	// rc by metap's HTTP/gRPC auth). No token means we cannot ask on the caller's behalf, and an
	// unchecked delete could orphan scan jobs/incidents — refuse rather than guess.
	if rc.ForwardedBearerToken == "" {
		return "", crud.NewServiceErrorMsg(503, "reference_check_unavailable",
			"Could not verify cross-service references (no caller token to forward); refusing to delete.")
	}
	forwardedRC := *rc

	checks := []struct {
		link   *crossServiceLink
		entity string
	}{
		{b.scanning, "waf.scan_jobs"},
		{b.alerting, "waf.incidents"},
		// waf.security_events is deliberately not here: it is an append-only log in the
		// Analytics plane, and a deleted zone's history ageing out by TTL is correct, not an
		// orphan to block on.
	}
	for _, check := range checks {
		backend := check.link.get()
		if backend == nil {
			return "", crud.NewServiceErrorMsg(503, "reference_check_unavailable",
				fmt.Sprintf("Could not verify cross-service references (%s unreachable); refusing to delete.", check.entity))
		}
		result, err := backend.List(ctx, check.entity, query.ListInput{Limit: 1, Filters: [][2]string{{"zoneId", zoneID.String()}}}, &forwardedRC)
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
	switch entity {
	case "waf.zones":
		hostname, ok := jsonAsString(data["hostname"])
		if !ok {
			// No hostname at all — let the real create reject it with its own "required field"
			// validation error rather than this wrapper inventing a different one.
			return b.inner.Create(ctx, entity, data, rc, reason)
		}
		domainID, err := b.resolveDomainID(ctx, hostname, rc)
		if err != nil {
			return crud.RecordDto{}, err
		}
		withDomain := cloneJsonObject(data)
		withDomain["domainId"] = jsonString(domainID.String())
		return b.inner.Create(ctx, entity, withDomain, rc, reason)
	case "waf.firewall_rules":
		if err := validateFirewallRule(data); err != nil {
			return crud.RecordDto{}, err
		}
		return b.inner.Create(ctx, entity, data, rc, reason)
	case "waf.ip_access_lists":
		if err := validateIPAccessList(data); err != nil {
			return crud.RecordDto{}, err
		}
		return b.inner.Create(ctx, entity, data, rc, reason)
	default:
		return b.inner.Create(ctx, entity, data, rc, reason)
	}
}

func (b *GuardedZonesBackend) Update(ctx context.Context, entity string, id uuid.UUID, expectedVersion int32, data crud.JsonObject, rc *permission.RequestContext, reason *string) (crud.RecordDto, error) {
	// domainId is create-only (matching the old zoneDomainGuard, which only ever intercepted
	// create) — a Zone's domain never changes once it's assigned.
	switch entity {
	case "waf.firewall_rules":
		if err := validateFirewallRule(data); err != nil {
			return crud.RecordDto{}, err
		}
	case "waf.ip_access_lists":
		if err := validateIPAccessList(data); err != nil {
			return crud.RecordDto{}, err
		}
	}
	return b.inner.Update(ctx, entity, id, expectedVersion, data, rc, reason)
}

func (b *GuardedZonesBackend) Transition(ctx context.Context, entity string, id uuid.UUID, action string, expectedVersion int32, data crud.JsonObject, rc *permission.RequestContext, reason *string) (crud.RecordDto, error) {
	return b.inner.Transition(ctx, entity, id, action, expectedVersion, data, rc, reason)
}

func (b *GuardedZonesBackend) Delete(ctx context.Context, entity string, id uuid.UUID, expectedVersion int32, rc *permission.RequestContext, reason *string) (crud.RecordDto, error) {
	if entity == "waf.zones" {
		blockingEntity, err := b.zoneReferencedBy(ctx, id, rc)
		if err != nil {
			return crud.RecordDto{}, err
		}
		if blockingEntity != "" {
			return crud.RecordDto{}, crud.NewServiceErrorMsg(409, "record_referenced",
				fmt.Sprintf("This zone is still referenced by %q and cannot be deleted.", blockingEntity))
		}
	}
	return b.inner.Delete(ctx, entity, id, expectedVersion, rc, reason)
}

func (b *GuardedZonesBackend) Aggregate(ctx context.Context, entity string, spec query.AggregateSpec, rc *permission.RequestContext) ([]json.RawMessage, error) {
	return b.inner.Aggregate(ctx, entity, spec, rc)
}
