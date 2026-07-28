package agentlaunch

import (
	"os"
	"strings"

	"github.com/qiangli/coreutils/pkg/fleet"
)

// THE RUNG LADDER — how well can we hear this tool?
//
// Every agent CLI in the fleet is driven through one of four transports, and
// they are not equivalent: they differ in whether bashy KNOWS a turn ended or
// GUESSES it. The ladder is ordered by that, best first:
//
//	1  ACP native     the tool speaks the Agent Client Protocol itself
//	2  ACP via adapter the tool speaks ACP through an external bridge (a Node
//	                  adapter, say) — a DEPENDENCY the host may not have, which
//	                  is the only reason it ranks below native
//	3  native events   the tool streams NDJSON and says `turn.end` (ycode today)
//	4  PTY scrape      the tool says nothing; a turn boundary is inferred from
//	                   25 seconds of silence (chat.Session.WaitIdle)
//
// Rungs 1 and 2 are the prize: ACP answers a prompt with a structured
// StopReason, so the end of a turn is a fact the agent REPORTED rather than a
// silence bashy interpreted. Rung 3 has the same virtue over a side channel.
// Rung 4 is the status quo for every third-party CLI, and it stays exactly as
// it is — the retrofit degrades, it never drops.
//
// The declaration is the whole input: a tool lands on the highest rung its
// fleet entry can support, and a tool that declares nothing new lands on the
// rung it was already on.
type Rung int

const (
	// RungACPNative is rung 1: acp_exec set, acp_rung "native".
	RungACPNative Rung = 1
	// RungACPAdapter is rung 2: acp_exec set, acp_rung "adapter".
	RungACPAdapter Rung = 2
	// RungEvents is rung 3: events_arg set (a real turn.end over NDJSON).
	RungEvents Rung = 3
	// RungPTY is rung 4: a pty and the silence heuristic. Everything else.
	RungPTY Rung = 4
)

// String names the rung the way the design doc does.
func (r Rung) String() string {
	switch r {
	case RungACPNative:
		return "acp-native"
	case RungACPAdapter:
		return "acp-adapter"
	case RungEvents:
		return "events"
	case RungPTY:
		return "pty"
	}
	return "unknown"
}

// IsACP reports whether this rung drives the tool over the Agent Client
// Protocol — the two rungs with a real end-of-turn signal.
func (r Rung) IsACP() bool { return r == RungACPNative || r == RungACPAdapter }

// RungFor is THE rung-selection function: given a tool's launch declaration,
// which transport does it get?
//
// It reads declarations only — no env, no host probing, no catalog lookup — so
// it is a pure function of what the registry says, and a test can pin all four
// answers. ACPExec is authoritative for the ACP rungs; ACPRung only decides
// WHICH of the two, and an unrecognized (or empty) value means native, because
// the field is documented as advisory and a tool that declares an exec and no
// rung is claiming to speak ACP itself.
func RungFor(l fleet.ToolLaunch) Rung {
	if strings.TrimSpace(l.ACPExec) != "" {
		if strings.EqualFold(strings.TrimSpace(l.ACPRung), "adapter") {
			return RungACPAdapter
		}
		return RungACPNative
	}
	if strings.TrimSpace(l.EventsArg) != "" {
		return RungEvents
	}
	return RungPTY
}

// EffectiveRung is the rung a launch will ACTUALLY be driven on right now:
// RungFor, then the opt-in gate.
//
// The two differ only while ACP is opt-in. A tool can declare rung 1 and still
// be driven on rung 3 or 4 today, because the ACP path has no fleet mileage yet
// and chat.Session is the hot path for every agent launch there is. When the
// gate is off, an ACP-declaring tool falls to the next rung its declaration
// supports — which is the same ladder, one step down, never a drop.
func EffectiveRung(l fleet.ToolLaunch) Rung {
	r := RungFor(l)
	if r.IsACP() && !ACPEnabled() {
		if strings.TrimSpace(l.EventsArg) != "" {
			return RungEvents
		}
		return RungPTY
	}
	return r
}

