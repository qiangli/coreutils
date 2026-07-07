package printenvcmd

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

func TestPrintenv(t *testing.T) {
	base := []string{"A=1", "B=two words", "EMPTY="}
	cases := []struct {
		args []string
		want string
		code int
	}{
		{nil, "A=1\nB=two words\nEMPTY=\n", 0},
		{[]string{"A"}, "1\n", 0},
		{[]string{"B"}, "two words\n", 0},
		{[]string{"A", "B"}, "1\ntwo words\n", 0},
		// Set-but-empty prints a blank line and succeeds.
		{[]string{"EMPTY"}, "\n", 0},
		// Unset name: nothing printed, exit 1.
		{[]string{"MISSING"}, "", 1},
		// Mixed: found ones still print, exit 1 overall.
		{[]string{"A", "MISSING", "B"}, "1\ntwo words\n", 1},
	}
	for _, c := range cases {
		out, errb, code := runTool(t, base, c.args...)
		if out != c.want || code != c.code || errb != "" {
			t.Errorf("printenv %q = (%q, %q, %d), want (%q, \"\", %d)", c.args, out, errb, code, c.want, c.code)
		}
	}
}

func TestPrintenvLastWins(t *testing.T) {
	out, _, code := runTool(t, []string{"A=1", "A=2"}, "A")
	if out != "2\n" || code != 0 {
		t.Errorf("duplicate env: out=%q code=%d, want \"2\\n\" 0", out, code)
	}
}

func TestPrintenvFlags(t *testing.T) {
	out, _, code := runTool(t, nil, "--help")
	if code != 0 || !strings.Contains(out, "Usage: printenv") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	_, errb, code := runTool(t, nil, "--frobnicate")
	if code != 2 || !strings.Contains(errb, "frobnicate") {
		t.Errorf("unknown flag: code=%d err=%q", code, errb)
	}
}

func TestPrintenvNull(t *testing.T) {
	base := []string{"A=1", "B=two"}
	out, _, code := runTool(t, base, "-0")
	if code != 0 || out != "A=1\x00B=two\x00" {
		t.Errorf("-0 all: out=%q code=%d", out, code)
	}

	out, _, code = runTool(t, base, "-0", "A", "B")
	if code != 0 || out != "1\x00two\x00" {
		t.Errorf("-0 named: out=%q code=%d", out, code)
	}
}
