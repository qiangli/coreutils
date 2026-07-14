package fleet

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/qiangli/coreutils/pkg/spacetime"
)

// Check is one entry's verdict at this host's coordinate.
type Check struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
	OK   bool   `json:"ok"`
	// Skipped marks an entry that was never a candidate — a harness we
	// recognize but do not drive, say. Not usable and not a failure: a
	// healthy host must not report an error for a tool it never intended
	// to launch.
	Skipped bool   `json:"skipped,omitempty"`
	Reason  string `json:"reason"`
	Detail  string `json:"detail,omitempty"` // version, target id, launch argv
	// Warn carries something true but not disqualifying — an entry that
	// works yet is missing something a caller may be counting on. It never
	// affects OK: a warning that failed the check would just get silenced.
	Warn string `json:"warn,omitempty"`
}

// Probes builds the probe set fleet checks read. It is the same engine
// pkg/skills gates applicability on, so a tool that a skill's `has=codex`
// clause can see is a tool `fleet verify` can see.
func Probes(cache spacetime.Cache) *spacetime.ProbeSet {
	return spacetime.DefaultProbes(cache)
}

// VerifyTool reports whether a tool can be driven headless on this host.
//
// Standalone and offline: it asks the PATH whether the binary exists and
// what version it reports. It never runs the tool's own work.
func (c *Catalog) VerifyTool(name string, ps *spacetime.ProbeSet) Check {
	chk := Check{Kind: KindTool, Name: name}
	t, ok := c.Tool(name)
	if !ok {
		chk.Reason = "not in the catalog"
		return chk
	}
	chk.Name = t.Name

	if !t.IsCLI() {
		chk.Skipped = true
		chk.Reason = fmt.Sprintf("kind %q is not an agentic CLI", t.Kind)
		return chk
	}
	if t.CLI.Launch.Exec == "" {
		chk.Skipped = true
		chk.Reason = "recognized for self-identification only; no launch template"
		return chk
	}

	bin := t.CLI.Binary
	if bin == "" {
		bin = t.Name
	}
	v, present := ps.Value("tool." + bin)
	if !present || v == "absent" {
		chk.Reason = "not installed: " + bin + " is not on PATH"
		return chk
	}
	if v != "present" {
		chk.Detail = v
	}

	chk.OK = true
	chk.Reason = "drivable; shell routed through bashy by the launcher"
	// codex runs the /etc/passwd login shell on macOS rather than $SHELL,
	// so shell-forcing does not reach it without an explicit install step.
	if t.Name == "codex" && runtime.GOOS == "darwin" {
		chk.Reason = "drivable; shell = the login shell (run `bashy install-agent codex` to route through bashy)"
	}
	if t.CLI.Launch.AuthHint != "" {
		chk.Reason += "; " + t.CLI.Launch.AuthHint
	}
	return chk
}

// VerifyModel reports whether a model is usable from this host.
//
// The default is a structural check with no network: a probe that dialed a
// provider on every `verify` would make an offline host look broken.
func (c *Catalog) VerifyModel(name string, _ *spacetime.ProbeSet) Check {
	chk := Check{Kind: KindModel, Name: name}
	m, ok := c.Model(name)
	if !ok {
		chk.Reason = "not in the catalog"
		return chk
	}
	chk.Name, chk.Detail = m.Name, m.Target()
	if m.Band < 1 {
		chk.Warn = "unpegged: no band, so a --min-band roster will never seat an agent bound to it"
	}

	switch m.Kind {
	case ModelKindAPI:
		if m.APIKeyRef == "" {
			chk.Reason = "kind is api but no api_key_ref is declared — nothing to bill against"
			return chk
		}
		chk.OK = true
		chk.Reason = "metered api; bills against the vault key " + m.APIKeyRef
	case ModelKindSubscription:
		chk.OK = true
		chk.Reason = "subscription seat; the CLI authenticates interactively on this host"
	case ModelKindLocal:
		chk.OK = true
		chk.Reason = "pooled local inference; served by a paired host"
	case "":
		chk.OK = true
		chk.Reason = "no access kind declared"
	default:
		chk.Reason = fmt.Sprintf("unknown kind %q (want subscription, api, or local)", m.Kind)
	}
	return chk
}

// VerifyAgent reports whether an agent can actually be launched: both
// halves of its binding resolve, the tool is operable, and the tool can
// select the model it is bound to.
func (c *Catalog) VerifyAgent(name string, ps *spacetime.ProbeSet) Check {
	chk := Check{Kind: KindAgent, Name: name}
	a, tool, model, err := c.Binding(name)
	if err != nil {
		chk.Reason = err.Error()
		return chk
	}
	chk.Name = a.Name

	if tc := c.VerifyTool(tool.Name, ps); !tc.OK {
		chk.Reason = "tool " + tool.Name + ": " + tc.Reason
		return chk
	}
	if mc := c.VerifyModel(model.Name, ps); !mc.OK {
		chk.Reason = "model " + model.Name + ": " + mc.Reason
		return chk
	}
	if !tool.TakesModel() {
		chk.Reason = fmt.Sprintf("tool %s has no {model} placeholder, so it cannot select %s — the binding is a label, not a selection", tool.Name, model.Name)
		return chk
	}

	chk.OK = true
	chk.Reason = "launchable"
	chk.Detail = strings.Join(tool.Argv(model.TargetFor(tool.Name), PromptToken), " ")
	return chk
}
