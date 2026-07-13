package weave

import (
	"fmt"
	"strings"

	"github.com/qiangli/coreutils/pkg/agentlaunch"
	"github.com/qiangli/coreutils/pkg/fleet"
)

// Launching an agent by name.
//
// `weave start -- <tool> <args...>` passes its trailing argv verbatim: the
// conductor writes the flags. That is exactly why a model was never selected —
// a model is not something you can spell in a flag list you wrote by hand for
// every issue.
//
// So a SINGLE trailing token naming an agent expands from the fleet registry
// into the tool's full headless argv, with the issue body as the prompt:
//
//	weave start --issue 3 -- 007
//	  → claude --dangerously-skip-permissions --model fable "<issue body>"
//
// Everything else is left alone. A multi-token argv is the conductor speaking
// deliberately, and a bare tool name (`-- claude`) keeps its current meaning —
// a raw launch, interactive under a PTY — because changing what that spawns
// would silently rewrite every conductor script that relies on it.

// weaveAgentLaunch is the shared agent launch resolved from the fleet registry.
type weaveAgentLaunch = agentlaunch.Launch

// weaveResolveAgent resolves a name to an agent: a nickname, an alias, or a
// bare tool:model. It returns (nil, nil) when the name is not an agent —
// a bare tool name, or something the registry has never heard of.
//
// Availability is NOT decided here. This answers "what would this launch",
// which is the question both `weave start` and `weave fleet` begin with.
func weaveResolveAgent(name string) (*weaveAgentLaunch, error) {
	cat := fleetCatalog()
	if _, ok := cat.Agent(name); !ok {
		if t, m, ok := strings.Cut(name, ":"); !ok || t == "" || m == "" {
			return nil, nil
		}
	}
	l, err := agentlaunch.ResolveWithCatalog(name, agentlaunch.Options{DryRun: true}, fleetCatalog)
	if err != nil {
		return nil, err
	}
	if l.ModelName == "" {
		return nil, nil
	}
	return &l, nil
}

// weaveExpandAgent expands a single agent token into a full launch argv.
//
// It returns (nil, nil) when toolArgs is not an agent reference — the caller
// then uses the argv verbatim, unchanged.
//
// The issue body is the prompt, because the body IS the sandbox contract. An
// issue with no body falls back to its title rather than handing the tool an
// empty argument, which most agent CLIs read as "no task" and stall on.
func weaveExpandAgent(toolArgs []string, body, title string) (*weaveAgentLaunch, []string, error) {
	if len(toolArgs) != 1 {
		return nil, nil, nil // the conductor wrote the argv; honor it
	}
	l, err := weaveResolveAgent(toolArgs[0])
	if err != nil || l == nil {
		return nil, nil, err
	}
	prompt := strings.TrimSpace(body)
	if prompt == "" {
		prompt = strings.TrimSpace(title)
	}
	if prompt == "" {
		return nil, nil, fmt.Errorf("agent %q needs a prompt: issue has neither a body nor a title", toolArgs[0])
	}
	return l, l.Argv(prompt), nil
}

// weaveAgentEnv stamps the spawned worker with the principal it is acting as.
//
// WEAVE_AGENT stays the per-issue SEAT (`007-a`) — the slot this run occupies,
// which is what the queue displays and what distinguishes two concurrent runs.
// BASHY_PRINCIPAL is the AGENT (`007`), the thing that persists across issues
// and that `bashy whois 007` resolves. A seat is not a principal, and writing
// the seat into the principal slot would put a name in the record that resolves
// to nothing.
func weaveAgentEnv(env []string, l *weaveAgentLaunch) []string {
	if l == nil {
		return env
	}
	return agentlaunch.PrincipalEnv(env, agentlaunch.Launch(*l))
}

// --- roster members ---------------------------------------------------------

// weaveMember is one roster entry: an agent (a nickname, an alias, or a bare
// tool:model) or a bare tool.
//
// Autopilot and fleet both take rosters, and both need the same three answers:
// what do I exec, which key do I record throttles and profiles under, and which
// model — if any — does this entry select.
type weaveMember struct {
	// Name is the entry as the operator wrote it.
	Name string
	// Tool is the registry tool name. It is the key for cooldowns and for the
	// persistent tool profile: two agents on one tool share both, because a
	// throttle and a launch contract belong to the binary, not the binding.
	Tool string
	// Bin is the executable. A tool's binary need not be its name.
	Bin string
	// agent is nil for a bare tool.
	agent *weaveAgentLaunch
}

