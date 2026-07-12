package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/qiangli/coreutils/pkg/capability"
	"github.com/qiangli/coreutils/pkg/fleet"
	"github.com/spf13/cobra"
)

const schemaVersion = "bashy-chat-v1"

// Options describes one unattended agent invocation. It is exported so workflow
// commands such as sdlc can use the same primitive as human operators.
type Options struct {
	Agent       string
	Role        string
	Instruction string
	Files       []string
	Context     []string
	Cwd         string
	Timeout     time.Duration
	Sandbox     string
	JSON        bool
	DryRun      bool
}

// Result is the stable envelope returned by Invoke and optionally printed by
// the CLI.
//
// Agent stays the executable that ran, so every existing consumer keeps
// working. Nick and Model are additive: they record what the caller asked for
// and which inference backend was actually selected.
type Result struct {
	SchemaVersion string   `json:"schema_version"`
	Agent         string   `json:"agent"`
	Nick          string   `json:"nick,omitempty"`  // the name the caller used: 007, claude:opus, claude
	Model         string   `json:"model,omitempty"` // the provider-side id passed to the tool
	Role          string   `json:"role,omitempty"`
	Cwd           string   `json:"cwd,omitempty"`
	Args          []string `json:"args,omitempty"`
	DryRun        bool     `json:"dry_run,omitempty"`
	ExitCode      int      `json:"exit_code"`
	Output        string   `json:"output,omitempty"`
}

// LaunchProfile is the minimal headless launch contract needed by chat.
//
// Deprecated: launch contracts now live in the fleet registry
// (coreutils/pkg/fleet), where one declaration serves chat, weave, and the
// capability matrix. seededProfiles remains only as the fallback for a tool
// the registry does not know.
type LaunchProfile struct {
	Args []string
}

// Launch is a fully resolved invocation: which binary, which model, which argv.
//
// Tool and Model are what the OS needs — an executable and the provider's own
// model id. ToolName and ModelName are what the REGISTRY calls them, and they
// are what attribution records: a binding written with a binary path would not
// match the capability matrix, whose rows are keyed by tool:model.
type Launch struct {
	Nick      string   // canonical agent nickname, or the bare name the caller typed
	Tool      string   // the executable to run
	ToolName  string   // the tool's registry name
	Model     string   // provider-side model id ("" when none was selected)
	ModelName string   // the model's registry alias
	Args      []string // argv after the binary, prompt NOT included
}

// Binding is the capability-matrix key for this launch, or "" when no model
// was selected.
func (l Launch) Binding() string {
	if l.ModelName == "" {
		return ""
	}
	return l.ToolName + ":" + l.ModelName
}

// Runner starts an agent process. Tests and higher-level workflows can replace
// it without spawning a real agent.
type Runner interface {
	Run(ctx context.Context, agent string, args []string, cwd string) (string, int, error)
}

type execRunner struct{}

