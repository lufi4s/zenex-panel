// Package web embeds the panel's static UI so the API serves it directly.
package web

import "embed"

// FS holds index.html and the assets/ directory.
//
//go:embed index.html assets
var FS embed.FS
