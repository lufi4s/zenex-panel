// Package web embeds the panel's production build (apps/panel, written to dist)
// so the API serves the UI directly from a single binary.
package web

import (
	"embed"
	"io/fs"
)

// dist holds index.html and the hashed assets/ directory produced by Vite.
//
//go:embed all:dist
var dist embed.FS

// FS returns the built panel with dist/ as its root.
func FS() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err) // dist is embedded at compile time; this cannot fail
	}
	return sub
}
