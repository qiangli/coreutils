package chgrpcmd

import (
	"bytes"
	"context"
	"os"
	"os/user"
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

// Setting a file's group to the user's own primary group is the only
// change a non-root test can perform; it exercises lookup + syscall.
func TestChgrpSelf(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chgrp is unix-only")
	}
	u, err := user.Current()
	if err != nil {
		t.Skipf("user.Current: %v", err)
	}
	specs := []string{u.Gid} // numeric
	if g, err := user.LookupGroupId(u.Gid); err == nil {
		specs = append(specs, g.Name) // by name
	}
	for _, spec := range specs {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "f"), nil, 0o644); err != nil {
			t.Fatal(err)
		}
		_, errb, code := runTool(t, dir, spec, "f")
		if code != 0 || errb != "" {
			t.Errorf("chgrp %q: code=%d err=%q", spec, code, errb)
		}
	}
}

func TestChgrpRecursive(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chgrp is unix-only")
	}
	u, err := user.Current()
	if err != nil {
		t.Skipf("user.Current: %v", err)
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "d", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "d", "sub", "f"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "-R", u.Gid, "d")
	if code != 0 || errb != "" {
		t.Errorf("chgrp -R: code=%d err=%q", code, errb)
	}
}

func TestChgrpErrors(t *testing.T) {
	dir := t.TempDir()
	_, errb, code := runTool(t, dir)
	if code != 2 || !strings.Contains(errb, "missing operand") {
		t.Errorf("no args: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, dir, "staff")
	if code != 2 || !strings.Contains(errb, "missing operand after 'staff'") {
		t.Errorf("no file: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, dir, "--frobnicate", "g", "f")
	if code != 2 || !strings.Contains(errb, "frobnicate") || !strings.Contains(errb, "pure-Go") {
		t.Errorf("unknown flag: code=%d err=%q", code, errb)
	}
	if runtime.GOOS == "windows" {
		return
	}
	_, errb, code = runTool(t, dir, "no-such-group-xyzzy", "f")
	if code != 1 || !strings.Contains(errb, "invalid group: 'no-such-group-xyzzy'") {
		t.Errorf("invalid group: code=%d err=%q", code, errb)
	}
	u, err := user.Current()
	if err == nil {
		_, errb, code = runTool(t, dir, u.Gid, "no-such-file")
		if code != 1 || !strings.Contains(errb, "cannot access 'no-such-file'") {
			t.Errorf("missing file: code=%d err=%q", code, errb)
		}
	}
}

func TestChgrpWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only assertion")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "somegroup", "f")
	if code != 1 || !strings.Contains(errb, "not supported on windows") {
		t.Errorf("windows: code=%d err=%q", code, errb)
	}
}

func TestChgrpHelpAndVersion(t *testing.T) {
	out, _, code := runTool(t, t.TempDir(), "--help")
	if code != 0 || !strings.Contains(out, "Usage: chgrp") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	if !strings.Contains(out, "--from") || !strings.Contains(out, "--no-dereference") {
		t.Errorf("--help missing uutils-compatible flags: %q", out)
	}
	out, _, code = runTool(t, t.TempDir(), "--version")
	if code != 0 || !strings.Contains(out, "chgrp") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}
}

func TestChgrpFromFlag(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chgrp is unix-only")
	}
	u, err := user.Current()
	if err != nil {
		t.Skipf("user.Current: %v", err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "--from=:"+u.Gid, u.Gid, "f")
	if code != 0 || errb != "" {
		t.Fatalf("chgrp --from group: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, dir, "--from=99999:99999", u.Gid, "f")
	if code != 0 || errb != "" {
		t.Fatalf("chgrp --from no match: code=%d err=%q", code, errb)
	}
}

func TestChgrpVerbose(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chgrp is unix-only")
	}
	u, err := user.Current()
	if err != nil {
		t.Skipf("user.Current: %v", err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	out, errb, code := runTool(t, dir, "-v", u.Gid, "f")
	if code != 0 || errb != "" {
		t.Fatalf("chgrp -v: code=%d err=%q", code, errb)
	}
	if !strings.Contains(out, "group of 'f' retained as") && !strings.Contains(out, "changed group of 'f'") {
		t.Errorf("expected verbose output, got: %q", out)
	}
}

func TestChgrpChanges(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chgrp is unix-only")
	}
	u, err := user.Current()
	if err != nil {
		t.Skipf("user.Current: %v", err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, code := runTool(t, dir, "-c", u.Gid, "f")
	if code != 0 {
		t.Fatalf("chgrp -c: code=%d", code)
	}
	if out != "" {
		t.Errorf("expected no output for unchanged group with -c, got: %q", out)
	}
}

func TestChgrpSilent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chgrp is unix-only")
	}
	dir := t.TempDir()
	_, errb, code := runTool(t, dir, "-f", "staff", "no-such-file")
	if code != 1 {
		t.Fatalf("chgrp -f: expected code=1, got=%d", code)
	}
	if errb != "" {
		t.Errorf("expected no stderr with -f, got: %q", errb)
	}
}

func TestChgrpReference(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chgrp is unix-only")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ref"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "f"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "--reference=ref", "f")
	if code != 0 || errb != "" {
		t.Fatalf("chgrp --reference: code=%d err=%q", code, errb)
	}
}

func TestChgrpPreserveRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chgrp is unix-only")
	}
	u, err := user.Current()
	if err != nil {
		t.Skipf("user.Current: %v", err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "-R", "--preserve-root", u.Gid, "f")
	if code != 0 || errb != "" {
		t.Fatalf("chgrp --preserve-root on non-root: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, dir, "-R", "--preserve-root", u.Gid, "/")
	if code != 1 || !strings.Contains(errb, "dangerous to operate recursively on '/'") {
		t.Fatalf("chgrp --preserve-root on /: code=%d err=%q", code, errb)
	}
}

func TestChgrpDereference(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chgrp is unix-only")
	}
	u, err := user.Current()
	if err != nil {
		t.Skipf("user.Current: %v", err)
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "target"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target", filepath.Join(dir, "link")); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}
	out, errb, code := runTool(t, dir, u.Gid, "link")
	if code != 0 || errb != "" {
		t.Fatalf("chgrp symlink (dereference): code=%d err=%q", code, errb)
	}
	_ = out
	// The dereference target's resulting gid is platform-specific to read
	// (syscall.Stat_t is unix-only and would break the windows vet/compile);
	// exit code 0 + empty stderr above is the portable assertion.
	if _, err := os.Stat(filepath.Join(dir, "target")); err != nil {
		t.Fatal(err)
	}
}

func TestChgrpNoDereference(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chgrp is unix-only")
	}
	u, err := user.Current()
	if err != nil {
		t.Skipf("user.Current: %v", err)
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "target"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target", filepath.Join(dir, "link")); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}
	out, errb, code := runTool(t, dir, "--no-dereference", u.Gid, "link")
	if code != 0 || errb != "" {
		t.Fatalf("chgrp --no-dereference: code=%d err=%q", code, errb)
	}
	_ = out
}

func TestChgrpQuietFlag(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chgrp is unix-only")
	}
	dir := t.TempDir()
	_, errb, code := runTool(t, dir, "--quiet", "staff", "no-such-file")
	if code != 1 {
		t.Fatalf("chgrp --quiet: expected code=1, got=%d", code)
	}
	if errb != "" {
		t.Errorf("expected no stderr with --quiet, got: %q", errb)
	}
}
