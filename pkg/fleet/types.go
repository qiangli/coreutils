package fleet

import (
	"sort"
	"strings"

	"github.com/qiangli/coreutils/pkg/assetring"
)

// Entry kinds — the noun a name resolves to.
const (
	KindTool   = "tool"
	KindModel  = "model"
	KindAgent  = "agent"
	KindPerson = "person"
	KindHost   = "host"
)

// Tool kind discriminators. The cloudbox Tool registry is shared between
// MCP-style function kits and agentic CLI harnesses; only ToolKindCLI is
// a fleet tool. The others are recognized so they can be skipped by name
// rather than silently mis-parsed.
const (
	ToolKindCLI    = "cli"
	ToolKindFunc   = "func"
	ToolKindWeb    = "web"
	ToolKindSystem = "system"
)

// Model kind — the access/billing discriminator, orthogonal to Source.
const (
	ModelKindSubscription = "subscription" // a seat/login plan; the CLI authenticates on the host
	ModelKindAPI          = "api"          // metered; bills against APIKeyRef
	ModelKindLocal        = "local"        // pooled local inference, managed by the outpost
)

// Model source — where the row came from, not how it is billed.
const (
	ModelSourceCloud = "cloud"
	ModelSourceLocal = "local"
)

// PromptToken and ModelToken are the launch-template placeholders.
const (
	PromptToken = "{prompt}"
	ModelToken  = "{model}"
)

// Tool is an agentic CLI harness.
//
// The canonical YAML keys are `name:` and `kind:`. Assets written before
// that was settled spell them `kit:` and `type:`; both are accepted on
// parse and neither is emitted. See parse.go.
type Tool struct {
	Name    string   `yaml:"name" json:"name"`
	Kind    string   `yaml:"kind" json:"kind"` // cli | func | web | system
	Aliases []string `yaml:"aliases,omitempty" json:"aliases,omitempty"`
	Display string   `yaml:"display,omitempty" json:"display,omitempty"`
	CLI     ToolCLI  `yaml:"cli,omitempty" json:"cli"`
	Quirks  string   `yaml:"quirks,omitempty" json:"quirks,omitempty"`

	// Harness scores the capabilities a tool governs regardless of the
	// model behind it (operability, shell, tool-use, isolation). The
	// capability matrix reads these as priors.
	Harness map[string]float64 `yaml:"harness,omitempty" json:"harness,omitempty"`

	Ring assetring.Ring `yaml:"-" json:"ring"`
}

type ToolCLI struct {
	Binary   string        `yaml:"binary,omitempty" json:"binary,omitempty"`
	Versions []ToolVersion `yaml:"versions,omitempty" json:"versions,omitempty"`
	Launch   ToolLaunch    `yaml:"launch,omitempty" json:"launch"`
}

type ToolVersion struct {
	Version  string `yaml:"version,omitempty" json:"version,omitempty"`
	Download string `yaml:"download,omitempty" json:"download,omitempty"`
	Install  string `yaml:"install,omitempty" json:"install,omitempty"`
}

// ToolLaunch is how the orchestrator invokes a tool headlessly.
type ToolLaunch struct {
	// Exec is the argv template. {prompt} is replaced by the task text and
	// {model} by the bound model's upstream id. When no model is bound,
	// {model} and the flag token immediately preceding it are dropped, so
	// a template with a model flag degrades exactly to one without.
	Exec string `yaml:"exec,omitempty" json:"exec,omitempty"`
	// PromptPosition records where the prompt goes for consumers that
	// cannot read the template (cloudbox conductor). Advisory here: the
	// {prompt} placeholder is authoritative.
	PromptPosition string `yaml:"prompt_position,omitempty" json:"prompt_position,omitempty"`
	// TrustPreseed names a config file the host must pre-seed so the CLI
	// does not no-op on a first-run trust prompt.
	TrustPreseed string       `yaml:"trust_preseed,omitempty" json:"trust_preseed,omitempty"`
	Watchdog     ToolWatchdog `yaml:"watchdog,omitempty" json:"watchdog"`

	// SupportsSay marks a tool that CAN be steered mid-run — a capability fact
	// about the tool, MEASURED (pkg/agentpty/steer_live_test.go), not asserted.
	SupportsSay bool `yaml:"supports_say,omitempty" json:"supports_say,omitempty"`

	// SteerExec is the argv template that ACTUALLY accepts steering, and it is
	// usually NOT Exec.
	//
	// A headless one-shot has nothing to steer: `codex exec` and `agy -p` run the
	// prompt and exit. Steering needs the tool's interactive session — bare `codex`,
	// or `agy -i` ("run an initial prompt interactively and CONTINUE the session").
	//
	// Two templates, because the choice is a real trade. Exec gives a clean captured
	// answer (stdout and stderr stay apart on a pipe). SteerExec gives a session you
	// can interrupt, at the cost of a pty that merges the tool's chrome into the
	// transcript. A launcher picks by what it needs; the registry refuses to pretend
	// one launch does both.
	SteerExec string `yaml:"steer_exec,omitempty" json:"steer_exec,omitempty"`
	// SupportsGracefulQuit marks a tool that exits cleanly on a quit signal.
	SupportsGracefulQuit bool `yaml:"supports_graceful_quit,omitempty" json:"supports_graceful_quit,omitempty"`
	// TrustClear is the steering input that clears a trust prompt.
	TrustClear string `yaml:"trust_clear,omitempty" json:"trust_clear,omitempty"`
	// AuthHint explains an interactive sign-in the tool needs before it
	// can run headless at all.
	AuthHint string `yaml:"auth_hint,omitempty" json:"auth_hint,omitempty"`
	// Notes is the free-text launch contract commentary.
	Notes string `yaml:"notes,omitempty" json:"notes,omitempty"`
	// EnvMarkers are environment variables whose presence identifies this
	// tool as the one currently running.
	EnvMarkers []string `yaml:"env_markers,omitempty" json:"env_markers,omitempty"`
}

