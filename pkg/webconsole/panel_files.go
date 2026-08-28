// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package webconsole

import (
	"net/http"

	"github.com/filebrowser/filebrowser/v2/fbembed"
)

// filesPanel mounts File Browser in-process.
//
// fbembed is the seam outpost already uses for its `files` builtin: stateless
// (no database — scope and permissions are recomputed from Options every boot),
// NoAuth because the EMBEDDING HOST is the access gate, and it already renders
// <base href>/StaticURL from X-Forwarded-Prefix, so it needs no per-mount config
// to work both on loopback and under a path prefix.
//
// Read-only by default. AllowWrite flips create/upload/edit/rename/delete
// together; command execution is never enabled.
//
// NOTE this panel is a DATA PLANE — it serves file bytes. That is fine on
// loopback and through outpost's direct tunnel; a download path that rides the
// cloudbox relay would violate the fail-closed data-plane block.
func filesPanel(scope string, allowWrite bool) (http.Handler, func() error, error) {
	return fbembed.New(fbembed.Options{
		Scope:      scope,
		AllowWrite: allowWrite,
	})
}