func (execRunner) Run(ctx context.Context, agent string, args []string, cwd string) (string, int, error) {
	// macOS: a cask/download-installed agent (e.g. codex) carries
	// com.apple.quarantine, so a background/CI launch (act_runner) hangs on the
	// Gatekeeper "downloaded from the Internet" popup. The operator explicitly
	// configured this agent as the conductor — strip the quarantine best-effort
	// so the headless launch proceeds. No-op off darwin / when already clear.
	if p, err := exec.LookPath(agent); err == nil {
		stripQuarantine(p)
	}
	cmd := exec.CommandContext(ctx, agent, args...)
	if cwd != "" {
		cmd.Dir = cwd
	}
	// Force the spawned agent to run its shell commands through bashy (this
	// running binary) rather than the system shell — so the pure-Go userland,
	// the space-time advisor, and OTel apply to every command the agent runs.
	// Covers claude (CLAUDE_CODE_SHELL), aider/opencode ($SHELL), and agy and any
	// bare-name `bash`/`sh`/`zsh` shell-out (PATH shim). codex reads /etc/passwd
	// and is unreachable this way — see `bashy install-agent codex`. On by
	// default; BASHY_FORCE_AGENT_SHELL=0 disables. cmd.Path is already resolved
	// against the real PATH, so prepending the shim dir never shadows the agent.
	env := os.Environ()
	if forceAgentShell() {
		if bashy, err := os.Executable(); err == nil && bashy != "" {
			env = forcedShellEnv(env, bashy, ensureShims(bashy))
		}
	}
	if l, ok := LaunchFrom(ctx); ok {
		env = principalEnv(env, l)
	}
	cmd.Env = env
	// Capture stdout and stderr SEPARATELY. The agent's actual answer is on
	// stdout; CLI chrome (banners, warnings, progress) goes to stderr and would
	// otherwise pollute a captured turn — and a truncated multibyte char in that
	// chrome becomes invalid UTF-8 that crashes a downstream tool when the turn is
	// replayed as its prompt. On success we return stdout only; on failure we
	// append stderr so the error is still visible to the caller.
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	// Because Stdout/Stderr are buffers rather than *os.File, os/exec pipes them
	// through copying goroutines, and Wait blocks until EVERY writer closes the
	// pipe. An agent CLI that spawns children (a shell, a language server) leaves
	// those children holding it, so killing the agent on ctx cancellation does NOT
	// unblock Wait — the "a wedged agent can't hang the round" timeout silently
	// waits for the grandchild instead. WaitDelay bounds that: after the context
	// ends, Wait gives the pipes this long to drain, then closes them and returns.
	cmd.WaitDelay = 5 * time.Second
	err := cmd.Run()
	out := stdout.String()
	if ctx.Err() != nil {
		return appendStderr(out, stderr.String()), 124, ctx.Err()
	}
	if err == nil {
		return out, 0, nil
	}
	out = appendStderr(out, stderr.String())
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return out, exitErr.ExitCode(), err
	}
	return out, 127, err
}

// appendStderr joins captured stderr onto stdout for error reporting.
func appendStderr(stdout, stderr string) string {
	stderr = strings.TrimSpace(stderr)
	if stderr == "" {
		return stdout
	}
	stdout = strings.TrimRight(stdout, "\n")
	if stdout != "" {
		stdout += "\n"
	}
	return stdout + stderr
}

// seededProfiles is the LAST-RESORT launch contract, for a tool the fleet
// registry has never heard of. The registry (coreutils/pkg/fleet) is the
// source of truth; these rows exist so an operator who names an unregistered
// binary still gets the behavior they had before the registry existed.
//
// Do not add rows here. Add a tool to the registry instead — a row here can
// only describe how to start a binary, never which model to give it.
var seededProfiles = map[string]LaunchProfile{
	"claude":   {Args: []string{"--dangerously-skip-permissions"}},
	"codex":    {Args: []string{"exec", "--skip-git-repo-check", "--sandbox", "workspace-write"}},
	"agy":      {Args: []string{"--dangerously-skip-permissions", "--print-timeout", "40m", "-p"}},
	"opencode": {Args: []string{"run"}},
	// aider takes its prompt via --message; the launcher appends the prompt as
	// the final arg, so it becomes the --message value. --yes-always makes it
	// non-interactive; --no-git keeps a turn advisory (no repo/commit writes).
	"aider": {Args: []string{"--yes-always", "--no-git", "--message"}},
}

// newCatalog builds the fleet catalog the launcher resolves against. It is a
// var so tests can pin it to a scratch store instead of the developer's own.
var newCatalog = func() *fleet.Catalog { return fleet.New() }

