// Layer: APP · config. What the outbound checks need to know, read once in cmd/ and passed in.

package netcheck

import (
	"net/http"

	"github.com/metap-org/metap/pkg/kit/httpx"
)

// Config is everything the outbound checks read from the environment. It is read once, in cmd/,
// and passed down; nothing below main calls env.* (docs/architecture/08-cross-cutting/20-code-structure-standard.md,
// 2.10).
type Config struct {
	// DoHURL is the DNS-over-HTTPS resolver every name lookup goes through.
	DoHURL string
	// EdgeCNAMETarget is what a customer's hostname is pointed at to route through the edge.
	EdgeCNAMETarget string
	// AllowPrivateTargets lets origin probes reach private addresses (dev only, see outbound.go).
	AllowPrivateTargets bool
}

// httpClient is the client for calls this service makes on its own behalf (DoH lookups).
// Redirects are off on purpose: an origin under test is attacker-adjacent input, and a redirect
// chain is not what "is this reachable" asks.
func (c Config) httpClient() *http.Client {
	client := httpx.NewClient(probeTimeout)
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return client
}
