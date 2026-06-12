package linkcmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

// runTool is the canonical test harness shape for cmds packages.
func runTool(t *testing.T, dir string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
	}
	code = cmd.Run(rc, args)
	return out.String(), errb.String(), code
}

func TestLink(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, errb, code := runTool(t, dir, "a", "b")
	if code != 0 || out != "" || errb != "" {
		t.Fatalf("link a b: code=%d out=%q err=%q", code, out, errb)
	}
	got, err := os.ReadFile(filepath.Join(dir, "b"))
	if err != nil || string(got) != "hello" {
		t.Errorf("content=%q err=%v", got, err)
	}
}

func TestLinkErrors(t *testing.T) {
	dir := t.TempDir()
	_, errb, code := runTool(t, dir)
	if code != 2 || !strings.Contains(errb, "missing operand") {
		t.Errorf("no args: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, dir, "a")
	if code != 2 || !strings.Contains(errb, "missing operand after 'a'") {
		t.Errorf("1 arg: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, dir, "a", "b", "c")
	if code != 2 || !strings.Contains(errb, "extra operand 'c'") {
		t.Errorf("3 args: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, dir, "missing", "b")
	if code != 1 || !strings.Contains(errb, "cannot create link") {
		t.Errorf("missing source: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, dir, "--frobnicate", "a", "b")
	if code != 2 || !strings.Contains(errb, "frobnicate") || !strings.Contains(errb, "pure-Go") {
		t.Errorf("unknown flag: code=%d err=%q", code, errb)
	}
}

func TestLinkHelpAndVersion(t *testing.T) {
	out, _, code := runTool(t, t.TempDir(), "--help")
	if code != 0 || !strings.Contains(out, "Usage: link") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	out, _, code = runTool(t, t.TempDir(), "--version")
	if code != 0 || !strings.Contains(out, "link") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}
}