// Label is what a human sees: the agent's nickname, or the tool's name.
func (m weaveMember) Label() string {
	if m.agent != nil {
		return m.agent.Nick
	}
	return m.Tool
}

// Binding is the capability-matrix key, or "" for a bare tool.
func (m weaveMember) Binding() string {
	if m.agent == nil {
		return ""
	}
	return m.agent.Binding()
}

// Model is the provider-side id this member selects, or "" for a bare tool.
func (m weaveMember) Model() string {
	if m.agent == nil {
		return ""
	}
	return m.agent.Model
}

// IsAgent reports whether this entry selects a model.
func (m weaveMember) IsAgent() bool { return m.agent != nil }

// resolveWeaveMember turns one roster entry into a member.
//
// A bare tool resolves to itself, unvalidated — exactly as every roster entry
// did before agents existed. An AGENT is validated, because naming one asserts
// a model, and a binding that cannot run is a configuration error the operator
// should hear about now rather than at failover time.
func resolveWeaveMember(name string) (weaveMember, error) {
	launch, err := weaveResolveAgent(name)
	if err != nil {
		return weaveMember{}, err
	}
	if launch == nil {
		return weaveMember{Name: name, Tool: name, Bin: name}, nil
	}
	if chk := fleetCatalog().VerifyModel(launch.ModelName, fleet.Probes(nil)); !chk.OK {
		return weaveMember{}, fmt.Errorf("agent %q: model %s: %s", name, launch.ModelName, chk.Reason)
	}
	return weaveMember{Name: name, Tool: launch.ToolName, Bin: launch.Tool, agent: launch}, nil
}

// resolveWeaveRoster resolves every entry, reporting the first that cannot run.
func resolveWeaveRoster(names []string) ([]weaveMember, error) {
	out := make([]weaveMember, 0, len(names))
	for _, n := range names {
		m, err := resolveWeaveMember(n)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

// headlessArgs is the flag list this member launches with, prompt excluded.
//
// A tool's persisted profile wins when it exists: `fleet interview --live`
// repairs a drifted launch contract there, and discarding that repair to
// re-render from the registry would reintroduce the very flags the interview
// found broken. The model is then layered on by flag, which is why a tool
// declares the flag's spelling rather than only the whole template.
func (m weaveMember) headlessArgs() []string {
	var persisted []string
	if toolsDir, err := weaveToolsDir(); err == nil {
		if p, ok := loadToolProfile(toolsDir, m.Tool); ok {
			persisted = append(persisted, p.HeadlessArgs...)
		}
	}
	if m.agent == nil {
		if persisted != nil {
			return persisted
		}
		seed, _ := seededContract(m.Tool)
		return seed.HeadlessArgs
	}
	if persisted == nil {
		return append([]string(nil), m.agent.Args...)
	}
	flag := ""
	if t, ok := fleetCatalog().Tool(m.Tool); ok {
		flag = t.ModelFlag()
	}
	if flag == "" {
		// The template positions the model without a flag; it can only be
		// rendered whole, so the registry's argv wins over the profile.
		return append([]string(nil), m.agent.Args...)
	}
	return withModelFlag(persisted, flag, m.agent.Model)
}

// withModelFlag sets flag's value in args, replacing an existing occurrence
// rather than appending a second one.
func withModelFlag(args []string, flag, value string) []string {
	out := append([]string(nil), args...)
	for i := 0; i+1 < len(out); i++ {
		if out[i] == flag {
			out[i+1] = value
			return out
		}
	}
	return append(out, flag, value)
}

// agentNick is the member's nickname, or "" for a bare tool.
func (m weaveMember) agentNick() string {
	if m.agent == nil {
		return ""
	}
	return m.agent.Nick
}

// memberLogFields renders a member for the lease log: always the tool, plus
// the binding when one was named. Existing log readers keep finding `tool=`.
func memberLogFields(m weaveMember) string {
	s := "tool=" + m.Tool
	if b := m.Binding(); b != "" {
		s += " agent=" + m.Label() + " binding=" + b
	}
	return s
}
