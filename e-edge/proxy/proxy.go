// Package proxy passes an allowed request through to the customer's origin.
//
// Scope, stated plainly: this is a reverse proxy adequate for a mitigation demo, not a
// production CDN edge. It does not do connection pooling per origin beyond what net/http's
// default Transport gives it, TLS to the origin is plain http unless the origin address says
// otherwise, and it buffers request bodies rather than streaming them. Those are real
// limitations, listed here rather than discovered later.
//
// What it does get right, because getting them wrong would be a security bug rather than a
// performance one:
//   - hop-by-hop headers are stripped (RFC 9110 §7.6.1), so Connection/Upgrade from a client
//     cannot leak into the origin request;
//   - X-Forwarded-For is SET, not appended to whatever the client sent — a client-supplied
//     X-Forwarded-For reaching the origin would let anyone forge their own source address for
//     every downstream system that trusts that header;
//   - the origin's own hop-by-hop headers are stripped on the way back.
package proxy

import (
	"bytes"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// hopByHop lists headers that describe one hop of a connection and must never be forwarded
// across it.
var hopByHop = map[string]bool{
	"Connection":          true,
	"Keep-Alive":          true,
	"Proxy-Authenticate":  true,
	"Proxy-Authorization": true,
	"Te":                  true,
	"Trailers":            true,
	"Transfer-Encoding":   true,
	"Upgrade":             true,
}

func BuildClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				// Bounded so a slow origin ties up a connection attempt for a known amount of
				// time rather than indefinitely; the caller turns a failure into a 502 page.
				Timeout: 5 * time.Second,
			}).DialContext,
			// An origin that accepts the connection and then never answers would otherwise pin
			// this request (and its buffered body) forever; the caller turns it into a 502.
			ResponseHeaderTimeout: 30 * time.Second,
		},
		// This proxy follows the origin's response verbatim, redirects included — a client
		// asked the ORIGIN for a resource, and the origin deciding to redirect is the origin's
		// business, not something this hop should silently follow on the client's behalf.
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// originURL builds the origin URL from the zone's configured origin address plus the incoming
// path.
//
// The address as authored in the portal may be bare (203.0.113.10, origin.example.com) or a full
// URL. A bare address is treated as http — the origin hop is typically inside the customer's own
// network, and guessing https for an origin that only speaks http fails in a way that looks like
// the WAF broke the site.
func originURL(originAddress, pathAndQuery string) (*url.URL, error) {
	base := originAddress
	if !strings.HasPrefix(base, "http://") && !strings.HasPrefix(base, "https://") {
		base = "http://" + strings.TrimSuffix(base, "/")
	} else {
		base = strings.TrimSuffix(base, "/")
	}
	return url.Parse(base + pathAndQuery)
}

type ErrKind int

const (
	// ErrBadOrigin: the zone's configured origin address does not form a usable URL.
	ErrBadOrigin ErrKind = iota
	ErrBadRequestBody
	ErrUnreachable
	// ErrBodyTooLarge: the request body exceeds the configured limit. It is buffered in memory
	// (see Forward), so an unbounded body is an unbounded allocation per request.
	ErrBodyTooLarge
)

type Error struct{ Kind ErrKind }

func (e *Error) Error() string {
	switch e.Kind {
	case ErrBadOrigin:
		return "bad origin address"
	case ErrBadRequestBody:
		return "bad request body"
	case ErrBodyTooLarge:
		return "request body too large"
	default:
		return "origin unreachable"
	}
}

// Forward proxies request to originAddress, rewriting headers per this package's own doc
// comment, and writes the origin's response (or an error) to w.
func Forward(client *http.Client, w http.ResponseWriter, r *http.Request, originAddress, clientIP, host string, maxBody int64) error {
	pathAndQuery := r.URL.Path
	if r.URL.RawQuery != "" {
		pathAndQuery += "?" + r.URL.RawQuery
	}
	if pathAndQuery == "" {
		pathAndQuery = "/"
	}
	target, err := originURL(originAddress, pathAndQuery)
	if err != nil {
		return &Error{Kind: ErrBadOrigin}
	}

	// Buffered, not streamed: this proxy's job is evaluating rules rather than moving large
	// uploads. A real edge would stream; this one is honest about not doing so.
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return &Error{Kind: ErrBodyTooLarge}
		}
		return &Error{Kind: ErrBadRequestBody}
	}

	upstreamReq, err := http.NewRequest(r.Method, target.String(), newReader(body))
	if err != nil {
		return &Error{Kind: ErrBadOrigin}
	}
	for name, values := range r.Header {
		if hopByHop[http.CanonicalHeaderKey(name)] {
			continue
		}
		for _, v := range values {
			upstreamReq.Header.Add(name, v)
		}
	}
	// The origin should see the hostname the visitor asked for, not the origin's own address.
	upstreamReq.Host = host
	// Overwritten, never appended — see the package doc.
	upstreamReq.Header.Set("X-Forwarded-For", clientIP)
	upstreamReq.Header.Set("X-Real-Ip", clientIP)
	upstreamReq.Header.Set("X-Forwarded-Proto", "http")
	// Lets an origin tell WAF-proxied traffic from direct traffic — the basis of any "only
	// accept traffic from the edge" origin lock-down.
	upstreamReq.Header.Set("X-Waf-Edge", "1")
	upstreamReq.ContentLength = int64(len(body))

	resp, err := client.Do(upstreamReq)
	if err != nil {
		return &Error{Kind: ErrUnreachable}
	}
	defer resp.Body.Close()

	for name, values := range resp.Header {
		if hopByHop[http.CanonicalHeaderKey(name)] {
			continue
		}
		for _, v := range values {
			w.Header().Add(name, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	// The origin's body streams back untouched; only the headers were filtered.
	_, copyErr := io.Copy(w, resp.Body)
	if copyErr != nil && !errors.Is(copyErr, io.ErrClosedPipe) {
		// The status/headers are already written at this point, so this is logged by the
		// caller, not turned into a different response.
		return nil
	}
	return nil
}

func newReader(b []byte) io.Reader {
	if len(b) == 0 {
		return nil
	}
	return bytes.NewReader(b)
}
