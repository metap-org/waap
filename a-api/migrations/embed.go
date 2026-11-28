// Package migrations holds analytics-api's ClickHouse schema, one NNNN_name.up.sql with its
// .down.sql each. ClickHouse takes one statement at a time: separate them with a line
// "-- migrate:statement-break". An applied file must not change; add a new one.
package migrations

import "embed"

// FS holds the .sql files.
//
//go:embed *.sql
var FS embed.FS