type ToolWatchdog struct {
	MaxRuntime string `yaml:"max_runtime,omitempty" json:"max_runtime,omitempty"`
	MemLimit   string `yaml:"mem_limit,omitempty" json:"mem_limit,omitempty"`
}

// IsCLI reports whether this tool is an agentic CLI — the only tool kind
// the fleet drives. A missing kind means an old asset that predates the
// discriminator; those were all function kits, so absence is not cli.
func (t Tool) IsCLI() bool { return t.Kind == ToolKindCLI }

// Argv renders the launch template. modelID is the bound model's upstream
// id ("" when the tool must choose its own model); prompt is the task
// text. When modelID is empty, {model} and any flag token immediately
// before it are dropped.
//
// Tokens are whitespace-separated: launch templates are flag lists, never
// shell. A template that needs quoting is a template in the wrong place.
func (t Tool) Argv(modelID, prompt string) []string {
	fields := strings.Fields(t.CLI.Launch.Exec)
	out := make([]string, 0, len(fields)+1)
	for _, f := range fields {
		switch f {
		case ModelToken:
			if modelID != "" {
				out = append(out, modelID)
			} else if n := len(out); n > 0 && strings.HasPrefix(out[n-1], "-") {
				out = out[:n-1] // drop the orphaned flag
			}
		case PromptToken:
			out = append(out, prompt)
		default:
			out = append(out, f)
		}
	}
	return out
}

// TakesModel reports whether the launch template can select a model. A
// tool without a {model} placeholder cannot: binding it to a model is a
// label, not a selection.
func (t Tool) TakesModel() bool { return strings.Contains(t.CLI.Launch.Exec, ModelToken) }

// ModelFlag is the flag token that carries the model — the token immediately
// before {model} in the launch template, when it is a flag.
//
// A caller that already holds an argv from somewhere else (a self-healed tool
// profile, say) needs the flag's spelling to add the model to it. It returns
// "" when the template positions the model without a flag, and such a template
// can only be rendered whole, by Argv.
func (t Tool) ModelFlag() string {
	fields := strings.Fields(t.CLI.Launch.Exec)
	for i, f := range fields {
		if f == ModelToken && i > 0 && strings.HasPrefix(fields[i-1], "-") {
			return fields[i-1]
		}
	}
	return ""
}

// Binary is the executable to run: the declared one, else the tool's name.
func (t Tool) Binary() string {
	if t.CLI.Binary != "" {
		return t.CLI.Binary
	}
	return t.Name
}

// SteerArgvPrefix renders the STEERABLE launch — the interactive session, not the
// headless one-shot.
//
// Reports false when the tool has no steer_exec, i.e. it has no session to open.
// A caller that wants to interrupt an agent must be told that plainly rather than
// handed a one-shot that will exit before the first steer arrives.
func (t Tool) SteerArgvPrefix(modelID string) ([]string, bool) {
	tmpl := t.CLI.Launch.SteerExec
	if strings.TrimSpace(tmpl) == "" {
		return nil, false
	}
	// The steer template may carry {prompt} (agy -i takes an opening prompt) or
	// not (codex/opencode open an empty session). Both are legal; the caller
	// appends the prompt only when the template asked for it.
	fields := strings.Fields(tmpl)
	out := make([]string, 0, len(fields))
	for _, f := range fields[1:] { // drop the binary
		switch f {
		case ModelToken:
			if modelID != "" {
				out = append(out, modelID)
			} else if n := len(out); n > 0 && strings.HasPrefix(out[n-1], "-") {
				out = out[:n-1]
			}
		case PromptToken:
			// left for the caller
		default:
			out = append(out, f)
		}
	}
	return out, true
}

