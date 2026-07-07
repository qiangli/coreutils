package chconcmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

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

func TestChconUsageErrors(t *testing.T) {
	dir := t.TempDir()
	_, errb, code := runTool(t, dir)
	if code != 2 || !strings.Contains(errb, "missing operand") {
		t.Fatalf("no args: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, dir, "system_u:object_r:tmp_t:s0")
	if code != 2 || !strings.Contains(errb, "missing operand after") {
		t.Fatalf("no file: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, dir, "--recursive", "ctx", "f")
	if code != 2 || !strings.Contains(errb, "recursive") || !strings.Contains(errb, "pure-Go") {
		t.Fatalf("unknown flag: code=%d err=%q", code, errb)
	}
}

func TestChconHelpAndVersion(t *testing.T) {
	out, _, code := runTool(t, t.TempDir(), "--help")
	if code != 0 || !strings.Contains(out, "Usage: chcon") {
		t.Fatalf("--help: code=%d out=%q", code, out)
	}
	out, _, code = runTool(t, t.TempDir(), "--version")
	if code != 0 || !strings.Contains(out, "chcon") {
		t.Fatalf("--version: code=%d out=%q", code, out)
	}
}

func TestChconUnsupportedOutsideLinux(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("non-linux assertion")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "system_u:object_r:tmp_t:s0", "f")
	if code != 1 || !strings.Contains(errb, "SELinux context changes are not supported on "+runtime.GOOS) {
		t.Fatalf("unsupported: code=%d err=%q", code, errb)
	}
}

func TestChconLinuxReportsSetxattrErrors(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux-only assertion")
	}
	dir := t.TempDir()
	_, errb, code := runTool(t, dir, "system_u:object_r:tmp_t:s0", "missing")
	if code != 1 || !strings.Contains(errb, "missing") || !strings.Contains(errb, "system_u:object_r:tmp_t:s0") {
		t.Fatalf("missing file: code=%d err=%q", code, errb)
	}
}
