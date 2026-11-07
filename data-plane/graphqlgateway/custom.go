// 9 custom, non-CRUD mutations — real WAF business actions (DNS verification, scan dispatch,
// incident correlation, ...) that no generic entity field can synthesize. Every resolver here is
// a thin proxy: forward the caller's own bearer token to the exact REST endpoint that already
// implements the real logic (each WAF service's own src/routes.go), and hand back its JSON
// response verbatim as the JSON scalar. No business logic is duplicated here — mirrors the Rust
// gateway's add_custom_fields exactly for the first 8; cleanupSecurityEvents is new (added with
// alertingservice/retention.go, a v1 capability data-plane/docs/02-domain-model.md's 2026-08-30
// retention decision had named but never built in either language).
package graphqlgateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/graphql-go/graphql"
)

// RestURLs are the 3 upstreams' REST base URLs (e.g. http://localhost:3000) — used only by the
// custom mutations below; every generic CRUD field goes over gRPC instead.
type RestURLs struct {
	Zones, Scanning, Alerting string
}

func httpClientForForwarding() *http.Client {
	return &http.Client{Timeout: 15 * time.Second}
}

// forwardPost forwards the caller's own bearer token to a POST REST endpoint and returns its
// JSON body verbatim as a GraphQL value. A missing token means this gateway's own auth gate
// (server.go) somehow let an unauthenticated request through — not expected to happen, but a
// clear error beats a confusing downstream 401.
func forwardPost(ctx context.Context, url string, body interface{}) (interface{}, error) {
	rc := requestContextFrom(ctx)
	if rc.ForwardedBearerToken == "" {
		return nil, fmt.Errorf("no caller token to forward — this should be unreachable")
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+rc.ForwardedBearerToken)
	resp, err := httpClientForForwarding().Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling %s: %w", url, err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response from %s: %w", url, err)
	}
	// False positive below: this is a bespoke, schema-less forwarding gateway proxying an
	// upstream REST response of unknown shape (any of the 8 custom business-action mutations,
	// each a different backend service's own JSON) — there is no single concrete struct to
	// deserialize into by design, the same reason metap-go's own JsonObject (map[string]any)
	// exists. Only ever read back through type-switched field lookups below, never trusted.
	// The rule's match spans from this declaration through the Unmarshal call below, and
	// Semgrep anchors a multi-line match's suppression to its FIRST line, not the call site.
	var parsed interface{} // nosemgrep: go.lang.security.deserialization.unsafe-deserialization-interface.go-unsafe-deserialization-interface
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("parsing response from %s: %w", url, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message := "request failed"
		if obj, ok := parsed.(map[string]interface{}); ok {
			if errObj, ok := obj["error"].(map[string]interface{}); ok {
				if m, ok := errObj["message"].(string); ok {
					message = m
				}
			}
		}
		return nil, fmt.Errorf("%s", message)
	}
	return parsed, nil
}

func idArg(p graphql.ResolveParams, name string) (string, error) {
	s, ok := p.Args[name].(string)
	if !ok || s == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	return s, nil
}

func addCustomFields(mutation graphql.Fields, rest RestURLs) {
	mutation["verifyZoneDns"] = &graphql.Field{
		Type: jsonScalar,
		Args: graphql.FieldConfigArgument{"zoneId": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.ID)}},
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			zoneID, err := idArg(p, "zoneId")
			if err != nil {
				return nil, err
			}
			url := fmt.Sprintf("%s/api/waf.zones/%s/verify-dns", rest.Zones, zoneID)
			return forwardPost(p.Context, url, map[string]interface{}{})
		},
	}

	mutation["verifyDomainDns"] = &graphql.Field{
		Type: jsonScalar,
		Args: graphql.FieldConfigArgument{"domainId": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.ID)}},
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			domainID, err := idArg(p, "domainId")
			if err != nil {
				return nil, err
			}
			url := fmt.Sprintf("%s/api/waf.domains/%s/verify-dns", rest.Zones, domainID)
			return forwardPost(p.Context, url, map[string]interface{}{})
		},
	}

	mutation["testZoneOrigin"] = &graphql.Field{
		Type: jsonScalar,
		Args: graphql.FieldConfigArgument{"zoneId": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.ID)}},
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			zoneID, err := idArg(p, "zoneId")
			if err != nil {
				return nil, err
			}
			url := fmt.Sprintf("%s/api/waf.zones/%s/test-origin", rest.Zones, zoneID)
			return forwardPost(p.Context, url, map[string]interface{}{})
		},
	}

	mutation["syncZoneConfigState"] = &graphql.Field{
		Type: jsonScalar,
		Args: graphql.FieldConfigArgument{"zoneId": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.ID)}},
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			zoneID, err := idArg(p, "zoneId")
			if err != nil {
				return nil, err
			}
			url := fmt.Sprintf("%s/api/waf.zones/%s/sync-config-state", rest.Zones, zoneID)
			return forwardPost(p.Context, url, map[string]interface{}{})
		},
	}

	mutation["runScanJob"] = &graphql.Field{
		Type: jsonScalar,
		Args: graphql.FieldConfigArgument{"jobId": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.ID)}},
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			jobID, err := idArg(p, "jobId")
			if err != nil {
				return nil, err
			}
			url := fmt.Sprintf("%s/api/waf.scan_jobs/%s/run", rest.Scanning, jobID)
			return forwardPost(p.Context, url, map[string]interface{}{})
		},
	}

	mutation["testAlertPolicy"] = &graphql.Field{
		Type: jsonScalar,
		Args: graphql.FieldConfigArgument{"policyId": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.ID)}},
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			policyID, err := idArg(p, "policyId")
			if err != nil {
				return nil, err
			}
			url := fmt.Sprintf("%s/api/waf.alert_policies/%s/test", rest.Alerting, policyID)
			return forwardPost(p.Context, url, map[string]interface{}{})
		},
	}

	mutation["correlateIncidents"] = &graphql.Field{
		Type: jsonScalar,
		Args: graphql.FieldConfigArgument{"zoneId": &graphql.ArgumentConfig{Type: graphql.String}},
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			url := fmt.Sprintf("%s/internal/incidents/correlate", rest.Alerting)
			body := map[string]interface{}{}
			if zoneID, ok := p.Args["zoneId"].(string); ok && zoneID != "" {
				body["zoneId"] = zoneID
			}
			return forwardPost(p.Context, url, body)
		},
	}

	mutation["evaluateAlerts"] = &graphql.Field{
		Type: jsonScalar,
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			url := fmt.Sprintf("%s/internal/alerts/evaluate", rest.Alerting)
			return forwardPost(p.Context, url, map[string]interface{}{})
		},
	}

	mutation["cleanupSecurityEvents"] = &graphql.Field{
		Type: jsonScalar,
		Args: graphql.FieldConfigArgument{"retentionDays": &graphql.ArgumentConfig{Type: graphql.Int}},
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			url := fmt.Sprintf("%s/internal/security-events/cleanup", rest.Alerting)
			body := map[string]interface{}{}
			if retentionDays, ok := p.Args["retentionDays"].(int); ok && retentionDays > 0 {
				body["retentionDays"] = retentionDays
			}
			return forwardPost(p.Context, url, body)
		},
	}
}
