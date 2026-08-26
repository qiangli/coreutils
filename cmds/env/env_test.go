package envcmd

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

type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }

func runTool(t *testing.T, env []string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   t.TempDir(),
		Env:   env,
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
	}
	code = cmd.Run(rc, args)
	return out.String(), errb.String(), code
}

func TestEnv(t *testing.T) {
	base := []string{"A=1", "B=2", "C=3"}
	cases := []struct {
		env  []string
		args []string
		want string
	}{
		// Print in environ order, not sorted.
		{[]string{"B=2", "A=1"}, nil, "B=2\nA=1\n"},
		// -i: empty environment.
		{base, []string{"-i"}, ""},
		{base, []string{"-i", "X=9"}, "X=9\n"},
		// '-' first operand is a synonym for -i.
		{base, []string{"-", "X=9"}, "X=9\n"},
		// -u removals (repeatable).
		{base, []string{"-u", "B"}, "A=1\nC=3\n"},
		{base, []string{"-u", "B", "-u", "A"}, "C=3\n"},
		{base, []string{"-u", "MISSING"}, "A=1\nB=2\nC=3\n"},
		// Assignments: existing NAME updates in place, new appends.
		{base, []string{"B=9"}, "A=1\nB=9\nC=3\n"},
		{base, []string{"D=4", "B="}, "A=1\nB=\nC=3\nD=4\n"},
		// Duplicate environ names: last value wins, first slot kept.
		{[]string{"A=1", "B=2", "A=3"}, nil, "A=3\nB=2\n"},
		// Empty value round-trips.
		{[]string{"A="}, nil, "A=\n"},
	}
	for _, c := range cases {
		out, errb, code := runTool(t, c.env, c.args...)
		if out != c.want || code != 0 || errb != "" {
			t.Errorf("env %q (env=%q) = (%q, %q, %d), want (%q, \"\", 0)", c.args, c.env, out, errb, code, c.want)
		}
	}
}

func TestEnvNullAndFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "envfile"), []byte("B=from-file\nD=4\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Env:   []string{"A=1", "B=2"},
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
	}
	code := cmd.Run(rc, []string{"--file", "envfile", "--null"})
	if code != 0 || errb.String() != "" {
		t.Fatalf("code=%d err=%q", code, errb.String())
	}
	if want := "A=1\x00B=from-file\x00D=4\x00"; out.String() != want {
		t.Fatalf("out=%q want %q", out.String(), want)
	}
}

func TestEnvRealShortAliases(t *testing.T) {
	dir := t.TempDir()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   t.TempDir(),
		Env:   []string{"A=1", "B=2"},
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
	}
	code := cmd.Run(rc, []string{"-i", "-C", dir, "-u", "A", "-0", "B=3"})
	if code != 0 || errb.String() != "" || out.String() != "B=3\x00" || rc.Dir != dir {
		t.Fatalf("real short aliases = (%q, %q, %d, dir=%q)", out.String(), errb.String(), code, rc.Dir)
	}

	help, _, code := runTool(t, nil, "--help")
	if code != 0 {
		t.Fatalf("env --help code = %d", code)
	}
	for _, opt := range []string{"-i, --ignore-environment", "-u, --unset", "-0, --null", "-C, --chdir", "-a, --argv0", "-S, --split-string"} {
		if !strings.Contains(help, opt) {
			t.Fatalf("env --help missing real short alias %q:\n%s", opt, help)
		}
	}
}

func TestEnvRunsCommandWithModifiedEnvironment(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires a POSIX shell")
	}
	path := os.Getenv("PATH")
	out, errb, code := runTool(t, []string{"PATH=" + path}, "FOO=1", "sh", "-c", `printf %s "$FOO"`)
	if code != 0 || errb != "" || out != "1" {
		t.Fatalf("assignment command = (%q, %q, %d)", out, errb, code)
	}

	out, errb, code = runTool(t, []string{"PATH=" + path}, "-u", "PATH", "sh", "-c", "exit 0")
	if code != 0 || errb != "" || out != "" {
		t.Fatalf("unset PATH command lookup = (%q, %q, %d)", out, errb, code)
	}
	out, errb, code = runTool(t, []string{"PATH=" + path}, "-u", "PATH", "/usr/bin/env")
	if code != 0 || errb != "" || strings.Contains(out, "PATH=") {
		t.Fatalf("unset PATH child environment = (%q, %q, %d)", out, errb, code)
	}

	out, errb, code = runTool(t, []string{"PATH=" + path, "ENV_SENTINEL=present"}, "-i", "sh", "-c", "env")
	if code != 0 || errb != "" || strings.Contains(out, "ENV_SENTINEL=") {
		t.Fatalf("empty command environment = (%q, %q, %d)", out, errb, code)
	}
}

