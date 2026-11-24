package analytics

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/metap-org/metap/pkg/auth/jwks"
	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/kit/errcode"
	"github.com/metap-org/metap/pkg/kit/scope"
	"github.com/metap-org/metap/pkg/permission"
)

// A read token is how the Analytics plane learns what one caller may see (roadmap 121). Management
// signs it for ONE read, for the Analytics API alone (its audience is not the platform's), with a
// lifetime of a minute and the grant worked out from the caller's permissions at that moment. The
// Analytics API then enforces the grant itself, from the signature: it asks management nothing, and
// an ordinary login token reads no events there at all.
//
// The strings below are the other side of a-api/auth (ReadAudience, ZoneAccess*, the "zones" claim);
// the two modules cannot share a package, and a test on each side pins them.
const (
	ReadAudience      = "waap-analytics"
	ZoneAccessAll     = "all"
	ZoneAccessLimited = "limited"
	ClaimZoneAccess   = "zoneAccess"
	ClaimZones        = "zones"

	// ReadTTL is how long a read token lives: long enough for one read, short enough that a role
	// taken away is gone from the next one.
	ReadTTL = time.Minute
)

// EventsResource is the virtual resource a role is granted on one zone to see that zone's events
// (the entity-level policy on SecurityEvents grants every zone). It is declared here so an
// administrator can grant it in the role editor like any other permission.
const EventsResource = "waf.security_events"

func init() {
	permission.DeclareResource(permission.Resource{
		Name:        EventsResource,
		Description: "Security events of a zone",
		Actions:     []permission.ResourceAction{{Name: "read", Description: "See the security events of a zone"}},
	})
}

// Grants answers what a caller may read: the whole entity (every zone) or, failing that, the zones
// they hold a granting role on. metap's PermissionService does.
type Grants interface {
	CanReadEntity(ctx context.Context, rc *permission.RequestContext, entity string) (permission.PermissionDecision, error)
	ScopeFor(ctx context.Context, rc *permission.RequestContext, resource, action string) (scope.Scope, permission.PermissionDecision, error)
}

// ReadTokens decides what a caller may read and signs the token that says so.
type ReadTokens struct {
	// Signer is management's own signing key (the single issuer). Nil mints nothing.
	Signer *jwks.TokenSigner
	// Grants decides who may read which zones. Nil refuses every read.
	Grants Grants
}

func forbidden(msg string) error {
	return crud.NewServiceErrorMsg(http.StatusForbidden, errcode.Forbidden, msg)
}

// ForCaller returns a read token for what rc may see. Every zone when the tenant's policies grant the
// caller the events entity; otherwise only the zones they hold a role on that grants the events
// resource; otherwise a 403, and nothing is signed.
func (t *ReadTokens) ForCaller(ctx context.Context, rc *permission.RequestContext) (string, error) {
	if t == nil || t.Grants == nil {
		return "", forbidden("Security events are not readable here.")
	}
	wide, err := t.Grants.CanReadEntity(ctx, rc, SecurityEvents)
	if err != nil {
		return "", err
	}
	if wide.Allowed {
		return t.mint(rc, ZoneAccessAll, nil)
	}
	sc, d, err := t.Grants.ScopeFor(ctx, rc, EventsResource, "read")
	if err != nil {
		return "", err
	}
	if !d.Allowed {
		reason := wide.Reason
		if reason == "" {
			reason = errcode.Forbidden
		}
		return "", crud.NewServiceError(http.StatusForbidden, reason)
	}
	zones, limited := sc.Limits()["zone"]
	if !limited {
		// Granted on the whole tenant through the resource: the same as the entity grant.
		return t.mint(rc, ZoneAccessAll, nil)
	}
	return t.mint(rc, ZoneAccessLimited, zones)
}

// TenantWide returns a read token for every zone of rc's tenant, with no permission check. It is for
// the system's own sweeps (incident correlation, alert evaluation), whose route has already decided
// who may run them; a person's read goes through ForCaller.
func (t *ReadTokens) TenantWide(rc *permission.RequestContext) (string, error) {
	if t == nil {
		return "", forbidden("Security events are not readable here.")
	}
	return t.mint(rc, ZoneAccessAll, nil)
}

func (t *ReadTokens) mint(rc *permission.RequestContext, access string, zones []string) (string, error) {
	tenant, err := uuid.Parse(rc.TenantID)
	if err != nil {
		return "", errors.New("analytics: the caller has no tenant")
	}
	user, _ := uuid.Parse(rc.UserID) // a service identity may have none
	claims := map[string]any{ClaimZoneAccess: access}
	if access == ZoneAccessLimited {
		if zones == nil {
			zones = []string{}
		}
		claims[ClaimZones] = zones
	}
	return jwks.MintDownstream(t.Signer, jwks.DownstreamToken{
		Audience: ReadAudience, TenantID: tenant, UserID: user, TTL: ReadTTL, Claims: claims,
	})
}
