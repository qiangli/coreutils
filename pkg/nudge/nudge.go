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

// SuggestFailure returns a hint for a command-line idiom that commonly crosses
// from BSD userlands into GNU tools, or "" when the argv is not recognized.
// Callers must invoke it only after the command has failed: valid GNU commands
// are never annotated, and the arguments are never rewritten.
func SuggestFailure(args []string) string {
	if len(args) == 0 {
		return ""
	}
	switch args[0] {
	case "stat":
		if hasBSDStatFormat(args[1:]) {
			return "-f is --file-system in GNU; BSD's format string is -c/--format"
		}
	case "sed":
		if hasBSDInPlace(args[1:]) {
			return "GNU sed takes -i with no argument; BSD's `-i ''` is just `-i`"
		}
	}
	return ""
}

// OnFailure emits at most one structured hint for a failed invocation. It is
// the error-path counterpart to Nudger.OnAudit and uses the same schema and
// BASHY_HINTS gate. env is the invoking tool's environment, not the host
// process environment.
func OnFailure(w io.Writer, args, env []string) {
	if w == nil || !enabledIn(env) {
		return
	}
	suggest := SuggestFailure(args)
	if suggest == "" {
		return
	}
	emitMode(w, args[0], suggest, agentOutputIn(env))
}

// hasBSDStatFormat recognizes the BSD spelling only when -f is immediately
// followed by a format-looking operand. The stat command calls OnFailure only
// when that particular operand failed, so a real GNU filesystem operand stays
// silent.
func hasBSDStatFormat(args []string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "-f" && strings.HasPrefix(args[i+1], "%") {
			return true
		}
	}
	return false
}

func hasBSDInPlace(args []string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "-i" && args[i+1] == "" {
			return true
		}
	}
	return false
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

func enabledIn(env []string) bool {
	switch strings.ToLower(envValue(env, "BASHY_AGENTIC")) {
	case "0", "false", "off", "no":
		return false
	}
	switch strings.ToLower(envValue(env, "BASHY_HINTS")) {
	case "0", "false", "off", "no":
		return false
	case "1", "true", "on", "yes":
		return true
	}
	return agentOutputIn(env)
}

func agentOutputIn(env []string) bool {
	switch strings.ToLower(envValue(env, "BASHY_AGENTIC")) {
	case "1", "true", "on", "yes":
		return true
	}
	return weavecli.IsAgentDriven()
}

func envValue(env []string, key string) string {
	prefix := key + "="
	for i := len(env) - 1; i >= 0; i-- {
		if strings.HasPrefix(env[i], prefix) {
			return env[i][len(prefix):]
		}
	}
	return ""
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
	emitMode(n.w, tool, suggest, weavecli.IsAgentDriven())
}

func emitMode(w io.Writer, tool, suggest string, agentOutput bool) {
	if agentOutput {
		b, _ := json.Marshal(line{Schema: SchemaVersion, Kind: "hint", Tool: tool, Suggest: suggest, Off: "BASHY_HINTS=off"})
		fmt.Fprintf(w, "%s\n", b)
		return
	}
	fmt.Fprintf(w, "─── hint ─── %s (silence: BASHY_HINTS=off)\n", suggest)
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
