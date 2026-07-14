package agentlaunch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/qiangli/coreutils/pkg/agentpty"
	"github.com/qiangli/coreutils/pkg/fleet"
)

// Options controls launch rendering for callers that need to layer local
// policy such as read-only review or a codex sandbox override.
type Options struct {
	Sandbox  string
	ReadOnly bool
	DryRun   bool

	// Steer resolves the tool's INTERACTIVE launch (steer_exec) instead of its
	// headless one-shot, because a one-shot has nothing to interrupt: it runs the
	// prompt and exits before any steer could arrive.
	//
	// Fails loudly when the tool has no steerable launch. A caller that asked to
	// be able to interrupt an agent must not be silently handed one it cannot.
	Steer bool
}

// Launch is a fully resolved agent invocation. Args excludes the prompt.
type Launch struct {
	Nick      string
	Tool      string
	ToolName  string
	Model     string
	ModelName string
	Args      []string

	// TakesPrompt reports whether the prompt goes on the command line. A steerable
	// launch may open an EMPTY session (codex, opencode) and expect the first
	// message to arrive over the control channel instead.
	TakesPrompt bool
}

func (l Launch) Binding() string {
	if l.ModelName == "" {
		return ""
	}
	return l.ToolName + ":" + l.ModelName
}

func (l Launch) Named() bool {
	return l.Nick != "" && l.Nick != l.ToolName && !strings.Contains(l.Nick, ":")
}

func (l Launch) Argv(prompt string) []string {
	out := make([]string, 0, len(l.Args)+2)
	out = append(out, l.Tool)
	out = append(out, l.Args...)
	return append(out, prompt)
}

type LaunchProfile struct {
	Args       []string
	UnsafeArgs []string
}

var SeededProfiles = map[string]LaunchProfile{
	// -p is print mode, and it is stated rather than inferred. Without it claude
	// decides it is headless by looking at whether stdout is a terminal — true on
	// a pipe, FALSE the moment the agent is given a PTY (to clear a trust prompt,
	// or to be steered mid-run), at which point it opens its REPL and waits
	// forever. Every other tool here already declares its headless mode; claude
	// was the only one relying on the guess.
	"claude":   {Args: []string{"-p"}, UnsafeArgs: []string{"--dangerously-skip-permissions"}},
	"codex":    {Args: []string{"exec", "--skip-git-repo-check", "--sandbox", "workspace-write"}},
	"agy":      {Args: []string{"--print-timeout", "40m", "-p"}, UnsafeArgs: []string{"--dangerously-skip-permissions"}},
	"opencode": {Args: []string{"run"}},
	"aider":    {Args: []string{"--no-git", "--message"}, UnsafeArgs: []string{"--yes-always"}},
	"ycode":    {Args: []string{"prompt", "--print"}, UnsafeArgs: []string{"--danger-skip-permissions"}},
}

var NewCatalog = func() *fleet.Catalog { return fleet.New() }

type CatalogFunc func() *fleet.Catalog

func Resolve(name string, opt Options) (Launch, error) {
	return ResolveWithCatalog(name, opt, NewCatalog)
}

