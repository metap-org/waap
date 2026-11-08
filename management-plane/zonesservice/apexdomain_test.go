package zonesservice

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

func TestApexDomainOfACompoundTLDIsNotSpeciallyHandledAcceptedV1Limitation(t *testing.T) {
	// Groups on "co.uk", not the real apex "example.co.uk" — documented limitation, not a bug.
	got := ApexDomain("shop.example.co.uk")
	assert.Equal(t, "co.uk", got, "got %q", got)
}
