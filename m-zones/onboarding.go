// Onboarding helpers (UC-20 to UC-24): what a hostname resolves to before a zone exists for it, how
// an origin is probed with its own protocol, port and host header, and the HTTP-file way of proving a
// domain is the customer's.
package zonesservice

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/kit/errcode"
	"github.com/metap-org/waap/model"
)

// originSpec is how an origin is reached.
type originSpec struct {
	Address    string
	Protocol   string // "http" or "https"; empty means https
	Port       int    // 0 means the protocol's own
	HostHeader string // empty means the address
}

// originSpecOf reads the origin settings off a zone's fields.
func originSpecOf(data crud.JsonObject) originSpec {
	addr, _ := jsonAsString(data[model.FieldOriginAddress])
	proto, _ := jsonAsString(data[model.FieldOriginProtocol])
	port, _ := jsonAsFloat(data[model.FieldOriginPort])
	host, _ := jsonAsString(data[model.FieldOriginHostHeader])
	return originSpec{Address: addr, Protocol: proto, Port: int(port), HostHeader: host}
}

// originTarget works out the URL, and the host (for the Host header and TLS name) a spec reaches. A
// scheme or path typed into the address is accepted and dropped, as people paste URLs there; the
// protocol field, then a scheme typed in, then https, decide the scheme.
func originTarget(spec originSpec) (target, host string, err error) {
	addr := strings.TrimSpace(spec.Address)
	scheme := spec.Protocol
	if i := strings.Index(addr, "://"); i >= 0 {
		if scheme == "" {
			scheme = strings.ToLower(addr[:i])
		}
		addr = addr[i+3:]
	}
	if i := strings.IndexAny(addr, "/?#"); i >= 0 {
		addr = addr[:i]
	}
	if scheme == "" {
		scheme = "https"
	}
	if scheme != "http" && scheme != "https" {
		return "", "", fmt.Errorf("the origin protocol must be http or https")
	}
	if addr == "" {
		return "", "", fmt.Errorf("enter the origin's address")
	}
	hostPart, portPart := addr, ""
	if h, p, e := net.SplitHostPort(addr); e == nil {
		hostPart, portPart = h, p
	}
	port := spec.Port
	if port == 0 && portPart != "" {
		port, _ = strconv.Atoi(portPart)
	}
	if port < 0 || port > 65535 {
		return "", "", fmt.Errorf("the origin port must be between 1 and 65535")
	}
	hostPart = strings.Trim(hostPart, "[]")
	netHost := hostPart
	if port != 0 {
		netHost = net.JoinHostPort(hostPart, strconv.Itoa(port))
	} else if strings.Contains(hostPart, ":") {
		netHost = "[" + hostPart + "]"
	}
	return scheme + "://" + netHost + "/", hostPart, nil
}

// probeOrigin makes one request to the origin and reports whether it answered. It follows nothing and
// reaches only public addresses (outbound.go); an origin that answers with an error status still
// counts as reachable, since what is asked is whether the edge can talk to it.
func probeOrigin(ctx context.Context, spec originSpec) map[string]any {
	target, host, err := originTarget(spec)
	if err != nil {
		return map[string]any{"reachable": false, "error": err.Error(), "latencyMs": 0, "url": ""}
	}
	hostHeader := spec.HostHeader
	if hostHeader == "" {
		hostHeader = host
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return map[string]any{"reachable": false, "error": err.Error(), "latencyMs": 0, "url": target}
	}
	req.Host = hostHeader
	req.Header.Set("User-Agent", "waf-origin-check/1")
	started := time.Now()
	resp, err := outboundClient(probeTimeout, hostHeader).Do(req)
	elapsed := time.Since(started).Milliseconds()
	if err != nil {
		return map[string]any{"reachable": false, "error": err.Error(), "latencyMs": elapsed, "url": target}
	}
	defer resp.Body.Close()
	return map[string]any{"reachable": true, "status": resp.StatusCode, "latencyMs": elapsed, "url": target}
}

