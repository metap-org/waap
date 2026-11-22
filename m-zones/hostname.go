package zonesservice

import (
	"errors"
	"net"
	"strings"
)

// NormalizeHostname turns what a person typed into the hostname a zone stores: lower case, without
// a scheme, path, port or trailing dot ("https://Shop.Example.com:8443/x" becomes
// "shop.example.com"). It refuses what cannot be a protected site: an IP address, a single label,
// non-ASCII text (give the punycode form, "xn--…"), a label that is empty, too long or has characters
// a DNS label cannot, or a wildcard anywhere but the whole first label.
//
// A wildcard "*.example.com" is accepted: one zone for every subdomain of a domain (UC-23).
func NormalizeHostname(raw string) (string, error) {
	h := strings.TrimSpace(raw)
	if i := strings.Index(h, "://"); i >= 0 {
		h = h[i+3:]
	}
	if i := strings.IndexAny(h, "/?#"); i >= 0 {
		h = h[:i]
	}
	if i := strings.LastIndex(h, "@"); i >= 0 {
		h = h[i+1:]
	}
	// A port: only when what follows the last colon is digits, so an IPv6 literal is not mangled
	// here but is refused below.
	if i := strings.LastIndex(h, ":"); i >= 0 && !strings.Contains(h[:i], ":") && isDigits(h[i+1:]) {
		h = h[:i]
	}
	h = strings.ToLower(strings.TrimRight(h, "."))
	if h == "" {
		return "", errors.New("enter a hostname such as shop.example.com")
	}
	if net.ParseIP(strings.Trim(h, "[]")) != nil {
		return "", errors.New("an IP address cannot be protected as a site; use its hostname")
	}
	if len(h) > 253 {
		return "", errors.New("a hostname is at most 253 characters")
	}
	for _, r := range h {
		if r > 127 {
			return "", errors.New("use the ASCII form of the hostname (internationalised names as xn--…)")
		}
	}
	labels := strings.Split(h, ".")
	if len(labels) < 2 {
		return "", errors.New("a hostname needs at least two labels, such as example.com")
	}
	for i, l := range labels {
		if l == "*" {
			if i != 0 {
				return "", errors.New("a wildcard is only allowed as the first label (*.example.com)")
			}
			if len(labels) < 3 {
				return "", errors.New("a wildcard needs a domain after it (*.example.com)")
			}
			continue
		}
		if l == "" || len(l) > 63 {
			return "", errors.New("each part of a hostname is 1 to 63 characters")
		}
		if l[0] == '-' || l[len(l)-1] == '-' {
			return "", errors.New("a part of a hostname cannot start or end with a hyphen")
		}
		for _, r := range l {
			if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
				return "", errors.New("a hostname may only contain letters, digits and hyphens")
			}
		}
	}
	return h, nil
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
