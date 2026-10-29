// Package migrations contains the authoritative, ordered SQLite schema upgrades.
package migrations

import "embed"

//go:embed *.sql
var Files embed.FS
