// Package migrations exposes the SQL migration files as an embed.FS so
// the audit store can run them at startup without depending on a
// separate goose binary or filesystem layout at runtime.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
