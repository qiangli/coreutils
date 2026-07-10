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
type Result struct {
	SchemaVersion string   `json:"schema_version"`
	Agent         string   `json:"agent"`
	Role          string   `json:"role,omitempty"`
	Cwd           string   `json:"cwd,omitempty"`
	Args          []string `json:"args,omitempty"`
	DryRun        bool     `json:"dry_run,omitempty"`
	ExitCode      int      `json:"exit_code"`
	Output        string   `json:"output,omitempty"`
}

// LaunchProfile is the minimal headless launch contract needed by chat.
type LaunchProfile struct {
	Args []string
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
	if forceAgentShell() {
		if bashy, err := os.Executable(); err == nil && bashy != "" {
			cmd.Env = forcedShellEnv(os.Environ(), bashy, ensureShims(bashy))
		}
	}
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

var seededProfiles = map[string]LaunchProfile{
	"claude":   {Args: []string{"--dangerously-skip-permissions"}},
	"codex":    {Args: []string{"exec", "--skip-git-repo-check", "--sandbox", "workspace-write"}},
	"agy":      {Args: []string{"--dangerously-skip-permissions", "--print-timeout", "40m", "-p"}},
	"opencode": {Args: []string{"run"}},
	// aider takes its prompt via --message; resolveLaunchArgs appends the prompt
	// as the final arg, so it becomes the --message value. --yes-always makes it
	// non-interactive; --no-git keeps a turn advisory (no repo/commit writes).
	"aider": {Args: []string{"--yes-always", "--no-git", "--message"}},
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
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opt.Instruction = strings.TrimSpace(strings.Join(append([]string{opt.Instruction}, args...), " "))
			}
			// --capability routes to the best-fit ROUTABLE agent from the living
			// matrix (see pkg/capability). We dispatch by tool (the model comes
			// from the tool's own config); the intended tool:model is logged.
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
				opt.Agent = capability.ToolOf(best.Agent)
				fmt.Fprintf(cmd.ErrOrStderr(), "chat: capability %s → %s (q=%.2f); launching tool %q\n",
					c, best.Agent, best.Cell.Quality, opt.Agent)
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
	agent, err := ResolveAgent(opt.Agent, opt.Role)
	if err != nil {
		return Result{SchemaVersion: schemaVersion, Agent: opt.Agent, Role: opt.Role, ExitCode: 2}, err
	}
	prompt, err := BuildPrompt(opt)
	if err != nil {
		return Result{SchemaVersion: schemaVersion, Agent: agent, Role: opt.Role, ExitCode: 2}, err
	}
	args := append(resolveLaunchArgs(agent, opt), prompt)
	cwd := opt.Cwd
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	res := Result{
		SchemaVersion: schemaVersion,
		Agent:         agent,
		Role:          opt.Role,
		Cwd:           cwd,
		Args:          args,
		DryRun:        opt.DryRun,
	}
	if opt.DryRun {
		res.ExitCode = 0
		res.Output = strings.Join(append([]string{agent}, args...), " ")
		return res, nil
	}
	if opt.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opt.Timeout)
		defer cancel()
	}
	out, code, err := runner.Run(ctx, agent, args, cwd)
	res.Output, res.ExitCode = out, code
	return res, err
}

func resolveLaunchArgs(agent string, opt Options) []string {
	profile := seededProfiles[agent]
	args := append([]string{}, profile.Args...)
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