// SteerTakesPrompt reports whether the steerable launch accepts an opening prompt
// on the command line (agy -i does; codex and opencode open an empty session).
func (t Tool) SteerTakesPrompt() bool {
	return strings.Contains(t.CLI.Launch.SteerExec, PromptToken)
}

// ArgvPrefix renders everything between the binary and the prompt, for
// launchers that append the prompt themselves.
//
// It reports false when the template has no {prompt}, or when {prompt} is
// not the final token. Both cases mean the launcher cannot simply append —
// and quietly appending anyway would hand the task text to the wrong flag.
func (t Tool) ArgvPrefix(modelID string) ([]string, bool) {
	if t.CLI.Launch.Exec == "" || !strings.Contains(t.CLI.Launch.Exec, PromptToken) {
		return nil, false
	}
	argv := t.Argv(modelID, PromptToken)
	if len(argv) < 2 || argv[len(argv)-1] != PromptToken {
		return nil, false
	}
	return argv[1 : len(argv)-1], true
}

// Model is an inference backend.
type Model struct {
	Name    string   `yaml:"name" json:"name"` // the alias clients pass
	Aliases []string `yaml:"aliases,omitempty" json:"aliases,omitempty"`
	Display string   `yaml:"display,omitempty" json:"display,omitempty"`
	Kind    string   `yaml:"kind,omitempty" json:"kind,omitempty"`
	Source  string   `yaml:"source,omitempty" json:"source,omitempty"`

	Provider  string `yaml:"provider,omitempty" json:"provider,omitempty"`
	BaseURL   string `yaml:"base_url,omitempty" json:"base_url,omitempty"`
	APIKeyRef string `yaml:"api_key_ref,omitempty" json:"api_key_ref,omitempty"`
	// UpstreamID is the provider-side model id — the value handed to a
	// tool's --model flag. Its YAML key is `model:`, matching the asset
	// registry's column.
	UpstreamID string `yaml:"model,omitempty" json:"model,omitempty"`

	// ToolIDs override UpstreamID for a specific tool, because THE ID A MODEL
	// ANSWERS TO IS A PROPERTY OF THE TOOL, NOT OF THE MODEL.
	//
	// One model, three spellings, all live today:
	//
	//   aider/opencode  deepseek/deepseek-v4-pro   (litellm wants provider/model)
	//   ycode           deepseek-v4-pro            (it detects the provider itself)
	//   agy             Gemini 3.1 Pro (High)      (a display string, not a slug)
	//
	// Treating UpstreamID as one global value made ycode's bindings dead on
	// arrival: the registry handed it litellm's prefixed form and ycode rejected
	// it, while the same model worked perfectly when ycode was run by hand. That
	// is the whole dead-binding failure mode again, and `agents verify --live`
	// caught it within a minute of the tool being registered.
	//
	// Keyed by TOOL name. Absent → UpstreamID.
	ToolIDs map[string]string `yaml:"ids,omitempty" json:"ids,omitempty"`

	// Family and Version make the canonical name version-explicit. The
	// catalog derives the floating family alias from them: `opus` names
	// whichever member of family `opus` has the highest Version. A record
	// therefore stores `claude:opus4.8`, which is true forever, while the
	// convenient `opus` re-points on its own when a release lands.
	//
	// Family is declared, never parsed out of the name: `kimi-k2.7-code`
	// and `kimi-k2.6` are separate product lines, and no amount of clever
	// suffix-stripping gets that right.
	Family  string `yaml:"family,omitempty" json:"family,omitempty"`
	Version string `yaml:"version,omitempty" json:"version,omitempty"`

	// Band is the model's capability peg, 1 (basic) to MaxBand (frontier); 0 is
	// unpegged. It is normalized ACROSS providers — a provider's own tier
	// ladder is never mapped positionally, so four vendor tiers may all
	// land in one band. Agents inherit it; they never carry their own.
	Band int `yaml:"band,omitempty" json:"band,omitempty"`

	// BandSource says whether the band was MEASURED or merely DECLARED, and it
	// exists because the fleet has already been burned once by not knowing.
	//
	// "declared" is a considered guess from provider tier + priors. "measured"
	// means the model was run up a difficulty ladder and pegged at the highest
	// rung it reliably cleared — which is the only thing a band actually means.
	//
	// The distinction is load-bearing: a quiz cannot validate a band. Every agent
	// in this fleet scores 5/5 on L1-difficulty questions, so passing an easy test
	// is evidence of nothing. A band is the highest rung you CLEAR, not a score,
	// and until a model has failed something it has not been placed.
	//
	// Empty means declared. Nothing should present an unmeasured band as fact.
	BandSource string `yaml:"band_source,omitempty" json:"band_source,omitempty"`

	// Tier is the provider's own word for its tier, carried from an org
	// overlay. It is not Band and is not routable.
	Tier          string   `yaml:"tier,omitempty" json:"tier,omitempty"`
	Capabilities  []string `yaml:"capabilities,omitempty" json:"capabilities,omitempty"`
	Domain        []string `yaml:"domain,omitempty" json:"domain,omitempty"`
	ContextLength int64    `yaml:"context_length,omitempty" json:"context_length,omitempty"`
	Price         float64  `yaml:"price,omitempty" json:"price,omitempty"`

	// Quality is the model's overall capability prior in [0,1]; Spec holds
	// per-capability adjustments where a model is notably stronger or
	// weaker than its tier. CostMicro is the relative per-turn cost the
	// routing objective divides by. All three are read by the capability
	// matrix.
	Quality   float64            `yaml:"quality,omitempty" json:"quality,omitempty"`
	CostMicro int64              `yaml:"cost_micro,omitempty" json:"cost_micro,omitempty"`
	Spec      map[string]float64 `yaml:"spec,omitempty" json:"spec,omitempty"`

	XHosts []ModelHost `yaml:"x_hosts,omitempty" json:"x_hosts,omitempty"`

	// Derived holds names the catalog computed at load — today, the family
	// alias. It is a function of the whole catalog, not of this entry, so
	// it is never persisted (`yaml:"-"`): writing it back would freeze a
	// pointer that is supposed to float.
	Derived []string `yaml:"-" json:"derived,omitempty"`

	Ring assetring.Ring `yaml:"-" json:"ring"`
}