func TestEnvChdirArgv0AndSplitString(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires a POSIX shell")
	}
	dir := t.TempDir()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   t.TempDir(),
		Env:   []string{"PATH=" + os.Getenv("PATH")},
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
	}
	code := cmd.Run(rc, []string{"--chdir", dir, "--argv0", "chosen-argv0", "sh", "-c", `printf '%s\n%s' "$PWD" "$0"`})
	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 || errb.String() != "" || out.String() != realDir+"\nchosen-argv0" || rc.Dir != dir {
		t.Fatalf("chdir/argv0 = (%q, %q, %d, dir=%q)", out.String(), errb.String(), code, rc.Dir)
	}

	out.Reset()
	code = cmd.Run(rc, []string{"-S", `SPLIT_VALUE=split sh -c 'printf %s "$SPLIT_VALUE"'`})
	if code != 0 || errb.String() != "" || out.String() != "split" {
		t.Fatalf("split-string = (%q, %q, %d)", out.String(), errb.String(), code)
	}
}

func TestEnvIgnoreSignal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires POSIX signal and shell semantics")
	}
	out, errb, code := runTool(t, []string{"PATH=" + os.Getenv("PATH")}, "--ignore-signal=TERM", "sh", "-c", `kill -TERM $$; printf survived`)
	if code != 0 || errb != "" || out != "survived" {
		t.Fatalf("ignore signal = (%q, %q, %d)", out, errb, code)
	}

	out, errb, code = runTool(t, []string{"A=1"}, "--ignore-signal=INT")
	if code != 0 || errb != "" || out != "A=1\n" {
		t.Fatalf("signal data mode = (%q, %q, %d)", out, errb, code)
	}
	_, errb, code = runTool(t, nil, "--ignore-signal=NOPE")
	if code != 2 || !strings.Contains(errb, "unknown signal") {
		t.Fatalf("bad signal code=%d err=%q", code, errb)
	}
}

func TestEnvCommandStdioAndExitStatus(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires a POSIX shell")
	}
	for _, want := range []int{0, 7} {
		_, errb, code := runTool(t, []string{"PATH=" + os.Getenv("PATH")}, "sh", "-c", "exit "+string(rune('0'+want)))
		if code != want || errb != "" {
			t.Errorf("exit %d = code %d, stderr %q", want, code, errb)
		}
	}
	_, _, code := runTool(t, []string{"PATH=" + os.Getenv("PATH")}, "sh", "-c", "kill -TERM $$")
	if code != 143 {
		t.Errorf("signal exit = code %d, want 143", code)
	}

	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(),
		Dir: t.TempDir(),
		Env: []string{"PATH=" + os.Getenv("PATH")},
		Stdio: tool.Stdio{
			In:  strings.NewReader("streamed\n"),
			Out: &out,
			Err: &errb,
		},
	}
	code = cmd.Run(rc, []string{"sh", "-c", `read value; printf %s "$value"; printf err >&2`})
	if code != 0 || out.String() != "streamed" || errb.String() != "err" {
		t.Fatalf("stdio = (%q, %q, %d)", out.String(), errb.String(), code)
	}
}

func TestEnvCommandNotFoundAndNotExecutable(t *testing.T) {
	_, errb, code := runTool(t, []string{"PATH="}, "definitely-not-an-env-test-command")
	if code != 127 || !strings.Contains(errb, "No such file or directory") {
		t.Fatalf("not found = code %d, stderr %q", code, errb)
	}

	if runtime.GOOS == "windows" {
		return
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "not-executable")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errOut},
	}
	code = cmd.Run(rc, []string{"./not-executable"})
	if code != 126 || !strings.Contains(errOut.String(), "permission denied") {
		t.Fatalf("not executable = code %d, stderr %q", code, errOut.String())
	}
}

func TestEnvErrors(t *testing.T) {
	_, errb, code := runTool(t, nil, "-u", "A=B")
	if code != 2 || !strings.Contains(errb, "cannot unset") {
		t.Errorf("-u with '=': code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, nil, "--frobnicate")
	if code != 2 || !strings.Contains(errb, "frobnicate") {
		t.Errorf("unknown flag: code=%d err=%q", code, errb)
	}
}

func TestEnvHelp(t *testing.T) {
	out, _, code := runTool(t, nil, "--help")
	if code != 0 || !strings.Contains(out, "Usage: env") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	for _, removed := range []string{"--default-signal", "--block-signal", "--list-signal-handling"} {
		if strings.Contains(out, removed) {
			t.Errorf("--help advertises unsupported option %q:\n%s", removed, out)
		}
	}
}

func TestEnvWriteErrorDiagnostic(t *testing.T) {
	var errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(),
		Dir: t.TempDir(),
		Env: []string{"A=1"},
		Stdio: tool.Stdio{
			In:  strings.NewReader(""),
			Out: failingWriter{err: os.ErrClosed},
			Err: &errb,
		},
	}
	if code := cmd.Run(rc, nil); code != 1 {
		t.Fatalf("write failure code=%d, want 1", code)
	}
	if got := errb.String(); !strings.Contains(got, "env: write error:") {
		t.Fatalf("write failure stderr=%q, want diagnostic", got)
	}
}
