package chmodcmd

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

// TestModeApply exercises the mode engine without touching the
// filesystem, so it runs on every platform.
func TestModeApply(t *testing.T) {
	cases := []struct {
		mode  string
		old   uint32
		isDir bool
		umask uint32
		want  uint32
	}{
		{"644", 0o777, false, 0, 0o644},
		{"755", 0o644, false, 0, 0o755},
		{"7777", 0, false, 0, 0o7777},
		{"0", 0o777, false, 0, 0},
		// GNU keep-directory-setid rule for short octal modes.
		{"755", 0o2775, true, 0, 0o2755},
		{"00755", 0o2775, true, 0, 0o755},
		{"755", 0o2775, false, 0, 0o755},
		// Symbolic, explicit who (umask must not interfere).
		{"u+x", 0o644, false, 0o22, 0o744},
		{"go-w", 0o666, false, 0o22, 0o644},
		{"u+x,go-w", 0o666, false, 0, 0o744},
		{"a=r", 0o777, false, 0o22, 0o444},
		{"u=rwx", 0o644, false, 0, 0o744},
		{"u=", 0o755, false, 0, 0o055},
		{"g=u", 0o741, false, 0, 0o771},
		{"o=g", 0o754, false, 0, 0o755},
		{"u+s", 0o755, false, 0, 0o4755},
		{"g+s", 0o755, false, 0, 0o2755},
		{"+t", 0o755, false, 0o22, 0o1755},
		{"o-t", 0o1777, false, 0, 0o777},
		{"u-s", 0o4755, false, 0, 0o755},
		// X: execute only for directories or already-executable files.
		{"a+X", 0o644, false, 0, 0o644},
		{"a+X", 0o644, true, 0, 0o755},
		{"a+X", 0o744, false, 0, 0o755},
		// Empty who: umask-masked, per the GNU manual.
		{"+x", 0o644, false, 0o22, 0o755},
		{"+w", 0o444, false, 0o22, 0o644},
		{"-w", 0o666, false, 0o22, 0o466},
		{"=rwx", 0o644, false, 0o22, 0o755},
		// Multiple operators in one clause.
		{"u+rw-x", 0o111, false, 0, 0o611},
	}
	for _, c := range cases {
		mc, err := parseMode(c.mode)
		if err != nil {
			t.Errorf("parseMode(%q): %v", c.mode, err)
			continue
		}
		if got := mc.apply(c.old, c.isDir, c.umask); got != c.want {
			t.Errorf("%q on %04o (dir=%v umask=%03o) = %04o, want %04o",
				c.mode, c.old, c.isDir, c.umask, got, c.want)
		}
	}
}

func TestParseModeInvalid(t *testing.T) {
	for _, mode := range []string{"", "z+x", "u~x", "rwx", "u", "u+z", "8", "12345", "u=gw", ","} {
		if _, err := parseMode(mode); err == nil {
			t.Errorf("parseMode(%q): expected error", mode)
		}
	}
}

func TestExtractDashMode(t *testing.T) {
	mode, rest := extractDashMode([]string{"-w", "f"})
	if mode != "-w" || len(rest) != 1 || rest[0] != "f" {
		t.Errorf("-w: mode=%q rest=%v", mode, rest)
	}
	mode, rest = extractDashMode([]string{"-R", "-rx", "f"})
	if mode != "-rx" || len(rest) != 2 || rest[0] != "-R" {
		t.Errorf("-R -rx: mode=%q rest=%v", mode, rest)
	}
	mode, rest = extractDashMode([]string{"--", "-w"})
	if mode != "" || len(rest) != 2 {
		t.Errorf("--: mode=%q rest=%v", mode, rest)
	}
}

func TestChmodFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod is unix-only")
	}
	dir := t.TempDir()
	f := filepath.Join(dir, "f")
	if err := os.WriteFile(f, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	check := func(want os.FileMode) {
		t.Helper()
		fi, err := os.Stat(f)
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm() != want {
			t.Errorf("mode=%v want %v", fi.Mode().Perm(), want)
		}
	}
	if _, errb, code := runTool(t, dir, "600", "f"); code != 0 {
		t.Fatalf("chmod 600: code=%d err=%q", code, errb)
	}
	check(0o600)
	if _, _, code := runTool(t, dir, "u+x,g+r", "f"); code != 0 {
		t.Fatal("chmod u+x,g+r failed")
	}
	check(0o740)
	if _, errb, code := runTool(t, dir, "u-x", "f"); code != 0 {
		t.Fatalf("chmod u-x: code=%d err=%q", code, errb)
	}
	check(0o640)
	// Dash-prefixed mode operand must survive flag parsing. (0640 has
	// no group/other write bits, so the result is umask-independent.)
	if _, errb, code := runTool(t, dir, "-w", "f"); code != 0 {
		t.Fatalf("chmod -w: code=%d err=%q", code, errb)
	}
	check(0o440)
}