// resolveLaunch turns a name into a runnable invocation.
//
// The name may be an agent nickname (007), a bare tool:model binding
// (claude:opus), or a plain tool (claude). Only the first two can select a
// model — which is the whole point: before the registry, `claude:opus` was a
// label the launcher logged and threw away, and every run silently used
// whatever model the tool's own config happened to name.
func resolveLaunch(name string, opt Options) (Launch, error) {
	lnch := Launch{Nick: name, Tool: name, ToolName: name}
	cat := newCatalog()

	// An agent nickname resolves to both halves of its binding. Catalog.Agent
	// also accepts a bare tool:model, so `claude:opus` finds the seeded agent.
	//
	// When it does, the CANONICAL nickname becomes the identity, not the string
	// the caller typed. Attribution has to record a name that resolves: an
	// agent stamped as `claude:opus` could never be written `@claude:opus` in
	// prose, because a mention cannot contain a colon.
	var toolName, modelName string
	if a, ok := cat.Agent(name); ok {
		toolName, modelName, lnch.Nick = a.Tool, a.Model, a.Name
	} else if t, m, ok := strings.Cut(name, ":"); ok && t != "" && m != "" {
		// A binding nobody has nicknamed yet.
		toolName, modelName = t, m
	} else {
		toolName = name
	}
	lnch.Tool, lnch.ToolName = toolName, toolName

	// The id handed to --model is the provider's, not ours: opencode wants
	// `deepseek/deepseek-v4`, not the alias `deepseek-v4`. An unregistered
	// model passes through verbatim rather than being dropped.
	if modelName != "" {
		lnch.Model, lnch.ModelName = modelName, modelName
		if m, ok := cat.Model(modelName); ok {
			lnch.Model, lnch.ModelName = m.Target(), m.Name
		}
	}

	tool, known := cat.Tool(toolName)
	if known {
		lnch.Tool = tool.Binary()
		if lnch.Model != "" && !tool.TakesModel() {
			return lnch, fmt.Errorf("chat: tool %q cannot select a model, so %q is a label, not a selection (its launch template has no %s)",
				tool.Name, name, fleet.ModelToken)
		}
		if args, ok := tool.ArgvPrefix(lnch.Model); ok {
			out, err := finalizeArgs(tool.Name, args, opt)
			if err != nil {
				return lnch, err
			}
			lnch.Args = out
			return lnch, nil
		}
	}

	// Fallback: a tool the registry does not describe. It cannot take a model.
	if lnch.Model != "" {
		return lnch, fmt.Errorf("chat: no launch template for tool %q, so model %q cannot be passed to it; add it with `bashy tools add`",
			toolName, modelName)
	}
	out, err := finalizeArgs(toolName, append([]string{}, seededProfiles[toolName].Args...), opt)
	if err != nil {
		return lnch, err
	}
	lnch.Args = out
	return lnch, nil
}

// forceAgentShell reports whether the launcher routes a spawned agent's shell
// through bashy. On by default; BASHY_FORCE_AGENT_SHELL=0 disables (and --posix
// hosts / the lean `bash` binary never reach this path).
func forceAgentShell() bool { return os.Getenv("BASHY_FORCE_AGENT_SHELL") != "0" }

// shimDir is the directory of sh/bash/zsh symlinks to the bashy binary, prepended
// to a spawned agent's PATH so bare-name shell lookups resolve to bashy. Override
// with BASHY_SHIM_DIR (used by tests).
func shimDir() string {
	if d := os.Getenv("BASHY_SHIM_DIR"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".bashy", "shims")
}

// ensureShims makes shimDir hold sh/bash/zsh symlinks to bashy (idempotent,
// best-effort). No-op on Windows (POSIX shell names don't apply) or when the dir
// is unavailable; returns "" in those cases so forcedShellEnv skips the PATH shim.
func ensureShims(bashy string) string {
	if runtime.GOOS == "windows" || bashy == "" {
		return ""
	}
	dir := shimDir()
	if dir == "" || os.MkdirAll(dir, 0o755) != nil {
		return ""
	}
	for _, name := range []string{"sh", "bash", "zsh"} {
		link := filepath.Join(dir, name)
		if target, err := os.Readlink(link); err == nil && target == bashy {
			continue
		}
		_ = os.Remove(link)
		_ = os.Symlink(bashy, link)
	}
	return dir
}

