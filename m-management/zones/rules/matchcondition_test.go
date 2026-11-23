package rules

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func mustJSON(t *testing.T, v string) json.RawMessage {
	t.Helper()
	return json.RawMessage(v)
}

func TestNoneAndNullAreAlwaysValid(t *testing.T) {
	assert.True(t, IsValidMatchCondition(nil), "nil should be valid")
	assert.True(t, IsValidMatchCondition(mustJSON(t, "null")), "null should be valid")
}

func TestASimplePredicateIsValid(t *testing.T) {
	raw := mustJSON(t, `{"field":"uri.path","op":"contains","value":"/admin"}`)
	assert.True(t, IsValidMatchCondition(raw), "expected valid")
}

func TestAnUnknownFieldOrOpIsInvalid(t *testing.T) {
	assert.False(t, IsValidMatchCondition(mustJSON(t, `{"field":"bogus","op":"eq","value":"x"}`)), "expected invalid field to be rejected")
	assert.False(t, IsValidMatchCondition(mustJSON(t, `{"field":"method","op":"bogus","value":"x"}`)), "expected invalid op to be rejected")
}

func TestHeaderFieldRequiresAParamOrHeaderName(t *testing.T) {
	assert.False(t, IsValidMatchCondition(mustJSON(t, `{"field":"header","op":"eq","value":"1"}`)), "expected missing param/header to be rejected")
	assert.True(t, IsValidMatchCondition(mustJSON(t, `{"field":"header","op":"eq","value":"1","param":"x-api-key"}`)), "expected valid with param")
}

func TestInAndNotInRequireNonEmptyValues(t *testing.T) {
	assert.False(t, IsValidMatchCondition(mustJSON(t, `{"field":"sourceIp","op":"in","values":[]}`)), "expected empty values to be rejected")
	assert.True(t, IsValidMatchCondition(mustJSON(t, `{"field":"sourceIp","op":"in","values":["1.2.3.4"]}`)), "expected non-empty values to be valid")
}

func TestRegexOpRequiresAPatternThatActuallyCompiles(t *testing.T) {
	assert.True(t, IsValidMatchCondition(mustJSON(t, `{"field":"uri.path","op":"regex","value":"^/admin/[0-9]+$"}`)), "expected a compiling regex to be valid")
	assert.False(t, IsValidMatchCondition(mustJSON(t, `{"field":"uri.path","op":"regex","value":"[invalid(regex"}`)), "expected a non-compiling regex to be rejected")
}

func TestAllAnyNotRecurseAndRequireEveryChildValid(t *testing.T) {
	valid := mustJSON(t, `{"all":[{"field":"method","op":"eq","value":"POST"},{"not":{"field":"sourceIp","op":"eq","value":"10.0.0.1"}}]}`)
	assert.True(t, IsValidMatchCondition(valid), "expected valid nested condition")

	invalid := mustJSON(t, `{"any":[{"field":"method","op":"eq","value":"POST"},{"field":"bogus","op":"eq","value":"x"}]}`)
	assert.False(t, IsValidMatchCondition(invalid), "expected invalid nested condition to be rejected")
}

// One rule may not cost the shared edge unbounded work (audit 08): depth, comparisons, list sizes
// and pattern length are capped; ordinary conditions are unaffected.
func TestMatchConditionCostLimits(t *testing.T) {
	leaf := `{"field":"uri.path","op":"eq","value":"/x"}`
	nest := func(depth int) string {
		s := leaf
		for i := 0; i < depth; i++ {
			s = `{"not":` + s + `}`
		}
		return s
	}
	assert.True(t, IsValidMatchCondition(mustJSON(t, nest(MaxMatchDepth-1))))
	assert.False(t, IsValidMatchCondition(mustJSON(t, nest(MaxMatchDepth+2))), "too deep")

	many := func(n int) string {
		items := make([]string, n)
		for i := range items {
			items[i] = leaf
		}
		return `{"any":[` + strings.Join(items, ",") + `]}`
	}
	assert.True(t, IsValidMatchCondition(mustJSON(t, many(MaxMatchPredicates))))
	assert.False(t, IsValidMatchCondition(mustJSON(t, many(MaxMatchPredicates+1))), "too many comparisons")

	long := strings.Repeat("a", MaxRegexLength+1)
	assert.False(t, IsValidMatchCondition(mustJSON(t, `{"field":"uri.path","op":"regex","value":"`+long+`"}`)), "pattern too long")
	assert.True(t, IsValidMatchCondition(mustJSON(t, `{"field":"uri.path","op":"regex","value":"^/api/v[0-9]+/"}`)))

	values := make([]string, MaxMatchValues+1)
	for i := range values {
		values[i] = `"203.0.113.1"`
	}
	assert.False(t, IsValidMatchCondition(mustJSON(t, `{"field":"ip","op":"in","values":[`+strings.Join(values, ",")+`]}`)), "too many values")
	assert.False(t, IsValidMatchCondition(mustJSON(t, `{"field":"uri.path","op":"contains","value":"`+strings.Repeat("b", MaxMatchValueLength+1)+`"}`)), "value too long")
}
