package ruleset

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// Producer half of the control-plane -> edge-plane Redis contract. See
// ../../contracts/ruleset/README.md; edge-plane/go/ruleset/contract_test.go is the consumer.

var update = flag.Bool("update", false, "rewrite the contract golden files from this producer's output")

const contractDir = "../../contracts/ruleset"

func strp(s string) *string { return &s }

func pred(f Field, op Op, value string) MatchExpr {
	return PredicateExpr(Predicate{Field: f, Op: op, Value: strp(value)})
}

// contractZone exercises every Action, Field, Op and MatchExpr shape. Adding a constant to this
// package without adding it here leaves the consumer untested against it — keep it exhaustive.
func contractZone() CompiledZone {
	return CompiledZone{
		SchemaVersion:  RulesetSchemaVersion,
		ZoneID:         "11111111-1111-1111-1111-111111111111",
		TenantID:       "22222222-2222-2222-2222-222222222222",
		Hostname:       "shop.example.com",
		OriginAddress:  "http://10.0.0.5:8080",
		Status:         "active",
		ProtectionMode: "enforce",
		ConfigVersion:  7,
		CompiledAt:     "2026-11-07T00:00:00Z",
		Ddos: []CompiledDdos{
			{ID: "d1", Sensitivity: "high", Action: ActionChallenge, RequestRateThreshold: 500, BurstWindowSeconds: 10,
				PathPrefix: strp("/login"), HttpMethod: strp("POST"), Priority: 1},
			{ID: "d2", Sensitivity: "low", Action: ActionBlock, RequestRateThreshold: 5000, BurstWindowSeconds: 60, Priority: 2},
		},
		Rules: []CompiledRule{
			{ID: "r1", Name: "always log", RuleType: "custom", Action: ActionLog, Priority: 10, MatchExpr: Always()},
			{ID: "r2", Name: "admin from outside", RuleType: "custom", Action: ActionBlock, Priority: 20, MatchExpr: All([]MatchExpr{
				pred(FieldUriPath, OpStartsWith, "/admin"),
				Not(PredicateExpr(Predicate{Field: FieldSourceIpCidr, Op: OpIn, Values: []string{"10.0.0.0/8", "192.168.0.0/16"}})),
			})},
			{ID: "r3", Name: "scanners", RuleType: "custom", Action: ActionChallenge, Priority: 30, MatchExpr: Any([]MatchExpr{
				pred(FieldUserAgent, OpContainsCi, "sqlmap"),
				pred(FieldUriQuery, OpContains, "union select"),
				pred(FieldUriQuery, OpNotContains, "safe=1"),
				pred(FieldUriPath, OpEndsWith, ".php"),
				pred(FieldUriPath, OpRegex, `^/wp-(admin|login)`),
				PredicateExpr(Predicate{Field: FieldHeader, Op: OpEq, Param: strp("x-debug"), Value: strp("1")}),
				pred(FieldMethod, OpNotEq, "GET"),
				pred(FieldSourceIp, OpEq, "203.0.113.9"),
				PredicateExpr(Predicate{Field: FieldCountry, Op: OpNotIn, Values: []string{"VN", "SG"}}),
			})},
			{ID: "r4", Name: "api rate limit", RuleType: "rateLimit", Action: ActionAllow, Priority: 40,
				MatchExpr: pred(FieldUriPath, OpStartsWith, "/api"),
				RateLimit: &RateLimit{Threshold: 100, WindowSeconds: 60}},
		},
	}
}

func contractKeys() map[string]any {
	return map[string]any{
		"schemaVersion":  RulesetSchemaVersion,
		"zoneKey":        ZoneKey("shop.example.com"),
		"zoneVersionKey": ZoneVersionKey("shop.example.com"),
		"zoneIndexKey":   ZoneIndexKey,
		"epochKey":       EpochKey,
	}
}

func assertGolden(t *testing.T, name string, value any) {
	t.Helper()
	got, err := json.MarshalIndent(value, "", "  ")
	require.NoError(t, err)
	got = append(got, '\n')
	path := filepath.Join(contractDir, name)
	if *update {
		require.NoError(t, os.WriteFile(path, got, 0o644))
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err, "golden file missing — run `go test ./ruleset/ -run Contract -update`")
	require.JSONEq(t, string(want), string(got),
		"control-plane's rule-set JSON drifted from the contract the edge is tested against — "+
			"if intended, rerun with -update and make edge-plane/go/ruleset/contract_test.go pass")
}

func TestContractZoneMatchesGolden(t *testing.T) { assertGolden(t, "zone.golden.json", contractZone()) }

func TestContractKeysMatchGolden(t *testing.T) { assertGolden(t, "keys.golden.json", contractKeys()) }