// forcedShellEnv returns base with the bashy-shell routing vars applied: shimDir
// prepended to PATH (when non-empty), and SHELL + CLAUDE_CODE_SHELL pinned to
// bashy. Pure and deterministic so it can be tested without spawning a process.
func forcedShellEnv(base []string, bashy, shimDir string) []string {
	out := make([]string, 0, len(base)+2)
	pathSet := false
	for _, kv := range base {
		switch {
		case shimDir != "" && strings.HasPrefix(kv, "PATH="):
			out = append(out, "PATH="+shimDir+string(os.PathListSeparator)+kv[len("PATH="):])
			pathSet = true
		case strings.HasPrefix(kv, "SHELL="), strings.HasPrefix(kv, "CLAUDE_CODE_SHELL="):
			// re-added canonically below
		default:
			out = append(out, kv)
		}
	}
	if shimDir != "" && !pathSet {
		out = append(out, "PATH="+shimDir)
	}
	if runtime.GOOS != "windows" {
		out = append(out, "SHELL="+bashy)
	}
	out = append(out, "CLAUDE_CODE_SHELL="+bashy)
	return out
}

// principalEnv tells the spawned process who it is.
//
// The child can only sign its work with what the launcher gave it, which is
// what makes "agent 007 commented" trustworthy inside one host: forging the
// name means already controlling the launcher. Nothing is stamped for a bare
// tool — a tool is not an agent, and inventing a nickname for it would put a
// name in the record that resolves to nothing.
func principalEnv(base []string, l Launch) []string {
	// A raw `tool:model` binding nobody has nicknamed is not stampable either:
	// a mention cannot contain a colon, so `@claude:opus` would never resolve.
	// Mint a nickname (`bashy agents add`) to give the run a name.
	//
	// The comparison is against the tool's NAME, not its executable: a tool
	// whose binary differs from its name (cursor → cursor-agent) is still a
	// bare tool, and stamping it as an agent would invent a principal.
	if l.Nick == "" || l.Nick == l.ToolName || strings.Contains(l.Nick, ":") {
		return base
	}
	out := make([]string, 0, len(base)+4)
	for _, kv := range base {
		switch {
		case strings.HasPrefix(kv, "BASHY_PRINCIPAL="),
			strings.HasPrefix(kv, "BASHY_AGENT_ID="),
			strings.HasPrefix(kv, "BASHY_AGENT_BINDING="):
			// re-stamped below; a nested launch must not inherit its parent
		default:
			out = append(out, kv)
		}
	}
	out = append(out,
		"BASHY_PRINCIPAL=dhnt:agent/"+l.Nick,
		"BASHY_AGENT_ID="+l.Nick,
	)
	if b := l.Binding(); b != "" {
		out = append(out, "BASHY_AGENT_BINDING="+b)
	}
	return out
}

var roleDefaults = map[string]string{
	"conductor": "claude",
	"reviewer":  "codex",
	"qa":        "codex",
	"release":   "claude",
}

