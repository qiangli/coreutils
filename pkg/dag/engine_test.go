// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package dag

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	// Register the coreutils userland so a body's `cat` resolves in-process
	// through shell.Handler() — proving hermetic, pure-Go execution.
	_ "github.com/qiangli/coreutils/cmds/all"
)

func engineFor(t *testing.T, dir, md string) *Engine {
	t.Helper()
	g, err := BuildGraph(doc(t, md))
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
	return &Engine{
		Graph: g, Dir: dir, Env: os.Environ(),
		Concurrency: 1, FailFast: true,
		Stdout: new(bytes.Buffer), Stderr: new(bytes.Buffer),
	}
}

func TestEngineDependencyChain(t *testing.T) {
	dir := t.TempDir()
	// build writes via shell redirection; check reads via coreutils `cat`.
	md := "## Tasks\n\n" +
		"### clean\n" + block("bash", "rm -f out.txt") +
		"### build\nRequires: clean\n" + block("bash", "echo hello > out.txt") +
		"### check\nRequires: build\n" + block("bash", "cat out.txt")

	out := new(bytes.Buffer)
	eng := engineFor(t, dir, md)
	eng.Stdout = out

	report, err := eng.Run(context.Background(), "check")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.Failed {
		t.Fatalf("report failed: %+v", report.Results)
	}
	if len(report.Results) != 3 {
		t.Fatalf("want 3 results, got %d", len(report.Results))
	}
	if !strings.Contains(out.String(), "hello") {
		t.Errorf("check did not read the built file; out=%q", out.String())
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "out.txt")); strings.TrimSpace(string(data)) != "hello" {
		t.Errorf("out.txt = %q", string(data))
	}
	for _, r := range report.Results {
		if r.Status != StatusDone {
			t.Errorf("task %s status = %s", r.Name, r.Status)
		}
	}
}

func TestEngineFailurePropagates(t *testing.T) {
	dir := t.TempDir()
	md := "## Tasks\n\n" +
		"### boom\n" + block("bash", "exit 3") +
		"### after\nRequires: boom\n" + block("bash", "echo should-not-run")
	eng := engineFor(t, dir, md)
	eng.FailFast = false // so we observe the skipped dependent

	report, err := eng.Run(context.Background(), "after")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !report.Failed {
		t.Fatal("want report.Failed")
	}
	byName := map[string]TaskResult{}
	for _, r := range report.Results {
		byName[r.Name] = r
	}
	if byName["boom"].Status != StatusFailed || byName["boom"].ExitCode != 3 {
		t.Errorf("boom = %+v", byName["boom"])
	}
	if byName["after"].Status != StatusSkipped {
		t.Errorf("after should be skipped, got %s", byName["after"].Status)
	}
}
