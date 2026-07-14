package foremancmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/qiangli/coreutils/pkg/chat"
	"github.com/qiangli/coreutils/pkg/foreman"
	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "foreman",
	Synopsis: "Drive a persistent, steerable agent session.",
	Usage:    "foreman start [--detach] --goal TEXT [--agent AGENT]\n   or: foreman tell <id> TEXT\n   or: foreman status <id>\n   or: foreman list\n   or: foreman --once --agent AGENT --instruction TEXT",
}

var runner chat.Runner

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	if rc.Ctx == nil {
		rc.Ctx = context.Background()
	}
	if len(args) == 0 {
		return runREPLWithFlags(rc, nil, nil)
	}
	// --help/-h and --version short-circuit before the manual flag parser,
	// which would otherwise treat "--help" as a value-taking flag and drop
	// into the interactive REPL (no rest args). The framework does not strip
	// these because this tool does its own arg dispatch.
	for _, a := range args {
		switch a {
		case "--help", "-h":
			fmt.Fprintln(rc.Out, cmd.Usage)
			return 0
		case "--version":
			fmt.Fprintf(rc.Out, "foreman %s\n", tool.Version)
			return 0
		}
	}
	global, rest := parseKVFlags(args)
	if global["once"] == "true" {
		return runOnce(rc, global)
	}
	if len(rest) == 0 {
		return runREPLWithFlags(rc, global, nil)
	}
	sub := rest[0]
	subFlags, subArgs := parseKVFlags(rest[1:])
	jsonOut := global["json"] == "true" || subFlags["json"] == "true"
	switch sub {
	case "start":
		return runStart(rc, subFlags, subArgs, jsonOut)
	case "serve":
		return runServe(rc, subArgs)
	case "tell":
		return runTell(rc, subArgs, jsonOut)
	case "status":
		return runStatus(rc, subArgs, jsonOut)
	case "list":
		return runList(rc, jsonOut)
	case "pause", "resume", "skip", "stop":
		return runControl(rc, sub, subArgs, jsonOut)
	case "prio":
		return runPrio(rc, subArgs, jsonOut)
	case "run":
		if len(subArgs) > 0 && strings.HasSuffix(strings.ToLower(subArgs[0]), ".md") {
			return runDAG(rc, subFlags, subArgs, jsonOut)
		}
		return runREPLWithFlags(rc, subFlags, subArgs)
	default:
		return usage(rc, "unknown subcommand %q", sub)
	}
}

func runOnce(rc *tool.RunContext, flags map[string]string) int {
	res, err := chat.Invoke(rc.Ctx, chat.Options{
		Agent:       flags["agent"],
		Role:        flags["role"],
		Instruction: flags["instruction"],
		Cwd:         rc.Dir,
		JSON:        flags["json"] == "true",
	}, runner)
	if flags["json"] == "true" {
		return emitJSON(rc, res)
	}
	if res.Output != "" {
		fmt.Fprint(rc.Out, res.Output)
		if !strings.HasSuffix(res.Output, "\n") {
			fmt.Fprintln(rc.Out)
		}
	}
	if err != nil {
		fmt.Fprintln(rc.Err, err)
	}
	return res.ExitCode
}

func runStart(rc *tool.RunContext, flags map[string]string, args []string, jsonOut bool) int {
	goal := flags["goal"]
	if goal == "" && len(args) > 0 {
		goal = strings.Join(args, " ")
	}
	s, err := foreman.Start(rc.Ctx, foreman.Options{
		ID:     flags["id"],
		Goal:   goal,
		Agent:  flags["agent"],
		Role:   flags["role"],
		Cwd:    rc.Dir,
		Runner: runner,
	})
	if err != nil {
		return fail(rc, jsonOut, err)
	}
	if flags["detach"] == "true" {
		if err := spawnServe(s.State().ID); err != nil {
			return fail(rc, jsonOut, err)
		}
	}
	if jsonOut {
		return emitJSON(rc, s.State())
	}
	fmt.Fprintln(rc.Out, s.State().ID)
	if flags["detach"] != "true" {
		ready := make(chan string, 1)
		go func() { <-ready }()
		if err := s.ServeControl(rc.Ctx, ready); err != nil {
			return fail(rc, jsonOut, err)
		}
	}
	return 0
}

func runServe(rc *tool.RunContext, args []string) int {
	if len(args) != 1 {
		return usage(rc, "serve requires id")
	}
	s, err := foreman.Open("", args[0], runner)
	if err != nil {
		return fail(rc, false, err)
	}
	if err := s.ServeControl(rc.Ctx, nil); err != nil {
		return fail(rc, false, err)
	}
	return 0
}

func runTell(rc *tool.RunContext, args []string, jsonOut bool) int {
	if len(args) < 2 {
		return usage(rc, "tell requires id and message")
	}
	id, msg := args[0], strings.Join(args[1:], " ")
	if err := foreman.Tell("", id, msg); err != nil {
		return fail(rc, jsonOut, err)
	}
	return ok(rc, jsonOut, map[string]any{"id": id, "sent": msg})
}

func runStatus(rc *tool.RunContext, args []string, jsonOut bool) int {
	if len(args) != 1 {
		return usage(rc, "status requires id")
	}
	st, err := foreman.NewStore("", args[0]).LoadState()
	if err != nil {
		return fail(rc, jsonOut, err)
	}
	if jsonOut {
		return emitJSON(rc, st)
	}
	fmt.Fprintf(rc.Out, "%s\t%s\t%s\n", st.ID, st.Status, st.Goal)
	return 0
}

