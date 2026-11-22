// Layer: APP · outbound call. DNS routing, domain ownership and the origin check: what needs the network.

// Package netcheck is everything the zones domain asks the outside world: the DNS a customer set
// (verifyDns, zone and domain), whether their origin answers (TestOrigin, ProbeOrigin) and the
// hostname lookup of onboarding. Every address it dials goes through outbound.go, which refuses
// private ones unless the dev flag is set.
package netcheck

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"
	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/httpserver"
	"github.com/metap-org/metap/pkg/kit/errcode"
	"github.com/metap-org/metap/pkg/kit/standard"
	"github.com/metap-org/metap/pkg/permission"
	"github.com/metap-org/metap/pkg/query"
	"github.com/metap-org/waap/m-management/zones/internal/jx"
	"github.com/metap-org/waap/model"
)

// probeTimeout bounds one DNS or origin probe.
const probeTimeout = 10 * time.Second

// zoneCascadeLimit is how many zones one domain verification cascades its verified status onto.
const zoneCascadeLimit = 100

// dnsLookup answers one DoH question, returning every answer record's data string.
func (c Config) dnsLookup(name, recordType string) []string {
	u, err := url.Parse(c.DoHURL)
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
	resp, err := c.httpClient().Do(req)
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

// VerifyDNS is the action waf.zones.verifyDns — the informational DNS routing check: is
// this zone's own hostname actually pointed at edge-plane yet? Never gates activation, purely
// informational.
func VerifyDNS(ctx context.Context, cfg Config, state *httpserver.AppState, rc *permission.RequestContext, zoneID uuid.UUID) (map[string]any, error) {
	got, err := state.Crud.Get(ctx, model.Zones, zoneID, rc)
	if err != nil {
		return nil, err
	}
	record := got.Record

	hostname, _ := jx.AsString(record.Data[model.FieldHostname])
	if hostname == "" {
		return nil, crud.NewServiceErrorMsg(http.StatusBadRequest, errcode.ValidationFailed, "Zone has no hostname.")
	}

	cname := cfg.dnsLookup(hostname, "CNAME")
	target := cfg.EdgeCNAMETarget
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
		model.FieldDNSRoutingStatus: jx.String(status),
		model.FieldLastDNSCheckAt:   jx.String(standard.NowRFC3339()),
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

type VerifyDNSBody struct {
	// Method is "dnsTxt" (the default) or "httpFile" (UC-21); Host is where the file is fetched
	// from for httpFile (the domain itself, or a name under it).
	Method string `json:"method"`
	Host   string `json:"host"`
	// ExpectedToken is an optional override for local/manual testing — normally the check reads
	// the domain's own verificationToken and never trusts a caller-supplied value.
	ExpectedToken *string `json:"expectedToken"`
}

// VerifyDomainDNS is the action waf.domains.verifyDns — domain-ownership check, proven once
// per apex domain rather than once per Zone.
//
// On a real match of the domain's own verificationToken in a _waf-verify.<apexDomain> TXT
// record, this both marks the Domain itself verified and cascades that onto every Zone under it
// — writing each zone's own verificationStatus mirror field directly.
func VerifyDomainDNS(ctx context.Context, cfg Config, state *httpserver.AppState, rc *permission.RequestContext, domainID uuid.UUID, body VerifyDNSBody) (map[string]any, error) {

	got, err := state.Crud.Get(ctx, model.Domains, domainID, rc)
	if err != nil {
		return nil, err
	}
	domain := got.Record

	apexDomain, _ := jx.AsString(domain.Data[model.FieldApexDomain])
	if apexDomain == "" {
		return nil, crud.NewServiceErrorMsg(http.StatusBadRequest, errcode.ValidationFailed, "Domain has no apexDomain.")
	}
	expectedToken := ""
	if body.ExpectedToken != nil {
		expectedToken = *body.ExpectedToken
	} else {
		expectedToken, _ = jx.AsString(domain.Data[model.FieldVerificationToken])
	}

	method := body.Method
	if method == "" {
		method = model.VerificationMethodDNSTxt
	}
	var txt []string
	ownershipOK := false
	checked := map[string]any{"method": method, "expectedToken": expectedToken}
	switch method {
	case model.VerificationMethodHTTPFile:
		if expectedToken != "" {
			found, detail, err := cfg.fetchVerificationFile(ctx, apexDomain, body.Host, expectedToken)
			if err != nil {
				return nil, err
			}
			ownershipOK = found
			checked["file"] = detail
		}
	case model.VerificationMethodDNSTxt:
		txt = cfg.dnsLookup("_waf-verify."+apexDomain, "TXT")
		checked["txt"] = txt
		for _, t := range txt {
			if expectedToken != "" && t == expectedToken {
				ownershipOK = true
			}
		}
	default:
		return nil, crud.NewServiceErrorFields(http.StatusUnprocessableEntity, errcode.ValidationFailed,
			map[string][]string{"method": {"must be dnsTxt or httpFile"}})
	}

	if !ownershipOK {
		return map[string]any{"domain": domain, "ownershipVerified": false, "checked": checked}, nil
	}

	patch := crud.JsonObject{model.FieldVerificationStatus: jx.String(model.VerificationVerified), model.FieldVerificationMethod: jx.String(method)}
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
			zonePatch := crud.JsonObject{model.FieldVerificationStatus: jx.String(model.VerificationVerified)}
			if _, err := state.Crud.Update(ctx, model.Zones, zone.ID, zone.Version, zonePatch, rc, nil); err != nil {
				slog.Error("domain verified but cascading verificationStatus onto this zone failed",
					"zone_id", zone.ID, "domain_id", domainID, standard.LogError, err)
			}
		}
	}

	return map[string]any{"domain": updatedDomain, "ownershipVerified": true, "checked": checked}, nil
}

// TestOrigin is the action waf.zones.TestOrigin — "can we actually reach the origin the
// customer gave us" with the zone's own origin settings. The result is also kept on the zone
// (originCheckedAt, originReachable) so onboarding can show how far a person got; a failed save of
// that does not hide the answer.
func TestOrigin(ctx context.Context, cfg Config, state *httpserver.AppState, rc *permission.RequestContext, zoneID uuid.UUID) (map[string]any, error) {
	got, err := state.Crud.Get(ctx, model.Zones, zoneID, rc)
	if err != nil {
		return nil, err
	}
	spec := OriginSpecOf(got.Record.Data)
	if spec.Address == "" {
		return nil, crud.NewServiceErrorMsg(http.StatusBadRequest, errcode.ValidationFailed, "Zone has no origin address.")
	}
	if spec.HostHeader == "" {
		spec.HostHeader, _ = jx.AsString(got.Record.Data[model.FieldHostname])
	}
	payload := cfg.ProbeOrigin(ctx, spec)
	reachable, _ := payload["reachable"].(bool)
	patch := crud.JsonObject{model.FieldOriginCheckedAt: jx.String(standard.NowRFC3339()), model.FieldOriginReachable: jx.Bool(reachable)}
	if _, err := state.Crud.Update(ctx, model.Zones, zoneID, got.Record.Version, patch, rc, nil); err != nil {
		slog.Warn("origin check done but not saved on the zone", "zone", zoneID, "error", err)
	}
	return payload, nil
}
