package zonesservice

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsValidIPOrCIDR(t *testing.T) {
	valid := []string{
		"10.0.0.5", "2001:db8::1",
		"10.0.0.0/16", "10.0.0.5/24", "2001:db8::/32",
	}
	for _, v := range valid {
		assert.True(t, IsValidIPOrCIDR(v), "expected %q to be valid", v)
	}

	invalid := []string{
		"10.0.0.0/33", "2001:db8::/129",
		"", "not-an-ip", "10.0.0.0/not-a-number", "10.0.0.0/-1",
	}
	for _, v := range invalid {
		assert.False(t, IsValidIPOrCIDR(v), "expected %q to be invalid", v)
	}
}
