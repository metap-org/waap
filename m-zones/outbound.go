// Outbound requests made on a customer's say-so: probing an origin, fetching an ownership file. The
// address is theirs to choose, so it is also theirs to point at our own network; the client here
// resolves the name itself and refuses to connect to anything that is not a public address, unless
// ALLOW_PRIVATE_TARGETS=true (dev, where origins and test hostnames are on localhost or a mock).
//
// Names are resolved the way the rest of this service resolves them (DoH, dohURL), so a dev run
// against dns-mock resolves test domains and production resolves what the world does.
package zonesservice

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"time"

	"github.com/metap-org/metap/pkg/kit/env"
)

func allowPrivateTargets() bool {
	v, _ := env.Lookup("ALLOW_PRIVATE_TARGETS")
	return v == "true"
}

// publicAddress reports whether ip is one a customer's site could legitimately live on.
func publicAddress(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() || ip.IsUnspecified() || ip.IsInterfaceLocalMulticast() {
		return false
	}
	// Carrier-grade NAT (100.64.0.0/10), the "this network" block and the documentation-free
	// reserved space that netip does not call private.
	for _, p := range []string{"100.64.0.0/10", "0.0.0.0/8", "192.0.0.0/24", "198.18.0.0/15", "240.0.0.0/4", "fc00::/7", "fe80::/10"} {
		if netip.MustParsePrefix(p).Contains(ip) {
			return false
		}
	}
	return true
}

// resolveAddrs returns the addresses of host: itself when it is a literal, else A and AAAA over DoH,
// else the system resolver.
func resolveAddrs(ctx context.Context, host string) ([]netip.Addr, error) {
	if ip, err := netip.ParseAddr(host); err == nil {
		return []netip.Addr{ip}, nil
	}
	var out []netip.Addr
	client := httpClient()
	for _, t := range []string{"A", "AAAA"} {
		for _, v := range dnsLookup(client, host, t) {
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

var errPrivateTarget = errors.New("this address is not a public one, so it is not contacted")

// outboundClient is an HTTP client that connects only to public addresses (or any when private
// targets are allowed), follows no redirect, and presents serverName in TLS when it is set.
func outboundClient(timeout time.Duration, serverName string) *http.Client {
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			ips, err := resolveAddrs(ctx, host)
			if err != nil {
				return nil, err
			}
			var last error = errPrivateTarget
			for _, ip := range ips {
				if !allowPrivateTargets() && !publicAddress(ip) {
					continue
				}
				conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
				if err == nil {
					return conn, nil
				}
				last = err
			}
			return nil, last
		},
		TLSClientConfig:       &tls.Config{ServerName: serverName, MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: timeout,
		DisableKeepAlives:     true,
	}
	return &http.Client{
		Timeout:       timeout,
		Transport:     transport,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}
