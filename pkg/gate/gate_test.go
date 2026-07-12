// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package gate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A project with NO gate is an error, never a pass. This is the whole point: a
// project that has not said what passing means has not passed. Returning success
// here is how a green check mark comes to mean nothing — which this project
// already lived through, with a CI conformance gate that reported green for ten
// merges without running.
func TestNoGateIsAnError(t *testing.T) {
	_, err := Resolve(t.TempDir(), "")
	if err == nil {
		t.Fatal("a project with no gate resolved successfully — it must be an error, not a pass")
	}
	if !errors.Is(err, ErrNoGate) {
		t.Fatalf("wrong error: %v", err)
	}
	// The message must tell you how to fix it, not just that you are wrong.
	if !strings.Contains(err.Error(), DefinitionFile) {
		t.Fatalf("the error does not say how to define a gate: %v", err)
	}
}

// Every project already using weave must keep working with NO migration. The
// point of unifying is to stop breaking people, not to start.
func TestLegacyWeaveGateStillWorks(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, LegacyWeaveFile, "exit 0\n")

	def, err := Resolve(dir, "")
	if err != nil {
		t.Fatalf("a project with weave's existing suite-gate stopped working: %v", err)
	}
	if def.Source != LegacyWeaveFile {
		t.Fatalf("source = %q, want the legacy weave file", def.Source)
	}
}

// The new definition WINS over the legacy one, so a project can migrate by adding
// a file rather than by deleting one.
func TestNewDefinitionTakesPrecedence(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, LegacyWeaveFile, "exit 1\n") // the old, failing gate
	write(t, dir, DefinitionFile, "exit 0\n")  // the new one

	def, err := Resolve(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if def.Source != DefinitionFile {
		t.Fatalf("source = %q, want the new definition to win", def.Source)
	}
}

// A gate STOPS at the first failure. It is a decision, not a test report: the
// decision is made the moment one check fails, and running the rest wastes time an
// agent is waiting on while burying the one line that matters.
func TestStopsAtFirstFailure(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, DefinitionFile, "true\nfalse\ntouch SHOULD_NOT_EXIST\n")

	def, err := Resolve(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	res, err := Run(context.Background(), def, "/bin/sh")
	if err != nil {
		t.Fatal(err)
	}
	if res.Passed {
		t.Fatal("gate passed with a failing check")
	}
	if len(res.Checks) != 2 {
		t.Fatalf("ran %d checks, want 2 (it must stop at the first failure)", len(res.Checks))
	}
	if _, err := os.Stat(filepath.Join(dir, "SHOULD_NOT_EXIST")); err == nil {
		t.Fatal("the third check ran after a failure — the gate did not stop")
	}
}

// A failing check must carry its output, or an agent is told "it failed" with no
// way to know why and has to re-run the whole thing to find out.
func TestFailureCarriesOutput(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, DefinitionFile, "echo the-real-reason >&2; exit 7\n")

	def, _ := Resolve(dir, "")
	res, err := Run(context.Background(), def, "/bin/sh")
	if err != nil {
		t.Fatal(err)
	}
	if res.Passed {
		t.Fatal("gate passed on exit 7")
	}
	c := res.Checks[0]
	if c.Exit != 7 {
		t.Fatalf("exit = %d, want 7 (the real code, not a flattened 1)", c.Exit)
	}
	if !strings.Contains(c.Output, "the-real-reason") {
		t.Fatalf("the failure did not carry its output: %q", c.Output)
	}
}

// A passing gate does NOT carry output. A green gate is not interesting, and
// dumping a successful build log into an agent's context window is pure cost.
func TestPassCarriesNoOutput(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, DefinitionFile, "echo lots and lots of chatter\n")

	def, _ := Resolve(dir, "")
	res, _ := Run(context.Background(), def, "/bin/sh")
	if !res.Passed {
		t.Fatal("gate failed on a passing command")
	}
	if res.Checks[0].Output != "" {
		t.Fatalf("a passing check carried output: %q", res.Checks[0].Output)
	}
}

func TestOverrideAndEnvPrecedence(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, DefinitionFile, "exit 1\n")

	def, err := Resolve(dir, "exit 0")
	if err != nil {
		t.Fatal(err)
	}
	if def.Source != "--command" {
		t.Fatalf("an explicit --command must win: source = %q", def.Source)
	}

	t.Setenv("BASHY_GATE", "exit 0")
	def, err = Resolve(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if def.Source != "BASHY_GATE" {
		t.Fatalf("BASHY_GATE must beat the file: source = %q", def.Source)
	}
}
