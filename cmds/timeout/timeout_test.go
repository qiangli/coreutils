package timeoutcmd

import (
	"bytes"
	"context"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/coreutils/tool"
)

func TestParseDuration(t *testing.T) {
	cases := map[string]time.Duration{
		"5":    5 * time.Second,
		"5s":   5 * time.Second,
		"2m":   2 * time.Minute,
		"1h":   time.Hour,
		"1d":   24 * time.Hour,
		"0.5":  500 * time.Millisecond,
		"1.5s": 1500 * time.Millisecond,
		"0":    0,
	}
	for in, want := range cases {
		got, err := parseDuration(in)
		if err != nil || got != want {
			t.Errorf("parseDuration(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "abc", "5x", "s"} {
		if _, err := parseDuration(bad); err == nil {
			t.Errorf("parseDuration(%q) should error", bad)
		}
	}
}

func TestSignalByName(t *testing.T) {
	for _, name := range []string{"TERM", "SIGKILL", "int", "9"} {
		if signalByName(name) == nil {
			t.Errorf("signalByName(%q) = nil, want a signal", name)
		}
	}
	if signalByName("NOSUCH") != nil {
		t.Error("signalByName(NOSUCH) should be nil")
	}
}

func TestExitStatus(t *testing.T) {
	if got := exitStatus(nil, true, false); got != 124 {
		t.Errorf("timed-out exit = %d, want 124", got)
	}
	if got := exitStatus(nil, false, false); got != 0 {
		t.Errorf("clean exit = %d, want 0", got)
	}
}

func TestTimeoutHelpAndVersion(t *testing.T) {
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   t.TempDir(),
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
	}

	code := cmd.Run(rc, []string{"--help"})
	if code != 0 || !strings.Contains(out.String(), "Usage: timeout") {
		t.Errorf("--help: code=%d out=%q", code, out.String())
	}
	for _, want := range []string{"-f, --foreground", "-k, --kill-after", "-p, --preserve-status", "-s, --signal", "-v, --verbose", "-h, --help", "-V, --version"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("--help missing %q in %q", want, out.String())
		}
	}

	out.Reset()
	code = cmd.Run(rc, []string{"--version"})
	if code != 0 || !strings.Contains(out.String(), "timeout") {
		t.Errorf("--version: code=%d out=%q", code, out.String())
	}

	out.Reset()
	code = cmd.Run(rc, []string{"-h"})
	if code != 0 || !strings.Contains(out.String(), "Usage: timeout") {
		t.Errorf("-h: code=%d out=%q", code, out.String())
	}

	out.Reset()
	code = cmd.Run(rc, []string{"-V"})
	if code != 0 || !strings.Contains(out.String(), "timeout") {
		t.Errorf("-V: code=%d out=%q", code, out.String())
	}
}

func TestTimeoutShortOptionSurface(t *testing.T) {
	// A no-op COMMAND that exists per platform: /bin/true has no windows
	// counterpart, so use cmd.exe there.
	env := []string{"PATH=/bin:/usr/bin"}
	noop := []string{"true"}
	if runtime.GOOS == "windows" {
		env = []string{"PATH=" + os.Getenv("PATH")}
		noop = []string{"cmd", "/c", "exit 0"}
	}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(),
		Dir: t.TempDir(),
		Env: env,
		Stdio: tool.Stdio{
			In:  strings.NewReader(""),
			Out: &out,
			Err: &errb,
		},
	}
	code := cmd.Run(rc, append([]string{"-f", "-p", "-s", "TERM", "-k", "1s", "1s"}, noop...))
	if code != 0 || errb.String() != "" {
		t.Fatalf("short options: code=%d out=%q err=%q", code, out.String(), errb.String())
	}
}
