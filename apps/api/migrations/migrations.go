// Package migrations embeds the central PostgreSQL schema so the API binary
// carries its own schema and nothing needs to be copied to the server.
package migrations

import "embed"

// FS holds every *.sql file in this directory, applied in lexical order.
//
//go:embed *.sql
var FS embed.FS
