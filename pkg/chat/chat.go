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
	"strings"
	"time"

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
	cmd := exec.CommandContext(ctx, agent, args...)
	if cwd != "" {
		cmd.Dir = cwd
	}
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return string(out), 124, ctx.Err()
	}
	if err == nil {
		return string(out), 0, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return string(out), exitErr.ExitCode(), err
	}
	return string(out), 127, err
}

var seededProfiles = map[string]LaunchProfile{
	"claude":   {Args: []string{"--dangerously-skip-permissions"}},
	"codex":    {Args: []string{"exec", "--skip-git-repo-check", "--sandbox", "workspace-write"}},
	"agy":      {Args: []string{"--dangerously-skip-permissions", "--print-timeout", "40m", "-p"}},
	"opencode": {Args: []string{"run"}},
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
	cmd := &cobra.Command{
		Use:   "chat --agent AGENT --instruction TEXT",
		Short: "invoke an agent with a single unattended instruction",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opt.Instruction = strings.TrimSpace(strings.Join(append([]string{opt.Instruction}, args...), " "))
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
	cmd.Flags().StringVar(&opt.Instruction, "instruction", "", "instruction to send to the agent")
	cmd.Flags().StringArrayVar(&opt.Files, "file", nil, "append file contents to the instruction")
	cmd.Flags().StringArrayVar(&opt.Context, "context", nil, "append context text to the instruction")
	cmd.Flags().StringVar(&opt.Cwd, "cwd", "", "working directory for the agent process")
	cmd.Flags().DurationVar(&opt.Timeout, "timeout", 0, "agent timeout, for example 30m")
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
	profile := seededProfiles[agent]
	args := append(append([]string{}, profile.Args...), prompt)
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
