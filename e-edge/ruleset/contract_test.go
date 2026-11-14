package ruleset

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Consumer half of the control-plane -> edge-plane Redis contract: the golden files are written
// by control-plane/go/ruleset/contract_test.go from control-plane's own types. See
// ../../contracts/ruleset/README.md.

const contractDir = "../../contracts/ruleset"

func readGolden(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(contractDir, name))
	require.NoError(t, err)
	return b
}

func TestContractZoneGoldenDecodesStrictly(t *testing.T) {
	dec := json.NewDecoder(bytes.NewReader(readGolden(t, "zone.golden.json")))
	// A field control-plane writes that the edge doesn't declare is drift, not something to
	// silently ignore.
	dec.DisallowUnknownFields()
	var z CompiledZone
	require.NoError(t, dec.Decode(&z), "edge cannot decode what control-plane now writes")

	assert.Equal(t, SchemaVersion, z.SchemaVersion)
	assert.Equal(t, "shop.example.com", z.Hostname)
	assert.True(t, z.Enforcing())

	require.Len(t, z.Ddos, 2)
	require.NotNil(t, z.Ddos[0].PathPrefix)
	assert.Equal(t, "/login", *z.Ddos[0].PathPrefix)
	assert.Equal(t, ActionChallenge, z.Ddos[0].Action)
	assert.Nil(t, z.Ddos[1].HttpMethod)

	require.Len(t, z.Rules, 4)
	assert.Equal(t, MatchAlways, z.Rules[0].MatchExpr.Kind)

	r2 := z.Rules[1].MatchExpr
	require.Equal(t, MatchAll, r2.Kind)
	require.Len(t, r2.Children, 2)
	assert.Equal(t, MatchPredicate, r2.Children[0].Kind)
	require.Equal(t, MatchNot, r2.Children[1].Kind)
	notInner := r2.Children[1].Not.Predicate
	require.NotNil(t, notInner)
	assert.Equal(t, FieldSourceIpCidr, notInner.Field)
	assert.Equal(t, []string{"10.0.0.0/8", "192.168.0.0/16"}, notInner.Values)

	r3 := z.Rules[2].MatchExpr
	require.Equal(t, MatchAny, r3.Kind)
	var header *Predicate
	for _, c := range r3.Children {
		if c.Predicate != nil && c.Predicate.Field == FieldHeader {
			header = c.Predicate
		}
	}
	require.NotNil(t, header, "header predicate missing")
	require.NotNil(t, header.Param)
	assert.Equal(t, "x-debug", *header.Param)

	require.NotNil(t, z.Rules[3].RateLimit)
	assert.Equal(t, RateLimit{Threshold: 100, WindowSeconds: 60}, *z.Rules[3].RateLimit)
}

func TestContractGoldenUsesEveryActionFieldAndOpTheEdgeKnows(t *testing.T) {
	var z CompiledZone
	require.NoError(t, json.Unmarshal(readGolden(t, "zone.golden.json"), &z))

	actions, fields, ops := map[Action]bool{}, map[Field]bool{}, map[Op]bool{}
	var walk func(m MatchExpr)
	walk = func(m MatchExpr) {
		if m.Predicate != nil {
			fields[m.Predicate.Field], ops[m.Predicate.Op] = true, true
		}
		for _, c := range m.Children {
			walk(c)
		}
		if m.Not != nil {
			walk(*m.Not)
		}
	}
	for _, d := range z.Ddos {
		actions[d.Action] = true
	}
	for _, r := range z.Rules {
		actions[r.Action] = true
		walk(r.MatchExpr)
	}
	for _, a := range []Action{ActionAllow, ActionLog, ActionChallenge, ActionBlock} {
		assert.True(t, actions[a], "golden never uses action %q", a)
	}
	for _, f := range []Field{FieldUriPath, FieldUriQuery, FieldMethod, FieldHeader, FieldSourceIp, FieldSourceIpCidr, FieldCountry, FieldUserAgent} {
		assert.True(t, fields[f], "golden never uses field %q", f)
	}
	for _, o := range []Op{OpEq, OpNotEq, OpContains, OpNotContains, OpStartsWith, OpEndsWith, OpIn, OpNotIn, OpContainsCi, OpRegex} {
		assert.True(t, ops[o], "golden never uses op %q", o)
	}
}

func TestContractKeysMatchGolden(t *testing.T) {
	var keys struct {
		SchemaVersion  uint32 `json:"schemaVersion"`
		ZoneKey        string `json:"zoneKey"`
		ZoneVersionKey string `json:"zoneVersionKey"`
		ZoneIndexKey   string `json:"zoneIndexKey"`
		EpochKey       string `json:"epochKey"`
	}
	require.NoError(t, json.Unmarshal(readGolden(t, "keys.golden.json"), &keys))
	assert.Equal(t, SchemaVersion, keys.SchemaVersion)
	assert.Equal(t, ZoneKey("shop.example.com"), keys.ZoneKey)
	assert.Equal(t, ZoneVersionKey("shop.example.com"), keys.ZoneVersionKey)
	assert.Equal(t, ZoneIndexKey, keys.ZoneIndexKey)
	assert.Equal(t, EpochKey, keys.EpochKey)
}
