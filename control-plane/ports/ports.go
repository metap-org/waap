// Package ports declares the two seams of the control-plane pipeline, so the compile/sync/resync
// logic is unit-testable without a live management-plane or Redis:
//
//	ConfigSource (management-plane GraphQL, management.Client) -> compile -> RuleSink (Redis, distribute.Distributor)
//
// Mocks: ports/portsmock (mockery, `go generate ./ports/...`).
package ports

//go:generate go run github.com/vektra/mockery/v3@v3.8.1 --config ../.mockery.yml

import (
	"context"

	"github.com/metap-org/waap/control-plane/analytics"
	"github.com/metap-org/waap/control-plane/distribute"
	"github.com/metap-org/waap/control-plane/management"
	"github.com/metap-org/waap/control-plane/ruleset"
)

// ConfigSource reads the portal's source-of-truth config.
type ConfigSource interface {
	Zone(ctx context.Context, zoneID string) (*management.Record, error)
	AllZones(ctx context.Context) ([]management.Record, error)
	DdosPoliciesFor(ctx context.Context, zoneID string) ([]management.Record, error)
	RulesFor(ctx context.Context, zoneID string) ([]management.Record, error)
	TenantWideRulesFor(ctx context.Context) ([]management.Record, error)
	IPAccessListsFor(ctx context.Context, zoneID string) ([]management.Record, error)
	TenantWideIPAccessListsFor(ctx context.Context) ([]management.Record, error)
}

// EventSink writes edge telemetry into the Analytics plane, one batch per call. It returns how
// many events the sink rejected as invalid (dropped, not retryable).
type EventSink interface {
	WriteSecurityEvents(ctx context.Context, events []map[string]any) (int, error)
}

// RuleSink is where compiled rule-sets are published for the edge to read.
type RuleSink interface {
	Publish(ctx context.Context, zone ruleset.CompiledZone) error
	Unpublish(ctx context.Context, hostname string) error
	PublishedHostnames(ctx context.Context) ([]string, error)
	BumpEpoch(ctx context.Context) (int64, error)
	Ping(ctx context.Context) error
}

var (
	_ ConfigSource = (*management.Client)(nil)
	_ EventSink    = (*analytics.Client)(nil)
	_ RuleSink     = (*distribute.Distributor)(nil)
)
