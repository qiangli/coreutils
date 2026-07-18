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

func TestLogname(t *testing.T) {
	u, userErr := user.Current()
	if userErr != nil || strings.TrimSpace(u.Username) == "" {
		t.Skipf("user.Current: %v", userErr)
	}
	want := bareUser(strings.TrimSpace(u.Username))

	var out, err bytes.Buffer
	code := run(&tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &err}}, nil)
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

func TestLognameIgnoresEnvironmentAccountNames(t *testing.T) {
	want := loginName()
	if want == "" {
		t.Skip("current account name unavailable")
	}
	for _, key := range []string{"LOGNAME", "USER", "LNAME", "USERNAME"} {
		t.Setenv(key, "not-the-login-account")
	}

	var out, err bytes.Buffer
	code := run(&tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &err}}, nil)
	if code != 0 || out.String() != want+"\n" || err.Len() != 0 {
		t.Fatalf("logname = (%q, %q, %d), want (%q, %q, 0)", out.String(), err.String(), code, want+"\n", "")
	}
}

func TestLognameRejectsOperandsAndUnknownOptions(t *testing.T) {
	for _, args := range [][]string{{"extra"}, {"--unknown"}} {
		var out, err bytes.Buffer
		code := run(&tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &err}}, args)
		if code != 2 || err.Len() == 0 {
			t.Errorf("args %q = (%q, %q, %d), want usage error", args, out.String(), err.String(), code)
		}
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
