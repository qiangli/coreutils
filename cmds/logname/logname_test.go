package lognamecmd

import (
	"bytes"
	"context"
	"os/user"
	"runtime"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func newRC(out, err *bytes.Buffer) *tool.RunContext {
	return &tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: out, Err: err}}
}

func TestLogname(t *testing.T) {
	want := loginName()
	if want == "" {
		t.Skip("login name unavailable on this host")
	}
	var out, err bytes.Buffer
	code := run(newRC(&out, &err), nil)
	if code != 0 {
		t.Fatalf("code=%d err=%s", code, err.String())
	}
	if got := out.String(); got != want+"\n" {
		t.Fatalf("output=%q, want %q", got, want+"\n")
	}
	if err.Len() != 0 {
		t.Fatalf("stderr=%q, want empty", err.String())
	}
}

// POSIX: logname must not consult LOGNAME/USER/LNAME/USERNAME.
func TestLognameIgnoresEnvironmentAccountNames(t *testing.T) {
	want := loginName()
	if want == "" {
		t.Skip("login name unavailable on this host")
	}
	for _, key := range []string{"LOGNAME", "USER", "LNAME", "USERNAME"} {
		t.Setenv(key, "not-the-login-account")
	}
	var out, err bytes.Buffer
	code := run(newRC(&out, &err), nil)
	if code != 0 || out.String() != want+"\n" || err.Len() != 0 {
		t.Fatalf("logname = (%q, %q, %d), want (%q, %q, 0)", out.String(), err.String(), code, want+"\n", "")
	}
}

// POSIX stderr contract when getlogin() has no answer:
// "logname: no login name\n", exit non-zero.
func TestLognameNoLoginName(t *testing.T) {
	var out, err bytes.Buffer
	code := runWith(newRC(&out, &err), nil, func() string { return "" })
	if code != 1 {
		t.Fatalf("code=%d, want 1", code)
	}
	if out.Len() != 0 {
		t.Fatalf("stdout=%q, want empty", out.String())
	}
	if got, want := err.String(), "logname: no login name\n"; got != want {
		t.Fatalf("stderr=%q, want %q", got, want)
	}
}

func TestLognameRejectsOperandsAndUnknownOptions(t *testing.T) {
	for _, args := range [][]string{{"extra"}, {"--unknown"}} {
		var out, err bytes.Buffer
		code := run(newRC(&out, &err), args)
		if code != 2 || err.Len() == 0 {
			t.Errorf("args %q = (%q, %q, %d), want usage error", args, out.String(), err.String(), code)
		}
	}
}

func TestResolveLoginUID(t *testing.T) {
	// Unset / empty / unresolvable uids must never yield a name.
	for _, uid := range []string{"", "   ", "4294967295", "-1", "999999987"} {
		if got := resolveLoginUID(uid); got != "" {
			t.Errorf("resolveLoginUID(%q) = %q, want empty", uid, got)
		}
	}
	// root (uid 0) resolves on every POSIX host that has a passwd entry.
	if u, err := user.LookupId("0"); err == nil && strings.TrimSpace(u.Username) != "" {
		want := bareUser(strings.TrimSpace(u.Username))
		if got := resolveLoginUID("0"); got != want {
			t.Errorf("resolveLoginUID(\"0\") = %q, want %q", got, want)
		}
		// Whitespace/newline exactly as read from /proc must be trimmed.
		if got := resolveLoginUID("  0  \n"); got != want {
			t.Errorf("resolveLoginUID with whitespace = %q, want %q", got, want)
		}
	}
}

func TestLoginNameFromLoginUIDEmptyOffLinux(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("only asserts the non-Linux short-circuit")
	}
	if got := loginNameFromLoginUID(); got != "" {
		t.Errorf("loginNameFromLoginUID = %q, want empty off linux", got)
	}
}

func TestBareUser(t *testing.T) {
	want := `domain\name`
	if runtime.GOOS == "windows" {
		want = "name"
	}
	if got := bareUser(`domain\name`); got != want {
		t.Fatalf("bareUser = %q, want %q", got, want)
	}
}
