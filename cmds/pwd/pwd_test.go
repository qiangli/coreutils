package pwdcmd

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

func runIn(t *testing.T, dir string, args ...string) (stdout, stderr string, code int) {
	return runInEnv(t, dir, []string{"PWD=" + dir}, args...)
}

func runInEnv(t *testing.T, dir string, env []string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Env:   env,
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
	}
	code = cmd.Run(rc, args)
	return out.String(), errb.String(), code
}

func TestPwdDefaultPrintsDir(t *testing.T) {
	dir := t.TempDir()
	out, errb, code := runIn(t, dir)
	if code != 0 || out != dir+"\n" || errb != "" {
		t.Errorf("pwd = (%q, %q, %d), want (%q, \"\", 0)", out, errb, code, dir+"\n")
	}
	out, _, code = runIn(t, dir, "-L")
	if code != 0 || out != dir+"\n" {
		t.Errorf("pwd -L = (%q, %d)", out, code)
	}
}

func TestPwdPhysicalResolvesSymlinks(t *testing.T) {
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "lnk")
	if err := os.Symlink(real, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlink creation not permitted: %v", err)
		}
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(link)
	if err != nil {
		t.Fatal(err)
	}

	// default (-L): the logical name, symlink intact
	out, _, code := runIn(t, link)
	if code != 0 || out != link+"\n" {
		t.Errorf("pwd = (%q, %d), want (%q, 0)", out, code, link+"\n")
	}
	// -P: fully resolved
	out, _, code = runIn(t, link, "-P")
	if code != 0 || out != want+"\n" {
		t.Errorf("pwd -P = (%q, %d), want (%q, 0)", out, code, want+"\n")
	}
	// last one wins, both orders and combined form
	out, _, _ = runIn(t, link, "-P", "-L")
	if out != link+"\n" {
		t.Errorf("pwd -P -L = %q, want logical %q", out, link+"\n")
	}
	out, _, _ = runIn(t, link, "-L", "-P")
	if out != want+"\n" {
		t.Errorf("pwd -L -P = %q, want physical %q", out, want+"\n")
	}
	out, _, _ = runIn(t, link, "-LP")
	if out != want+"\n" {
		t.Errorf("pwd -LP = %q, want physical %q", out, want+"\n")
	}
}

func TestPwdLogicalUsesValidatedPWD(t *testing.T) {
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "lnk")
	if err := os.Symlink(real, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlink creation not permitted: %v", err)
		}
		t.Fatal(err)
	}
	physical, err := filepath.EvalSymlinks(real)
	if err != nil {
		t.Fatal(err)
	}

	for _, args := range [][]string{nil, {"-L"}} {
		out, errb, code := runInEnv(t, physical, []string{"PWD=" + link}, args...)
		if code != 0 || out != link+"\n" || errb != "" {
			t.Errorf("pwd %v = (%q, %q, %d), want logical path", args, out, errb, code)
		}
	}

	out, errb, code := runInEnv(t, physical, []string{"PWD=" + link}, "-P")
	if code != 0 || out != physical+"\n" || errb != "" {
		t.Errorf("pwd -P = (%q, %q, %d), want %q", out, errb, code, physical+"\n")
	}
}

func TestPwdLogicalRejectsInvalidPWD(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "child"), 0o755); err != nil {
		t.Fatal(err)
	}
	physical, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	other := t.TempDir()

	cases := []struct {
		name string
		env  []string
	}{
		{name: "unset"},
		{name: "relative", env: []string{"PWD=relative"}},
		{name: "dot component", env: []string{"PWD=" + dir + string(filepath.Separator) + "."}},
		{name: "dot dot component", env: []string{"PWD=" + dir + string(filepath.Separator) + "child" + string(filepath.Separator) + ".."}},
		{name: "different directory", env: []string{"PWD=" + other}},
		{name: "last assignment wins", env: []string{"PWD=" + dir, "PWD=" + other}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, errb, code := runInEnv(t, dir, tc.env, "-L")
			if code != 0 || out != physical+"\n" || errb != "" {
				t.Errorf("pwd -L = (%q, %q, %d), want physical fallback %q", out, errb, code, physical+"\n")
			}
		})
	}
}

func TestPwdIgnoresOperands(t *testing.T) {
	dir := t.TempDir()
	out, errb, code := runIn(t, dir, "ignored")
	if code != 0 || out != dir+"\n" || !strings.Contains(errb, "ignoring non-option arguments") {
		t.Errorf("pwd ignored = (%q, %q, %d)", out, errb, code)
	}
}

func TestPwdErrors(t *testing.T) {
	_, errb, code := runIn(t, t.TempDir(), "--frobnicate")
	if code != 2 || !strings.Contains(errb, "frobnicate") || !strings.Contains(errb, "pure-Go") {
		t.Errorf("unknown flag: code=%d err=%q", code, errb)
	}
	_, errb, code = runIn(t, "")
	if code != 1 || !strings.Contains(errb, "cannot determine current directory") {
		t.Errorf("empty dir: code=%d err=%q", code, errb)
	}
	// -P on a directory that does not exist
	_, _, code = runIn(t, filepath.Join(t.TempDir(), "gone"), "-P")
	if code != 1 {
		t.Errorf("-P on missing dir: code=%d, want 1", code)
	}
}

func TestPwdHelpAndVersion(t *testing.T) {
	out, _, code := runIn(t, t.TempDir(), "--help")
	if code != 0 || !strings.Contains(out, "Usage: pwd") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	out, _, code = runIn(t, t.TempDir(), "-h")
	if code != 0 || !strings.Contains(out, "Usage: pwd") {
		t.Errorf("-h: code=%d out=%q", code, out)
	}
	out, _, code = runIn(t, t.TempDir(), "--version")
	if code != 0 || !strings.Contains(out, "pwd") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}
	out, _, code = runIn(t, t.TempDir(), "-V")
	if code != 0 || !strings.Contains(out, "pwd") {
		t.Errorf("-V: code=%d out=%q", code, out)
	}
}
