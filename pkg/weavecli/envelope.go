// Package weavecli holds the agent-friendly CLI conventions every
// `ycode weave` subverb shares: versioned-envelope marshaling, stable
// exit-code constants, tty/agent-mode detection, and the BASHY_AGENTIC
// switch that flips all the user-visible defaults to machine-friendly.
//
// Lives outside cmd/ycode so internal callers (autopilot worker,
// test scaffolding) can also produce envelope-shaped output without
// importing the cobra command tree.
package weavecli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/qiangli/coreutils/pkg/fleet"
)

// SchemaVersion is stamped into every envelope's schema_version field.
// Bump when output shape changes; clients pin against the version they
// tested.
const SchemaVersion = "loom-v2"

// Stable exit codes. See docs/loom-v2-plan.md "Agent-friendly CLI →
// Stable exit codes" table.
const (
	ExitOK            = 0
	ExitGenericFail   = 1
	ExitInvalidArg    = 2
	ExitPrecondFail   = 3
	ExitStateConflict = 4
	ExitDepUnhealthy  = 5
)

// Envelope is the structured response shape returned in --json mode.
// All fields except SchemaVersion / Command / Status are optional.
type Envelope struct {
	SchemaVersion string         `json:"schema_version"`
	Command       string         `json:"command"`
	Status        string         `json:"status"` // ok | error | partial
	Result        any            `json:"result,omitempty"`
	Error         *EnvelopeError `json:"error,omitempty"`
	Hints         []Hint         `json:"hints,omitempty"`
}

// EnvelopeError carries the agent-actionable error code (matching one
// of the Exit* constants by suffix) and a human-readable message.
type EnvelopeError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Hint is a structured suggestion emitted by the agent-mode engine
// (e.g. "you used `weave start` outside auto-attach; consider --tool
// flag instead of trailing --").
type Hint struct {
	Why     string `json:"why"`
	Suggest string `json:"suggest"`
}

// OutputMode controls how a subverb renders its result.
type OutputMode int

const (
	OutputAuto  OutputMode = iota // auto-detect from tty
	OutputJSON                    // --json: emit envelope
	OutputPlain                   // --plain: no ANSI, no spinners
	OutputQuiet                   // --quiet: final result line only
)

// IsAgent reports whether AGENT MODE was explicitly requested: force --json, no
// prompts, no spinners. The single signal is BASHY_AGENTIC (matching the `--agentic`
// flag family).
//
// This is deliberately NARROW, and stays narrow. It decides OUTPUT FORMAT for ~30
// subverbs, so widening it to "an agent CLI is driving the shell" would silently turn
// every `weave list` and `weave status` in a Claude session from a human-readable table
// into a JSON blob — a real change to what a human reads in their transcript, and not
// one to make by ambient detection. Agent mode is asked for; it is not sniffed.
//
// The question "is a machine at the wheel?" — which is what the AFFORDANCES (the
// advisor, the nudges) actually want — is IsAgentDriven. See its doc: they are
// different questions, and the bug was one predicate answering both.
func IsAgent() bool {
	return truthyAgent(os.Getenv("BASHY_AGENTIC"))
}

// IsAgentDriven reports whether a MACHINE is at the wheel — by either route.
//
// # The bug this fixes
//
// BASHY_AGENTIC is set in exactly ONE place: when bashy itself launches an agentic
// worker (weave, `bashy run`). A human who types `claude` in a terminal — with bashy as
// its shell, which is the ENTIRE POINT of `bashy install-agent` — sets nothing. So the
// advisor and the nudges, which gated on IsAgent(), were SILENTLY DARK in the single
// most common agentic configuration there is. Two shipped features that never once fired
// for the users they were built for.
//
// The same wrong gate would have neutered the coordination guard, which is how it was
// finally caught: a guard that no-ops in exactly the sessions that collide.
//
//	BASHY_AGENTIC     bashy ORCHESTRATED this run (a weave worker, `bashy run`)
//	fleet.DetectTool  an agent CLI is DRIVING this shell (CLAUDECODE, CODEX_SANDBOX, …)
//
// # Why this is a separate predicate and not just a wider IsAgent
//
// Because the two questions have different blast radii, and conflating them trades one
// bug for a worse one:
//
//   - An AFFORDANCE (an advisor hint on stderr when a command fails) is additive. Firing
//     it for an agent that did not ask can only help.
//   - A FORMAT (stdout as JSON instead of a table) is a contract. Flipping it by sniffing
//     the environment changes what every existing script and human sees.
//   - A WORLD (shimming `go` to a self-downloaded toolchain — `bashy go` does not wrap
//     the host one) would shadow the compiler a developer actually installed.
//
// A machine at the wheel earns better HINTS. Only an explicit request earns a different
// FORMAT, and only bashy orchestrating the run earns a different WORLD.
func IsAgentDriven() bool {
	if IsAgent() {
		return true
	}
	_, detected := fleet.DetectTool()
	return detected
}

