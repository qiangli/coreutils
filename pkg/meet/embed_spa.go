//go:build meetspa

package meet

import (
	"embed"
	"io/fs"
)

// The SPA, compiled in. See embed.go for why this is behind a build tag: web/dist
// is another track's build output, and an unconditional go:embed of a directory
// that does not exist yet is a compile error for the whole package.
//
// Build the shipping binary with `go build -tags meetspa`.

//go:embed all:web/dist
var spaEmbed embed.FS

func init() {
	// all: is deliberate — Vite emits dot-prefixed files (.vite/manifest.json)
	// that a bare go:embed silently skips.
	if sub, err := fs.Sub(spaEmbed, "web/dist"); err == nil {
		spaFS = sub
	}
}
