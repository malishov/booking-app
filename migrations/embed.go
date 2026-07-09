// Package migrations embeds the goose SQL migration files so they can be
// applied at runtime without shipping the raw *.sql files alongside the binary.
package migrations

import "embed"

// FS holds all embedded migration files.
//
//go:embed *.sql
var FS embed.FS
