// Package weave is the filesystem-based multi-agent workspace orchestrator
// re-homed from ycode into the AgentOS hub. It fans a queue of independent
// issues out to parallel agent CLIs (claude, codex, opencode, gemini, …),
// each in an isolated git-clone sandbox, then converges with verification.
//
// The engine is pure-filesystem (a per-repo queue + git-clone sandboxes
// under ~/.bashy/weave); it depends only on weavecli, cobra, and a PTY — no
// Gitea, no
// loom service, no agent-specific coupling. The AgentOS shell `bashy` mounts
// it as `bashy weave`; ycode's `ycode weave` is deprecated in favor of it.
package weave

import "github.com/spf13/cobra"

// NewWeaveCmd returns the `weave` cobra command tree — the host-agnostic
// entry point a front-end mounts (e.g. `bashy weave`). It Execute()s with
// the host's os.Stdin/Stdout/Stderr so the PTY-attached verbs (start, log,
// shell) drive a real terminal.
func NewWeaveCmd() *cobra.Command { return newWeaveCmd() }
