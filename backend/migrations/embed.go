// Package migrations embeds the SQL migration files so the compiled binary can
// self-migrate. golang-migrate's CLI also reads these same .sql files directly
// (see the `migrate-*` Makefile targets) — keep both in sync via this dir.
package migrations

import "embed"

// FS holds every *.sql migration file in this directory.
//
//go:embed *.sql
var FS embed.FS
