package rules

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestApexDomainOfABareApexHostnameIsUnchanged(t *testing.T) {
	got := ApexDomain("example.com")
	assert.Equal(t, "example.com", got, "got %q", got)
}

func TestApexDomainOfASubdomainReducesToItsApex(t *testing.T) {
	got := ApexDomain("shop.example.com")
	assert.Equal(t, "example.com", got, "got %q", got)
	got = ApexDomain("api.shop.example.com")
	assert.Equal(t, "example.com", got, "got %q", got)
}

func TestApexDomainOfASingleLabelHostnameIsUnchanged(t *testing.T) {
	got := ApexDomain("localhost")
	assert.Equal(t, "localhost", got, "got %q", got)
}

func TestApexDomainKnowsMultiLabelSuffixes(t *testing.T) {
	for host, want := range map[string]string{
		"shop.example.co.uk":  "example.co.uk",
		"shop.abc.com.vn":     "abc.com.vn",
		"abc.com.vn":          "abc.com.vn",
		"a.b.alice.github.io": "alice.github.io", // a private suffix: each user is their own owner
		"*.abc.com.vn":        "abc.com.vn",
		"Shop.ABC.com.vn.":    "ABC.com.vn",
		"z1.edge.waf.local":   "waf.local", // a suffix the list does not know: the last two labels
		"com.vn":              "com.vn",    // a suffix itself has nothing to group on
	} {
		assert.Equal(t, want, ApexDomain(host), host)
	}
	assert.NotEqual(t, ApexDomain("a.one.com.vn"), ApexDomain("a.two.com.vn"), "two customers, two domains")
}
