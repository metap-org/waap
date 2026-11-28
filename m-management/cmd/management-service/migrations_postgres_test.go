//go:build integration

package main

import (
	"bytes"
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/metap-org/metap/pkg/app"
	"github.com/metap-org/metap/pkg/testkit"
	"github.com/stretchr/testify/require"
)

// The committed SQL, on a database that has only the platform's tables: it gives every entity what
// it needs (status checks each real table), comes off again, and applies a second time.
func TestTheCommittedMigrationsBuildAndRemoveTheSchema(t *testing.T) {
	pool := testkit.NewPostgres(t)
	u, err := url.Parse(testkit.BaseDatabaseURL())
	require.NoError(t, err)
	u.Path = "/" + pool.Config().ConnConfig.Database
	t.Setenv("DATABASE_URL", u.String())

	run := func(args ...string) (string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		var out bytes.Buffer
		err := app.MigrateCommand(ctx, options(), args, &out)
		return out.String(), err
	}
	_, err = run("status")
	require.ErrorIs(t, err, app.ErrSchemaNotReady)
	out, err := run("up")
	require.NoError(t, err, out)
	out, err = run("status")
	require.NoError(t, err, out)
	require.NotContains(t, out, "warning:", "the files leave nothing the entities do not ask for")

	out, err = run("down", "--steps", "100", "--yes")
	require.NoError(t, err, out)
	var tables int
	require.NoError(t, pool.QueryRow(context.Background(), "SELECT count(*) FROM pg_tables WHERE schemaname = 'waf'").Scan(&tables))
	require.Zero(t, tables)
	out, err = run("up")
	require.NoError(t, err, out)
	out, err = run("status")
	require.NoError(t, err, out)
}
