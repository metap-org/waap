package regexcache

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompilesAndCachesAValidPattern(t *testing.T) {
	first, ok := Compiled(`^/admin/\d+$`)
	require.True(t, ok, "expected a valid pattern to compile")
	assert.True(t, first.MatchString("/admin/42"), "expected /admin/42 to match")
	assert.False(t, first.MatchString("/admin/abc"), "expected /admin/abc to not match")
	// Second call must hit the cache (same *Regexp, not a fresh compile).
	second, ok := Compiled(`^/admin/\d+$`)
	assert.False(t, !ok || second != first, "expected the second call to return the cached *Regexp")
}

func TestAnInvalidPatternReturnsFalseAndStaysFalse(t *testing.T) {
	_, ok := Compiled("[invalid(regex")
	assert.False(t, ok, "expected an invalid pattern to not compile")
	// Cached as a miss too — calling again must not panic or somehow succeed.
	_, ok = Compiled("[invalid(regex")
	assert.False(t, ok, "expected the cached miss to still report not-ok")
}