// ModelHost names a paired host serving a projected local model.
type ModelHost struct {
	Host  string `yaml:"host" json:"host"`
	Owner string `yaml:"owner,omitempty" json:"owner,omitempty"`
}

// Target is the id passed to a tool's model flag: the provider-side id
// when known, else the alias itself.
//
// Prefer TargetFor: the id a model answers to depends on WHICH TOOL is asking.
func (m Model) Target() string {
	if m.UpstreamID != "" {
		return m.UpstreamID
	}
	return m.Name
}

// TargetFor is the id THIS TOOL will accept for this model.
//
// The same model is spelled differently by different harnesses — litellm wants
// `deepseek/deepseek-v4-pro`, ycode wants `deepseek-v4-pro`, agy wants
// `Gemini 3.1 Pro (High)`. A registry that stores one global id hands the wrong
// string to somebody, and a wrong model id is a DEAD BINDING: it looks perfectly
// healthy until an agent tries to speak.
func (m Model) TargetFor(tool string) string {
	if id, ok := m.ToolIDs[tool]; ok && id != "" {
		return id
	}
	return m.Target()
}

// AgentFile is the on-disk envelope for agents. It mirrors the asset
// registry's shape, where one file may declare several agents.
type AgentFile struct {
	New      bool    `yaml:"new,omitempty" json:"new,omitempty"`
	LogLevel string  `yaml:"log_level,omitempty" json:"log_level,omitempty"`
	Agents   []Agent `yaml:"agents" json:"agents"`
}

// Agent is a tool bound to a model, under a nickname.
type Agent struct {
	Name        string   `yaml:"name" json:"name"` // the primary nickname
	Aliases     []string `yaml:"aliases,omitempty" json:"aliases,omitempty"`
	Display     string   `yaml:"display,omitempty" json:"display,omitempty"`
	Description string   `yaml:"description,omitempty" json:"description,omitempty"`

	// Nick is the agent's human name — the one you say out loud. Leave it
	// empty and the catalog assigns one deterministically from the binding,
	// so every agent has a memorable handle without anyone naming it.
	Nick string `yaml:"nick,omitempty" json:"nick,omitempty"`

	Tool  string `yaml:"tool" json:"tool"`   // → Tool.Name
	Model string `yaml:"model" json:"model"` // → Model.Name

	Role        *AgentRole        `yaml:"role,omitempty" json:"role,omitempty"`
	Ledger      *AgentLedger      `yaml:"ledger,omitempty" json:"ledger,omitempty"`
	Instruction *AgentInstruction `yaml:"instruction,omitempty" json:"instruction,omitempty"`
	Functions   []string          `yaml:"functions,omitempty" json:"functions,omitempty"`

	// AutoNick and Derived are computed by the catalog at load: the
	// assigned human name (when Nick is empty) and the floating family
	// alias (`claude-opus` for a binding on `opus4.8`). Both are functions
	// of the whole catalog, so neither is ever persisted.
	AutoNick string   `yaml:"-" json:"auto_nick,omitempty"`
	Derived  []string `yaml:"-" json:"derived,omitempty"`

	Ring assetring.Ring `yaml:"-" json:"ring"`
}

