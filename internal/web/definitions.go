package web

import (
	"embed"
	"io/fs"
)

// pictures holds a <type>.svg drawn across and a <type>.down.svg drawn
// down per definition in batch.Definitions, by internal/web/diagrams/gen
// (task diagrams).
//
//go:embed diagrams/*.svg
var pictures embed.FS

func diagrams() fs.FS {
	sub, err := fs.Sub(pictures, "diagrams")
	if err != nil {
		panic(err)
	}
	return sub
}
