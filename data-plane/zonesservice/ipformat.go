// Format validation for waf.ip_access_lists.value, used by GuardedZonesBackend at save time.
//
// Mirrors the parsing edge-plane/waf-edge does at match time — a bare IP, or "network/prefix"
// with a prefix valid for that address family — but only checks the value parses at all, not
// whether the address is the correctly-masked base of its own prefix. "10.0.0.5/24" is accepted:
// the edge masks both sides when matching a request's source IP against this value, so an
// unaligned network address is not an authoring mistake worth rejecting, just a slightly
// imprecise one.
package zonesservice

import (
	"net"
	"strconv"
	"strings"
)

func IsValidIPOrCIDR(value string) bool {
	network, prefix, ok := strings.Cut(value, "/")
	if !ok {
		return net.ParseIP(value) != nil
	}
	ip := net.ParseIP(network)
	if ip == nil {
		return false
	}
	prefixLen, err := strconv.Atoi(prefix)
	if err != nil {
		return false
	}
	if ip.To4() != nil {
		return prefixLen >= 0 && prefixLen <= 32
	}
	return prefixLen >= 0 && prefixLen <= 128
}
