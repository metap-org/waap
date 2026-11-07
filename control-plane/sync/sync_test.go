package sync

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

func record(id string, fields map[string]any) dataplane.Record {
	data := map[string]json.RawMessage{}
	for k, v := range fields {
		b, _ := json.Marshal(v)
		data[k] = b
	}
	return dataplane.Record{ID: id, Data: data}
}

func zone(status, hostname string) *dataplane.Record {
	z := record("z1", map[string]any{
		"status": status, "hostname": hostname, "originAddress": "http://origin", "protectionMode": "enforce", "configVersion": 3,
	})
	return &z
}

func strp(s string) *string { return &s }

func expectNoChildren(src *portsmock.ConfigSource) {
	src.EXPECT().DdosPoliciesFor(mock.Anything, "z1").Return(nil, nil)
	src.EXPECT().RulesFor(mock.Anything, "z1").Return(nil, nil)
	src.EXPECT().IPAccessListsFor(mock.Anything, "z1").Return(nil, nil)
}

func TestActiveZoneIsCompiledAndPublished(t *testing.T) {
	src, sink := portsmock.NewConfigSource(t), portsmock.NewRuleSink(t)
	src.EXPECT().Zone(mock.Anything, "z1").Return(zone("active", "shop.example.com"), nil)
	expectNoChildren(src)
	sink.EXPECT().Publish(mock.Anything, mock.MatchedBy(func(z ruleset.CompiledZone) bool {
		return z.Hostname == "shop.example.com" && z.TenantID == "t1" && z.SchemaVersion == ruleset.RulesetSchemaVersion
	})).Return(nil)

	require.NoError(t, Zone(context.Background(), src, sink, "z1", "t1", nil))
}

func TestDeletedZoneIsUnpublishedUnderItsLastKnownHostname(t *testing.T) {
	src, sink := portsmock.NewConfigSource(t), portsmock.NewRuleSink(t)
	src.EXPECT().Zone(mock.Anything, "z1").Return(nil, nil)
	sink.EXPECT().Unpublish(mock.Anything, "old.example.com").Return(nil)

	require.NoError(t, Zone(context.Background(), src, sink, "z1", "t1", strp("old.example.com")))
}

func TestDeletedZoneWithNoKnownHostnameIsLeftToResync(t *testing.T) {
	src, sink := portsmock.NewConfigSource(t), portsmock.NewRuleSink(t)
	src.EXPECT().Zone(mock.Anything, "z1").Return(nil, nil)

	require.NoError(t, Zone(context.Background(), src, sink, "z1", "t1", nil)) // sink untouched
}

func TestUnpublishableZoneIsTakenOffTheEdge(t *testing.T) {
	for _, status := range []string{"pending", "suspended"} {
		t.Run(status, func(t *testing.T) {
			src, sink := portsmock.NewConfigSource(t), portsmock.NewRuleSink(t)
			src.EXPECT().Zone(mock.Anything, "z1").Return(zone(status, "shop.example.com"), nil)
			sink.EXPECT().Unpublish(mock.Anything, "shop.example.com").Return(nil)

			require.NoError(t, Zone(context.Background(), src, sink, "z1", "t1", nil))
		})
	}
}

func TestRenamedZoneUnpublishesTheOldHostnameBeforePublishingTheNewOne(t *testing.T) {
	src, sink := portsmock.NewConfigSource(t), portsmock.NewRuleSink(t)
	src.EXPECT().Zone(mock.Anything, "z1").Return(zone("active", "new.example.com"), nil)
	expectNoChildren(src)
	unpublish := sink.EXPECT().Unpublish(mock.Anything, "old.example.com").Return(nil).Call
	sink.EXPECT().Publish(mock.Anything, mock.MatchedBy(func(z ruleset.CompiledZone) bool {
		return z.Hostname == "new.example.com"
	})).Return(nil).NotBefore(unpublish)

	require.NoError(t, Zone(context.Background(), src, sink, "z1", "t1", strp("old.example.com")))
}

func TestADataPlaneErrorAbortsWithoutTouchingTheEdge(t *testing.T) {
	src, sink := portsmock.NewConfigSource(t), portsmock.NewRuleSink(t)
	boom := errors.New("data-plane down")
	src.EXPECT().Zone(mock.Anything, "z1").Return(zone("active", "shop.example.com"), nil)
	src.EXPECT().DdosPoliciesFor(mock.Anything, "z1").Return(nil, boom)

	err := Zone(context.Background(), src, sink, "z1", "t1", nil)
	assert.ErrorIs(t, err, boom)
}
