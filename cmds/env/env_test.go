package envcmd

import (
	"bytes"
	"context"
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