func ResolveWithCatalog(name string, opt Options, newCatalog CatalogFunc) (Launch, error) {
	if newCatalog == nil {
		newCatalog = NewCatalog
	}
	lnch := Launch{Nick: name, Tool: name, ToolName: name}
	cat := newCatalog()

	var toolName, modelName string
	namedAgent := false
	if a, ok := cat.Agent(name); ok {
		toolName, modelName, lnch.Nick = a.Tool, a.Model, a.Name
		namedAgent = true
	} else if t, m, ok := strings.Cut(name, ":"); ok && t != "" && m != "" {
		toolName, modelName = t, m
	} else {
		toolName = name
	}
	lnch.Tool, lnch.ToolName = toolName, toolName

	tool, known := cat.Tool(toolName)

	// The model id is resolved AFTER the tool is known, because the id a model
	// answers to is a property of the TOOL: litellm wants `deepseek/deepseek-v4-pro`,
	// ycode wants the bare `deepseek-v4-pro`, agy wants `Gemini 3.1 Pro (High)`.
	// Resolving it first — as this used to — hands somebody the wrong string, and
	// a wrong model id is a dead binding that looks perfectly healthy right up
	// until an agent tries to speak.
	if modelName != "" {
		lnch.Model, lnch.ModelName = modelName, modelName
		if m, ok := cat.Model(modelName); ok {
			lnch.Model, lnch.ModelName = m.TargetFor(toolName), m.Name

		}
	}
	if namedAgent && !known {
		return lnch, fmt.Errorf("agent launch: agent %q names tool %q, which is not in the catalog (see `bashy tools list`)", name, toolName)
	}
	if known {
		lnch.Tool = tool.Binary()
		if lnch.Model != "" && !tool.TakesModel() {
			return lnch, fmt.Errorf("agent launch: tool %q cannot select a model, so %q is a label, not a selection (its launch template has no %s)",
				tool.Name, name, fleet.ModelToken)
		}
		render := tool.ArgvPrefix
		if opt.Steer {
			render = tool.SteerArgvPrefix
		}
		if args, ok := render(lnch.Model); ok {
			out, err := FinalizeArgs(tool.Name, args, opt)
			if err != nil {
				return lnch, err
			}
			lnch.Args = out
			lnch.TakesPrompt = !opt.Steer || tool.SteerTakesPrompt()
			return lnch, nil
		}
		if opt.Steer {
			return lnch, fmt.Errorf("agent launch: %q cannot be steered — tool %q declares no interactive launch (steer_exec). "+
				"Its headless launch is a one-shot: it runs the prompt and exits, so there is nothing to interrupt", name, tool.Name)
		}
	}

	if lnch.Model != "" {
		return lnch, fmt.Errorf("agent launch: no launch template for tool %q, so model %q cannot be passed to it; add it with `bashy tools add`",
			toolName, modelName)
	}

	// A STEER cannot be resolved from a fallback.
	//
	// Below this line is the unknown-tool path: no catalog entry, so no launch
	// template, so no `steer_exec` — we are guessing an argv from a seeded profile
	// or from nothing at all. That guess is a reasonable headless one-shot (it is
	// how you run an agent CLI the registry has never heard of), and it is a
	// catastrophic steerable session: the caller would get a process that exits
	// immediately and a control socket nobody is listening on, and every symptom
	// would look like success.
	//
	// This was the bug. `known` is false here, so the "cannot be steered" check
	// above never ran, and opt.Steer fell straight through to a headless argv —
	// a steerable session resolved by the ABSENCE of a tool definition.
	if opt.Steer {
		if !known {
			return lnch, fmt.Errorf("agent launch: %q cannot be steered — tool %q is not in the catalog, "+
				"so there is no interactive launch to resolve (see `bashy tools list`)", name, toolName)
		}
		return lnch, fmt.Errorf("agent launch: %q cannot be steered — tool %q declares no interactive launch (steer_exec)", name, toolName)
	}

	prof := SeededProfiles[toolName]
	base := append([]string{}, prof.Args...)
	if len(prof.UnsafeArgs) > 0 {
		if ok, _ := UnsafeLaunchAllowed(); ok {
			base = append(append([]string{}, prof.UnsafeArgs...), base...)
		}
	}
	out, err := FinalizeArgs(toolName, base, opt)
	if err != nil {
		return lnch, err
	}
	lnch.Args = out
	lnch.TakesPrompt = true // a headless one-shot always takes its prompt on the command line
	return lnch, nil
}

const UnsafeLaunchEnv = "BASHY_ALLOW_UNSAFE_AGENT_LAUNCH"

var Containerized = func() bool {
	for _, p := range []string{"/.dockerenv", "/run/.containerenv"} {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	return false
}

func UnsafeLaunchAllowed() (bool, string) {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(UnsafeLaunchEnv))) {
	case "", "0", "false", "off", "no":
	default:
		return true, UnsafeLaunchEnv + " is set"
	}
	if Containerized() {
		return true, "running inside a container"
	}
	return false, ""
}

var UnsafeLaunchFlags = map[string]string{
	"--dangerously-skip-permissions":             "disables the agent's approval gate",
	"--dangerously-bypass-approvals-and-sandbox": "disables the agent's approval gate AND its sandbox",
	"--yolo":       "disables the agent's approval gate",
	"--yes-always": "auto-confirms every action, disabling the agent's approval gate",

	// ycode's own. It belongs here for the same reason as everyone else's: the
	// guard must REFUSE it on an uncontained host, and ReadOnly must STRIP it so a
	// judge or a meeting participant cannot touch the thing it is reviewing.
	//
	// It was missing, and the omission was invisible in the only way that matters:
	// ycode's exec template never passed it either, so nothing ever tripped the
	// guard and nothing ever looked wrong. The model would work out the answer,
	// discover it had no write permission, print the code into its reply, and exit
	// 0. Measured in a three-harness bake-off — ycode produced a correct
	// implementation and wrote NOTHING to disk, and it said so:
	//
	//   "I have the implementation ready, but I don't have workspace-write
	//    permissions in this environment to write the file."
	"--danger-skip-permissions": "disables the agent's approval gate",
}

