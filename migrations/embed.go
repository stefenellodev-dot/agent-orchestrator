// Package migrations embeds the SQL migration files so the binary is
// self-contained on Piave (no external migration tooling required).
package migrations

import "embed"

// FS holds the ordered SQL migration files.
//
//go:embed *.sql
var FS embed.FS
