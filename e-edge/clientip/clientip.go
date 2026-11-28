// Package clientip works out who is really calling the edge. Every IP rule, rate limit and DDoS
// budget is keyed by this address, so nothing a caller can type may choose it.
package clientip

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"

	"github.com/metap-org/metap/pkg/kit/httpx"
)

const forwardedFor = "X-Forwarded-For"

// Resolver answers the client address of a request. The zero value uses the TCP peer.
type Resolver struct {
	header  string
	trusted []netip.Prefix
}

// New builds a resolver from CLIENT_IP_HEADER (header, "" when unset) and TRUSTED_PROXIES (CIDRs
// or addresses). A header without a trusted proxy is refused: any caller can send a header, so it
// is evidence only when it comes from a proxy known to write it.
func New(header string, proxies []string) (Resolver, error) {
	trusted, err := httpx.ParseTrustedProxies(proxies)
	if err != nil {
		return Resolver{}, err
	}
	header = strings.TrimSpace(header)
	if header != "" && len(trusted) == 0 {
		return Resolver{}, fmt.Errorf("CLIENT_IP_HEADER=%s needs TRUSTED_PROXIES: the addresses of the proxies in front of this node", header)
	}
	return Resolver{header: http.CanonicalHeaderKey(header), trusted: trusted}, nil
}

// Of is the client's address: the TCP peer, unless the peer is a trusted proxy. Then
// X-Forwarded-For is read from the right, past every trusted proxy (what a caller put at the front
// of the list is never reached); any other header is taken as the one address that proxy wrote.
func (r Resolver) Of(req *http.Request) net.IP {
	peer := peerOf(req)
	if r.header == "" {
		return net.ParseIP(peer)
	}
	if r.header == forwardedFor {
		return net.ParseIP(httpx.ClientIP(req, r.trusted))
	}
	if !r.trusts(peer) {
		return net.ParseIP(peer)
	}
	// A header sent twice means somebody else wrote one of them, so neither is believed.
	values := req.Header.Values(r.header)
	if len(values) == 1 {
		if ip := net.ParseIP(strings.TrimSpace(values[0])); ip != nil {
			return ip
		}
	}
	return net.ParseIP(peer)
}

func (r Resolver) trusts(peer string) bool {
	addr, err := netip.ParseAddr(peer)
	if err != nil {
		return false
	}
	for _, p := range r.trusted {
		if p.Contains(addr.Unmap()) {
			return true
		}
	}
	return false
}

func peerOf(req *http.Request) string {
	host, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		return req.RemoteAddr
	}
	return host
}
