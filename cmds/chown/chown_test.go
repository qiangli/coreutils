package chowncmd

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

func currentUser(t *testing.T) *user.User {
	t.Helper()
	u, err := user.Current()
	if err != nil {
		t.Skipf("user.Current: %v", err)
	}
	return u
}

// Self-chown (same uid/gid) is the only ownership change a non-root
// test can perform, but it exercises spec parsing and the syscall path.
func TestChownSelf(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chown is unix-only")
	}
	u := currentUser(t)
	specs := []string{
		u.Username,          // name
		u.Uid,               // numeric
		u.Uid + ":" + u.Gid, // numeric uid:gid
		u.Username + ":",    // trailing colon: login group
		":" + u.Gid,         // group only
		":",                 // no-op
	}
	for _, spec := range specs {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "f"), nil, 0o644); err != nil {
			t.Fatal(err)
		}
		_, errb, code := runTool(t, dir, spec, "f")
		if code != 0 || errb != "" {
			t.Errorf("chown %q: code=%d err=%q", spec, code, errb)
		}
	}
}

func TestChownRecursive(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chown is unix-only")
	}
	u := currentUser(t)
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "d", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "d", "sub", "f"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "-R", u.Uid, "d")
	if code != 0 || errb != "" {
		t.Errorf("chown -R: code=%d err=%q", code, errb)
	}
}

func TestChownErrors(t *testing.T) {
	dir := t.TempDir()
	_, errb, code := runTool(t, dir)
	if code != 2 || !strings.Contains(errb, "missing operand") {
		t.Errorf("no args: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, dir, "root")
	if code != 2 || !strings.Contains(errb, "missing operand after 'root'") {
		t.Errorf("no file: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, dir, "--frobnicate", "u", "f")
	if code != 2 || !strings.Contains(errb, "frobnicate") || !strings.Contains(errb, "pure-Go") {
		t.Errorf("unknown flag: code=%d err=%q", code, errb)
	}
	if runtime.GOOS == "windows" {
		return
	}
	_, errb, code = runTool(t, dir, "no-such-user-xyzzy", "f")
	if code != 1 || !strings.Contains(errb, "invalid user: 'no-such-user-xyzzy'") {
		t.Errorf("invalid user: code=%d err=%q", code, errb)
	}
	u := currentUser(t)
	_, errb, code = runTool(t, dir, u.Uid, "no-such-file")
	if code != 1 || !strings.Contains(errb, "cannot access 'no-such-file'") {
		t.Errorf("missing file: code=%d err=%q", code, errb)
	}
}

func TestChownWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only assertion")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "someone", "f")
	if code != 1 || !strings.Contains(errb, "not supported on windows") {
		t.Errorf("windows: code=%d err=%q", code, errb)
	}
}

func TestChownHelpAndVersion(t *testing.T) {
	out, _, code := runTool(t, t.TempDir(), "--help")
	if code != 0 || !strings.Contains(out, "Usage: chown") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	if !strings.Contains(out, "--no-dereference") {
		t.Errorf("--help missing --no-dereference: %q", out)
	}
	out, _, code = runTool(t, t.TempDir(), "--version")
	if code != 0 || !strings.Contains(out, "chown") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}
}

func TestChownVerbose(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chown is unix-only")
	}
	u := currentUser(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	out, errb, code := runTool(t, dir, "-v", u.Uid, "f")
	if code != 0 || errb != "" {
		t.Fatalf("chown -v: code=%d err=%q", code, errb)
	}
	if !strings.Contains(out, "ownership of 'f' retained") && !strings.Contains(out, "changed ownership of 'f'") {
		t.Errorf("expected verbose output, got: %q", out)
	}
}

func TestChownChanges(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chown is unix-only")
	}
	u := currentUser(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, code := runTool(t, dir, "-c", u.Uid, "f")
	if code != 0 {
		t.Fatalf("chown -c: code=%d", code)
	}
	if out != "" {
		t.Errorf("expected no output for unchanged ownership with -c, got: %q", out)
	}
}

func TestChownSilent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chown is unix-only")
	}
	dir := t.TempDir()
	_, errb, code := runTool(t, dir, "-f", "nobody", "no-such-file")
	if code != 1 {
		t.Fatalf("chown -f: expected code=1, got=%d", code)
	}
	if errb != "" {
		t.Errorf("expected no stderr with -f, got: %q", errb)
	}
}

func TestChownReference(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chown is unix-only")
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
		t.Fatalf("chown --reference: code=%d err=%q", code, errb)
	}
}

func TestChownFromFlag(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chown is unix-only")
	}
	u := currentUser(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "--from="+u.Uid, u.Uid, "f")
	if code != 0 || errb != "" {
		t.Fatalf("chown --from: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, dir, "--from=99999", u.Uid, "f")
	if code != 0 {
		t.Fatalf("chown --from (no match): code=%d err=%q", code, errb)
	}
}

func TestChownPreserveRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chown is unix-only")
	}
	u := currentUser(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "-R", "--preserve-root", u.Uid, "f")
	if code != 0 || errb != "" {
		t.Fatalf("chown --preserve-root on non-root: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, dir, "-R", "--preserve-root", u.Uid, "/")
	if code != 1 || !strings.Contains(errb, "dangerous to operate recursively on '/'") {
		t.Fatalf("chown --preserve-root on /: code=%d err=%q", code, errb)
	}
}

func TestChownDereference(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chown is unix-only")
	}
	u := currentUser(t)
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "target"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target", filepath.Join(dir, "link")); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}
	out, errb, code := runTool(t, dir, u.Uid, "link")
	if code != 0 || errb != "" {
		t.Fatalf("chown symlink (dereference): code=%d err=%q", code, errb)
	}
	_ = out
}

func TestChownNoDereference(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chown is unix-only")
	}
	u := currentUser(t)
	for _, option := range []string{"-h", "--no-dereference"} {
		t.Run(option, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.MkdirAll(filepath.Join(dir, "target"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink("target", filepath.Join(dir, "link")); err != nil {
				t.Skipf("symlinks not supported: %v", err)
			}
			_, errb, code := runTool(t, dir, option, u.Uid, "link")
			if code != 0 || errb != "" {
				t.Fatalf("chown %s: code=%d err=%q", option, code, errb)
			}
		})
	}
}
