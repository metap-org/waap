// Package cache is the edge's view of configuration: a hostname -> rule-set snapshot, refreshed
// from Redis.
//
// Three properties this is built around, in priority order:
//
//  1. A request never touches Redis. Lookups hit an in-memory map behind an atomic pointer swap;
//     the refresh happens on a timer in the background. A config store being slow or down must
//     never become request latency.
//  2. The last good snapshot survives everything. If Redis is unreachable, if a payload fails
//     to parse, if the schema is newer than this binary — the previous snapshot keeps serving.
//     A config-distribution problem must degrade to staleness, never to an outage or (worse) to
//     an unprotected zone.
//  3. Refresh is cheap. The ticker reads the version key per zone (a small integer) and only
//     re-fetches the rule-sets whose configVersion actually moved. That is what
//     Zone.configVersion exists for — the alternative, re-downloading every zone every few
//     seconds, scales with fleet size for no reason.
//
// The refresh interval is what the 10-30s config-propagation SLA in the architecture doc
// actually is, in code.
package cache

import (
	"context"
	"encoding/json"
	"log/slog"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/metap-org/waap/edge-plane/ruleset"
)

type snapshot struct {
	zones map[string]*ruleset.CompiledZone
	epoch uint64
}

func emptySnapshot() *snapshot {
	return &snapshot{zones: map[string]*ruleset.CompiledZone{}}
}

type RuleSetCache struct {
	client  *redis.Client
	current atomic.Pointer[snapshot]
}

func New(redisURL string) (*RuleSetCache, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, err
	}
	c := &RuleSetCache{client: redis.NewClient(opts)}
	c.current.Store(emptySnapshot())
	return c, nil
}

// ZoneFor is the hot-path lookup: exact hostname match, no allocation, no I/O.
//
// Unknown hostname -> (nil, false) -> the request is refused. An edge that passed traffic for a
// hostname it has no configuration for would be an open proxy.
func (c *RuleSetCache) ZoneFor(hostname string) (*ruleset.CompiledZone, bool) {
	zone, ok := c.current.Load().zones[hostname]
	return zone, ok
}

func (c *RuleSetCache) ZoneCount() int {
	return len(c.current.Load().zones)
}

func (c *RuleSetCache) Epoch() uint64 {
	return c.current.Load().epoch
}

// refreshOnce is one refresh pass. Returns the number of zones re-fetched, for the log line.
func (c *RuleSetCache) refreshOnce(ctx context.Context) (int, error) {
	hostnames, err := c.client.SMembers(ctx, ruleset.ZoneIndexKey).Result()
	if err != nil {
		return 0, err
	}
	epoch := uint64(0)
	if e, err := c.client.Get(ctx, ruleset.EpochKey).Uint64(); err == nil {
		epoch = e
	}
	previous := c.current.Load()

	zones := make(map[string]*ruleset.CompiledZone, len(hostnames))
	fetched := 0

	for _, hostname := range hostnames {
		versionStr, verErr := c.client.Get(ctx, ruleset.ZoneVersionKey(hostname)).Result()
		if verErr == nil {
			if version, err := strconv.ParseInt(versionStr, 10, 64); err == nil {
				if existing, ok := previous.zones[hostname]; ok && existing.ConfigVersion == version {
					// Unchanged — carry the parsed value forward rather than re-downloading and
					// re-parsing it. This is the case for almost every zone on almost every
					// tick.
					zones[hostname] = existing
					continue
				}
			}
		}

		raw, getErr := c.client.Get(ctx, ruleset.ZoneKey(hostname)).Result()
		if getErr != nil {
			// Indexed but no rule-set: the control-plane is mid-write, or mid-delete. Keep
			// whatever we already had for this hostname and let the next tick settle it.
			if existing, ok := previous.zones[hostname]; ok {
				zones[hostname] = existing
			}
			continue
		}

		var zone ruleset.CompiledZone
		if err := json.Unmarshal([]byte(raw), &zone); err != nil {
			slog.Error("failed to parse rule-set, keeping previous", "hostname", hostname, "error", err)
			if existing, ok := previous.zones[hostname]; ok {
				zones[hostname] = existing
			}
			continue
		}
		if zone.SchemaVersion > ruleset.SchemaVersion {
			// A newer control-plane wrote a contract this binary doesn't understand. Keeping
			// the old snapshot for this zone is the only safe answer: parsing it partially
			// could drop a rule the operator is relying on.
			slog.Error("rule-set schema is newer than this build — keeping the previous snapshot for this zone",
				"hostname", hostname, "schema_version", zone.SchemaVersion, "supported", ruleset.SchemaVersion)
			if existing, ok := previous.zones[hostname]; ok {
				zones[hostname] = existing
			}
			continue
		}
		fetched++
		zones[hostname] = &zone
	}

	// Hostnames that left the index are simply absent from the new map — that is how a deleted
	// or suspended zone stops being served, without a separate eviction path.
	c.current.Store(&snapshot{zones: zones, epoch: epoch})
	return fetched, nil
}

// LoadInitial loads once at startup, failing loudly. Unlike the refresh loop, this one is
// fatal: an edge node that starts with no configuration would answer every request with
// "unknown host", which looks exactly like a total outage to every zone it should be serving.
func (c *RuleSetCache) LoadInitial(ctx context.Context) error {
	fetched, err := c.refreshOnce(ctx)
	if err != nil {
		return err
	}
	slog.Info("initial rule-sets loaded", "zones", c.ZoneCount(), "fetched", fetched, "epoch", c.Epoch())
	return nil
}

// RunRefreshLoop runs refreshOnce on interval until shutdown is closed.
func (c *RuleSetCache) RunRefreshLoop(ctx context.Context, interval time.Duration, shutdown <-chan struct{}) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-shutdown:
			slog.Info("rule-set refresh loop shutting down")
			return
		case <-ticker.C:
			fetched, err := c.refreshOnce(ctx)
			if err != nil {
				// Never fatal, and deliberately not escalating: the previous snapshot is still
				// serving traffic correctly. Redis being down degrades config freshness, not
				// availability.
				slog.Warn("rule-set refresh failed, serving previous snapshot", "error", err)
				continue
			}
			if fetched > 0 {
				slog.Info("rule-sets refreshed", "zones", c.ZoneCount(), "fetched", fetched)
			}
		}
	}
}
