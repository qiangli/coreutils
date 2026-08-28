// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

// Package webconsole is bashy's browser console: ONE local HTTP surface with a
// start page of tiles, and every other bashy web surface deep-linked beneath it.
//
// The shape is the one docs/agent-interaction-surfaces-design.md settled on: one
// console owning a port, not one cooperative app per verb, because "five
// unrelated tiles is not bashy on the web — one nav, one auth, one design
// system". A verb's --web-ui modifier is meant to deep-link INTO this, never to
// stand up a second server.
//
// # Panels
//
// A panel is either mounted in-process or reverse-proxied, and the rule is not
// about convenience:
//
//	in-process    the panel's state lives in THIS process, or in files this
//	              process already owns (the room, the terminal, the file tree)
//	proxy         another process owns the lifecycle (loom, dag --serve, ycode)
//
// Mounting the room in-process rather than proxying a separate `relay serve` is
// what keeps one lease holder and one transcript: the console adds a transport,
// never a second truth. Proxying anything supervised elsewhere is what keeps the
// console from quietly becoming a process supervisor.
//
// # Discovery
//
// The tile list is not hardcoded. A verb declares a browser UI by carrying an
// atlas.WebSurface, and the console renders whatever the atlas reports, so a new
// surface appears with no console edit. `bashy commands --view web` renders the
// same data in the terminal — one source, two renderers.
//
// # Trust
//
// The gate is meet's: ungated on direct loopback (the machine owner, on their
// own machine), cloud-vouched when arriving through outpost's tunnel, 403
// otherwise. Binding a non-loopback address additionally requires a system
// login, because the terminal panel hands out a shell running as this user and
// the file panel reads this user's files.
//
// The console is a DATA PLANE for the Files panel. That is fine on loopback and
// through outpost's direct tunnel; a download path that rides the cloudbox relay
// would violate the fail-closed data-plane block (dhnt docs/cloudbox-data-plane-block.md).
package webconsole