type AgentRole struct {
	Skills       []string `yaml:"skills,omitempty" json:"skills,omitempty"`
	AllowedTools []string `yaml:"allowed_tools,omitempty" json:"allowed_tools,omitempty"`
	Scope        string   `yaml:"scope,omitempty" json:"scope,omitempty"`
}

type AgentLedger struct {
	Reliability string `yaml:"reliability,omitempty" json:"reliability,omitempty"`
	Notes       string `yaml:"notes,omitempty" json:"notes,omitempty"`
}

type AgentInstruction struct {
	Content string `yaml:"content,omitempty" json:"content,omitempty"`
}

// MatrixKey is the agent's identity: tool:model. Every nickname for the
// same binding yields the same key, which is why aliasing never
// fragments the capability matrix.
func (a Agent) MatrixKey() string { return a.Tool + ":" + a.Model }

// Person is a human principal. Standalone-first: a local entry needs no
// account. When the host is paired, Email is the authoritative identity.
type Person struct {
	Handle  string   `yaml:"handle" json:"handle"`
	Aliases []string `yaml:"aliases,omitempty" json:"aliases,omitempty"`
	Display string   `yaml:"display,omitempty" json:"display,omitempty"`
	Email   string   `yaml:"email,omitempty" json:"email,omitempty"`

	// OSUsers maps a host name to this person's account name there. It is
	// deliberately per-host: assuming the local $USER exists on a remote
	// box is the single most common way a cross-host reach fails.
	OSUsers map[string]string `yaml:"os_users,omitempty" json:"os_users,omitempty"`
	// DefaultOSUser is used for hosts absent from OSUsers.
	DefaultOSUser string `yaml:"default_os_user,omitempty" json:"default_os_user,omitempty"`

	Hosts  []string `yaml:"hosts,omitempty" json:"hosts,omitempty"`
	Source string   `yaml:"source,omitempty" json:"source,omitempty"` // local | cloud

	Ring assetring.Ring `yaml:"-" json:"ring"`
}

// OSUserFor returns this person's account name on host, and whether the
// binding was explicit. A false second result means the caller is about
// to guess — say so rather than silently assuming.
func (p Person) OSUserFor(host string) (string, bool) {
	if u, ok := p.OSUsers[host]; ok && u != "" {
		return u, true
	}
	if p.DefaultOSUser != "" {
		return p.DefaultOSUser, true
	}
	return "", false
}

// names returns an entry's canonical name followed by its aliases.
func names(name string, aliases []string) []string {
	out := make([]string, 0, len(aliases)+1)
	if name != "" {
		out = append(out, name)
	}
	seen := map[string]bool{name: true}
	for _, a := range aliases {
		if a != "" && !seen[a] {
			seen[a] = true
			out = append(out, a)
		}
	}
	return out
}

// Names returns the tool's canonical name and every alias.
func (t Tool) Names() []string { return names(t.Name, t.Aliases) }

// Names returns the model's canonical name and every alias it answers to,
// including the catalog-derived family alias.
func (m Model) Names() []string {
	return names(m.Name, append(append([]string{}, m.Aliases...), m.Derived...))
}

// Names returns the agent's canonical nickname and every alias it answers
// to: declared aliases, its human name, and the catalog-derived family
// alias. One list, so every resolver — whois, chat, meet, weave — sees the
// same set of names without knowing which were declared and which derived.
func (a Agent) Names() []string {
	extra := append([]string{}, a.Aliases...)
	if n := a.NickName(); n != "" {
		extra = append(extra, n)
	}
	return names(a.Name, append(extra, a.Derived...))
}

// NickName is the agent's human name: the one it was given, else the one
// the catalog assigned it.
func (a Agent) NickName() string {
	if a.Nick != "" {
		return a.Nick
	}
	return a.AutoNick
}

// Names returns the person's handle and every alias.
func (p Person) Names() []string { return names(p.Handle, p.Aliases) }

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
