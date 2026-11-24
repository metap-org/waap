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

	"github.com/metap-org/metap/pkg/action"
	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/kit/errcode"
	"github.com/metap-org/metap/pkg/kit/standard"
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

// zoneHostname is what verifyDns reads of a zone.
type zoneHostname struct {
	Hostname string `json:"hostname"`
}

// ZoneDNSCheck is what verifyDns on a zone answers.
type ZoneDNSCheck struct {
	Zone      crud.RecordDto `json:"zone"`
	DNSRouted bool           `json:"dnsRouted"`
	Checked   map[string]any `json:"checked"`
}

// VerifyZone is the action waf.zones.verifyDns — the informational DNS routing check: is this
// zone's own hostname actually pointed at edge-plane yet? Never gates activation, purely
// informational; what it learns is kept on the zone.
func VerifyZone(cfg Config) action.Action {
	return action.Record(model.Zones, model.ActionVerifyDNS,
		func(ctx context.Context, a *action.Ctx, z zoneHostname, _ action.None) (ZoneDNSCheck, error) {
			if z.Hostname == "" {
				return ZoneDNSCheck{}, crud.NewServiceErrorMsg(http.StatusBadRequest, errcode.ValidationFailed, "Zone has no hostname.")
			}
			cname := cfg.dnsLookup(z.Hostname, "CNAME")
			routed := routesToEdge(cname, cfg.EdgeCNAMETarget)
			status := model.DNSNotRouted
			if routed {
				status = model.DNSRouted
			}
			patch := crud.JsonObject{
				model.FieldDNSRoutingStatus: jx.String(status),
				model.FieldLastDNSCheckAt:   jx.String(standard.NowRFC3339()),
			}
			updated, err := a.Records.Update(ctx, model.Zones, a.ID, a.Current.Version, patch, a.RC, nil)
			if err != nil {
				return ZoneDNSCheck{}, err
			}
			return ZoneDNSCheck{Zone: updated, DNSRouted: routed, Checked: map[string]any{"cname": cname, "expectedTarget": cfg.EdgeCNAMETarget}}, nil
		})
}