// NewChatCmd returns the `bashy chat` command.
func NewChatCmd() *cobra.Command {
	var opt Options
	var capStr string
	cmd := &cobra.Command{
		Use:   "chat --agent AGENT --instruction TEXT",
		Short: "invoke an agent with a single unattended instruction",
		// A failed launch is a runtime error, not a usage error. Dumping the
		// flag list on "this tool cannot select a model" buries the sentence
		// that explains what to do about it.
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opt.Instruction = strings.TrimSpace(strings.Join(append([]string{opt.Instruction}, args...), " "))
			}
			// --capability routes to the best-fit ROUTABLE agent from the living
			// matrix (see pkg/capability). The matrix is keyed by tool:model, and
			// that whole binding is what we launch: the router picked a model for
			// a reason, and dispatching by tool alone would silently run whatever
			// the tool's own config happened to name.
			if strings.TrimSpace(capStr) != "" && opt.Agent == "" {
				c, ok := capability.ParseCapability(capStr)
				if !ok {
					return fmt.Errorf("chat: unknown capability %q", capStr)
				}
				m, err := capability.Load()
				if err != nil {
					return err
				}
				ranked := m.Best(c, true, capability.ByValue)
				if len(ranked) == 0 {
					return fmt.Errorf("chat: no routable agent for capability %q", capStr)
				}
				best := ranked[0]
				opt.Agent = best.Agent
				fmt.Fprintf(cmd.ErrOrStderr(), "chat: capability %s → %s (q=%.2f)\n",
					c, best.Agent, best.Cell.Quality)
			}
			plain, _ := cmd.Flags().GetBool("plain")
			opt.JSON = opt.JSON || (os.Getenv("BASHY_AGENTIC") != "" && !plain)
			res, err := Invoke(cmd.Context(), opt, execRunner{})
			if opt.JSON {
				b, _ := json.Marshal(res)
				fmt.Fprintln(cmd.OutOrStdout(), string(b))
			} else if res.Output != "" {
				fmt.Fprint(cmd.OutOrStdout(), res.Output)
				if !strings.HasSuffix(res.Output, "\n") {
					fmt.Fprintln(cmd.OutOrStdout())
				}
			}
			return err
		},
	}
	cmd.CompletionOptions.DisableDefaultCmd = true
	cmd.Flags().StringVar(&opt.Agent, "agent", "", "agent command to run, such as claude, codex, agy, or opencode")
	cmd.Flags().StringVar(&opt.Role, "role", "", "role alias when --agent is omitted: conductor, reviewer, qa, release")
	cmd.Flags().StringVar(&capStr, "capability", "", "route to the best-fit routable agent for this capability (e.g. deep-research, coding)")
	cmd.Flags().StringVar(&opt.Instruction, "instruction", "", "instruction to send to the agent")
	cmd.Flags().StringArrayVar(&opt.Files, "file", nil, "append file contents to the instruction")
	cmd.Flags().StringArrayVar(&opt.Context, "context", nil, "append context text to the instruction")
	cmd.Flags().StringVar(&opt.Cwd, "cwd", "", "working directory for the agent process")
	cmd.Flags().DurationVar(&opt.Timeout, "timeout", 0, "agent timeout, for example 30m")
	cmd.Flags().StringVar(&opt.Sandbox, "sandbox", "", "agent sandbox override, for example workspace-write or danger-full-access")
	cmd.Flags().BoolVar(&opt.JSON, "json", false, "print a bashy-chat-v1 JSON result envelope")
	cmd.Flags().Bool("plain", false, "force plain output even under BASHY_AGENTIC")
	cmd.Flags().BoolVar(&opt.DryRun, "dry-run", false, "print the resolved invocation without running the agent")
	_ = cmd.Flags().MarkHidden("plain")
	return cmd
}

// Invoke resolves the agent, builds the prompt, and runs it.
func Invoke(ctx context.Context, opt Options, runner Runner) (Result, error) {
	if runner == nil {
		runner = execRunner{}
	}
	name, err := ResolveAgent(opt.Agent, opt.Role)
	if err != nil {
		return Result{SchemaVersion: schemaVersion, Agent: opt.Agent, Role: opt.Role, ExitCode: 2}, err
	}
	lnch, err := resolveLaunch(name, opt)
	if err != nil {
		return Result{SchemaVersion: schemaVersion, Agent: lnch.Tool, Nick: name, Role: opt.Role, ExitCode: 2}, err
	}
	prompt, err := BuildPrompt(opt)
	if err != nil {
		return Result{SchemaVersion: schemaVersion, Agent: lnch.Tool, Nick: name, Role: opt.Role, ExitCode: 2}, err
	}
	args := append(lnch.Args, prompt)
	cwd := opt.Cwd
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	res := Result{
		SchemaVersion: schemaVersion,
		Agent:         lnch.Tool,
		Role:          opt.Role,
		Cwd:           cwd,
		Args:          args,
		DryRun:        opt.DryRun,
	}
	if lnch.Nick != lnch.ToolName {
		res.Nick = lnch.Nick
	}
	res.Model = lnch.Model
	if opt.DryRun {
		res.ExitCode = 0
		res.Output = strings.Join(append([]string{lnch.Tool}, args...), " ")
		return res, nil
	}
	if opt.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opt.Timeout)
		defer cancel()
	}
	// The launcher is the only place that knows which principal is about to
	// act, so it is the only place that can tell the spawned process who it
	// is. execRunner reads this back out to stamp the child's environment.
	out, code, err := runner.Run(withLaunch(ctx, lnch), lnch.Tool, args, cwd)
	res.Output, res.ExitCode = out, code
	return res, err
}

