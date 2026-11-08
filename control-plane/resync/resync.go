// Package resync is the periodic full sweep — what actually guarantees the edge converges.
//
// RunOnce runs once at boot (so a cold start doesn't wait for the first change event before the
// edge has any config at all) and then on a timer via RunLoop. It is deliberately the SIMPLE
// path: read every zone, recompile, publish, and delete anything published that no longer
// belongs. No diffing against what was published before — republishing an unchanged zone writes
// the same bytes, and the cost of a wrong diff is a zone silently serving stale rules.
//
// This is also the only thing that removes a zone whose delete event went missing: the index in
// Redis is compared against the set of hostnames management-plane currently says should be served.
package resync

import (
	"context"
	"log/slog"
	"time"

	"github.com/metap-org/waap/control-plane/ports"

	"github.com/metap-org/waap/control-plane/compile"
)

func RunOnce(ctx context.Context, dp ports.ConfigSource, distributor ports.RuleSink) error {
	zones, err := dp.AllZones(ctx)
	if err != nil {
		return err
	}
	// Fetched once per sweep, not once per zone: this worker serves exactly one tenant (the
	// service account it logs in as), so "tenant-wide" rules are the same list for every zone
	// below. CompileZone doesn't care where rules came from — merging is purely this caller's
	// job (concatenate, then let CompileZone's own priority sort interleave them).
	tenantWideRules, err := dp.TenantWideRulesFor(ctx)
	if err != nil {
		return err
	}
	tenantWideAccessLists, err := dp.TenantWideIPAccessListsFor(ctx)
	if err != nil {
		return err
	}
	expected := make(map[string]struct{})

	for _, zone := range zones {
		hostname, ok := zone.Str("hostname")
		if !ok {
			continue
		}
		if !compile.Publishable(zone) {
			continue
		}
		// tenantId is not part of a record's own field bag — it is the tenant the service
		// account is scoped to, which for this worker is the tenant it logged into. Reading it
		// from the record would be wrong anyway: the API only ever returns this tenant's rows.
		tenantID, _ := zone.Str("tenantId")

		ddosPolicies, err := dp.DdosPoliciesFor(ctx, zone.ID)
		if err != nil {
			return err
		}
		rules, err := dp.RulesFor(ctx, zone.ID)
		if err != nil {
			return err
		}
		rules = append(rules, tenantWideRules...)
		accessLists, err := dp.IPAccessListsFor(ctx, zone.ID)
		if err != nil {
			return err
		}
		accessLists = append(accessLists, tenantWideAccessLists...)

		compiled, ok := compile.CompileZone(zone, tenantID, ddosPolicies, rules, accessLists)
		if !ok {
			continue
		}
		if err := distributor.Publish(ctx, compiled); err != nil {
			return err
		}
		expected[hostname] = struct{}{}
	}

	// Anything published that management-plane no longer considers servable: a deleted zone, one
	// suspended or paused back to pending, or a hostname that was renamed.
	published, err := distributor.PublishedHostnames(ctx)
	if err != nil {
		return err
	}
	for _, hostname := range published {
		if _, ok := expected[hostname]; !ok {
			slog.Info("resync found an orphaned rule-set, removing", "hostname", hostname)
			if err := distributor.Unpublish(ctx, hostname); err != nil {
				return err
			}
		}
	}

	epoch, err := distributor.BumpEpoch(ctx)
	if err != nil {
		return err
	}
	slog.Info("full resync complete", "zones", len(expected), "epoch", epoch)
	return nil
}

// RunLoop runs RunOnce on interval until shutdown is closed.
//
// A slow sweep (a large fleet, a sluggish management-plane) never makes this fire repeatedly in a
// burst to "catch up" — unlike tokio's default MissedTickBehavior::Burst, Go's time.Ticker
// always drops a missed tick (its channel is buffered to exactly 1), which is already the
// "Skip" behavior the Rust original had to opt into explicitly.
func RunLoop(ctx context.Context, dp ports.ConfigSource, distributor ports.RuleSink, interval time.Duration, shutdown <-chan struct{}) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-shutdown:
			slog.Info("resync loop shutting down")
			return
		case <-ticker.C:
			if err := RunOnce(ctx, dp, distributor); err != nil {
				// Never fatal: the next tick retries, and the incremental path is still
				// running. A resync failure degrades freshness, it does not stop the worker.
				slog.Error("resync failed", "error", err)
			}
		}
	}
}
