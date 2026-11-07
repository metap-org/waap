// Package sync turns a change into a published rule-set — the shared middle both entry points
// use.
//
// Two paths reach here and they must not drift, which is why they share one function:
//   - subscribe (incremental): an outbox event says something about zone X changed.
//   - resync (periodic): sweep every zone, whatever has or hasn't been heard about.
//
// The incremental path is an OPTIMISATION. Convergence is guaranteed by the sweep, because an
// event can always be missed: a message dead-letters, a replica dies mid-handle, someone edits
// the database directly. Anything that only worked because an event arrived would be a latent
// outage.
package sync

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/metap-org/waap/control-plane/ports"

	"github.com/metap-org/waap/control-plane/compile"
)

// Zone recompiles and republishes one zone by id, or unpublishes it if it has gone away or is no
// longer in a servable state. Idempotent — running it twice against unchanged input writes the
// same bytes, which is what makes the periodic sweep safe to run every minute.
//
// knownHostname is the hostname this zone was last published under, when the caller knows it. It
// matters for the delete case: once the record is gone there is nothing left to read a hostname
// from, so without it the stale rule-set would sit in Redis — still protecting (or still
// exposing) that hostname — until the next full resync noticed.
func Zone(ctx context.Context, dp ports.ConfigSource, distributor ports.RuleSink, zoneID, tenantID string, knownHostname *string) error {
	zone, err := dp.Zone(ctx, zoneID)
	if err != nil {
		return err
	}
	if zone == nil {
		if knownHostname != nil {
			return distributor.Unpublish(ctx, *knownHostname)
		}
		slog.Debug("zone deleted and no known hostname — leaving it to the next resync", "zone_id", zoneID)
		return nil
	}

	hostname, hasHostname := zone.Str("hostname")
	if !compile.Publishable(*zone) {
		if hasHostname {
			return distributor.Unpublish(ctx, hostname)
		}
		if knownHostname != nil {
			return distributor.Unpublish(ctx, *knownHostname)
		}
		return nil
	}

	ddosPolicies, err := dp.DdosPoliciesFor(ctx, zoneID)
	if err != nil {
		return err
	}
	rules, err := dp.RulesFor(ctx, zoneID)
	if err != nil {
		return err
	}
	accessLists, err := dp.IPAccessListsFor(ctx, zoneID)
	if err != nil {
		return err
	}
	compiled, ok := compile.CompileZone(*zone, tenantID, ddosPolicies, rules, accessLists)
	if !ok {
		slog.Warn("zone could not be compiled (missing hostname?), skipping", "zone_id", zoneID)
		return nil
	}

	// A hostname rename leaves the rule-set under the old key, which would keep the old hostname
	// protected by a config nobody can see in the portal any more.
	if knownHostname != nil && *knownHostname != compiled.Hostname {
		if err := distributor.Unpublish(ctx, *knownHostname); err != nil {
			return err
		}
	}

	return distributor.Publish(ctx, compiled)
}

// ZoneIDFromEvent resolves which zone a change event is about. A waf.zones event names the zone
// directly; a policy/rule/access-list event names its own record, whose zoneId is the zone to
// recompile. Returns ("", false) for a waf.firewall_rules or waf.ip_access_lists event whose
// zoneId is JSON null too — a tenant-wide ("global") create/update/delete has no single zone to
// resolve to, so the caller falls through to its existing "let the next resync pick it up" path
// for exactly the same reason it does for a delete event. This is an accepted v1 trade-off, not
// an oversight: a tenant-wide change takes effect within one resync interval instead of
// immediately, rather than building a "fan this event out to every zone in the tenant" path for
// the incremental route.
func ZoneIDFromEvent(entity string, payload json.RawMessage) (string, bool) {
	var envelope struct {
		RecordID *string                    `json:"recordId"`
		Data     map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return "", false
	}
	switch entity {
	case "waf.zones":
		if envelope.RecordID != nil {
			return *envelope.RecordID, true
		}
		return "", false
	case "waf.ddos_policies", "waf.firewall_rules", "waf.ip_access_lists":
		raw, ok := envelope.Data["zoneId"]
		if !ok {
			return "", false
		}
		var zoneID string
		if err := json.Unmarshal(raw, &zoneID); err != nil {
			return "", false
		}
		return zoneID, true
	default:
		return "", false
	}
}

// HostnameFromEvent returns the hostname carried in a waf.zones event's payload, when there is
// one. A delete event has no data at all, which is exactly why Zone also accepts the hostname
// from the caller's own index.
func HostnameFromEvent(payload json.RawMessage) (string, bool) {
	var envelope struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return "", false
	}
	raw, ok := envelope.Data["hostname"]
	if !ok {
		return "", false
	}
	var hostname string
	if err := json.Unmarshal(raw, &hostname); err != nil {
		return "", false
	}
	return hostname, true
}