// resolveHostnameView is what the Add zone form shows under the hostname as it is typed.
type resolveHostnameView struct {
	Hostname string   `json:"hostname"`
	A        []string `json:"a"`
	AAAA     []string `json:"aaaa"`
	CNAME    []string `json:"cname"`
	// PointsToEdge: the name already routes to this service (a CNAME at the edge target).
	PointsToEdge bool `json:"pointsToEdge"`
	// SuggestedOrigin is where the site is hosted today, to offer as the origin: the first address
	// the name resolves to, unless it already points at the edge (then there is nothing to suggest).
	SuggestedOrigin string `json:"suggestedOrigin,omitempty"`
	EdgeTarget      string `json:"edgeTarget"`
	// Warnings are codes the form translates: "unresolved" (nothing found), "pointsToEdge".
	Warnings []string `json:"warnings"`
}

// resolveHostname normalises a typed hostname and says what it resolves to now (UC-20). A name that
// is not a valid hostname is a validation error naming the field.
func resolveHostname(ctx context.Context, input crud.JsonObject) (resolveHostnameView, error) {
	raw, _ := jsonAsString(input[model.FieldHostname])
	host, err := NormalizeHostname(raw)
	if err != nil {
		return resolveHostnameView{}, crud.NewServiceErrorFields(http.StatusUnprocessableEntity, errcode.ValidationFailed,
			map[string][]string{model.FieldHostname: {err.Error()}})
	}
	lookupHost := strings.TrimPrefix(host, "*.") // a wildcard resolves through its domain
	client := httpClient()
	view := resolveHostnameView{
		Hostname: host, EdgeTarget: edgeCNAMETarget(), Warnings: []string{},
		A: dnsLookup(client, lookupHost, "A"), AAAA: dnsLookup(client, lookupHost, "AAAA"), CNAME: dnsLookup(client, lookupHost, "CNAME"),
	}
	for _, c := range view.CNAME {
		if hasSuffix(c, view.EdgeTarget) {
			view.PointsToEdge = true
		}
	}
	switch {
	case view.PointsToEdge:
		view.Warnings = append(view.Warnings, "pointsToEdge")
	case len(view.A)+len(view.AAAA)+len(view.CNAME) == 0:
		view.Warnings = append(view.Warnings, "unresolved")
	case len(view.A) > 0:
		view.SuggestedOrigin = view.A[0]
	case len(view.AAAA) > 0:
		view.SuggestedOrigin = view.AAAA[0]
	case len(view.CNAME) > 0:
		view.SuggestedOrigin = view.CNAME[0]
	}
	return view, nil
}

// verificationURL is where the ownership file is fetched from; a variable so a test can point it at
// a local server (the real one is always plain HTTP on port 80).
var verificationURL = func(host, token string) string {
	return "http://" + host + "/.well-known/waf-verify/" + token
}

// fetchVerificationFile reads http://<host>/.well-known/waf-verify/<token> and reports whether its
// body is the token (UC-21, the HTTP-file way). The host must be the apex domain or a name under it,
// so a customer cannot make this service fetch somewhere unrelated to the domain being proven.
func fetchVerificationFile(ctx context.Context, apex, host, token string) (found bool, detail string, err error) {
	host = strings.ToLower(strings.TrimSpace(strings.TrimSuffix(host, ".")))
	if host == "" {
		host = apex
	}
	if host != apex && !strings.HasSuffix(host, "."+apex) {
		return false, "", crud.NewServiceErrorFields(http.StatusUnprocessableEntity, errcode.ValidationFailed,
			map[string][]string{"host": {"must be " + apex + " or a name under it"}})
	}
	url := verificationURL(host, token)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, "", err
	}
	resp, err := outboundClient(probeTimeout, "").Do(req)
	if err != nil {
		return false, err.Error(), nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Sprintf("the file answered %d", resp.StatusCode), nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	if strings.TrimSpace(string(body)) != token {
		return false, "the file does not contain the token", nil
	}
	return true, "", nil
}
