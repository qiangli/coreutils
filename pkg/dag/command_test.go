// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package dag

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeDAG(t *testing.T, md string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "DAG.md")
	if err := os.WriteFile(p, []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCommandListJSON(t *testing.T) {
	md := "## Tasks\n\n" +
		"### build\nCompile it.\nRequires: clean\n" + block("bash", "echo build") +
		"### clean\n" + block("bash", "echo clean")
	path := writeDAG(t, md)

	cmd := NewDagCmd()
	out, errOut := new(bytes.Buffer), new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(errOut)
	cmd.SetArgs([]string{"--list", "--json", "--file", path})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v (stderr=%s)", err, errOut.String())
	}

	var env struct {
		SchemaVersion string `json:"schema_version"`
		Command       string `json:"command"`
		Status        string `json:"status"`
		Result        struct {
			Tasks []struct {
				Name     string   `json:"name"`
				Desc     string   `json:"description"`
				Requires []string `json:"requires"`
			} `json:"tasks"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal envelope: %v\n%s", err, out.String())
	}
	if env.SchemaVersion != "dag-v1" || env.Command != "dag" || env.Status != "ok" {
		t.Errorf("envelope = %+v", env)
	}
	if len(env.Result.Tasks) != 2 || env.Result.Tasks[0].Name != "build" {
		t.Fatalf("tasks = %+v", env.Result.Tasks)
	}
	if env.Result.Tasks[0].Desc != "Compile it." {
		t.Errorf("desc = %q", env.Result.Tasks[0].Desc)
	}
}

func TestCommandNoArgListsByDefault(t *testing.T) {
	// No frontmatter default and no "default" target => no-arg lists targets
	// (like a Makefile whose .DEFAULT_GOAL is help), rather than running one.
	path := writeDAG(t, "## Tasks\n\n### danger\n"+block("bash", "echo SHOULD-NOT-RUN"))
	cmd := NewDagCmd()
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"--file", path})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if strings.Contains(out.String(), "SHOULD-NOT-RUN") {
		t.Errorf("no-arg invocation ran a target instead of listing; out=%q", out.String())
	}
	if !strings.Contains(out.String(), "danger") {
		t.Errorf("no-arg invocation should list targets; out=%q", out.String())
	}
}

func TestCommandDefaultGoalFrontmatter(t *testing.T) {
	md := "---\ndefault: greet\n---\n\n## Tasks\n\n" +
		"### greet\n" + block("bash", "echo hello-default") +
		"### other\n" + block("bash", "echo other")
	path := writeDAG(t, md)
	cmd := NewDagCmd()
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"--file", path}) // no target -> default goal "greet"
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out.String(), "hello-default") {
		t.Errorf("default goal not run; out=%q", out.String())
	}
}

func TestCommandVarOverride(t *testing.T) {
	// make-style: `dag echo NAME=world` injects NAME into the body's env.
	md := "## Tasks\n\n### echo\n" + block("bash", "echo \"hi ${NAME:-nobody}\"")
	path := writeDAG(t, md)
	cmd := NewDagCmd()
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"--file", path, "echo", "NAME=world"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out.String(), "hi world") {
		t.Errorf("variable override not applied; out=%q", out.String())
	}
}

func TestCommandUnknownTarget(t *testing.T) {
	path := writeDAG(t, "## Tasks\n\n### a\n"+block("bash", "echo a"))
	cmd := NewDagCmd()
	out, errOut := new(bytes.Buffer), new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(errOut)
	cmd.SetArgs([]string{"--json", "--file", path, "nope"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("want error for unknown target")
	}
	if ExitCodeOf(err) != 2 {
		t.Errorf("want exit 2, got %d", ExitCodeOf(err))
	}
}
