// Layer: APP · outbound call. How this service reaches something outside it, through metap's kit (httpx, safehttp).

// Outbound requests made on a customer's say-so: probing an origin, fetching an ownership file. The
// address is theirs to choose, so it is also theirs to point at our own network; the client is
// metap's safehttp, which resolves the name itself and refuses to connect to anything that is not a
// public address, unless Config.AllowPrivateTargets (ALLOW_PRIVATE_TARGETS=true; dev, where origins
// and test hostnames are on localhost or a mock).
//
// Names are resolved the way the rest of this service resolves them (DoH, Config.DoHURL), so a dev
// run against a DNS mock resolves test domains and production resolves what the world does.
package netcheck

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"time"

	"github.com/metap-org/metap/pkg/kit/safehttp"
)

// resolveAddrs returns the addresses of host: A and AAAA over DoH, else the system resolver.
func (c Config) resolveAddrs(ctx context.Context, host string) ([]netip.Addr, error) {
	var out []netip.Addr
	for _, t := range []string{"A", "AAAA"} {
		for _, v := range c.dnsLookup(host, t) {
			if ip, err := netip.ParseAddr(v); err == nil {
				out = append(out, ip)
			}
		}
	}
	if len(out) > 0 {
		return out, nil
	}
	ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, fmt.Errorf("%s does not resolve", host)
	}
	return ips, nil
}

// outboundClient is an HTTP client for a customer-chosen address; serverName is presented in TLS
// when set.
func (c Config) outboundClient(timeout time.Duration, serverName string) *http.Client {
	return safehttp.NewClient(safehttp.Options{
		Timeout: timeout, ServerName: serverName, AllowPrivate: c.AllowPrivateTargets, Resolve: c.resolveAddrs,
	})
}
