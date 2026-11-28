// Layer: APP · guard. Keeps the proof of domain ownership out of a client's hands.

package guard

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/kit/errcode"
	"github.com/metap-org/metap/pkg/permission"
	"github.com/metap-org/waap/m-management/zones/internal/jx"
	"github.com/metap-org/waap/m-management/zones/rules"
	"github.com/metap-org/waap/model"
)

// ---- ownership ---------------------------------------------------------------------------------

// ownershipGuard keeps what activating a zone rests on out of a client's hands: hasConfig (computed
// from the zone's rules) and the proof of ownership. It refuses a write that would make a domain or a
// zone count as verified, or carry a proof over to a name nobody proved. The only way to "verified" is the verifyDns action, which
// writes through the unguarded service; domainGuard, below this one, copies a proved domain's status
// onto a new zone.
//
// What a client may send is the value already stored (a form saves the record back as it read it).
type ownershipGuard struct{ passthrough }

const (
	msgOnlyVerification = "is set by domain verification only"
	msgCannotChange     = "cannot be changed"
	msgComputed         = "is worked out from the zone's rules (syncConfigState)"
	msgOtherDomain      = "must stay under the zone's domain"
)

func (g *ownershipGuard) Create(ctx context.Context, entity string, data crud.JsonObject, rc *permission.RequestContext, reason *string) (crud.RecordDto, error) {
	if entity != model.Domains && entity != model.Zones {
		return g.inner.Create(ctx, entity, data, rc, reason)
	}
	if raw, ok := data[model.FieldVerificationStatus]; ok && !unverified(raw) {
		return crud.RecordDto{}, refuse(map[string][]string{model.FieldVerificationStatus: {msgOnlyVerification}})
	}
	if raw, ok := data[model.FieldHasConfig]; ok && entity == model.Zones && !falsy(raw) {
		return crud.RecordDto{}, refuse(map[string][]string{model.FieldHasConfig: {msgComputed}})
	}
	data = jx.Clone(data)
	if entity == model.Domains {
		// The server picks the token: one chosen by the client could name a TXT value that is
		// already published under _waf-verify.
		data[model.FieldVerificationToken] = jx.String(newVerificationToken())
		data[model.FieldVerificationStatus] = jx.String(model.VerificationUnverified)
	} else {
		delete(data, model.FieldVerificationStatus)
	}
	return g.inner.Create(ctx, entity, data, rc, reason)
}

func (g *ownershipGuard) Update(ctx context.Context, entity string, id uuid.UUID, expectedVersion int32, data crud.JsonObject, rc *permission.RequestContext, reason *string) (crud.RecordDto, error) {
	if err := g.check(ctx, entity, id, data, rc); err != nil {
		return crud.RecordDto{}, err
	}
	return g.inner.Update(ctx, entity, id, expectedVersion, data, rc, reason)
}

func (g *ownershipGuard) Transition(ctx context.Context, entity string, id uuid.UUID, action string, expectedVersion int32, data crud.JsonObject, rc *permission.RequestContext, reason *string) (crud.RecordDto, error) {
	if err := g.check(ctx, entity, id, data, rc); err != nil {
		return crud.RecordDto{}, err
	}
	return g.inner.Transition(ctx, entity, id, action, expectedVersion, data, rc, reason)
}

// protectedFields are the fields of each entity a change to which this guard judges.
var protectedFields = map[string][]string{
	model.Domains: {model.FieldVerificationStatus, model.FieldVerificationToken, model.FieldApexDomain},
	model.Zones:   {model.FieldVerificationStatus, model.FieldDomainID, model.FieldHostname, model.FieldHasConfig},
}

// check judges a change to an existing record. The record is read only when data names a protected
// field, as the caller, so a record the caller may not read cannot be changed here either.
func (g *ownershipGuard) check(ctx context.Context, entity string, id uuid.UUID, data crud.JsonObject, rc *permission.RequestContext) error {
	var named []string
	for _, field := range protectedFields[entity] {
		if _, ok := data[field]; ok {
			named = append(named, field)
		}
	}
	if len(named) == 0 {
		return nil
	}
	current, err := g.inner.Get(ctx, entity, id, rc)
	if err != nil {
		return err
	}
	stored := current.Record.Data
	fieldErrors := map[string][]string{}
	for _, field := range named {
		next := data[field]
		if sameValue(stored[field], next) {
			continue
		}
		switch field {
		case model.FieldVerificationStatus:
			// A zone that was never verified may have no status stored at all.
			if stored[field] == nil && isString(next, model.VerificationUnverified) {
				continue
			}
			fieldErrors[field] = []string{msgOnlyVerification}
		case model.FieldHasConfig:
			// The other half of what activate checks. Never computed reads as false.
			if falsy(stored[field]) && falsy(next) {
				continue
			}
			fieldErrors[field] = []string{msgComputed}
		case model.FieldHostname:
			// A zone may be renamed, but only to a name its own domain's proof covers.
			before, _ := jx.AsString(stored[field])
			after, ok := jx.AsString(next)
			if !ok || !strings.EqualFold(rules.ApexDomain(before), rules.ApexDomain(after)) {
				fieldErrors[field] = []string{msgOtherDomain}
			}
		default:
			fieldErrors[field] = []string{msgCannotChange}
		}
	}
	if len(fieldErrors) > 0 {
		return refuse(fieldErrors)
	}
	return nil
}

func refuse(fieldErrors map[string][]string) error {
	return crud.NewServiceErrorFields(422, errcode.ValidationFailed, fieldErrors)
}

func newVerificationToken() string { return fmt.Sprintf("waf-verify-%s", uuid.New()) }

// unverified reports whether a status a client sent on create claims nothing.
func unverified(raw json.RawMessage) bool {
	return isString(raw, model.VerificationUnverified) || string(bytes.TrimSpace(raw)) == "null"
}

// falsy reports whether a boolean field's value claims nothing: absent, null or false.
func falsy(raw json.RawMessage) bool {
	v := string(bytes.TrimSpace(raw))
	return v == "" || v == "null" || v == "false"
}

func isString(raw json.RawMessage, want string) bool {
	got, ok := jx.AsString(raw)
	return ok && got == want
}

// sameValue compares two JSON scalars; an absent value equals null.
func sameValue(a, b json.RawMessage) bool {
	var x, y any
	if a != nil && json.Unmarshal(a, &x) != nil {
		return false
	}
	if b != nil && json.Unmarshal(b, &y) != nil {
		return false
	}
	switch x.(type) {
	case nil, string, bool, float64:
		return x == y
	}
	return false
}