func TestChmodRecursive(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod is unix-only")
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "d", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	inner := filepath.Join(dir, "d", "sub", "f")
	if err := os.WriteFile(inner, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, errb, code := runTool(t, dir, "-R", "u=rwx,go=", "d"); code != 0 {
		t.Fatalf("chmod -R: code=%d err=%q", code, errb)
	}
	for _, p := range []string{filepath.Join(dir, "d"), filepath.Join(dir, "d", "sub"), inner} {
		fi, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm() != 0o700 {
			t.Errorf("%s mode=%v want 0700", p, fi.Mode().Perm())
		}
	}
}

func TestChmodErrors(t *testing.T) {
	dir := t.TempDir()
	_, errb, code := runTool(t, dir)
	if code != 2 || !strings.Contains(errb, "missing operand") {
		t.Errorf("no args: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, dir, "644")
	if code != 2 || !strings.Contains(errb, "missing operand after '644'") {
		t.Errorf("no file: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, dir, "z+x", "f")
	if code != 1 || !strings.Contains(errb, "invalid mode: 'z+x'") {
		t.Errorf("invalid mode: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, dir, "--frobnicate", "644", "f")
	if code != 2 || !strings.Contains(errb, "frobnicate") || !strings.Contains(errb, "pure-Go") {
		t.Errorf("unknown flag: code=%d err=%q", code, errb)
	}
	if runtime.GOOS != "windows" {
		if err := os.WriteFile(filepath.Join(dir, "exists"), nil, 0o644); err != nil {
			t.Fatal(err)
		}
		_, errb, code = runTool(t, dir, "644", "no-such-file")
		if code != 1 || !strings.Contains(errb, "cannot access 'no-such-file'") {
			t.Errorf("missing file: code=%d err=%q", code, errb)
		}
	}
}

func TestChmodWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only assertion")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "644", "f")
	if code != 1 || !strings.Contains(errb, "not supported on windows") {
		t.Errorf("windows: code=%d err=%q", code, errb)
	}
}

func TestChmodHelpAndVersion(t *testing.T) {
	out, _, code := runTool(t, t.TempDir(), "--help")
	if code != 0 || !strings.Contains(out, "Usage: chmod") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	out, _, code = runTool(t, t.TempDir(), "--version")
	if code != 0 || !strings.Contains(out, "chmod") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}
}

func TestChmodVerbose(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod is unix-only")
	}
	dir := t.TempDir()
	f := filepath.Join(dir, "f")
	if err := os.WriteFile(f, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	out, errb, code := runTool(t, dir, "-v", "644", "f")
	if code != 0 || errb != "" {
		t.Fatalf("chmod -v: code=%d err=%q", code, errb)
	}
	if !strings.Contains(out, "mode of 'f' retained as") && !strings.Contains(out, "mode of 'f' changed to") {
		t.Errorf("expected verbose output, got: %q", out)
	}
}

func TestChmodChanges(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod is unix-only")
	}
	dir := t.TempDir()
	f := filepath.Join(dir, "f")
	if err := os.WriteFile(f, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, code := runTool(t, dir, "-c", "644", "f")
	if code != 0 {
		t.Fatalf("chmod -c: code=%d", code)
	}
	if out != "" {
		t.Errorf("expected no output for unchanged mode with -c, got: %q", out)
	}
	out, _, code = runTool(t, dir, "-c", "600", "f")
	if code != 0 {
		t.Fatalf("chmod -c 600: code=%d", code)
	}
	if !strings.Contains(out, "mode of 'f' changed to 0600") {
		t.Errorf("expected changed output with -c, got: %q", out)
	}
}

func TestChmodSilent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod is unix-only")
	}
	dir := t.TempDir()
	_, errb, code := runTool(t, dir, "-f", "644", "no-such-file")
	if code != 1 {
		t.Fatalf("chmod -f: expected code=1, got=%d", code)
	}
	if errb != "" {
		t.Errorf("expected no stderr with -f, got: %q", errb)
	}
}

func TestChmodReference(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod is unix-only")
	}
	dir := t.TempDir()
	ref := filepath.Join(dir, "ref")
	if err := os.WriteFile(ref, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	f := filepath.Join(dir, "f")
	if err := os.WriteFile(f, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "--reference=ref", "f")
	if code != 0 || errb != "" {
		t.Fatalf("chmod --reference: code=%d err=%q", code, errb)
	}
	fi, err := os.Stat(f)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("expected mode 0600 from reference, got %#o", fi.Mode().Perm())
	}
}

func TestChmodPreserveRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod is unix-only")
	}
	dir := t.TempDir()
	f := filepath.Join(dir, "f")
	if err := os.WriteFile(f, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "-R", "--preserve-root", "644", "f")
	if code != 0 || errb != "" {
		t.Fatalf("chmod --preserve-root on non-root: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, dir, "-R", "--preserve-root", "644", "/")
	if code != 1 || !strings.Contains(errb, "dangerous to operate recursively on '/'") {
		t.Fatalf("chmod --preserve-root on /: code=%d err=%q", code, errb)
	}
}
