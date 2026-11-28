// Layer: APP · declared-action logic. A business function behind a declared action; the zones package wires it (actions.go).

// Checking a zone's policy before it matters (UC-42): the same data the control plane will compile,
// read from management and judged for mistakes a person can fix. It writes nothing. Errors are
// things that make a rule useless or wrong; warnings are things that probably are not what was
// meant; infos are things that are true but not a problem.
package policy

import (
	"context"
	"encoding/json"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/permission"
	"github.com/metap-org/waap/m-management/zones/internal/jx"
	"github.com/metap-org/waap/m-management/zones/rules"
	"github.com/metap-org/waap/m-management/zones/versions"
	"github.com/metap-org/waap/model"
)

// Finding severities.
const (
	SeverityError   = "error"
	SeverityWarning = "warning"
	SeverityInfo    = "info"
)

// Finding is one thing the check noticed. Code names it for the portal to translate; Subject is
// what it is about (a rule name, an address).
type Finding struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Entity   string `json:"entity,omitempty"`
	ID       string `json:"id,omitempty"`
	Subject  string `json:"subject,omitempty"`
}

// ValidationReport is the result of checking one zone.
type ValidationReport struct {
	Findings []Finding `json:"findings"`
	// Errors is the count of error findings: the zone's policy is fit to publish when it is zero.
	Errors  int `json:"errors"`
	Rules   int `json:"rules"`
	Entries int `json:"entries"`
}

func isOn(raw json.RawMessage) bool {
	on, ok := jx.AsBool(raw)
	return !ok || on // an absent flag means on, as the compiler reads it
}

func expiredAt(raw json.RawMessage, now time.Time) bool {
	s, ok := jx.AsString(raw)
	if !ok || s == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339, s)
	return err == nil && !t.After(now)
}

// checkSnapshot judges a zone's records. Pure, so it is tested without a database.
func checkSnapshot(snap versions.Snapshot, now time.Time) ValidationReport {
	var out []Finding
	add := func(sev, code, entity, id, subject string) {
		out = append(out, Finding{Severity: sev, Code: code, Entity: entity, ID: id, Subject: subject})
	}
	name := func(d crud.JsonObject) string { s, _ := jx.AsString(d[model.FieldName]); return s }

	live := 0
	priorities := map[float64][]versions.SnapshotItem{}
	for _, r := range snap[model.FirewallRules] {
		subject := name(r.Data)
		if cond, ok := r.Data[model.FieldMatchCondition]; !ok || !rules.IsValidMatchCondition(cond) {
			add(SeverityError, "ruleConditionInvalid", model.FirewallRules, r.ID, subject)
			continue
		}
		if !isOn(r.Data[model.FieldEnabled]) {
			continue
		}
		if expiredAt(r.Data[model.FieldExpiresAt], now) {
			add(SeverityInfo, "ruleExpired", model.FirewallRules, r.ID, subject)
			continue
		}
		live++
		if rt, _ := jx.AsString(r.Data[model.FieldRuleType]); rt == "rateLimit" {
			if n, ok := jx.AsFloat(r.Data[model.FieldRateLimitThreshold]); !ok || n < 1 {
				add(SeverityWarning, "rateLimitThresholdMissing", model.FirewallRules, r.ID, subject)
			}
		}
		if p, ok := jx.AsFloat(r.Data[model.FieldPriority]); ok {
			priorities[p] = append(priorities[p], r)
		}
	}
	// Rules that share a priority: which one wins is not defined by the author.
	keys := make([]float64, 0, len(priorities))
	for p := range priorities {
		keys = append(keys, p)
	}
	sort.Float64s(keys)
	for _, p := range keys {
		if group := priorities[p]; len(group) > 1 {
			names := make([]string, 0, len(group))
			for _, r := range group {
				names = append(names, name(r.Data))
			}
			add(SeverityWarning, "rulePriorityShared", model.FirewallRules, group[0].ID, strings.Join(names, ", "))
		}
	}

	for _, p := range snap[model.DdosPolicies] {
		if isOn(p.Data[model.FieldEnabled]) {
			live++
		}
	}

	// An address on both lists: the allow wins, so the block never fires for it.
	allow, block := map[string]bool{}, map[string]string{}
	entries := 0
	for _, e := range snap[model.IPAccessLists] {
		if !isOn(e.Data[model.FieldEnabled]) || expiredAt(e.Data[model.FieldExpiresAt], now) {
			continue
		}
		entries++
		value, _ := jx.AsString(e.Data[model.FieldValue])
		switch typ, _ := jx.AsString(e.Data[model.FieldType]); typ {
		case "whitelist":
			allow[value] = true
		case "blacklist":
			block[value] = e.ID
		}
	}
	vals := make([]string, 0, len(block))
	for v := range block {
		vals = append(vals, v)
	}
	sort.Strings(vals)
	for _, v := range vals {
		if allow[v] {
			add(SeverityWarning, "ipOnBothLists", model.IPAccessLists, block[v], v)
		}
	}

	if live == 0 && entries == 0 {
		add(SeverityWarning, "noProtection", "", "", "")
	}
	errs := 0
	for _, f := range out {
		if f.Severity == SeverityError {
			errs++
		}
	}
	if out == nil {
		out = []Finding{}
	}
	return ValidationReport{Findings: out, Errors: errs, Rules: live, Entries: entries}
}

// Validate reads the zone's records as rc sees them and checks them.
func Validate(ctx context.Context, backend crud.RecordBackend, zoneID uuid.UUID, rc *permission.RequestContext) (ValidationReport, error) {
	snap, _, err := versions.ReadZoneState(ctx, backend, zoneID, rc)
	if err != nil {
		return ValidationReport{}, err
	}
	report := checkSnapshot(snap, time.Now())
	if zoneID == uuid.Nil {
		// "Nothing protects this" is a statement about a zone, not about the tenant-wide records.
		report.Findings = slices.DeleteFunc(report.Findings, func(f Finding) bool { return f.Code == "noProtection" })
	}
	return report, nil
}
