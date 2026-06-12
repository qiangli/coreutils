package unlinkcmd

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

func TestUnlink(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	out, errb, code := runTool(t, dir, "f")
	if code != 0 || out != "" || errb != "" {
		t.Fatalf("unlink f: code=%d out=%q err=%q", code, out, errb)
	}
	if _, err := os.Lstat(filepath.Join(dir, "f")); !os.IsNotExist(err) {
		t.Error("file still exists")
	}
}

func TestUnlinkErrors(t *testing.T) {
	dir := t.TempDir()
	_, errb, code := runTool(t, dir)
	if code != 2 || !strings.Contains(errb, "missing operand") {
		t.Errorf("no args: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, dir, "a", "b")
	if code != 2 || !strings.Contains(errb, "extra operand 'b'") {
		t.Errorf("2 args: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, dir, "missing")
	if code != 1 || !strings.Contains(errb, "cannot unlink 'missing'") {
		t.Errorf("missing file: code=%d err=%q", code, errb)
	}
	if err := os.Mkdir(filepath.Join(dir, "d"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, errb, code = runTool(t, dir, "d")
	if code != 1 || !strings.Contains(errb, "Is a directory") {
		t.Errorf("directory: code=%d err=%q", code, errb)
	}
	if _, err := os.Stat(filepath.Join(dir, "d")); err != nil {
		t.Error("directory was removed")
	}
	_, errb, code = runTool(t, dir, "--frobnicate", "f")
	if code != 2 || !strings.Contains(errb, "frobnicate") || !strings.Contains(errb, "pure-Go") {
		t.Errorf("unknown flag: code=%d err=%q", code, errb)
	}
}

func TestUnlinkHelpAndVersion(t *testing.T) {
	out, _, code := runTool(t, t.TempDir(), "--help")
	if code != 0 || !strings.Contains(out, "Usage: unlink") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	out, _, code = runTool(t, t.TempDir(), "--version")
	if code != 0 || !strings.Contains(out, "unlink") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}
}
