// Layer: APP · pure business rules (the inner circle). No metap, HTTP or database: the standard library
// and the Public Suffix List (data compiled in). Enforced by .golangci.yml.

// The apex-domain grouping rule: the registrable domain, one label under the public suffix.
package rules

import (
	"strings"

	"golang.org/x/net/publicsuffix"
)

// ApexDomain is the domain somebody registers and can prove they own: "example.com" for
// "api.shop.example.com", "abc.com.vn" for "shop.abc.com.vn". Grouping on the last two labels
// instead would put every .com.vn customer under one "com.vn" domain, which nobody can prove and
// only one tenant could hold. A name with nothing under a suffix (a single label, or a suffix
// itself) comes back unchanged.
func ApexDomain(hostname string) string {
	hostname = strings.TrimPrefix(hostname, "*.") // a wildcard zone belongs to its domain
	hostname = strings.TrimRight(hostname, ".")
	apex, err := publicsuffix.EffectiveTLDPlusOne(strings.ToLower(hostname))
	if err != nil || len(apex) > len(hostname) {
		return hostname
	}
	return hostname[len(hostname)-len(apex):] // keeps the caller's spelling
}
