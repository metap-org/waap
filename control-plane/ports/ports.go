// Package ports declares the two seams of the control-plane pipeline, so the compile/sync/resync
// logic is unit-testable without a live data-plane or Redis:
//
//	ConfigSource (data-plane GraphQL, dataplane.DataPlane) -> compile -> RuleSink (Redis, distribute.Distributor)
//
// Mocks: ports/portsmock (mockery, `go generate ./ports/...`).
package ports

//go:generate go run github.com/vektra/mockery/v3@v3.8.1 --config ../.mockery.yml

import (
	"context"

	"github.com/metap-org/waap/control-plane/dataplane"
	"github.com/metap-org/waap/control-plane/distribute"
	"github.com/metap-org/waap/control-plane/ruleset"
)

// ConfigSource reads the portal's source-of-truth config.
type ConfigSource interface {
	Zone(ctx context.Context, zoneID string) (*dataplane.Record, error)
	AllZones(ctx context.Context) ([]dataplane.Record, error)
	DdosPoliciesFor(ctx context.Context, zoneID string) ([]dataplane.Record, error)
	RulesFor(ctx context.Context, zoneID string) ([]dataplane.Record, error)
	TenantWideRulesFor(ctx context.Context) ([]dataplane.Record, error)
	IPAccessListsFor(ctx context.Context, zoneID string) ([]dataplane.Record, error)
	TenantWideIPAccessListsFor(ctx context.Context) ([]dataplane.Record, error)
}

// EventSink writes edge telemetry back into the data-plane.
type EventSink interface {
	CreateSecurityEvent(ctx context.Context, payload any) error
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
	_ ConfigSource = (*dataplane.DataPlane)(nil)
	_ EventSink    = (*dataplane.DataPlane)(nil)
	_ RuleSink     = (*distribute.Distributor)(nil)
)
