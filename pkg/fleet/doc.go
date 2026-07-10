// Package fleet is the declarative registry of the things an agentic
// host runs with: tools, models, and agents.
//
//	tool   an agentic CLI harness (claude, codex, opencode, aider, agy)
//	model  an inference backend (a subscription seat, a metered API, a
//	       pooled local model)
//	agent  a tool bound to a model — written tool:model — under one or
//	       more nicknames
//
// A bare tool is not an agent and a bare model is not an agent: an agent
// always names both. Roles are an orthogonal axis and do not belong to
// the binding.
//
// # Nicknames
//
// An agent's identity is its tool:model binding; its names are aliases.
// `007` and `smarty` may both name claude:fable. Any number of nicknames
// collapse to one capability-matrix row, because that matrix is keyed by
// the binding, never by the nickname.
//
// # Rings, and where the truth lives
//
// Entries are merged over pkg/assetring's rings — embedded baseline,
// shared catalog dirs, an optional org overlay, and the host-local store,
// in that precedence order. Every local entry is one file whose bytes are
// exactly the Content blob an org catalog would serve, so a definition
// round-trips in both directions without a transform.
//
// The package is standalone-first and effect-free: it reads and writes
// YAML and probes the host, but it never spawns an agent. Launching is
// the launcher's job; fleet only says how.
package fleet
