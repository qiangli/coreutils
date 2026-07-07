package envcmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

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

func TestEnvChdirAndSplitStringCommandContract(t *testing.T) {
	dir := t.TempDir()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   t.TempDir(),
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
	}
	code := cmd.Run(rc, []string{"--chdir", dir, "-S", "printf 'two words'"})
	if code != 2 || !strings.Contains(errb.String(), "printf") || rc.Dir != dir {
		t.Fatalf("code=%d err=%q dir=%q", code, errb.String(), rc.Dir)
	}
}

func TestEnvSignalOptionsValidateInDataMode(t *testing.T) {
	out, errb, code := runTool(t, []string{"A=1"}, "--ignore-signal=INT,TERM", "--default-signal=HUP", "--block-signal=USR1", "--list-signal-handling")
	if code != 0 || errb != "" || out != "A=1\n" {
		t.Fatalf("signal data mode = (%q, %q, %d)", out, errb, code)
	}
	_, errb, code = runTool(t, nil, "--ignore-signal=NOPE")
	if code != 2 || !strings.Contains(errb, "unknown signal") {
		t.Fatalf("bad signal code=%d err=%q", code, errb)
	}
}

func TestEnvCommandNotSupported(t *testing.T) {
	_, errb, code := runTool(t, []string{"A=1"}, "X=2", "printenv", "X")
	if code != 2 || !strings.Contains(errb, "not supported") || !strings.Contains(errb, "printenv") {
		t.Errorf("COMMAND operand: code=%d err=%q, want contract error naming the command", code, errb)
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
}
