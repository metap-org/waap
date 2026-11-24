// Layer: APP · declared-action logic. A business function behind a declared action; the zones package wires it (actions.go).

// Presets (UC-26): a starting set of protections a zone can be given in one step instead of
// building rules by hand. Each is only data: a DDoS policy and some firewall rules, created through
// the same guarded path as anything a person adds (validation, plan limits, account, versions), and
// recorded as one policy version rather than one per rule. Applying a preset twice adds nothing
// the second time: a rule whose name the zone already has, and a DDoS policy when it has one, are
// left alone.
package presets

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/kit/errcode"
	"github.com/metap-org/metap/pkg/permission"
	"github.com/metap-org/metap/pkg/query"
	"github.com/metap-org/waap/m-management/zones/guard"
	"github.com/metap-org/waap/m-management/zones/internal/jx"
	"github.com/metap-org/waap/m-management/zones/versions"
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

// Result is what applying a preset did: which one, and the names of what it added (nothing when the
// zone already had it all).
type Result struct {
	Preset string   `json:"preset"`
	Added  []string `json:"added"`
}

// Apply gives the zone the preset's protections through backend (the guarded one), as rc, and
// returns what was added.
func Apply(ctx context.Context, backend *guard.Backend, zoneID uuid.UUID, key string, rc *permission.RequestContext) (Result, error) {
	p, ok := presets[key]
	if !ok {
		return Result{}, crud.NewServiceErrorFields(422, errcode.ValidationFailed, map[string][]string{"preset": {"unknown preset"}})
	}
	if _, err := backend.Get(ctx, model.Zones, zoneID, rc); err != nil {
		return Result{}, err
	}
	versions.EnsureBaseline(ctx, backend.Base(), zoneID, rc)
	quiet := versions.WithoutVersions(ctx)
	zone := jx.String(zoneID.String())
	added := []string{}

	policies, err := backend.List(ctx, model.DdosPolicies, query.ListInput{Limit: 1, Filters: [][2]string{{model.FieldZoneID, zoneID.String()}}}, rc)
	if err != nil {
		return Result{}, err
	}
	if len(policies.Data) == 0 {
		_, err := backend.Create(quiet, model.DdosPolicies, crud.JsonObject{
			model.FieldZoneID: zone, model.FieldSensitivity: jx.String(p.ddosSensitivity), model.FieldAction: jx.String(p.ddosAction),
			model.FieldRequestRateThreshold: json.RawMessage(fmt.Sprint(p.ddosThreshold)), model.FieldBurstWindow: json.RawMessage(fmt.Sprint(p.ddosWin)),
			model.FieldPriority: json.RawMessage("100"), model.FieldEnabled: jx.Bool(true),
		}, rc, nil)
		if err != nil {
			return Result{}, err
		}
		added = append(added, "DDoS policy")
	}

	existing, _, err := versions.ReadZoneState(ctx, backend.Base(), zoneID, rc)
	if err != nil {
		return Result{}, err
	}
	have := map[string]bool{}
	for _, r := range existing[model.FirewallRules] {
		n, _ := jx.AsString(r.Data[model.FieldName])
		have[n] = true
	}
	for _, r := range p.rules {
		if have[r.name] {
			continue
		}
		data := crud.JsonObject{
			model.FieldZoneID: zone, model.FieldName: jx.String(r.name), model.FieldPriority: json.RawMessage(fmt.Sprint(r.priority)),
			model.FieldMatchCondition: json.RawMessage(r.condition), model.FieldRuleType: jx.String("waf"),
			model.FieldAction: jx.String(r.action), model.FieldEnabled: jx.Bool(true),
		}
		if r.rateThreshold > 0 {
			data[model.FieldRuleType] = jx.String("rateLimit")
			data[model.FieldRateLimitThreshold] = json.RawMessage(fmt.Sprint(r.rateThreshold))
			data[model.FieldRateLimitWindow] = json.RawMessage(fmt.Sprint(r.rateWindow))
		}
		if _, err := backend.Create(quiet, model.FirewallRules, data, rc, nil); err != nil {
			return Result{}, err
		}
		added = append(added, r.name)
	}

	if len(added) > 0 {
		if err := versions.SyncHasConfig(ctx, backend.Base(), zoneID, rc); err != nil {
			return Result{}, err
		}
		versions.RecordChange(ctx, backend.Base(), zoneID, rc, "Applied the "+key+" preset:", fmt.Sprint(len(added), " items"))
	}
	return Result{Preset: key, Added: added}, nil
}
