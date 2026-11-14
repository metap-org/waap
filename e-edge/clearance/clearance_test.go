package clearance

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// asCookieHeader: Issue() returns a full Set-Cookie header value; this pulls just the
// name=value pair out of it, i.e. what a browser would echo back on the next request's Cookie:
// header.
func asCookieHeader(setCookie string) string {
	parts := strings.Split(setCookie, ";")
	return parts[0]
}

func TestIssuedClearanceVerifiesForTheSameZoneAndIp(t *testing.T) {
	cookie := Issue("waf_clearance", "secret", "zone-1", "1.2.3.4", 60*time.Second)
	header := asCookieHeader(cookie)
	assert.True(t, Verify(header, "waf_clearance", "secret", "zone-1", "1.2.3.4"), "expected the issued clearance to verify")
}

func TestClearanceDoesNotTransferToADifferentZone(t *testing.T) {
	cookie := Issue("waf_clearance", "secret", "zone-1", "1.2.3.4", 60*time.Second)
	header := asCookieHeader(cookie)
	assert.False(t, Verify(header, "waf_clearance", "secret", "zone-2", "1.2.3.4"), "expected the clearance to not transfer to a different zone")
}

func TestClearanceDoesNotTransferToADifferentIp(t *testing.T) {
	cookie := Issue("waf_clearance", "secret", "zone-1", "1.2.3.4", 60*time.Second)
	header := asCookieHeader(cookie)
	assert.False(t, Verify(header, "waf_clearance", "secret", "zone-1", "9.9.9.9"), "expected the clearance to not transfer to a different IP")
}

func TestWrongSecretIsRejected(t *testing.T) {
	cookie := Issue("waf_clearance", "secret-a", "zone-1", "1.2.3.4", 60*time.Second)
	header := asCookieHeader(cookie)
	assert.False(t, Verify(header, "waf_clearance", "secret-b", "zone-1", "1.2.3.4"), "expected a wrong secret to be rejected")
}

func TestTamperedExpiryIsRejectedBecauseExpiryIsInsideTheSignedInput(t *testing.T) {
	cookie := Issue("waf_clearance", "secret", "zone-1", "1.2.3.4", 60*time.Second)
	header := asCookieHeader(cookie)
	// Append a digit to the numeric expiry prefix (before the signature) to try to extend the
	// clearance's lifetime — the signature was computed over the original expiry, so this must
	// fail rather than being accepted with a longer expiry than was issued.
	lastDot := strings.LastIndex(header, ".")
	nameAndExpiry, signature := header[:lastDot], header[lastDot+1:]
	forged := nameAndExpiry + "9." + signature
	assert.False(t, Verify(forged, "waf_clearance", "secret", "zone-1", "1.2.3.4"), "expected a tampered expiry to be rejected")
}

func TestExpiredClearanceIsRejected(t *testing.T) {
	// Built directly with sign() (same-package access) rather than sleeping past a real TTL —
	// an expiry timestamp already in the past, signed correctly, must still fail.
	past := nowSecs() - 10
	signature := sign("secret", "zone-1", "1.2.3.4", past)
	header := "waf_clearance=" + strconv.FormatUint(past, 10) + "." + signature
	assert.False(t, Verify(header, "waf_clearance", "secret", "zone-1", "1.2.3.4"), "expected an expired clearance to be rejected")
}

func TestMissingCookieIsRejected(t *testing.T) {
	assert.False(t, Verify("other_cookie=xyz", "waf_clearance", "secret", "zone-1", "1.2.3.4"), "expected a missing cookie to be rejected")
	assert.False(t, Verify("", "waf_clearance", "secret", "zone-1", "1.2.3.4"), "expected an empty header to be rejected")
}

func TestMalformedCookieValueIsRejectedNotPanicking(t *testing.T) {
	assert.False(t, Verify("waf_clearance=not-a-valid-format", "waf_clearance", "secret", "zone-1", "1.2.3.4"), "expected a malformed value to be rejected")
	assert.False(t, Verify("waf_clearance=123", "waf_clearance", "secret", "zone-1", "1.2.3.4"), "expected a value with no signature to be rejected")
	assert.False(t, Verify("waf_clearance=abc.def", "waf_clearance", "secret", "zone-1", "1.2.3.4"), "expected a non-numeric expiry to be rejected")
}

func TestCookieValueExtractsFromAMultiCookieHeader(t *testing.T) {
	header := "a=1; waf_clearance=999.abc; b=2"
	v, ok := cookieValue(header, "waf_clearance")
	assert.False(t, !ok || v != "999.abc", "expected 999.abc, got %q ok=%v", v, ok)
	_, ok = cookieValue(header, "missing")
	assert.False(t, ok, "expected a missing cookie name to report not-found")
}

func TestIssuedCookieCarriesTheExpectedAttributes(t *testing.T) {
	cookie := Issue("waf_clearance", "secret", "zone-1", "1.2.3.4", 1800*time.Second)
	assert.Contains(t, cookie, "HttpOnly", "expected HttpOnly")
	assert.Contains(t, cookie, "SameSite=Lax", "expected SameSite=Lax")
	assert.Contains(t, cookie, "Path=/", "expected Path=/")
	assert.Contains(t, cookie, "Max-Age=1800", "expected Max-Age=1800")
}
