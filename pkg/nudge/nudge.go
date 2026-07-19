// Package nudge is the proactive half of the agent-hint subsystem: when an
// agent runs a legacy tool that has a better agentic counterpart, it emits ONE
// rate-limited hint pointing at it — never changing the tool's behavior.
//
// It lives in coreutils (not a single product) so every shell that installs the
// coreutils in-process userland — bashy AND ycode and any other consumer — gets
// the same steering toward the composable structured verbs (grep --agentic,
// ast refs/symbols/map). A traditional shell (zsh) cannot do this; a hint at the
// exec-audit chokepoint is the elevator from "the tool exists" to "the agent
// uses it".
//
// Invariants: help, don't obstruct. Hints are stderr-only (stdout stays pure
// data), rate-limited to once per tool per process, and fully silenceable. They
// are observers on interp.WithAuditHandler — they never block or alter a command.
package nudge

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"mvdan.cc/sh/v3/interp"

	"github.com/qiangli/coreutils/pkg/weavecli"
)

// SchemaVersion is the agent-facing hint wire schema. It is the established
// contract agents already parse; keep it stable.
const SchemaVersion = "bashy-hint-v1"

// builtinRules maps a state-mutating builtin to the suggestion shown when an
// agent uses it. Behavior is never changed, only annotated.
var builtinRules = map[string]string{
	"cd":    "to run a single command elsewhere, prefer `awd DIR -- CMD` — it won't leave the shell in a new directory (cd persists and strands the next command).",
	"pushd": "for a one-off command in another directory, prefer `awd DIR -- CMD` over pushd/popd — no directory-stack state to unwind.",
	"popd":  "for a one-off command in another directory, prefer `awd DIR -- CMD` over pushd/popd — no directory-stack state to unwind.",
}

// Suggest returns the hint text for a command (args[0] is the tool name), or ""
// when there's nothing to suggest. Pure (argv in, text out) so callers share the
// rule set.
func Suggest(args []string, isBuiltin bool) string {
	if len(args) == 0 {
		return ""
	}
	name := args[0]
	if isBuiltin {
		return builtinRules[name]
	}
	return routingHint(name, args)
}

// routingHint suggests a faster/structural path for legacy search tools based
// only on argv (no behavior change). This is the load-bearing steer: it points
// classic grep/find at the composable structured verbs the in-process userland
// adds — grep --agentic/--json and the ast code-intel commands.
func routingHint(name string, args []string) string {
	switch name {
	case "grep":
		if hasArg(args, "--agentic") || hasArg(args, "--json") || !hasRecursiveFlag(args) {
			return ""
		}
		return "repo-wide grep also walks ignored noise (node_modules/.git/vendor). Add `--agentic` to skip it, `--json` to emit NDJSON you can pipe into jq, or use `ast refs <symbol>` / `ast map` for structural, token-budgeted code search."
	case "find":
		if hasArg(args, "--agentic") {
			return ""
		}
		return "find walks ignored directories too. Add `--agentic` to skip .gitignore/node_modules, or use `ast symbols` / `ast map` to map the codebase structurally."
	}
	return ""
}

func hasArg(args []string, want string) bool {
	for _, a := range args[1:] {
		if a == want {
			return true
		}
	}
	return false
}

// hasRecursiveFlag reports whether grep was asked to recurse (-r/-R, long forms,
// or a combined short cluster like -rn).
func hasRecursiveFlag(args []string) bool {
	for _, a := range args[1:] {
		switch a {
		case "-r", "-R", "--recursive", "--dereference-recursive":
			return true
		}
		if len(a) > 1 && a[0] == '-' && a[1] != '-' && strings.ContainsAny(a, "rR") {
			return true
		}
	}
	return false
}

// Enabled reports whether proactive hints should fire. BASHY_AGENTIC off is the
// master kill; otherwise BASHY_HINTS is the explicit control (off-ish silences,
// on-ish forces on); unset defaults to agent-driven mode only.
func Enabled() bool {
	switch strings.ToLower(os.Getenv("BASHY_AGENTIC")) {
	case "0", "false", "off", "no":
		return false
	}
	switch strings.ToLower(os.Getenv("BASHY_HINTS")) {
	case "0", "false", "off", "no":
		return false
	case "1", "true", "on", "yes":
		return true
	}
	return weavecli.IsAgentDriven()
}

// Nudger emits proactive tool-hints, rate-limited to once per tool for the life
// of the process (a shell session).
type Nudger struct {
	w    io.Writer
	mu   sync.Mutex
	seen map[string]bool
}

// New returns a Nudger writing to w (typically os.Stderr).
func New(w io.Writer) *Nudger {
	return &Nudger{w: w, seen: make(map[string]bool)}
}

// OnAudit is the interp.WithAuditHandler callback. It fires once per simple
// command (post-expansion); it acts only on watched tools, once per tool per
// process, and only when hints are Enabled.
func (n *Nudger) OnAudit(ev interp.AuditEvent) {
	if n == nil || n.w == nil || len(ev.Args) == 0 || !Enabled() {
		return
	}
	name := ev.Args[0]
	suggest := Suggest(ev.Args, ev.IsBuiltin)
	if suggest == "" {
		return
	}
	n.mu.Lock()
	if n.seen[name] {
		n.mu.Unlock()
		return
	}
	n.seen[name] = true
	n.mu.Unlock()
	n.emit(name, suggest)
}

type line struct {
	Schema  string `json:"schema_version"`
	Kind    string `json:"kind"`
	Tool    string `json:"tool"`
	Suggest string `json:"suggest"`
	Off     string `json:"off"`
}

func (n *Nudger) emit(tool, suggest string) {
	if weavecli.IsAgentDriven() {
		b, _ := json.Marshal(line{Schema: SchemaVersion, Kind: "hint", Tool: tool, Suggest: suggest, Off: "BASHY_HINTS=off"})
		fmt.Fprintf(n.w, "%s\n", b)
		return
	}
	fmt.Fprintf(n.w, "─── hint ─── %s (silence: BASHY_HINTS=off)\n", suggest)
}

var (
	defaultOnce sync.Once
	defaultN    *Nudger
)

// Default returns a process-singleton Nudger on os.Stderr, so rate-limiting
// (once per tool) holds across every command in a session even when each is run
// on its own interp.Runner.
func Default() *Nudger {
	defaultOnce.Do(func() { defaultN = New(os.Stderr) })
	return defaultN
}