func runList(rc *tool.RunContext, jsonOut bool) int {
	items, err := foreman.List("")
	if err != nil {
		return fail(rc, jsonOut, err)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	if jsonOut {
		return emitJSON(rc, items)
	}
	for _, st := range items {
		fmt.Fprintf(rc.Out, "%s\t%s\t%s\n", st.ID, st.Status, st.Goal)
	}
	return 0
}

func runControl(rc *tool.RunContext, verb string, args []string, jsonOut bool) int {
	if len(args) < 1 || len(args) > 2 {
		return usage(rc, "%s requires id", verb)
	}
	cmd := foreman.Command{Verb: verb}
	if len(args) == 2 {
		cmd.Target = args[1]
	}
	if err := foreman.SendCommand("", args[0], cmd); err != nil {
		return fail(rc, jsonOut, err)
	}
	return ok(rc, jsonOut, map[string]any{"id": args[0], "verb": verb})
}

func runPrio(rc *tool.RunContext, args []string, jsonOut bool) int {
	if len(args) < 2 || len(args) > 3 {
		return usage(rc, "prio requires id [target] priority")
	}
	c := foreman.Command{Verb: foreman.CommandPrio, Priority: args[len(args)-1]}
	if len(args) == 3 {
		c.Target = args[1]
	}
	if err := foreman.SendCommand("", args[0], c); err != nil {
		return fail(rc, jsonOut, err)
	}
	return ok(rc, jsonOut, map[string]any{"id": args[0], "priority": args[len(args)-1]})
}

func runDAG(rc *tool.RunContext, flags map[string]string, args []string, jsonOut bool) int {
	path := rc.Path(args[0])
	targets := args[1:]
	goal := flags["goal"]
	if goal == "" {
		goal = "run " + args[0]
	}
	s, err := foreman.Start(rc.Ctx, foreman.Options{
		ID:     flags["id"],
		Goal:   goal,
		Agent:  flags["agent"],
		Role:   flags["role"],
		Cwd:    rc.Dir,
		Runner: runner,
	})
	if err != nil {
		return fail(rc, jsonOut, err)
	}
	report, err := s.RunDAG(rc.Ctx, foreman.DAGOptions{Path: path, Targets: targets})
	if err != nil {
		return fail(rc, jsonOut, err)
	}
	if jsonOut {
		return emitJSON(rc, report)
	}
	for _, name := range report.Targets {
		fmt.Fprintln(rc.Out, name)
	}
	return 0
}

func spawnServe(id string) error {
	if os.Getenv("BASHY_FOREMAN_NO_SPAWN") != "" {
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	base := strings.TrimSuffix(filepath.Base(exe), ".exe")
	args := []string{exe}
	// Any multicall HOST binary needs the `foreman` verb prepended; only a binary
	// that IS foreman takes `serve` directly.
	//
	// This used to check `base == "coreutils"`, which meant that under `bashy` —
	// the binary everyone actually runs — it spawned `bashy serve <id>`. That is a
	// real command, and a completely different one: the warm-session server. So
	// `foreman start --detach` cheerfully launched the wrong daemon, never created
	// a control socket, and every subsequent `foreman tell` died on
	// "dial unix …/ctl.sock: no such file or directory".
	//
	// Naming the ONE case that is right, instead of guessing at the set that is
	// wrong, is what keeps a third multicall host from re-breaking this.
	if base != "foreman" {
		args = append(args, "foreman")
	}
	args = append(args, "serve", id)
	p, err := os.StartProcess(exe, args, &os.ProcAttr{
		Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
		Env:   os.Environ(),
	})
	if err != nil {
		return err
	}
	go func() {
		time.Sleep(100 * time.Millisecond)
		_, _ = p.Wait()
	}()
	return nil
}

func parseKVFlags(args []string) (map[string]string, []string) {
	flags := map[string]string{}
	var rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "--") || a == "--" {
			rest = append(rest, args[i:]...)
			break
		}
		name := strings.TrimPrefix(a, "--")
		if strings.Contains(name, "=") {
			parts := strings.SplitN(name, "=", 2)
			flags[parts[0]] = parts[1]
			continue
		}
		switch name {
		case "json", "once", "detach":
			flags[name] = "true"
		default:
			if i+1 >= len(args) {
				flags[name] = ""
				continue
			}
			flags[name] = args[i+1]
			i++
		}
	}
	return flags, rest
}

func usage(rc *tool.RunContext, format string, a ...any) int {
	fmt.Fprintf(rc.Err, "foreman: "+format+"\n", a...)
	fmt.Fprintln(rc.Err, cmd.Usage)
	return 2
}

func fail(rc *tool.RunContext, jsonOut bool, err error) int {
	if jsonOut {
		return emitJSON(rc, map[string]any{"ok": false, "error": err.Error()})
	}
	fmt.Fprintln(rc.Err, "foreman:", err)
	return 1
}

func ok(rc *tool.RunContext, jsonOut bool, v map[string]any) int {
	if jsonOut {
		v["ok"] = true
		return emitJSON(rc, v)
	}
	return 0
}

func emitJSON(rc *tool.RunContext, v any) int {
	data, _ := json.Marshal(v)
	fmt.Fprintln(rc.Out, string(data))
	return 0
}
