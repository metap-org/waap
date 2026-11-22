// Layer: APP · guard. Field checks on what a person typed.

package guard

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/kit/errcode"
	"github.com/metap-org/metap/pkg/permission"
	"github.com/metap-org/waap/m-management/zones/internal/jx"
	"github.com/metap-org/waap/m-management/zones/rules"
	"github.com/metap-org/waap/model"
)

// ---- field checks ------------------------------------------------------------------------------

// validationGuard cleans and checks what a person typed: a zone's own fields, a rule's match
// condition, an IP entry's value, an expiry.
type validationGuard struct{ passthrough }

func (g *validationGuard) Create(ctx context.Context, entity string, data crud.JsonObject, rc *permission.RequestContext, reason *string) (crud.RecordDto, error) {
	switch entity {
	case model.Zones:
		if hasHostname(data) {
			normalized, err := normalizeZoneFields(data)
			if err != nil {
				return crud.RecordDto{}, err
			}
			data = normalized
		}
	case model.FirewallRules:
		if err := validateFirewallRule(data); err != nil {
			return crud.RecordDto{}, err
		}
	case model.IPAccessLists:
		if err := validateIPAccessList(data); err != nil {
			return crud.RecordDto{}, err
		}
	}
	return g.inner.Create(ctx, entity, data, rc, reason)
}

func (g *validationGuard) Update(ctx context.Context, entity string, id uuid.UUID, expectedVersion int32, data crud.JsonObject, rc *permission.RequestContext, reason *string) (crud.RecordDto, error) {
	switch entity {
	case model.Zones:
		normalized, err := normalizeZoneFields(data)
		if err != nil {
			return crud.RecordDto{}, err
		}
		data = normalized
	case model.FirewallRules:
		if err := validateFirewallRule(data); err != nil {
			return crud.RecordDto{}, err
		}
	case model.IPAccessLists:
		if err := validateIPAccessList(data); err != nil {
			return crud.RecordDto{}, err
		}
	}
	return g.inner.Update(ctx, entity, id, expectedVersion, data, rc, reason)
}

func validateFirewallRule(data crud.JsonObject) error {
	condition, ok := data[model.FieldMatchCondition]
	if ok && !rules.IsValidMatchCondition(condition) {
		return crud.NewServiceErrorFields(422, errcode.ValidationFailed,
			map[string][]string{model.FieldMatchCondition: {"not a recognized rule expression"}})
	}
	return validateExpiry(data)
}

// validateIPAccessList checks an IP entry's value and expiry.
func validateIPAccessList(data crud.JsonObject) error {
	value, ok := jx.AsString(data[model.FieldValue])
	if ok && !rules.IsValidIPOrCIDR(value) {
		return crud.NewServiceErrorFields(422, errcode.ValidationFailed,
			map[string][]string{model.FieldValue: {"not a valid IP address or CIDR"}})
	}
	return validateExpiry(data)
}

// validateExpiry accepts an absent or empty expiresAt, or an RFC 3339 timestamp (a temporary rule or
// IP entry, UC-51); anything else is refused rather than stored as a protection that never lapses.
func validateExpiry(data crud.JsonObject) error {
	if raw, ok := jx.AsString(data[model.FieldExpiresAt]); ok && raw != "" {
		if _, err := time.Parse(time.RFC3339, raw); err != nil {
			return crud.NewServiceErrorFields(422, errcode.ValidationFailed,
				map[string][]string{model.FieldExpiresAt: {"not a valid timestamp"}})
		}
	}
	return nil
}

// normalizeZoneFields cleans what a person typed into a zone's own fields before they are stored:
// the hostname to its canonical form, and tags to a short list of distinct labels. The origin port
// must be a real port. Fields absent from data stay absent (an update touches only what it names).
func normalizeZoneFields(data crud.JsonObject) (crud.JsonObject, error) {
	out := jx.Clone(data)
	fieldErrors := map[string][]string{}
	if raw, ok := jx.AsString(data[model.FieldHostname]); ok {
		h, err := rules.NormalizeHostname(raw)
		if err != nil {
			fieldErrors[model.FieldHostname] = []string{err.Error()}
		} else {
			out[model.FieldHostname] = jx.String(h)
		}
	}
	if port, ok := jx.AsFloat(data[model.FieldOriginPort]); ok && (port < 1 || port > 65535 || port != float64(int(port))) {
		fieldErrors[model.FieldOriginPort] = []string{"must be a port between 1 and 65535"}
	}
	if raw, ok := data[model.FieldTags]; ok && string(raw) != "null" {
		tags, err := normalizeTags(raw)
		if err != nil {
			fieldErrors[model.FieldTags] = []string{err.Error()}
		} else {
			out[model.FieldTags] = tags
		}
	}
	if len(fieldErrors) > 0 {
		return nil, crud.NewServiceErrorFields(422, errcode.ValidationFailed, fieldErrors)
	}
	return out, nil
}

const (
	maxTags      = 10
	maxTagLength = 32
)

// normalizeTags accepts a JSON array of strings and returns it trimmed, de-duplicated (ignoring
// case) and bounded: a tag is a short label, not free text.
func normalizeTags(raw json.RawMessage) (json.RawMessage, error) {
	var in []string
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, fmt.Errorf("must be a list of text labels")
	}
	seen := map[string]bool{}
	out := []string{}
	for _, t := range in {
		t = strings.TrimSpace(t)
		if t == "" || seen[strings.ToLower(t)] {
			continue
		}
		if len([]rune(t)) > maxTagLength {
			return nil, fmt.Errorf("a tag is at most %d characters", maxTagLength)
		}
		seen[strings.ToLower(t)] = true
		out = append(out, t)
	}
	if len(out) > maxTags {
		return nil, fmt.Errorf("at most %d tags", maxTags)
	}
	b, _ := json.Marshal(out)
	return b, nil
}
