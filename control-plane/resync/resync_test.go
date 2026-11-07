package resync

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/metap-org/waap/control-plane/dataplane"
	"github.com/metap-org/waap/control-plane/ports/portsmock"
	"github.com/metap-org/waap/control-plane/ruleset"
)

func zoneRecord(id, status, hostname string) dataplane.Record {
	data := map[string]json.RawMessage{}
	for k, v := range map[string]any{"status": status, "hostname": hostname, "tenantId": "t1", "originAddress": "http://o", "protectionMode": "enforce"} {
		b, _ := json.Marshal(v)
		data[k] = b
	}
	return dataplane.Record{ID: id, Data: data}
}

func TestRunOncePublishesEveryLiveZoneRemovesOrphansAndBumpsTheEpoch(t *testing.T) {
	src, sink := portsmock.NewConfigSource(t), portsmock.NewRuleSink(t)
	src.EXPECT().AllZones(mock.Anything).Return([]dataplane.Record{
		zoneRecord("z1", "active", "a.example.com"),
		zoneRecord("z2", "paused", "b.example.com"),
		zoneRecord("z3", "pending", "c.example.com"), // not publishable: skipped, and so removed if present
	}, nil)
	src.EXPECT().TenantWideRulesFor(mock.Anything).Return(nil, nil)
	src.EXPECT().TenantWideIPAccessListsFor(mock.Anything).Return(nil, nil)
	for _, id := range []string{"z1", "z2"} {
		src.EXPECT().DdosPoliciesFor(mock.Anything, id).Return(nil, nil)
		src.EXPECT().RulesFor(mock.Anything, id).Return(nil, nil)
		src.EXPECT().IPAccessListsFor(mock.Anything, id).Return(nil, nil)
	}
	var published []string
	sink.EXPECT().Publish(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, z ruleset.CompiledZone) error {
		published = append(published, z.Hostname)
		return nil
	}).Times(2)
	sink.EXPECT().PublishedHostnames(mock.Anything).Return([]string{"a.example.com", "c.example.com", "gone.example.com"}, nil)
	sink.EXPECT().Unpublish(mock.Anything, "c.example.com").Return(nil)
	sink.EXPECT().Unpublish(mock.Anything, "gone.example.com").Return(nil)
	sink.EXPECT().BumpEpoch(mock.Anything).Return(7, nil)

	require.NoError(t, RunOnce(context.Background(), src, sink))
	assert.ElementsMatch(t, []string{"a.example.com", "b.example.com"}, published)
}

func TestRunOnceStopsBeforeRemovingAnythingWhenTheDataPlaneFails(t *testing.T) {
	src, sink := portsmock.NewConfigSource(t), portsmock.NewRuleSink(t)
	boom := errors.New("data-plane down")
	src.EXPECT().AllZones(mock.Anything).Return(nil, boom)

	// The convergence guarantee must never read "data-plane unreachable" as "zero zones" and
	// wipe the edge: no PublishedHostnames/Unpublish/BumpEpoch expected.
	require.ErrorIs(t, RunOnce(context.Background(), src, sink), boom)
}
