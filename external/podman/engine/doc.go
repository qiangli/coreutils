// Package engine is coreutils' in-process podman wrapper, consuming the pure-Go
// (no-cgo) embed API of the qiangli/podman fork (external/podman/src, replaced as
// go.podman.io/podman/v6). It is the podman analog of external/ollama's in-process
// server: bashy and outpost drive an ISOLATED podman engine here without shelling
// out to a host install, so behavior is identical across Windows/macOS/Linux and
// we own the version + issue surface.
//
// Status: foundation landed — the qiangli/podman submodule + the
// go.podman.io/podman/v6 => ./external/podman/src replace are wired, and the
// embed bindings compile into coreutils. The machine lifecycle (init/start an
// isolated `bashy` machine on macOS/Windows; native socket on Linux), image
// build/push, and the binary embeds (podman/vfkit/gvproxy, behind the
// embed_podman build tag) port from ycode/internal/container next — see
// dhnt/docs/local-p2p-cicd.md.
package engine

import (
	// Anchor the go.podman.io/podman/v6 require on the embed API so `go mod tidy`
	// keeps the fork wired while the wrapper is built out.
	_ "go.podman.io/podman/v6/embed"
)

// MachineName is the dedicated, ISOLATED podman machine name (macOS/Windows).
// Distinct from ycode's `ycode-default` so the two never collide on one host —
// the "always own" coexistence guarantee.
const MachineName = "bashy"