// launchKey carries the resolved Launch to execRunner without widening the
// Runner interface, which meet, foreman, and sdlc all implement against.
type launchKey struct{}

func withLaunch(ctx context.Context, l Launch) context.Context {
	return context.WithValue(ctx, launchKey{}, l)
}

// LaunchFrom returns the invocation being run, when the caller came through
// Invoke. A Runner built by hand sees no value and simply skips attribution.
func LaunchFrom(ctx context.Context) (Launch, bool) {
	l, ok := ctx.Value(launchKey{}).(Launch)
	return l, ok
}

// unsafeLaunchFlags are the flags by which an agent CLI's OWN approval gate is
// switched off. Passing one hands the agent unattended, unreviewed access to
// whatever the launching process can reach — so each is legitimate only when
// something ELSE is already containing the agent (a container), and is a
// self-inflicted wound otherwise. Their vendors named them "dangerously"; we
// take that literally.
//
// codex's `--sandbox danger-full-access` is here for the same reason: it turns
// codex's built-in sandbox off. `--sandbox workspace-write` (the default) is
// codex sandboxing itself and is NOT unsafe.
var unsafeLaunchFlags = map[string]string{
	"--dangerously-skip-permissions":             "disables the agent's approval gate",
	"--dangerously-bypass-approvals-and-sandbox": "disables the agent's approval gate AND its sandbox",
	"--yolo": "disables the agent's approval gate",
}

// UnsafeLaunchEnv opts a host into launching agents with their own safety
// systems disabled. It is the operator's explicit, auditable acceptance of the
// risk — never a default.
const UnsafeLaunchEnv = "BASHY_ALLOW_UNSAFE_AGENT_LAUNCH"

// containerized reports whether this process is already inside an OCI
// container, i.e. whether something else is containing the agent. A var so
// tests can simulate a contained host.
//
// This deliberately re-probes on every call instead of reading the shared
// spacetime probe cache: that cache is a user-writable file, and a security
// gate must not be unlockable by writing `"container":"true"` into it. The
// signals mirror spacetime's own probeContainer.
var containerized = func() bool {
	for _, p := range []string{"/.dockerenv", "/run/.containerenv"} {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	return false
}

// unsafeLaunchAllowed reports whether stripping an agent's safety systems is
// permissible here, and why.
func unsafeLaunchAllowed() (bool, string) {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(UnsafeLaunchEnv))) {
	case "", "0", "false", "off", "no":
	default:
		return true, UnsafeLaunchEnv + " is set"
	}
	if containerized() {
		return true, "running inside a container"
	}
	return false, ""
}

// guardUnsafeArgs refuses a launch that would disable an agent CLI's own safety
// systems on a host where nothing else is containing it.
//
// This is the ONE choke point for it: every launch — registry-templated or
// seeded-fallback — renders its argv through resolveLaunch, so a dangerous flag
// cannot reach an agent by any other route, including a `bashy tools add`
// template written later.
//
// It refuses rather than silently stripping the flag: stripping would leave a
// headless agent blocking forever on an approval prompt nobody can answer, and
// a hang is a worse failure than a clear error. The operator gets a one-line fix.
func guardUnsafeArgs(tool string, args []string) error {
	if ok, _ := unsafeLaunchAllowed(); ok {
		return nil
	}
	for i, a := range args {
		why, bad := unsafeLaunchFlags[a]
		// codex spells its kill-switch as a flag PAIR.
		if !bad && a == "--sandbox" && i+1 < len(args) && args[i+1] == "danger-full-access" {
			why, bad = "disables the agent's sandbox", true
			a = "--sandbox danger-full-access"
		}
		if !bad {
			continue
		}
		return fmt.Errorf(`chat: refusing to launch %q with %s

%s, giving it unattended full access to this machine — and nothing here is
containing it. Choose one:

  • contain it     run the fleet inside a container (e.g. bashy podman), or
  • accept it      %s=1 (explicit, logged risk acceptance)`,
			tool, a, why, UnsafeLaunchEnv)
	}
	return nil
}

