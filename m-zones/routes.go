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
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/httpserver"
	"github.com/metap-org/metap/pkg/kit/env"
	"github.com/metap-org/metap/pkg/kit/errcode"
	"github.com/metap-org/metap/pkg/kit/standard"
	"github.com/metap-org/metap/pkg/permission"
	"github.com/metap-org/metap/pkg/query"
	"github.com/metap-org/waap/model"
)

func scanningURL() string {
	v, _ := env.Lookup("SCANNING_URL")
	return orDefault(v, "http://localhost:3010")
}
func alertingURL() string {
	v, _ := env.Lookup("ALERTING_URL")
	return orDefault(v, "http://localhost:3020")
}
func dohURL() string {
	v, _ := env.Lookup("DOH_RESOLVER_URL")
	return orDefault(v, "https://dns.google/resolve")
}
func edgeCNAMETarget() string {
	v, _ := env.Lookup("EDGE_CNAME_TARGET")
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
// probeTimeout bounds one DNS or origin probe.
const probeTimeout = 10 * time.Second

// existenceCheckLimit asks a list for one row: "is there any", not "how many".
const existenceCheckLimit = 1

// zoneCascadeLimit is how many zones one domain verification cascades its verified status onto.
const zoneCascadeLimit = 100

func httpClient() *http.Client {
	return &http.Client{
		Timeout:       probeTimeout,
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

// VerifyDNS is POST /api/waf.zones/{id}/verify-dns — the informational DNS routing check: is
// this zone's own hostname actually pointed at edge-plane yet? Never gates activation, purely
// informational.
func verifyDNS(ctx context.Context, state *httpserver.AppState, rc *permission.RequestContext, zoneID uuid.UUID) (map[string]any, error) {
	got, err := state.Crud.Get(ctx, model.Zones, zoneID, rc)
	if err != nil {
		return nil, err
	}
	record := got.Record

	hostname, _ := jsonAsString(record.Data[model.FieldHostname])
	if hostname == "" {
		return nil, crud.NewServiceErrorMsg(http.StatusBadRequest, errcode.ValidationFailed, "Zone has no hostname.")
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

	status := model.DNSNotRouted
	if routed {
		status = model.DNSRouted
	}
	patch := crud.JsonObject{
		model.FieldDNSRoutingStatus: jsonString(status),
		model.FieldLastDNSCheckAt:   jsonString(standard.NowRFC3339()),
	}
	updated, err := state.Crud.Update(ctx, model.Zones, zoneID, record.Version, patch, rc, nil)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"zone": updated, "dnsRouted": routed,
		"checked": map[string]any{"cname": cname, "expectedTarget": target},
	}, nil
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
func verifyDomainDNS(ctx context.Context, state *httpserver.AppState, rc *permission.RequestContext, domainID uuid.UUID, body verifyDNSBody) (map[string]any, error) {

	got, err := state.Crud.Get(ctx, model.Domains, domainID, rc)
	if err != nil {
		return nil, err
	}
	domain := got.Record

	apexDomain, _ := jsonAsString(domain.Data[model.FieldApexDomain])
	if apexDomain == "" {
		return nil, crud.NewServiceErrorMsg(http.StatusBadRequest, errcode.ValidationFailed, "Domain has no apexDomain.")
	}
	expectedToken := ""
	if body.ExpectedToken != nil {
		expectedToken = *body.ExpectedToken
	} else {
		expectedToken, _ = jsonAsString(domain.Data[model.FieldVerificationToken])
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
		return map[string]any{
			"domain": domain, "ownershipVerified": false,
			"checked": map[string]any{"txt": txt, "expectedToken": expectedToken},
		}, nil
	}

	patch := crud.JsonObject{model.FieldVerificationStatus: jsonString(model.VerificationVerified)}
	updatedDomain, err := state.Crud.Update(ctx, model.Domains, domainID, domain.Version, patch, rc, nil)
	if err != nil {
		return nil, err
	}

	// Cascade onto every zone under this domain — best-effort per zone (one failing zone must
	// not stop the others, and must not undo the domain's own now-verified status).
	listInput := query.ListInput{Limit: zoneCascadeLimit, Filters: [][2]string{{model.FieldDomainID, domainID.String()}}}
	zones, err := state.Crud.List(ctx, model.Zones, listInput, rc)
	if err == nil {
		for _, zone := range zones.Data {
			zonePatch := crud.JsonObject{model.FieldVerificationStatus: jsonString(model.VerificationVerified)}
			if _, err := state.Crud.Update(ctx, model.Zones, zone.ID, zone.Version, zonePatch, rc, nil); err != nil {
				slog.Error("domain verified but cascading verificationStatus onto this zone failed",
					"zone_id", zone.ID, "domain_id", domainID, standard.LogError, err)
			}
		}
	}

	return map[string]any{
		"domain": updatedDomain, "ownershipVerified": true,
		"checked": map[string]any{"txt": txt, "expectedToken": expectedToken},
	}, nil
}

// TestOrigin is POST /api/waf.zones/{id}/test-origin — "can we actually reach the origin the
// customer gave us", a one-shot connectivity check. Not stored on the zone.
func testOrigin(ctx context.Context, state *httpserver.AppState, rc *permission.RequestContext, zoneID uuid.UUID) (map[string]any, error) {
	got, err := state.Crud.Get(ctx, model.Zones, zoneID, rc)
	if err != nil {
		return nil, err
	}
	origin, _ := jsonAsString(got.Record.Data[model.FieldOriginAddress])
	if origin == "" {
		return nil, crud.NewServiceErrorMsg(http.StatusBadRequest, errcode.ValidationFailed, "Zone has no origin address.")
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
	return payload, nil
}

// SyncConfigState is POST /api/waf.zones/{id}/sync-config-state — recomputes hasConfig from the
// zone's actual DdosPolicy/FirewallRule records and writes it back. Idempotent, so calling it at
// any time (or twice) is harmless.
func syncConfigState(ctx context.Context, state *httpserver.AppState, rc *permission.RequestContext, zoneID uuid.UUID) (map[string]any, error) {

	anyForZone := func(entity string) (bool, error) {
		input := query.ListInput{Limit: existenceCheckLimit, Filters: [][2]string{{model.FieldZoneID, zoneID.String()}}}
		result, err := state.Crud.List(ctx, entity, input, rc)
		if err != nil {
			return false, err
		}
		return len(result.Data) > 0, nil
	}

	// A caller who cannot list rules cannot be told a zone has none either — surfacing "false"
	// here would silently flip hasConfig off on a permission error.
	hasDdos, err := anyForZone(model.DdosPolicies)
	if err != nil {
		return nil, err
	}
	hasFirewall, err := anyForZone(model.FirewallRules)
	if err != nil {
		return nil, err
	}
	hasConfig := hasDdos || hasFirewall

	got, err := state.Crud.Get(ctx, model.Zones, zoneID, rc)
	if err != nil {
		return nil, err
	}
	zone := got.Record

	currentHasConfig, _ := jsonAsBool(zone.Data[model.FieldHasConfig])
	if currentHasConfig == hasConfig {
		// Already correct — skip the write so this doesn't bump version/updatedAt on every call
		// and turn an idempotent sync into a source of version conflicts for the portal.
		return map[string]any{"hasConfig": hasConfig, "changed": false}, nil
	}

	patch := crud.JsonObject{model.FieldHasConfig: jsonBool(hasConfig)}
	_, err = state.Crud.Update(ctx, model.Zones, zoneID, zone.Version, patch, rc, nil)
	if err != nil {
		return nil, err
	}
	return map[string]any{"hasConfig": hasConfig, "changed": true}, nil
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

// MountRoutes registers this service's custom (non-generic-CRUD) endpoints onto r. REST stays
// available next to the declared actions (RegisterActions) for any caller that wants it.
func MountRoutes(r chi.Router, state *httpserver.AppState) {
	r.Post("/api/waf.zones/{id}/verify-dns", httpserver.RecordActionRoute(state, "zone", func(ctx context.Context, rc *permission.RequestContext, id uuid.UUID) (map[string]any, error) {
		return verifyDNS(ctx, state, rc, id)
	}))
	r.Post("/api/waf.domains/{id}/verify-dns", func(w http.ResponseWriter, req *http.Request) {
		rc, ok := httpserver.RequireAuth(w, req, state)
		if !ok {
			return
		}
		id, err := uuid.Parse(chi.URLParam(req, "id"))
		if err != nil {
			httpserver.WriteServiceError(w, http.StatusBadRequest, errcode.ValidationFailed, "Invalid domain id.", nil)
			return
		}
		var body verifyDNSBody
		if req.ContentLength != 0 && !httpserver.DecodeJSONBody(w, req, &body) {
			return
		}
		out, err := verifyDomainDNS(req.Context(), state, rc, id, body)
		if err != nil {
			httpserver.WriteCrudError(w, err)
			return
		}
		httpserver.WriteJSON(w, http.StatusOK, map[string]any{"data": out})
	})
	r.Post("/api/waf.zones/{id}/test-origin", httpserver.RecordActionRoute(state, "zone", func(ctx context.Context, rc *permission.RequestContext, id uuid.UUID) (map[string]any, error) {
		return testOrigin(ctx, state, rc, id)
	}))
	r.Post("/api/waf.zones/{id}/sync-config-state", httpserver.RecordActionRoute(state, "zone", func(ctx context.Context, rc *permission.RequestContext, id uuid.UUID) (map[string]any, error) {
		return syncConfigState(ctx, state, rc, id)
	}))
	r.Get("/internal/health/deep", DeepHealth)
}

// RegisterActions registers the handlers for the actions declared on this service's
// entities (see entities: Actions). Same cores as the REST routes above.
func RegisterActions(state *httpserver.AppState, reg *crud.ActionRegistry) error {
	for _, a := range []struct {
		entity, name string
		run          crud.ActionHandler
	}{
		{model.Zones, model.ActionVerifyDNS, func(ctx context.Context, c crud.ActionCall) (crud.JsonObject, error) {
			return crud.ToObject(verifyDNS(ctx, state, c.RC, c.ID))
		}},
		{model.Zones, model.ActionTestOrigin, func(ctx context.Context, c crud.ActionCall) (crud.JsonObject, error) {
			return crud.ToObject(testOrigin(ctx, state, c.RC, c.ID))
		}},
		{model.Zones, model.ActionSyncConfigState, func(ctx context.Context, c crud.ActionCall) (crud.JsonObject, error) {
			return crud.ToObject(syncConfigState(ctx, state, c.RC, c.ID))
		}},
		{model.Zones, model.ActionPlanUsage, func(ctx context.Context, c crud.ActionCall) (crud.JsonObject, error) {
			return crud.ToObject(tenantUsage(ctx, state.Crud, PlanClientFromEnv(), c.RC))
		}},
		{model.PolicyVersions, model.ActionRollback, func(ctx context.Context, c crud.ActionCall) (crud.JsonObject, error) {
			// state.Crud, not the guarded wrapper: the rollback records its own single version.
			result, err := rollbackToVersion(ctx, state.Crud, c.ID, c.RC)
			if err != nil {
				return nil, err
			}
			return crud.ToObject(result, nil)
		}},
		{model.Domains, model.ActionVerifyDNS, func(ctx context.Context, c crud.ActionCall) (crud.JsonObject, error) {
			var body verifyDNSBody
			if raw, ok := c.Input["expectedToken"]; ok {
				if tok, ok := jsonAsString(raw); ok {
					body.ExpectedToken = &tok
				}
			}
			return crud.ToObject(verifyDomainDNS(ctx, state, c.RC, c.ID, body))
		}},
	} {
		if err := reg.Register(a.entity, a.name, a.run); err != nil {
			return err
		}
	}
	return nil
}
