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
		// POSIX octal modes are absolute, including on directories.
		{"755", 0o2775, true, 0, 0o755},
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
		// Empty who: filtered through the invocation umask per Issue 7.
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
	if mode != "-w" || strings.Join(rest, " ") != "-- f" {
		t.Errorf("-w: mode=%q rest=%v", mode, rest)
	}
	mode, rest = extractDashMode([]string{"-R", "-rx", "f"})
	if mode != "-rx" || strings.Join(rest, " ") != "-R -- f" {
		t.Errorf("-R -rx: mode=%q rest=%v", mode, rest)
	}
	mode, rest = extractDashMode([]string{"-w", "--", "file"})
	if mode != "-w" || strings.Join(rest, " ") != "-- file" {
		t.Errorf("-w --: mode=%q rest=%v", mode, rest)
	}
	mode, rest = extractDashMode([]string{"-R", "-w", "--", "file"})
	if mode != "-w" || strings.Join(rest, " ") != "-R -- file" {
		t.Errorf("-R -w --: mode=%q rest=%v", mode, rest)
	}
	mode, rest = extractDashMode([]string{"--", "-w"})
	if mode != "" || len(rest) != 2 {
		t.Errorf("--: mode=%q rest=%v", mode, rest)
	}
	mode, rest = extractDashMode([]string{"644", "-w"})
	if mode != "" || strings.Join(rest, " ") != "644 -w" {
		t.Errorf("file named -w after mode: mode=%q rest=%v", mode, rest)
	}
	mode, rest = extractDashMode([]string{"--reference", "-w", "target"})
	if mode != "" || strings.Join(rest, " ") != "--reference -w target" {
		t.Errorf("--reference value: mode=%q rest=%v", mode, rest)
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
	out, errb, code := runTool(t, dir, "600", "f")
	if code != 0 {
		t.Fatalf("chmod 600: code=%d err=%q", code, errb)
	}
	// POSIX: standard output is not used.
	if out != "" {
		t.Errorf("chmod 600 wrote to stdout: %q", out)
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
	if err := os.WriteFile(filepath.Join(dir, "exists"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	_, errb, code = runTool(t, dir, "644", "no-such-file")
	if code != 1 || !strings.Contains(errb, "cannot access 'no-such-file'") {
		t.Errorf("missing file: code=%d err=%q", code, errb)
	}
}

// TestReadOnlyProjection pins the Windows rule host-independently: the
// modes the bash-5.3 fixtures use (alias, glob-test, posix2, shopt run
// chmod +x / -x / 644 / 311 / a=wx / 0000 on temp files) project onto the
// read-only attribute — any write bit keeps the file writable, none makes
// it read-only — and never produce a refusal (Story #682).
func TestReadOnlyProjection(t *testing.T) {
	cases := []struct {
		mode string
		old  uint32 // what os.Stat reports on Windows: 0666 writable, 0444 read-only
		want os.FileMode
	}{
		{"+x", 0o666, 0o666},
		{"-x", 0o666, 0o666},
		{"u+x", 0o666, 0o666},
		{"644", 0o666, 0o666},
		{"311", 0o666, 0o666}, // owner w only
		{"a=wx", 0o666, 0o666},
		{"0000", 0o666, 0o444},
		{"444", 0o666, 0o444},
		{"a-w", 0o666, 0o444},
		{"u-w", 0o666, 0o666},   // group/other w remain
		{"+w", 0o444, 0o666},    // read-only file made writable again
		{"+x", 0o444, 0o444},    // x alone does not touch the attribute
		{"u=rwx", 0o444, 0o666}, // an explicit-who = sets w back
	}
	for _, c := range cases {
		change, err := parseMode(c.mode)
		if err != nil {
			t.Fatalf("parseMode(%q): %v", c.mode, err)
		}
		bits := change.apply(c.old, false, 0o022)
		if got := readOnlyProjection(bits); got != c.want {
			t.Errorf("chmod %s on %04o: bits %04o -> host mode %04o, want %04o", c.mode, c.old, bits, got, c.want)
		}
	}
	if readOnlyHost != (runtime.GOOS == "windows") {
		t.Errorf("readOnlyHost = %v on %s", readOnlyHost, runtime.GOOS)
	}
	if runtime.GOOS != "windows" {
		if got := hostMode(0o4755); got != bitsToFileMode(0o4755) {
			t.Errorf("hostMode on Unix altered the bits: %v", got)
		}
	}
}

// TestChmodWindows runs chmod for real on a Windows host: every mode the
// fixtures use exits 0, and the read-only attribute follows the write bits.
func TestChmodWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only assertion")
	}
	dir := t.TempDir()
	f := filepath.Join(dir, "f")
	if err := os.WriteFile(f, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "d", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "d", "sub", "g"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	writable := func(path string) bool {
		t.Helper()
		fi, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		return fi.Mode().Perm()&0o200 != 0
	}
	for _, mode := range []string{"+x", "-x", "u+x", "644", "311", "a=wx"} {
		if _, errb, code := runTool(t, dir, mode, "f"); code != 0 || errb != "" {
			t.Errorf("chmod %s: code=%d err=%q", mode, code, errb)
		}
		if !writable(f) {
			t.Errorf("chmod %s left f read-only", mode)
		}
	}
	for _, mode := range []string{"0000", "444", "a-w"} {
		if _, errb, code := runTool(t, dir, mode, "f"); code != 0 || errb != "" {
			t.Errorf("chmod %s: code=%d err=%q", mode, code, errb)
		}
		if writable(f) {
			t.Errorf("chmod %s did not set read-only on f", mode)
		}
		if _, errb, code := runTool(t, dir, "+w", "f"); code != 0 || errb != "" {
			t.Errorf("chmod +w: code=%d err=%q", code, errb)
		}
		if !writable(f) {
			t.Errorf("chmod +w did not clear read-only on f")
		}
	}
	// -R applies the projection to every entry, children first.
	if _, errb, code := runTool(t, dir, "-R", "a-w", "d"); code != 0 || errb != "" {
		t.Errorf("chmod -R a-w: code=%d err=%q", code, errb)
	}
	if writable(filepath.Join(dir, "d", "sub", "g")) {
		t.Errorf("chmod -R a-w did not reach d/sub/g")
	}
	if _, errb, code := runTool(t, dir, "-R", "u+w", "d"); code != 0 || errb != "" {
		t.Errorf("chmod -R u+w: code=%d err=%q", code, errb)
	}
	if !writable(filepath.Join(dir, "d", "sub", "g")) {
		t.Errorf("chmod -R u+w did not restore d/sub/g")
	}
	out, _, code := runTool(t, dir, "-v", "755", "f")
	if code != 0 || !strings.Contains(out, "mode of 'f'") {
		t.Errorf("-v on windows: code=%d out=%q", code, out)
	}
	_, errb, code := runTool(t, dir, "644", "no-such-file")
	if code != 1 || !strings.Contains(errb, "cannot access 'no-such-file'") {
		t.Errorf("missing file: code=%d err=%q", code, errb)
	}
}

func TestChmodHelpAndVersion(t *testing.T) {
	out, _, code := runTool(t, t.TempDir(), "--help")
	if code != 0 || !strings.Contains(out, "Usage: chmod") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	for _, flag := range []string{"--dereference", "--no-dereference", "-H", "-L", "-P"} {
		if !strings.Contains(out, flag) {
			t.Errorf("--help missing %s: %q", flag, out)
		}
	}
	out, _, code = runTool(t, t.TempDir(), "--version")
	if code != 0 || !strings.Contains(out, "chmod") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}
}

func TestChmodNoDereferenceSkipsSymlinkOperand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod is unix-only")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "target"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target", filepath.Join(dir, "link")); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}
	_, errb, code := runTool(t, dir, "--no-dereference", "u+x", "link")
	if code != 0 || errb != "" {
		t.Fatalf("chmod --no-dereference: code=%d err=%q", code, errb)
	}
	fi, err := os.Stat(filepath.Join(dir, "target"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o644 {
		t.Fatalf("target mode=%#o want 0644", fi.Mode().Perm())
	}
}

func TestChmodNoDereferenceAbbreviationSkipsSymlinkOperand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod is unix-only")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "target"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target", filepath.Join(dir, "link")); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}
	_, errb, code := runTool(t, dir, "--no-deref", "u+x", "link")
	if code != 0 || errb != "" {
		t.Fatalf("chmod --no-deref: code=%d err=%q", code, errb)
	}
	fi, err := os.Stat(filepath.Join(dir, "target"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o644 {
		t.Fatalf("target mode=%#o want 0644", fi.Mode().Perm())
	}
}

func TestChmodDefaultDereferencesSymlinkOperand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod is unix-only")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "target"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target", filepath.Join(dir, "link")); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}
	_, errb, code := runTool(t, dir, "u+x", "link")
	if code != 0 || errb != "" {
		t.Fatalf("chmod symlink operand: code=%d err=%q", code, errb)
	}
	fi, err := os.Stat(filepath.Join(dir, "target"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o744 {
		t.Fatalf("target mode=%#o want 0744", fi.Mode().Perm())
	}
}

func TestChmodDereferenceOptionsLastWins(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod is unix-only")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "target"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target", filepath.Join(dir, "link")); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}
	_, errb, code := runTool(t, dir, "--no-dereference", "--dereference", "u+x", "link")
	if code != 0 || errb != "" {
		t.Fatalf("chmod --dereference last: code=%d err=%q", code, errb)
	}
	fi, err := os.Stat(filepath.Join(dir, "target"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o744 {
		t.Fatalf("target mode after --dereference last=%#o want 0744", fi.Mode().Perm())
	}
	if err := os.Chmod(filepath.Join(dir, "target"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, errb, code = runTool(t, dir, "--dereference", "--no-dereference", "u+x", "link")
	if code != 0 || errb != "" {
		t.Fatalf("chmod --no-dereference last: code=%d err=%q", code, errb)
	}
	fi, err = os.Stat(filepath.Join(dir, "target"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o644 {
		t.Fatalf("target mode after --no-dereference last=%#o want 0644", fi.Mode().Perm())
	}
}

func TestChmodRecursiveSymlinkTraversalFlags(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod is unix-only")
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "target"), 0o755); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(dir, "target", "child")
	if err := os.WriteFile(child, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target", filepath.Join(dir, "linkdir")); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}
	_, errb, code := runTool(t, dir, "-R", "-P", "u+x", "linkdir")
	if code != 0 || errb != "" {
		t.Fatalf("chmod -R -P: code=%d err=%q", code, errb)
	}
	fi, err := os.Stat(child)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o644 {
		t.Fatalf("-P followed command-line symlink: mode=%#o", fi.Mode().Perm())
	}
	_, errb, code = runTool(t, dir, "-R", "-H", "u+x", "linkdir")
	if code != 0 || errb != "" {
		t.Fatalf("chmod -R -H: code=%d err=%q", code, errb)
	}
	fi, err = os.Stat(child)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o744 {
		t.Fatalf("-H did not follow command-line symlink: mode=%#o", fi.Mode().Perm())
	}
}

func TestChmodRecursiveTraversalOptionsLastWins(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod is unix-only")
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "target"), 0o755); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(dir, "target", "child")
	if err := os.WriteFile(child, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target", filepath.Join(dir, "linkdir")); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}
	_, errb, code := runTool(t, dir, "-R", "-P", "-H", "u+x", "linkdir")
	if code != 0 || errb != "" {
		t.Fatalf("chmod -P -H: code=%d err=%q", code, errb)
	}
	fi, err := os.Stat(child)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o744 {
		t.Fatalf("-H last did not follow command-line symlink: mode=%#o", fi.Mode().Perm())
	}
	if err := os.Chmod(child, 0o644); err != nil {
		t.Fatal(err)
	}
	_, errb, code = runTool(t, dir, "-R", "-H", "-P", "u+x", "linkdir")
	if code != 0 || errb != "" {
		t.Fatalf("chmod -H -P: code=%d err=%q", code, errb)
	}
	fi, err = os.Stat(child)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o644 {
		t.Fatalf("-P last followed command-line symlink: mode=%#o", fi.Mode().Perm())
	}
}

func TestChmodRecursiveFollowAllSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod is unix-only")
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "d"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "target"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../target", filepath.Join(dir, "d", "linkfile")); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}
	_, errb, code := runTool(t, dir, "-R", "-L", "u+x", "d")
	if code != 0 || errb != "" {
		t.Fatalf("chmod -R -L: code=%d err=%q", code, errb)
	}
	fi, err := os.Stat(filepath.Join(dir, "target"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o744 {
		t.Fatalf("-L did not follow encountered symlink: mode=%#o", fi.Mode().Perm())
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

func TestChmodReferenceValueThatLooksLikeMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod is unix-only")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "-w"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "target"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, option := range []string{"--reference", "--ref"} {
		if err := os.Chmod(filepath.Join(dir, "target"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, errb, code := runTool(t, dir, option, "-w", "target")
		if code != 0 || errb != "" {
			t.Fatalf("chmod %s -w: code=%d err=%q", option, code, errb)
		}
		fi, err := os.Stat(filepath.Join(dir, "target"))
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm() != 0o600 {
			t.Fatalf("%s target mode=%#o want 0600", option, fi.Mode().Perm())
		}
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
}
