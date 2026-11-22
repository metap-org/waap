// Layer: APP · guard. A decorator on metap's crud.RecordBackend, plugged in through app.Options.WrapBackend (see cmd/).

// Package guard is the backend every write to this service goes through: the chain of guards
// in this package around the service's own CrudService. It is what app.Options.WrapBackend serves (the
// REST record routes, /api/{entity}, which the portal reaches through the API gateway) and what
// presets write through, so a person's edit and a preset get the same checks.
//
// The zone-delete check reads scan jobs and incidents (entities of this same service) as the caller.
package guard

import (
	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/waap/m-management/billing"
	"github.com/metap-org/waap/m-management/zones/internal/jx"
	"github.com/metap-org/waap/model"
)

// Backend is the guarded backend: a crud.RecordBackend whose every call goes through the chain.
type Backend struct {
	// RecordBackend is the assembled chain; every call goes through it.
	crud.RecordBackend

	base crud.RecordBackend
	// plans resolves what a tenant may use; nil means no quotas (see WithPlanLimits).
	plans billing.PlanLookup
}

// New guards inner.
func New(inner crud.RecordBackend) *Backend {
	b := &Backend{base: inner}
	b.RecordBackend = b.chain()
	return b
}

// WithPlanLimits turns quota enforcement on. Without it (nil) nothing is limited.
func (b *Backend) WithPlanLimits(p billing.PlanLookup) *Backend {
	b.plans = p
	b.RecordBackend = b.chain()
	return b
}

// chain is the one list of guards and the order they run in, outermost first.
func (b *Backend) chain() crud.RecordBackend {
	var c crud.RecordBackend = b.base
	c = &versionGuard{passthrough{c}}
	c = &accountGuard{passthrough{c}}
	c = &domainGuard{passthrough{c}}
	c = &validationGuard{passthrough{c}}
	c = &referenceGuard{passthrough{c}}
	c = &quotaGuard{passthrough: passthrough{c}, plans: b.plans}
	return c
}

var _ crud.RecordBackend = (*Backend)(nil)

// hasHostname reports whether a zone payload names a hostname. Without one, the guards below step
// aside and let the real create reject it with its own "required field" error.
func hasHostname(data crud.JsonObject) bool {
	_, ok := jx.AsString(data[model.FieldHostname])
	return ok
}

// Base is the unguarded backend underneath the chain, for the caller that does its own bookkeeping
// (a preset writes through the chain, then records its own single version on Base).
func (b *Backend) Base() crud.RecordBackend { return b.base }
