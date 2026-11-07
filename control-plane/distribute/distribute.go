// Package distribute is the "distribute" half: writing compiled rule-sets into Redis/DragonflyDB
// for the edge to read.
//
// Why a plain Redis client and not metap-cache's RedisCache: that abstraction is a CACHE — one
// fixed TTL per instance and keys scoped by the cache's own convention. Neither fits here. This
// data is not a cache: if it expires, the edge loses its configuration and a zone silently stops
// being protected; there is no origin to re-read it from on the hot path. And the key layout is a
// PUBLISHED CONTRACT consumed by a process that deliberately does not depend on metap
// (package ruleset) — it cannot be an internal detail of a metap package. Tenancy is still in the
// payload, and hostnames are globally unique in data-plane (Zone.hostname is unique), so keying
// by hostname is safe.
//
// Writes are pipelined per zone so a reader never sees the index updated before the rule-set it
// points at.
package distribute

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/redis/go-redis/v9"

	"github.com/metap-org/waap/control-plane/ruleset"
)

type Distributor struct {
	client *redis.Client
}

func Connect(redisURL string) (*Distributor, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("opening redis at %s: %w", redisURL, err)
	}
	return &Distributor{client: redis.NewClient(opts)}, nil
}

// Publish writes one zone. Rule-set and version are written before the index gains the
// hostname, so an edge that reads the index mid-write always finds a complete rule-set behind
// it.
func (d *Distributor) Publish(ctx context.Context, zone ruleset.CompiledZone) error {
	payload, err := json.Marshal(zone)
	if err != nil {
		return fmt.Errorf("serializing compiled zone: %w", err)
	}
	pipe := d.client.TxPipeline()
	pipe.Set(ctx, ruleset.ZoneKey(zone.Hostname), payload, 0)
	pipe.Set(ctx, ruleset.ZoneVersionKey(zone.Hostname), zone.ConfigVersion, 0)
	pipe.SAdd(ctx, ruleset.ZoneIndexKey, zone.Hostname)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("writing compiled zone to redis: %w", err)
	}
	return nil
}

// Unpublish removes a zone the edge must stop serving (deleted, suspended, or back to pending).
// Index membership goes first here — the mirror of Publish — so a reader never finds a hostname
// in the index whose rule-set has already been deleted.
func (d *Distributor) Unpublish(ctx context.Context, hostname string) error {
	pipe := d.client.TxPipeline()
	pipe.SRem(ctx, ruleset.ZoneIndexKey, hostname)
	pipe.Del(ctx, ruleset.ZoneKey(hostname))
	pipe.Del(ctx, ruleset.ZoneVersionKey(hostname))
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("removing zone from redis: %w", err)
	}
	return nil
}

// PublishedHostnames returns hostnames currently published — what a full resync diffs against
// to find zones that have disappeared from data-plane without this worker ever seeing the
// delete event.
func (d *Distributor) PublishedHostnames(ctx context.Context) ([]string, error) {
	members, err := d.client.SMembers(ctx, ruleset.ZoneIndexKey).Result()
	if err != nil {
		return nil, fmt.Errorf("reading zone index: %w", err)
	}
	return members, nil
}

// BumpEpoch is bumped at the end of every successful full resync. An edge that has been
// disconnected long enough to distrust its incremental state compares this and reloads
// everything.
func (d *Distributor) BumpEpoch(ctx context.Context) (int64, error) {
	epoch, err := d.client.Incr(ctx, ruleset.EpochKey).Result()
	if err != nil {
		return 0, fmt.Errorf("bumping epoch: %w", err)
	}
	return epoch, nil
}

func (d *Distributor) Ping(ctx context.Context) error {
	if err := d.client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis ping: %w", err)
	}
	return nil
}