// finalizeArgs applies the --sandbox override, then gates the result. Both
// launch paths (registry template and seeded fallback) go through here.
func finalizeArgs(tool string, args []string, opt Options) ([]string, error) {
	args = applySandbox(tool, args, opt)
	// A dry run renders the argv but never executes it, so there is nothing to
	// contain. Gating it would be backwards: seeing the dangerous flag printed is
	// exactly how an operator discovers it is there.
	if opt.DryRun {
		return args, nil
	}
	if err := guardUnsafeArgs(tool, args); err != nil {
		return nil, err
	}
	return args, nil
}

// applySandbox layers the --sandbox override onto an already-rendered argv.
// The prompt is not yet present, so appending a flag pair is safe here.
func applySandbox(agent string, args []string, opt Options) []string {
	if agent == "codex" {
		sb := strings.TrimSpace(opt.Sandbox)
		// danger-full-access is the conductor's unattended full-access mode. Plain
		// `--sandbox danger-full-access` STILL shows codex approval/trust prompts —
		// a GUI popup that hangs a headless runner (act_runner / CI). codex's
		// --dangerously-bypass-approvals-and-sandbox is the documented fully
		// non-interactive equivalent, "intended solely for externally sandboxed
		// environments" (which the owner-gated act_runner is).
		if sb == "danger-full-access" {
			return replaceSandboxFlag(args, "--dangerously-bypass-approvals-and-sandbox")
		}
		if sb != "" {
			for i := 0; i < len(args)-1; i++ {
				if args[i] == "--sandbox" {
					args[i+1] = sb
					return args
				}
			}
			args = append(args, "--sandbox", sb)
		}
	}
	return args
}

// replaceSandboxFlag drops any `--sandbox <val>` pair and appends a standalone
// flag (e.g. --dangerously-bypass-approvals-and-sandbox).
func replaceSandboxFlag(args []string, flag string) []string {
	out := make([]string, 0, len(args)+1)
	for i := 0; i < len(args); i++ {
		if args[i] == "--sandbox" {
			i++ // skip its value too
			continue
		}
		out = append(out, args[i])
	}
	return append(out, flag)
}

// ResolveAgent maps either an explicit agent or a workflow role to a command.
func ResolveAgent(agent, role string) (string, error) {
	agent = strings.TrimSpace(agent)
	if agent != "" {
		return agent, nil
	}
	role = strings.TrimSpace(strings.ToLower(role))
	if role == "" {
		return "", errors.New("chat: --agent or --role is required")
	}
	if a := roleDefaults[role]; a != "" {
		return a, nil
	}
	return "", fmt.Errorf("chat: unknown role %q", role)
}

// BuildPrompt composes the user instruction, inline context, and file snippets.
func BuildPrompt(opt Options) (string, error) {
	var b bytes.Buffer
	if s := strings.TrimSpace(opt.Instruction); s != "" {
		b.WriteString(s)
		b.WriteString("\n")
	}
	for _, c := range opt.Context {
		if c = strings.TrimSpace(c); c != "" {
			fmt.Fprintf(&b, "\nContext:\n%s\n", c)
		}
	}
	for _, name := range opt.Files {
		if strings.TrimSpace(name) == "" {
			continue
		}
		clean := filepath.Clean(name)
		data, err := os.ReadFile(clean)
		if err != nil {
			return "", fmt.Errorf("chat: read %s: %w", name, err)
		}
		fmt.Fprintf(&b, "\nFile %s:\n", clean)
		_, _ = io.Copy(&b, bytes.NewReader(data))
		if !bytes.HasSuffix(data, []byte("\n")) {
			b.WriteByte('\n')
		}
	}
	prompt := strings.TrimSpace(b.String())
	if prompt == "" {
		return "", errors.New("chat: --instruction, positional text, --context, or --file is required")
	}
	return prompt, nil
}
