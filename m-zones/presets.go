// Presets (UC-26): a starting set of protections a zone can be given in one step instead of
// building rules by hand. Each is only data: a DDoS policy and some firewall rules, created through
// the same guarded path as anything a person adds (validation, plan limits, account, versions), and
// recorded as one policy version rather than one per rule. Applying a preset twice adds nothing
// the second time: a rule whose name the zone already has, and a DDoS policy when it has one, are
// left alone.
package zonesservice

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/kit/errcode"
	"github.com/metap-org/metap/pkg/permission"
	"github.com/metap-org/metap/pkg/query"
	"github.com/metap-org/waap/model"
)

type presetRule struct {
	name      string
	priority  int
	condition string // JSON, the match grammar
	action    string
	// rate limit rules only
	rateThreshold, rateWindow int
}

type preset struct {
	key                    string
	ddosSensitivity        string
	ddosAction             string
	ddosThreshold, ddosWin int
	rules                  []presetRule
}

// Preset keys.
const (
	PresetEssential = "essential"
	PresetAPI       = "api"
	PresetStrict    = "strict"
)

var sensitiveFiles = presetRule{name: "Block access to sensitive files", priority: 100, action: "block",
	condition: `{"any":[{"field":"uri.path","op":"contains","value":"/.env"},{"field":"uri.path","op":"contains","value":"/.git/"},{"field":"uri.path","op":"endsWith","value":".sql"},{"field":"uri.path","op":"endsWith","value":".bak"}]}`}

var presets = map[string]preset{
	// Reasonable for most sites: challenge floods, shut the doors that should never be open.
	PresetEssential: {key: PresetEssential, ddosSensitivity: "medium", ddosAction: "challenge", ddosThreshold: 500, ddosWin: 60,
		rules: []presetRule{sensitiveFiles}},
	// For a site that is mostly an API: tighter limits, and a brake on credential guessing.
	PresetAPI: {key: PresetAPI, ddosSensitivity: "high", ddosAction: "block", ddosThreshold: 300, ddosWin: 60,
		rules: []presetRule{sensitiveFiles, {name: "Rate limit sign-in", priority: 110, action: "block", rateThreshold: 20, rateWindow: 60,
			condition: `{"field":"uri.path","op":"contains","value":"/login"}`}}},
	// For a site under pressure or with little tolerance: blocks rather than challenges, and also
	// refuses methods a normal browser session does not use.
	PresetStrict: {key: PresetStrict, ddosSensitivity: "aggressive", ddosAction: "block", ddosThreshold: 200, ddosWin: 60,
		rules: []presetRule{sensitiveFiles, {name: "Refuse uncommon methods", priority: 120, action: "block",
			condition: `{"field":"method","op":"in","values":["TRACE","CONNECT","TRACK","DEBUG"]}`}}},
}

// PresetKeys lists the presets that exist.
func PresetKeys() []string { return []string{PresetEssential, PresetAPI, PresetStrict} }

// applyPreset gives the zone the preset's protections through backend (the guarded one), as rc, and
// returns what was added.
func applyPreset(ctx context.Context, backend *GuardedZonesBackend, zoneID uuid.UUID, key string, rc *permission.RequestContext) (map[string]any, error) {
	p, ok := presets[key]
	if !ok {
		return nil, crud.NewServiceErrorFields(422, errcode.ValidationFailed, map[string][]string{"preset": {"unknown preset"}})
	}
	if _, err := backend.Get(ctx, model.Zones, zoneID, rc); err != nil {
		return nil, err
	}
	ensureBaseline(ctx, backend.inner, zoneID, rc)
	quiet := withoutVersions(ctx)
	zone := jsonString(zoneID.String())
	added := []string{}

	policies, err := backend.List(ctx, model.DdosPolicies, query.ListInput{Limit: 1, Filters: [][2]string{{model.FieldZoneID, zoneID.String()}}}, rc)
	if err != nil {
		return nil, err
	}
	if len(policies.Data) == 0 {
		_, err := backend.Create(quiet, model.DdosPolicies, crud.JsonObject{
			model.FieldZoneID: zone, model.FieldSensitivity: jsonString(p.ddosSensitivity), model.FieldAction: jsonString(p.ddosAction),
			model.FieldRequestRateThreshold: json.RawMessage(fmt.Sprint(p.ddosThreshold)), model.FieldBurstWindow: json.RawMessage(fmt.Sprint(p.ddosWin)),
			model.FieldPriority: json.RawMessage("100"), model.FieldEnabled: jsonBool(true),
		}, rc, nil)
		if err != nil {
			return nil, err
		}
		added = append(added, "DDoS policy")
	}

	existing, _, err := readZoneState(ctx, backend.inner, zoneID, rc)
	if err != nil {
		return nil, err
	}
	have := map[string]bool{}
	for _, r := range existing[model.FirewallRules] {
		n, _ := jsonAsString(r.Data[model.FieldName])
		have[n] = true
	}
	for _, r := range p.rules {
		if have[r.name] {
			continue
		}
		data := crud.JsonObject{
			model.FieldZoneID: zone, model.FieldName: jsonString(r.name), model.FieldPriority: json.RawMessage(fmt.Sprint(r.priority)),
			model.FieldMatchCondition: json.RawMessage(r.condition), model.FieldRuleType: jsonString("waf"),
			model.FieldAction: jsonString(r.action), model.FieldEnabled: jsonBool(true),
		}
		if r.rateThreshold > 0 {
			data[model.FieldRuleType] = jsonString("rateLimit")
			data[model.FieldRateLimitThreshold] = json.RawMessage(fmt.Sprint(r.rateThreshold))
			data[model.FieldRateLimitWindow] = json.RawMessage(fmt.Sprint(r.rateWindow))
		}
		if _, err := backend.Create(quiet, model.FirewallRules, data, rc, nil); err != nil {
			return nil, err
		}
		added = append(added, r.name)
	}

	if len(added) > 0 {
		if err := syncHasConfig(ctx, backend.inner, zoneID, rc); err != nil {
			return nil, err
		}
		recordChange(ctx, backend.inner, zoneID, rc, "Applied the "+key+" preset:", fmt.Sprint(len(added), " items"))
	}
	return map[string]any{"preset": key, "added": added}, nil
}

type noVersionsKey struct{}

// withoutVersions marks a context so rule changes made under it do not each record a version; the
// caller records one for the whole step.
func withoutVersions(ctx context.Context) context.Context {
	return context.WithValue(ctx, noVersionsKey{}, true)
}

func versionsOff(ctx context.Context) bool { v, _ := ctx.Value(noVersionsKey{}).(bool); return v }
