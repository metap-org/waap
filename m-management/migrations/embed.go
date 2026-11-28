// Package migrations holds the SQL that creates and changes management-service's entity tables.
// The files are written by `management-service migrate generate <name>`, reviewed and committed;
// README.md has the workflow. They are embedded, so the binary applies exactly what it was built
// with.
package migrations

import "embed"

// FS is this directory.
//
//go:embed *
var FS embed.FS
