package mkdircmd

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

// runTool is the canonical test harness shape for cmds packages:
// output is captured after Run returns.
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

func TestMkdirSimple(t *testing.T) {
	dir := t.TempDir()
	out, errb, code := runTool(t, dir, "d")
	if code != 0 || out != "" || errb != "" {
		t.Fatalf("mkdir d: code=%d out=%q err=%q", code, out, errb)
	}
	fi, err := os.Stat(filepath.Join(dir, "d"))
	if err != nil || !fi.IsDir() {
		t.Errorf("directory not created: %v", err)
	}
}

func TestMkdirVerbose(t *testing.T) {
	dir := t.TempDir()
	out, _, code := runTool(t, dir, "-v", "d")
	if code != 0 || out != "mkdir: created directory 'd'\n" {
		t.Errorf("mkdir -v: code=%d out=%q", code, out)
	}
}

func TestMkdirExisting(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "d"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "d")
	if code != 1 || !strings.Contains(errb, "cannot create directory 'd'") ||
		!strings.Contains(strings.ToLower(errb), "exists") {
		t.Errorf("code=%d err=%q", code, errb)
	}
	// -p: existing directory is not an error
	out, errb, code := runTool(t, dir, "-p", "d")
	if code != 0 || out != "" || errb != "" {
		t.Errorf("mkdir -p existing: code=%d out=%q err=%q", code, out, errb)
	}
}

func TestMkdirParents(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join("a", "b", "c")
	out, errb, code := runTool(t, dir, "-pv", nested)
	if code != 0 {
		t.Fatalf("mkdir -pv: code=%d err=%q", code, errb)
	}
	want := "mkdir: created directory 'a'\n" +
		"mkdir: created directory '" + filepath.Join("a", "b") + "'\n" +
		"mkdir: created directory '" + nested + "'\n"
	if out != want {
		t.Errorf("out=%q want %q", out, want)
	}
	fi, err := os.Stat(filepath.Join(dir, "a", "b", "c"))
	if err != nil || !fi.IsDir() {
		t.Error("nested directory not created")
	}
}

func TestMkdirMissingParentWithoutP(t *testing.T) {
	dir := t.TempDir()
	_, errb, code := runTool(t, dir, filepath.Join("a", "b"))
	if code != 1 || !strings.Contains(errb, "cannot create directory") {
		t.Errorf("code=%d err=%q", code, errb)
	}
}

func TestMkdirMode(t *testing.T) {
	dir := t.TempDir()
	if runtime.GOOS == "windows" {
		_, errb, code := runTool(t, dir, "-m", "700", "d")
		if code != 2 || !strings.Contains(errb, "not supported") {
			t.Errorf("windows -m: code=%d err=%q", code, errb)
		}
		return
	}
	_, errb, code := runTool(t, dir, "-m", "700", "d")
	if code != 0 {
		t.Fatalf("mkdir -m 700: code=%d err=%q", code, errb)
	}
	fi, err := os.Stat(filepath.Join(dir, "d"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o700 {
		t.Errorf("mode = %o, want 700", fi.Mode().Perm())
	}
	// -m applies to the final directory only (-p intermediates get
	// defaults). 0o505 has no owner-write bit; a default-created
	// intermediate always keeps owner-write (umask does not mask the
	// owner bits in any sane configuration).
	_, errb, code = runTool(t, dir, "-p", "-m", "505", filepath.Join("x", "y"))
	if code != 0 {
		t.Fatalf("mkdir -p -m: code=%d err=%q", code, errb)
	}
	yfi, err := os.Stat(filepath.Join(dir, "x", "y"))
	if err != nil {
		t.Fatal(err)
	}
	if yfi.Mode().Perm() != 0o505 {
		t.Errorf("final mode = %o, want 505", yfi.Mode().Perm())
	}
	xfi, err := os.Stat(filepath.Join(dir, "x"))
	if err != nil {
		t.Fatal(err)
	}
	if xfi.Mode().Perm()&0o200 == 0 {
		t.Errorf("intermediate mode = %o; -m must apply to the final dir only", xfi.Mode().Perm())
	}
}

func TestMkdirModeErrors(t *testing.T) {
	dir := t.TempDir()
	if runtime.GOOS == "windows" {
		t.Skipf("-m is refused before value validation on windows")
	}
	_, errb, code := runTool(t, dir, "-m", "999", "d")
	if code != 2 || !strings.Contains(errb, "invalid mode '999'") {
		t.Errorf("-m 999: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, dir, "-m", "u+x", "d")
	if code != 2 || !strings.Contains(errb, "not supported") {
		t.Errorf("-m u+x: code=%d err=%q", code, errb)
	}
}

func TestMkdirUsageErrors(t *testing.T) {
	dir := t.TempDir()
	_, errb, code := runTool(t, dir)
	if code != 2 || !strings.Contains(errb, "missing operand") {
		t.Errorf("no args: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, dir, "--frobnicate", "d")
	if code != 2 || !strings.Contains(errb, "frobnicate") || !strings.Contains(errb, "pure-Go") {
		t.Errorf("unknown flag: code=%d err=%q", code, errb)
	}
}

func TestMkdirHelpAndVersion(t *testing.T) {
	dir := t.TempDir()
	out, _, code := runTool(t, dir, "--help")
	if code != 0 || !strings.Contains(out, "Usage: mkdir") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	out, _, code = runTool(t, dir, "--version")
	if code != 0 || !strings.Contains(out, "mkdir") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}
}
