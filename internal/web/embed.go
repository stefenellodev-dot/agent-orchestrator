// Package web embeds the compiled dashboard so the orchestrator ships as a
// single self-contained binary on Piave.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dashboard
var embedded embed.FS

// Dist returns the built dashboard assets.
func Dist() fs.FS {
	sub, err := fs.Sub(embedded, "dashboard")
	if err != nil {
		panic(err)
	}
	return sub
}