func truthyAgent(v string) bool {
	switch v {
	case "", "0", "false", "no":
		return false
	}
	return true
}

// StdoutIsTTY reports whether stdout is a character device (terminal).
// False for pipes, redirects, capture-by-subprocess, and BASHY_AGENTIC=1.
func StdoutIsTTY() bool {
	if IsAgent() {
		return false
	}
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// ResolveOutputMode picks the OutputMode from the per-call flags, with the
// BASHY_AGENTIC env as the fallback. It cannot express an explicit --json=false
// (a plain bool can't distinguish "false" from "unset"); for that escape hatch,
// callers use ResolveOutputModeEx with the flag's Changed bit.
//
// Precedence here: --json > --plain > --quiet > BASHY_AGENTIC > auto.
func ResolveOutputMode(jsonFlag, plainFlag, quietFlag bool) OutputMode {
	return ResolveOutputModeEx(jsonFlag, jsonFlag, plainFlag, quietFlag)
}

// ResolveOutputModeEx is ResolveOutputMode with an explicit-override escape
// hatch: jsonSet reports whether --json was given at all, so `--json=false`
// (jsonSet=true, jsonVal=false) can force text output even when BASHY_AGENTIC
// would otherwise default to JSON — the per-command opt-out for pipelines like
// `weave list --json=false | grep …`. An explicit --plain / --quiet likewise
// overrides the env.
//
// Precedence: explicit --json(=true/false) > --plain > --quiet > BASHY_AGENTIC
// (the global agent default) > auto (tty detection).
func ResolveOutputModeEx(jsonSet, jsonVal, plainFlag, quietFlag bool) OutputMode {
	if jsonSet {
		if jsonVal {
			return OutputJSON
		}
		// explicit --json=false: text output, overriding the BASHY_AGENTIC env.
		if quietFlag {
			return OutputQuiet
		}
		return OutputPlain
	}
	if plainFlag {
		return OutputPlain
	}
	if quietFlag {
		return OutputQuiet
	}
	if IsAgent() { // BASHY_AGENTIC: the global default when no explicit flag
		return OutputJSON
	}
	return OutputAuto
}

// EmitOK writes a status=ok envelope to w and returns ExitOK. The
// helper exists so subverbs don't repeat the marshal-or-print
// boilerplate at every return path.
func EmitOK(w io.Writer, mode OutputMode, command string, result any) int {
	if mode == OutputJSON {
		_ = encodeEnvelope(w, Envelope{
			SchemaVersion: SchemaVersion,
			Command:       command,
			Status:        "ok",
			Result:        result,
		})
		return ExitOK
	}
	// Plain/quiet/auto: caller-rendered output happens before EmitOK;
	// the helper only returns the exit code in non-JSON mode.
	return ExitOK
}

// EmitError writes a status=error envelope (in JSON mode) or a plain-
// text "<command>: <message>" line (otherwise) to stderr and returns
// the given exit code. Pairs with cobra's RunE return path so subverbs
// uniformly turn errors into the right exit shape.
func EmitError(stderr io.Writer, mode OutputMode, command string, code int, err error) int {
	if mode == OutputJSON {
		_ = encodeEnvelope(stderr, Envelope{
			SchemaVersion: SchemaVersion,
			Command:       command,
			Status:        "error",
			Error: &EnvelopeError{
				Code:    codeToString(code),
				Message: err.Error(),
			},
		})
		return code
	}
	fmt.Fprintf(stderr, "%s: %s\n", command, err.Error())
	return code
}

func codeToString(code int) string {
	switch code {
	case ExitInvalidArg:
		return "invalid_arg"
	case ExitPrecondFail:
		return "precondition_failed"
	case ExitStateConflict:
		return "state_conflict"
	case ExitDepUnhealthy:
		return "dependency_unhealthy"
	case ExitOK:
		return "ok"
	default:
		return "generic_failure"
	}
}

func encodeEnvelope(w io.Writer, e Envelope) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(e)
}