// routesToEdge reports whether one of the CNAME answers ends in the edge's target.
func routesToEdge(cname []string, target string) bool {
	for _, c := range cname {
		if hasSuffix(c, target) {
			return true
		}
	}
	return false
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

// domainFields is what verifyDns reads of the domain record.
type domainFields struct {
	ApexDomain        string `json:"apexDomain"`
	VerificationToken string `json:"verificationToken"`
}

// DomainVerification is what verifyDns on a domain answers.
type DomainVerification struct {
	Domain            crud.RecordDto `json:"domain"`
	OwnershipVerified bool           `json:"ownershipVerified"`
	Checked           map[string]any `json:"checked"`
}

// VerifyDomain is the action waf.domains.verifyDns — domain-ownership check, proven once per apex
// domain rather than once per Zone.
//
// The lookups (DNS, or a file over HTTP) happen first, outside any transaction. On a real match of
// the domain's own verificationToken this marks the Domain itself verified and cascades that onto
// every Zone under it — writing each zone's own verificationStatus mirror field directly — as one
// step, so the domain is never verified without its audit entry and its zones' mirrors.
//
// It reads and writes through the service's own backend, not the guard chain: the cascade writes a
// mirror field, which is not an edit a person makes.
func VerifyDomain(cfg Config) action.Action {
	return action.Record(model.Domains, model.ActionVerifyDNS,
		func(ctx context.Context, a *action.Ctx, d domainFields, in VerifyDNSBody) (DomainVerification, error) {
			if d.ApexDomain == "" {
				return DomainVerification{}, crud.NewServiceErrorMsg(http.StatusBadRequest, errcode.ValidationFailed, "Domain has no apexDomain.")
			}
			expectedToken := d.VerificationToken
			if in.ExpectedToken != nil {
				expectedToken = *in.ExpectedToken
			}
			method := in.Method
			if method == "" {
				method = model.VerificationMethodDNSTxt
			}
			proven, checked, err := cfg.checkOwnership(ctx, method, d.ApexDomain, in.Host, expectedToken)
			if err != nil {
				return DomainVerification{}, err
			}
			if !proven {
				return DomainVerification{Domain: a.Current, Checked: checked}, nil
			}

			var verified crud.RecordDto
			if err := a.Step(ctx, "verify", func(ctx context.Context) error {
				patch := crud.JsonObject{model.FieldVerificationStatus: jx.String(model.VerificationVerified), model.FieldVerificationMethod: jx.String(method)}
				updated, err := a.Records.Update(ctx, model.Domains, a.ID, a.Current.Version, patch, a.RC, nil)
				if err != nil {
					return err
				}
				verified = updated
				a.Audit("domain.ownership_verified", a.ID, map[string]string{"method": method, "apexDomain": d.ApexDomain})
				cascadeVerified(ctx, a)
				return nil
			}); err != nil {
				return DomainVerification{}, err
			}
			return DomainVerification{Domain: verified, OwnershipVerified: true, Checked: checked}, nil
		})
}

// cascadeVerified marks every zone under the domain verified — best-effort per zone: one failing
// zone must not stop the others, and must not undo the domain's own now-verified status.
func cascadeVerified(ctx context.Context, a *action.Ctx) {
	listInput := query.ListInput{Limit: zoneCascadeLimit, Filters: [][2]string{{model.FieldDomainID, a.ID.String()}}}
	zones, err := a.Records.List(ctx, model.Zones, listInput, a.RC)
	if err != nil {
		return
	}
	for _, zone := range zones.Data {
		patch := crud.JsonObject{model.FieldVerificationStatus: jx.String(model.VerificationVerified)}
		if _, err := a.Records.Update(ctx, model.Zones, zone.ID, zone.Version, patch, a.RC, nil); err != nil {
			slog.Error("domain verified but cascading verificationStatus onto this zone failed",
				"zone_id", zone.ID, "domain_id", a.ID, standard.LogError, err)
		}
	}
}

// checkOwnership asks the outside world whether the domain's owner placed expectedToken where only
// they could: a TXT record under _waf-verify, or a file served over HTTP. It reports what it
// looked at either way, and refuses a method it does not know.
func (c Config) checkOwnership(ctx context.Context, method, apexDomain, host, expectedToken string) (bool, map[string]any, error) {
	checked := map[string]any{"method": method, "expectedToken": expectedToken}
	switch method {
	case model.VerificationMethodHTTPFile:
		if expectedToken == "" {
			return false, checked, nil
		}
		found, detail, err := c.fetchVerificationFile(ctx, apexDomain, host, expectedToken)
		if err != nil {
			return false, nil, err
		}
		checked["file"] = detail
		return found, checked, nil
	case model.VerificationMethodDNSTxt:
		txt := c.dnsLookup("_waf-verify."+apexDomain, "TXT")
		checked["txt"] = txt
		return txtProves(txt, expectedToken), checked, nil
	default:
		return false, nil, crud.NewServiceErrorFields(http.StatusUnprocessableEntity, errcode.ValidationFailed,
			map[string][]string{"method": {"must be dnsTxt or httpFile"}})
	}
}

// txtProves reports whether one of the TXT answers is the expected token. An empty token proves nothing.
func txtProves(txt []string, expectedToken string) bool {
	if expectedToken == "" {
		return false
	}
	for _, t := range txt {
		if t == expectedToken {
			return true
		}
	}
	return false
}

// TestZoneOrigin is the action waf.zones.testOrigin — "can we actually reach the origin the
// customer gave us" with the zone's own origin settings. The result is also kept on the zone
// (originCheckedAt, originReachable) so onboarding can show how far a person got; a failed save of
// that does not hide the answer.
func TestZoneOrigin(cfg Config) action.Action {
	return action.Record(model.Zones, model.ActionTestOrigin,
		func(ctx context.Context, a *action.Ctx, _ action.None, _ action.None) (map[string]any, error) {
			spec := OriginSpecOf(a.Current.Data)
			if spec.Address == "" {
				return nil, crud.NewServiceErrorMsg(http.StatusBadRequest, errcode.ValidationFailed, "Zone has no origin address.")
			}
			if spec.HostHeader == "" {
				spec.HostHeader, _ = jx.AsString(a.Current.Data[model.FieldHostname])
			}
			payload := cfg.ProbeOrigin(ctx, spec)
			reachable, _ := payload["reachable"].(bool)
			patch := crud.JsonObject{model.FieldOriginCheckedAt: jx.String(standard.NowRFC3339()), model.FieldOriginReachable: jx.Bool(reachable)}
			if _, err := a.Records.Update(ctx, model.Zones, a.ID, a.Current.Version, patch, a.RC, nil); err != nil {
				slog.Warn("origin check done but not saved on the zone", "zone", a.ID, "error", err)
			}
			return payload, nil
		})
}
