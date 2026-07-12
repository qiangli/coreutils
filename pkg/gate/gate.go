// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

// Package gate answers the one question every stage transition depends on:
//
//	does this project pass?
//
// # Why this exists
//
// bashy had no Test verb. Not because nobody tested — because the gate, the
// command that decides pass/fail, was spelled FOUR incompatible ways across four
// packages, each privately:
//
//   - weave    — a shell command in `.agents/weave/suite-gate`
//   - sdlc     — a `healthcheck:` key in sdlc.yaml
//   - supervise— a `::`-delimited string inside `--task 'goal :: gate'`
//   - dag      — a target that happens to fail
//
// Every one of them means the SAME THING: run a command, and let its exit status
// be the verdict. They did not disagree about semantics. They disagreed about
// where the command lives. So there was no way to ask "does this project pass?"
// from a command line, no shared result schema, and four places to change when
// the answer changed.
//
// This package does not invent a gate. It is the one place the command lives, and
// the others read it.
//
// # The project, not the repo
//
// A gate is defined per PROJECT, and a project spans repos. bashy's own gate is
// the proof: `make test-bash` runs in bashy and compiles ../sh. A per-repo gate
// cannot express that, which is why the resolution below is anchored at the
// project's primary root rather than at whatever .git the caller happens to be
// standing in.
package gate

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// SchemaVersion is the result contract. One schema, so that weave, sdlc, dag and
// a human at a terminal all read the same verdict in the same shape.
const SchemaVersion = "bashy-gate-v1"

// DefinitionFile is where a project's gate lives. One file, one command per line
// (blank lines and #-comments ignored), so a gate can be several checks without
// inventing a config language for it.
const DefinitionFile = ".bashy/gate"

// LegacyWeaveFile is weave's existing gate location. Read as a FALLBACK so that
// every project already using weave keeps working with no migration — the whole
// point of unifying is to stop breaking people, not to start.
const LegacyWeaveFile = ".agents/weave/suite-gate"

// Check is one command in the gate.
type Check struct {
	Name     string `json:"name"`
	Command  string `json:"command"`
	Exit     int    `json:"exit"`
	Passed   bool   `json:"passed"`
	Duration string `json:"duration"`
	Output   string `json:"output,omitempty"` // captured on FAILURE only — a passing gate is not interesting
}

// Result is the verdict.
type Result struct {
	SchemaVersion string  `json:"schema_version"`
	Project       string  `json:"project"`
	Source        string  `json:"source"` // where the definition came from
	Checks        []Check `json:"checks"`
	Passed        bool    `json:"passed"`
	Duration      string  `json:"duration"`
}

// Definition is a project's gate: the commands, and where they were found.
type Definition struct {
	Root     string
	Source   string
	Commands []string
}

// ErrNoGate is returned when a project has not defined one.
//
// This is deliberately an ERROR, not an empty pass. A project with no gate has not
// "passed" — it has failed to say what passing MEANS. Returning success there is
// how a green check mark comes to mean nothing, which is precisely the failure this
// project already lived through: a CI conformance gate that reported green for ten
// merges without running.
var ErrNoGate = fmt.Errorf("no gate defined")

// Resolve finds the project's gate, in precedence order. The first hit wins, and
// the Source is recorded so a verdict can always say where its authority came from.
func Resolve(root string, override string) (*Definition, error) {
	if strings.TrimSpace(override) != "" {
		return &Definition{Root: root, Source: "--command", Commands: []string{strings.TrimSpace(override)}}, nil
	}
	if v := strings.TrimSpace(os.Getenv("BASHY_GATE")); v != "" {
		return &Definition{Root: root, Source: "BASHY_GATE", Commands: []string{v}}, nil
	}
	for _, rel := range []string{DefinitionFile, LegacyWeaveFile} {
		p := filepath.Join(root, rel)
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		cmds := parseCommands(string(b))
		if len(cmds) == 0 {
			continue
		}
		return &Definition{Root: root, Source: rel, Commands: cmds}, nil
	}
	return nil, fmt.Errorf("%w in %s.\n\nDefine one — it is the command that decides whether this project passes:\n"+
		"    echo 'make test' > %s\n\n"+
		"A project with no gate has not passed; it has failed to say what passing MEANS.",
		ErrNoGate, root, filepath.Join(root, DefinitionFile))
}

func parseCommands(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out
}

// Run executes the gate and returns the verdict.
//
// It stops at the FIRST failure. A gate is not a test report — it is a decision,
// and the decision is already made once one check fails. Running the rest wastes
// time an agent is waiting on, and buries the one line that matters.
func Run(ctx context.Context, def *Definition, shell string) (*Result, error) {
	start := time.Now()
	res := &Result{
		SchemaVersion: SchemaVersion,
		Project:       def.Root,
		Source:        def.Source,
		Passed:        true,
	}
	if shell == "" {
		shell = "/bin/sh"
	}
	for _, cmdline := range def.Commands {
		cs := time.Now()
		c := exec.CommandContext(ctx, shell, "-c", cmdline)
		c.Dir = def.Root
		out, err := c.CombinedOutput()
		code := c.ProcessState.ExitCode()
		if err != nil && code == 0 {
			code = 1 // could not even start it: that is a failure, not a pass
		}
		chk := Check{
			Name:     firstWord(cmdline),
			Command:  cmdline,
			Exit:     code,
			Passed:   code == 0,
			Duration: time.Since(cs).Round(time.Millisecond).String(),
		}
		if !chk.Passed {
			chk.Output = tail(string(out), 4000)
			res.Checks = append(res.Checks, chk)
			res.Passed = false
			break
		}
		res.Checks = append(res.Checks, chk)
	}
	res.Duration = time.Since(start).Round(time.Millisecond).String()
	return res, nil
}

// JSON renders the verdict.
func (r *Result) JSON() string {
	b, _ := json.MarshalIndent(r, "", "  ")
	return string(b)
}

func firstWord(s string) string {
	if i := strings.IndexAny(s, " \t"); i > 0 {
		return s[:i]
	}
	return s
}

// tail keeps the END of the output: a failing build's useful line is almost always
// the last one, and the first 4 KB of a compiler's chatter is the least useful 4 KB
// you could hand an agent.
func tail(s string, n int) string {
	s = strings.TrimRight(s, "\n")
	if len(s) <= n {
		return s
	}
	return "…(truncated)…\n" + s[len(s)-n:]
}