func GuardUnsafeArgs(tool string, args []string) error {
	if ok, _ := UnsafeLaunchAllowed(); ok {
		return nil
	}
	for i, a := range args {
		why, bad := UnsafeLaunchFlags[a]
		if !bad && a == "--sandbox" && i+1 < len(args) && args[i+1] == "danger-full-access" {
			why, bad = "disables the agent's sandbox", true
			a = "--sandbox danger-full-access"
		}
		if !bad {
			continue
		}
		return fmt.Errorf(`agent launch: refusing to launch %q with %s

%s, giving it unattended full access to this machine - and nothing here is
containing it. Choose one:

  - contain it     run the fleet inside a container (e.g. bashy podman), or
  - accept it      %s=1 (explicit, logged risk acceptance)`,
			tool, a, why, UnsafeLaunchEnv)
	}
	return nil
}

func FinalizeArgs(tool string, args []string, opt Options) ([]string, error) {
	if opt.ReadOnly {
		args = ReadOnlyArgs(tool, args)
	}
	args = ApplySandbox(tool, args, opt)
	if opt.DryRun {
		return args, nil
	}
	if err := GuardUnsafeArgs(tool, args); err != nil {
		return nil, err
	}
	return args, nil
}

func ReadOnlyArgs(tool string, args []string) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		if _, unsafe := UnsafeLaunchFlags[args[i]]; unsafe {
			continue
		}
		if args[i] == "--sandbox" && i+1 < len(args) {
			out = append(out, "--sandbox", "read-only")
			i++
			continue
		}
		out = append(out, args[i])
	}
	if tool == "codex" && !containsArg(out, "--sandbox") {
		out = append(out, "--sandbox", "read-only")
	}
	return out
}

func ApplySandbox(agent string, args []string, opt Options) []string {
	if agent == "codex" {
		sb := strings.TrimSpace(opt.Sandbox)
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

func PrincipalEnv(base []string, l Launch) []string {
	if !l.Named() {
		return base
	}
	out := make([]string, 0, len(base)+4)
	for _, kv := range base {
		switch {
		case strings.HasPrefix(kv, "BASHY_PRINCIPAL="),
			strings.HasPrefix(kv, "BASHY_AGENT_ID="),
			strings.HasPrefix(kv, "BASHY_AGENT_BINDING="):
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

type launchKey struct{}

func WithLaunch(ctx context.Context, l Launch) context.Context {
	return context.WithValue(ctx, launchKey{}, l)
}

func LaunchFrom(ctx context.Context) (Launch, bool) {
	l, ok := ctx.Value(launchKey{}).(Launch)
	return l, ok
}

// SendControlFrame is agentpty.SendFrame.
//
// This package used to carry its own copy of the transport — the same twenty
// lines, dialling the same socket, with the same file fallback. Two copies of one
// protocol is one protocol that will drift, and the comment in agentpty explaining
// why they were kept apart ("a terminal does not need to know what claude is") had
// the dependency backwards: agentpty depends on nothing, so anyone may import IT.
func SendControlFrame(path, frame string) error {
	return agentpty.SendFrame(path, frame)
}

func SendControlLine(path, line string) error {
	return SendControlFrame(path, line+"\n")
}

func SendJSONControl(path string, command, ack any, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	conn, err := net.DialTimeout("unix", path, timeout)
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := json.NewEncoder(conn).Encode(command); err != nil {
		return err
	}
	if ack == nil {
		return nil
	}
	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	return json.NewDecoder(conn).Decode(ack)
}

func containsArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

func replaceSandboxFlag(args []string, flag string) []string {
	out := make([]string, 0, len(args)+1)
	for i := 0; i < len(args); i++ {
		if args[i] == "--sandbox" {
			i++
			continue
		}
		out = append(out, args[i])
	}
	return append(out, flag)
}

var ErrNoAgent = errors.New("agent launch: not an agent")
