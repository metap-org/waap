package main

import (
	"bytes"
	"context"
	"testing"

	"github.com/metap-org/metap/pkg/app"
	"github.com/stretchr/testify/require"
)

// An entity changed without its migration would only show when the service refuses to start. Run
// `go run ./m-management/cmd/management-service migrate generate <name>` and commit the files.
func TestTheMigrationsCoverTheEntities(t *testing.T) {
	var out bytes.Buffer
	err := app.MigrateCommand(context.Background(), options(), []string{"generate", "--check", "--dir", "../../migrations"}, &out)
	require.NoError(t, err, out.String())
}
