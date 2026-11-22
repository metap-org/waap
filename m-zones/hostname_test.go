package zonesservice

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeHostnameCleansWhatPeopleType(t *testing.T) {
	for in, want := range map[string]string{
		"shop.example.com":                     "shop.example.com",
		"  Shop.Example.COM  ":                 "shop.example.com",
		"https://shop.example.com/":            "shop.example.com",
		"http://shop.example.com:8080/a?b=1#c": "shop.example.com",
		"shop.example.com.":                    "shop.example.com",
		"shop.example.com:443":                 "shop.example.com",
		"user@shop.example.com":                "shop.example.com",
		"*.example.com":                        "*.example.com",
		"xn--bcher-kva.example":                "xn--bcher-kva.example",
	} {
		got, err := NormalizeHostname(in)
		require.NoError(t, err, in)
		assert.Equal(t, want, got, in)
	}
}

func TestNormalizeHostnameRefusesWhatCannotBeASite(t *testing.T) {
	for _, in := range []string{
		"", "   ", "localhost", "203.0.113.5", "[2001:db8::1]", "2001:db8::1", "exa mple.com", "-a.example.com", "a-.example.com",
		"a..example.com", "bücher.example", "*.com", "shop.*.example.com", "a*.example.com",
		"under score!.example.com",
	} {
		_, err := NormalizeHostname(in)
		assert.Error(t, err, "%q", in)
	}
}

func TestApexOfAWildcardIsItsDomain(t *testing.T) {
	assert.Equal(t, "example.com", ApexDomain("*.example.com"))
	assert.Equal(t, "example.com", ApexDomain("a.b.example.com"))
}
