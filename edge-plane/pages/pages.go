// Package pages renders what a blocked or challenged visitor actually sees.
//
// Deliberately plain: no external assets (a blocked visitor cannot be assumed to reach a CDN),
// no branding (per-zone branding is a v2 feature), and a ray id so a customer reporting "I got
// blocked" gives their operator something to search for.
package pages

import (
	"fmt"
	"net/http"
	"time"
)

// RayID is a short random token shown to the visitor and attached to the security event, so a
// support conversation can connect the two. Not a security control — it identifies a request,
// it does not authorise anything.
func RayID() string {
	// Time-based rather than a UUID dependency: uniqueness only has to hold well enough to find
	// one request in a log, and the edge's dependency list is kept deliberately short.
	return fmt.Sprintf("%x", time.Now().UnixNano())
}

func page(title, headline, detail, ray string) string {
	return fmt.Sprintf(`<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>%s</title>
<style>
  body { font: 15px/1.5 system-ui, sans-serif; background: #0b0d10; color: #e6e8eb;
         display: flex; min-height: 100vh; margin: 0; align-items: center; justify-content: center; }
  main { max-width: 34rem; padding: 2rem; }
  h1 { font-size: 1.35rem; margin: 0 0 .5rem; }
  p { color: #9aa3ad; margin: 0 0 1rem; }
  code { font-size: .8rem; color: #6f7883; }
</style></head>
<body><main>
  <h1>%s</h1>
  <p>%s</p>
  <code>Ray ID: %s</code>
</main></body></html>`, title, headline, detail, ray)
}

func writeHTML(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// A blocked/challenge/error response must never be cached by anything in between — the rule
	// that produced it can be changed at any moment, and a cached response would outlive it.
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

func Blocked(w http.ResponseWriter, ray string) {
	html := page("Request blocked", "This request was blocked",
		"The security rules for this site do not allow this request. If you believe this is a "+
			"mistake, contact the site owner and quote the ray ID below.", ray)
	writeHTML(w, http.StatusForbidden, html)
}

// Challenge renders the challenge interstitial.
//
// This is a demo-grade challenge, and it is important not to overstate it. It proves the client
// runs JavaScript and honours cookies — enough to turn away trivial scripted floods, which is
// what the v1 challenge action is for. It is not a CAPTCHA and not a proof-of-work, so it does
// not stop a determined attacker who reads this page once.
func Challenge(w http.ResponseWriter, clearanceCookie, ray string) {
	html := fmt.Sprintf(`<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Checking your browser</title>
<style>
  body { font: 15px/1.5 system-ui, sans-serif; background: #0b0d10; color: #e6e8eb;
         display: flex; min-height: 100vh; margin: 0; align-items: center; justify-content: center; }
  main { max-width: 34rem; padding: 2rem; text-align: center; }
  h1 { font-size: 1.35rem; margin: 0 0 .5rem; }
  p { color: #9aa3ad; }
  code { font-size: .8rem; color: #6f7883; }
</style></head>
<body><main>
  <h1>Checking your browser&hellip;</h1>
  <p>This takes a moment. You will be redirected automatically.</p>
  <code>Ray ID: %s</code>
  <script>setTimeout(function () { location.reload(); }, 1200);</script>
</main></body></html>`, ray)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Retry-After", "2")
	w.Header().Set("Set-Cookie", clearanceCookie)
	w.WriteHeader(http.StatusServiceUnavailable)
	_, _ = w.Write([]byte(html))
}

// UnknownHost renders the response for a Host this node has no rule-set for. Not a 404: the
// request never reached an origin, and saying "not found" would imply it did. 421 is the
// accurate answer — this server is not configured to serve that authority.
func UnknownHost(w http.ResponseWriter) {
	html := page("Not configured", "This hostname is not configured",
		"No protected zone matches the host in this request.", "-")
	writeHTML(w, http.StatusMisdirectedRequest, html)
}

// OriginUnreachable renders the response for when the origin could not be reached. Distinct
// from Blocked on purpose: a customer debugging their site needs to know whether the WAF stopped
// a request or their own server did not answer.
func OriginUnreachable(w http.ResponseWriter, ray string) {
	html := page("Origin unreachable", "The origin server did not respond",
		"The request passed this site's security rules, but the origin server could not be "+
			"reached.", ray)
	writeHTML(w, http.StatusBadGateway, html)
}
