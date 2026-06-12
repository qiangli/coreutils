package seqcmd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func runTool(t *testing.T, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   t.TempDir(),
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
	}
	code = cmd.Run(rc, args)
	return out.String(), errb.String(), code
}

func TestSeq(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"3"}, "1\n2\n3\n"},
		{[]string{"2", "4"}, "2\n3\n4\n"},
		{[]string{"3", "2", "7"}, "3\n5\n7\n"},
		{[]string{"1", "1"}, "1\n"},
		// FIRST > LAST with positive INCREMENT: no output (GNU does
		// NOT flip the increment).
		{[]string{"5", "1"}, ""},
		{[]string{"1", "-1", "5"}, ""},
		// Counting down needs an explicit negative increment.
		{[]string{"5", "-1", "1"}, "5\n4\n3\n2\n1\n"},
		{[]string{"-3", "-1"}, "-3\n-2\n-1\n"},
		{[]string{"-1", "-1", "-3"}, "-1\n-2\n-3\n"},
		// Large integers stay exact (no %g mangling).
		{[]string{"1000000", "1000002"}, "1000000\n1000001\n1000002\n"},
		// Default float format: %.PRECf, PREC from FIRST/INCREMENT.
		{[]string{"0", "0.3", "1"}, "0.0\n0.3\n0.6\n0.9\n"},
		{[]string{"0", "0.1", "1"}, "0.0\n0.1\n0.2\n0.3\n0.4\n0.5\n0.6\n0.7\n0.8\n0.9\n1.0\n"},
		{[]string{"0.5", "2"}, "0.5\n1.5\n"},
		// LAST's precision does not widen the output (GNU rule).
		{[]string{"1", "1", "2.5"}, "1\n2\n"},
		// Separator.
		{[]string{"-s", ",", "3"}, "1,2,3\n"},
		{[]string{"--separator", " ", "3"}, "1 2 3\n"},
		// Equal width.
		{[]string{"-w", "8", "10"}, "08\n09\n10\n"},
		{[]string{"-w", "1", "3"}, "1\n2\n3\n"},
		{[]string{"-w", "-5", "5", "10"}, "-5\n00\n05\n10\n"},
		{[]string{"-w", "1", "0.5", "2"}, "1.0\n1.5\n2.0\n"},
		{[]string{"-w", "9.5", "10.5"}, "09.5\n10.5\n"},
		// -f printf formats (C default precision 6 when unspecified).
		{[]string{"-f", "%.2f", "1", "3"}, "1.00\n2.00\n3.00\n"},
		{[]string{"-f", "%.2e", "1", "2"}, "1.00e+00\n2.00e+00\n"},
		{[]string{"-f", "%f", "1", "1"}, "1.000000\n"},
		{[]string{"-f", "%g", "1", "1"}, "1\n"},
		{[]string{"-f", "%%%g", "1", "1"}, "%1\n"},
		{[]string{"-f", "==%.0f==", "1", "2"}, "==1==\n==2==\n"},
		// Scientific-notation operands fall back to %g.
		{[]string{"1e2", "1e2"}, "100\n"},
	}
	for _, c := range cases {
		out, errb, code := runTool(t, c.args...)
		if out != c.want || code != 0 || errb != "" {
			t.Errorf("seq %q = (%q, %q, %d), want (%q, \"\", 0)", c.args, out, errb, code, c.want)
		}
	}
}

func TestSeqErrors(t *testing.T) {
	cases := []struct {
		args    []string
		errPart string
	}{
		{[]string{}, "missing operand"},
		{[]string{"1", "1", "1", "1"}, "extra operand"},
		{[]string{"x"}, "invalid floating point argument"},
		{[]string{"1", "y", "3"}, "invalid floating point argument"},
		{[]string{"1", "0", "3"}, "invalid Zero increment"},
		{[]string{"-w", "-f", "%g", "3"}, "equal width"},
		{[]string{"-f", "%d", "3"}, "unknown %d directive"},
		{[]string{"-f", "no directive", "3"}, "no % directive"},
		{[]string{"-f", "%g%g", "3"}, "too many % directives"},
		{[]string{"-f", "%", "3"}, "ends in %"},
		{[]string{"--frobnicate", "3"}, "frobnicate"},
	}
	for _, c := range cases {
		_, errb, code := runTool(t, c.args...)
		if code != 2 || !strings.Contains(errb, c.errPart) {
			t.Errorf("seq %q: code=%d err=%q, want exit 2 containing %q", c.args, code, errb, c.errPart)
		}
	}
}

func TestSeqHelp(t *testing.T) {
	out, _, code := runTool(t, "--help")
	if code != 0 || !strings.Contains(out, "Usage: seq") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
}
