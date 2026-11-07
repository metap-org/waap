// Custom (non-generic-CRUD) HTTP surface for zones-service.
//
//   - verify-dns / test-origin — call out (DNS resolver, the customer's own origin), which no CRUD
//     route does.
//   - sync-config-state — recomputes Zone.hasConfig by counting the zone's related DdosPolicy/
//     FirewallRule records. PolicyCondition (the grammar a workflow guard is written in) has no
//     count operator over related records, which is exactly why zone.go documents hasConfig as a
//     technical field "the app layer flips" — this is that app layer.
//
// All of it is mounted by main.go as app.Options.Routes (an httpserver.Mount), so it gets the
// same CORS/rate-limit/tracing/security-header treatment as every core route.
//
// The 4 create/update/delete guards that used to live here (zoneDomainGuard/
// firewallRuleMatchConditionGuard/ipAccessListValueGuard/zoneDeleteGuard) live in
// guardedbackend.go instead — see that file's doc comment.
package zonesservice

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/httpserver"
	"github.com/metap-org/metap/pkg/query"
	"github.com/metap-org/metap/pkg/runtime"
)

func scanningURL() string {
	v, _ := runtime.Optional("SCANNING_URL")
	return orDefault(v, "http://localhost:3010")
}
func alertingURL() string {
	v, _ := runtime.Optional("ALERTING_URL")
	return orDefault(v, "http://localhost:3020")
}
func dohURL() string {
	v, _ := runtime.Optional("DOH_RESOLVER_URL")
	return orDefault(v, "https://dns.google/resolve")
}
func edgeCNAMETarget() string {
	v, _ := runtime.Optional("EDGE_CNAME_TARGET")
	return orDefault(v, "edge.waf.local")
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// httpClient: an origin under test is attacker-adjacent input (a customer types the address) and
// a redirect chain is not what "is this origin reachable" is asking about, so redirects are
// disabled on purpose.
func httpClient() *http.Client {
	return &http.Client{
		Timeout:       10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// dnsLookup answers one DoH question, returning every answer record's data string.
func dnsLookup(client *http.Client, name, recordType string) []string {
	u, err := url.Parse(dohURL())
	if err != nil {
		return nil
	}
	q := u.Query()
	q.Set("name", name)
	q.Set("type", recordType)
	u.RawQuery = q.Encode()
	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return nil
	}
	req.Header.Set("accept", "application/dns-json")
	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	var body struct {
		Answer []struct {
			Data string `json:"data"`
		} `json:"Answer"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil
	}
	out := make([]string, 0, len(body.Answer))
	for _, a := range body.Answer {
		// DoH returns TXT strings wrapped in quotes; a CNAME comes back with a trailing dot.
		// Normalising both here keeps the comparison below about content.
		d := trimOneLayer(a.Data, '"')
		for len(d) > 0 && d[len(d)-1] == '.' {
			d = d[:len(d)-1]
		}
		out = append(out, d)
	}
	return out
}

func trimOneLayer(s string, c byte) string {
	for len(s) > 0 && s[0] == c {
		s = s[1:]
	}
	for len(s) > 0 && s[len(s)-1] == c {
		s = s[:len(s)-1]
	}
	return s
}

func writeCrudError(w http.ResponseWriter, status uint16, errorCode string, message *string, fieldErrors map[string][]string) {
	msg := ""
	if message != nil {
		msg = *message
	}
	httpserver.WriteServiceError(w, int(status), errorCode, msg, fieldErrors)
}

// VerifyDNS is POST /api/waf.zones/{id}/verify-dns — the informational DNS routing check: is
// this zone's own hostname actually pointed at edge-plane yet? Never gates activation, purely
// informational.
func VerifyDNS(w http.ResponseWriter, r *http.Request, state *httpserver.AppState) {
	rc, ok := httpserver.RequireAuth(w, r, state)
	if !ok {
		return
	}
	zoneID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpserver.WriteServiceError(w, http.StatusBadRequest, "validation_failed", "Invalid zone id.", nil)
		return
	}
	got, err := state.Crud.Get(r.Context(), "waf.zones", zoneID, rc)
	if err != nil {
		httpserver.WriteInternalError(w, err)
		return
	}
	if !got.IsOk {
		writeCrudError(w, got.Status, got.ErrorCode, got.Message, got.FieldErrors)
		return
	}
	record := got.Data.Record

	hostname, _ := jsonAsString(record.Data["hostname"])
	if hostname == "" {
		httpserver.WriteServiceError(w, http.StatusBadRequest, "validation_failed", "Zone has no hostname.", nil)
		return
	}

	client := httpClient()
	cname := dnsLookup(client, hostname, "CNAME")
	target := edgeCNAMETarget()
	routed := false
	for _, c := range cname {
		if hasSuffix(c, target) {
			routed = true
			break
		}
	}

	status := "notRouted"
	if routed {
		status = "routed"
	}
	patch := crud.JsonObject{
		"dnsRoutingStatus": jsonString(status),
		"lastDnsCheckAt":   jsonString(time.Now().UTC().Format(time.RFC3339)),
	}
	updated, err := state.Crud.Update(r.Context(), "waf.zones", zoneID, record.Version, patch, rc, nil)
	if err != nil {
		httpserver.WriteInternalError(w, err)
		return
	}
	if !updated.IsOk {
		writeCrudError(w, updated.Status, updated.ErrorCode, updated.Message, updated.FieldErrors)
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"zone": updated.Data, "dnsRouted": routed,
		"checked": map[string]any{"cname": cname, "expectedTarget": target},
	}})
}

func hasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

type verifyDNSBody struct {
	// ExpectedToken is an optional override for local/manual testing — normally the check reads
	// the domain's own verificationToken and never trusts a caller-supplied value.
	ExpectedToken *string `json:"expectedToken"`
}

// VerifyDomainDNS is POST /api/waf.domains/{id}/verify-dns — domain-ownership check, proven once
// per apex domain rather than once per Zone.
//
// On a real match of the domain's own verificationToken in a _waf-verify.<apexDomain> TXT
// record, this both marks the Domain itself verified and cascades that onto every Zone under it
// — writing each zone's own verificationStatus mirror field directly.
func VerifyDomainDNS(w http.ResponseWriter, r *http.Request, state *httpserver.AppState) {
	rc, ok := httpserver.RequireAuth(w, r, state)
	if !ok {
		return
	}
	domainID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpserver.WriteServiceError(w, http.StatusBadRequest, "validation_failed", "Invalid domain id.", nil)
		return
	}
	var body verifyDNSBody
	if r.ContentLength != 0 {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}

	got, err := state.Crud.Get(r.Context(), "waf.domains", domainID, rc)
	if err != nil {
		httpserver.WriteInternalError(w, err)
		return
	}
	if !got.IsOk {
		writeCrudError(w, got.Status, got.ErrorCode, got.Message, got.FieldErrors)
		return
	}
	domain := got.Data.Record

	apexDomain, _ := jsonAsString(domain.Data["apexDomain"])
	if apexDomain == "" {
		httpserver.WriteServiceError(w, http.StatusBadRequest, "validation_failed", "Domain has no apexDomain.", nil)
		return
	}
	expectedToken := ""
	if body.ExpectedToken != nil {
		expectedToken = *body.ExpectedToken
	} else {
		expectedToken, _ = jsonAsString(domain.Data["verificationToken"])
	}

	client := httpClient()
	txt := dnsLookup(client, "_waf-verify."+apexDomain, "TXT")
	ownershipOK := expectedToken != ""
	if ownershipOK {
		found := false
		for _, t := range txt {
			if t == expectedToken {
				found = true
				break
			}
		}
		ownershipOK = found
	}

	if !ownershipOK {
		httpserver.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
			"domain": domain, "ownershipVerified": false,
			"checked": map[string]any{"txt": txt, "expectedToken": expectedToken},
		}})
		return
	}

	patch := crud.JsonObject{"verificationStatus": jsonString("verified")}
	updatedDomain, err := state.Crud.Update(r.Context(), "waf.domains", domainID, domain.Version, patch, rc, nil)
	if err != nil {
		httpserver.WriteInternalError(w, err)
		return
	}
	if !updatedDomain.IsOk {
		writeCrudError(w, updatedDomain.Status, updatedDomain.ErrorCode, updatedDomain.Message, updatedDomain.FieldErrors)
		return
	}

	// Cascade onto every zone under this domain — best-effort per zone (one failing zone must
	// not stop the others, and must not undo the domain's own now-verified status).
	listInput := query.ListInput{Limit: 100, Filters: [][2]string{{"domainId", domainID.String()}}}
	zones, err := state.Crud.List(r.Context(), "waf.zones", listInput, rc)
	if err == nil && zones.IsOk {
		for _, zone := range zones.Data {
			zonePatch := crud.JsonObject{"verificationStatus": jsonString("verified")}
			if _, err := state.Crud.Update(r.Context(), "waf.zones", zone.ID, zone.Version, zonePatch, rc, nil); err != nil {
				slog.Error("domain verified but cascading verificationStatus onto this zone failed",
					"zone_id", zone.ID, "domain_id", domainID, "error", err)
			}
		}
	}

	httpserver.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"domain": updatedDomain.Data, "ownershipVerified": true,
		"checked": map[string]any{"txt": txt, "expectedToken": expectedToken},
	}})
}

// TestOrigin is POST /api/waf.zones/{id}/test-origin — "can we actually reach the origin the
// customer gave us", a one-shot connectivity check. Not stored on the zone.
func TestOrigin(w http.ResponseWriter, r *http.Request, state *httpserver.AppState) {
	rc, ok := httpserver.RequireAuth(w, r, state)
	if !ok {
		return
	}
	zoneID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpserver.WriteServiceError(w, http.StatusBadRequest, "validation_failed", "Invalid zone id.", nil)
		return
	}
	got, err := state.Crud.Get(r.Context(), "waf.zones", zoneID, rc)
	if err != nil {
		httpserver.WriteInternalError(w, err)
		return
	}
	if !got.IsOk {
		writeCrudError(w, got.Status, got.ErrorCode, got.Message, got.FieldErrors)
		return
	}
	origin, _ := jsonAsString(got.Data.Record.Data["originAddress"])
	if origin == "" {
		httpserver.WriteServiceError(w, http.StatusBadRequest, "validation_failed", "Zone has no origin address.", nil)
		return
	}
	// A customer types "1.2.3.4" or "origin.example.com" as often as a full URL.
	targetURL := origin
	if len(origin) < 7 || (origin[:7] != "http://" && (len(origin) < 8 || origin[:8] != "https://")) {
		targetURL = "https://" + origin
	}

	started := time.Now()
	resp, err := httpClient().Get(targetURL)
	elapsedMs := time.Since(started).Milliseconds()

	var payload map[string]any
	if err != nil {
		payload = map[string]any{"reachable": false, "error": err.Error(), "latencyMs": elapsedMs, "url": targetURL}
	} else {
		defer resp.Body.Close()
		payload = map[string]any{"reachable": true, "status": resp.StatusCode, "latencyMs": elapsedMs, "url": targetURL}
	}
	httpserver.WriteJSON(w, http.StatusOK, map[string]any{"data": payload})
}

// SyncConfigState is POST /api/waf.zones/{id}/sync-config-state — recomputes hasConfig from the
// zone's actual DdosPolicy/FirewallRule records and writes it back. Idempotent, so calling it at
// any time (or twice) is harmless.
func SyncConfigState(w http.ResponseWriter, r *http.Request, state *httpserver.AppState) {
	rc, ok := httpserver.RequireAuth(w, r, state)
	if !ok {
		return
	}
	zoneID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpserver.WriteServiceError(w, http.StatusBadRequest, "validation_failed", "Invalid zone id.", nil)
		return
	}

	anyForZone := func(entity string) (bool, *crud.ServiceResult[[]crud.RecordDto], error) {
		input := query.ListInput{Limit: 1, Filters: [][2]string{{"zoneId", zoneID.String()}}}
		result, err := state.Crud.List(r.Context(), entity, input, rc)
		if err != nil {
			return false, nil, err
		}
		if !result.IsOk {
			return false, &result, nil
		}
		return len(result.Data) > 0, nil, nil
	}

	// A caller who cannot list rules cannot be told a zone has none either — surfacing "false"
	// here would silently flip hasConfig off on a permission error.
	hasDdos, errA, err := anyForZone("waf.ddos_policies")
	if err != nil {
		httpserver.WriteInternalError(w, err)
		return
	}
	if errA != nil {
		writeCrudError(w, errA.Status, errA.ErrorCode, errA.Message, errA.FieldErrors)
		return
	}
	hasFirewall, errB, err := anyForZone("waf.firewall_rules")
	if err != nil {
		httpserver.WriteInternalError(w, err)
		return
	}
	if errB != nil {
		writeCrudError(w, errB.Status, errB.ErrorCode, errB.Message, errB.FieldErrors)
		return
	}
	hasConfig := hasDdos || hasFirewall

	got, err := state.Crud.Get(r.Context(), "waf.zones", zoneID, rc)
	if err != nil {
		httpserver.WriteInternalError(w, err)
		return
	}
	if !got.IsOk {
		writeCrudError(w, got.Status, got.ErrorCode, got.Message, got.FieldErrors)
		return
	}
	zone := got.Data.Record

	currentHasConfig, _ := jsonAsBool(zone.Data["hasConfig"])
	if currentHasConfig == hasConfig {
		// Already correct — skip the write so this doesn't bump version/updatedAt on every call
		// and turn an idempotent sync into a source of version conflicts for the portal.
		httpserver.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"hasConfig": hasConfig, "changed": false}})
		return
	}

	patch := crud.JsonObject{"hasConfig": jsonBool(hasConfig)}
	updated, err := state.Crud.Update(r.Context(), "waf.zones", zoneID, zone.Version, patch, rc, nil)
	if err != nil {
		httpserver.WriteInternalError(w, err)
		return
	}
	if !updated.IsOk {
		writeCrudError(w, updated.Status, updated.ErrorCode, updated.Message, updated.FieldErrors)
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"hasConfig": hasConfig, "changed": true}})
}

// DeepHealth is GET /internal/health/deep — liveness of this service plus the two siblings it
// now depends on for the delete guard. Plain /health stays what a load balancer polls; this is
// for an operator asking why a zone delete just returned 503.
func DeepHealth(w http.ResponseWriter, r *http.Request) {
	client := httpClient()
	checks := map[string]any{}
	for _, upstream := range []struct{ name, base string }{
		{"scanning", scanningURL()}, {"alerting", alertingURL()},
	} {
		ok := false
		if resp, err := client.Get(upstream.base + "/health"); err == nil {
			ok = resp.StatusCode >= 200 && resp.StatusCode < 300
			resp.Body.Close()
		}
		checks[upstream.name] = map[string]any{"reachable": ok, "url": upstream.base}
	}
	httpserver.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"self": "ok", "upstreams": checks}})
}

func jsonAsBool(raw json.RawMessage) (bool, bool) {
	if len(raw) == 0 {
		return false, false
	}
	var b bool
	if err := json.Unmarshal(raw, &b); err != nil {
		return false, false
	}
	return b, true
}

func jsonBool(b bool) json.RawMessage {
	out, _ := json.Marshal(b)
	return out
}

// MountRoutes registers this service's custom (non-generic-CRUD) endpoints onto r.
func MountRoutes(r chi.Router, state *httpserver.AppState) {
	wrap := func(h func(http.ResponseWriter, *http.Request, *httpserver.AppState)) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) { h(w, r, state) }
	}
	r.Post("/api/waf.zones/{id}/verify-dns", wrap(VerifyDNS))
	r.Post("/api/waf.domains/{id}/verify-dns", wrap(VerifyDomainDNS))
	r.Post("/api/waf.zones/{id}/test-origin", wrap(TestOrigin))
	r.Post("/api/waf.zones/{id}/sync-config-state", wrap(SyncConfigState))
	r.Get("/internal/health/deep", DeepHealth)
}