// ACPEnv opts a host into the ACP rungs. Set BASHY_ACP=1.
//
// This is deliberately a gate and not a default. The ACP path replaces the pty
// transport that every agent launch in the fleet currently runs on; merging it
// hot would put an untested transport under foreman, meet, weave and coach at
// once. Off, the launcher behaves exactly as it did before ACP existed.
const ACPEnv = "BASHY_ACP"

// ACPEnabled reports whether the operator turned the ACP rungs on.
func ACPEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(ACPEnv))) {
	case "", "0", "false", "off", "no":
		return false
	}
	return true
}

// ACPArgv renders a tool's ACP launch: the binary to exec and the args after
// it. It reports false when the tool declares no acp_exec, i.e. it is not on an
// ACP rung and the caller must fall to the next one.
//
// {model} substitutes exactly as it does in Exec, including dropping an
// orphaned flag when no model was selected. There is no {prompt}: an ACP prompt
// travels in the session, not in argv, which is the whole reason the transport
// has a real turn boundary at all.
//
// # Which executable
//
// The two ACP rungs answer this DIFFERENTLY, and getting it wrong is silent:
//
//   - rung 1 (native): the tool speaks ACP itself, so the process to spawn is
//     the tool. The template's first field names it the way every other launch
//     template does, and the executable is the tool's declared binary — the
//     registry's binary path is authoritative over a bare name in a template.
//   - rung 2 (adapter): ACP is reached through an EXTERNAL BRIDGE — `npx
//     @zed/claude-code-acp`, say — and the tool's own binary is not in that
//     argv at all. The process to spawn is the ADAPTER, which is exactly what
//     the template's first field names.
//
// Resolving rung 2 to the tool binary would exec the tool and then speak
// JSON-RPC at something that does not answer it: the tool would open its
// ordinary interface, the handshake would hang or fail, and the failure would
// look like "the agent would not start" rather than "we launched the wrong
// program". No adapter is installed on any host in the fleet today, so nothing
// at runtime will catch this — it has to be right in code.
func ACPArgv(t fleet.Tool, modelID string) (bin string, args []string, ok bool) {
	tmpl := strings.TrimSpace(t.CLI.Launch.ACPExec)
	if tmpl == "" {
		return "", nil, false
	}
	fields := strings.Fields(tmpl)
	if len(fields) == 0 {
		return "", nil, false
	}
	out := make([]string, 0, len(fields)-1)
	for _, f := range fields[1:] {
		if f == fleet.ModelToken {
			if modelID != "" {
				out = append(out, modelID)
			} else if n := len(out); n > 0 && strings.HasPrefix(out[n-1], "-") {
				out = out[:n-1] // drop the orphaned flag, as renderLaunch does
			}
			continue
		}
		out = append(out, f)
	}
	if RungFor(t.CLI.Launch) == RungACPAdapter {
		return fields[0], out, true
	}
	return t.Binary(), out, true
}

// ACPTakesModel reports whether a tool can select a model on an ACP rung.
//
// fleet.Tool.TakesModel looks at the HEADLESS template alone, which is the
// right question for a headless launch and the wrong one here: a tool reachable
// ONLY over ACP declares its {model} slot in acp_exec and may have no exec at
// all. Asked the headless question, the launcher refuses `tool:model` outright
// — "cannot select a model, so this is a label, not a selection" — and an
// ACP-only tool loses model selection entirely, which is the one thing a
// binding exists to do.
//
// Either template counts. Widening rather than switching is deliberate: a tool
// whose exec takes a model and whose acp_exec does not keeps resolving exactly
// as it does today, so nothing that works now starts failing.
func ACPTakesModel(t fleet.Tool) bool {
	return t.TakesModel() || strings.Contains(t.CLI.Launch.ACPExec, fleet.ModelToken)
}
