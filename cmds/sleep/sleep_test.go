package sleepcmd

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/coreutils/tool"
)

func runTool(t *testing.T, ctx context.Context, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   ctx,
		Dir:   t.TempDir(),
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
	}
	code = cmd.Run(rc, args)
	return out.String(), errb.String(), code
}

func TestSleepZeroish(t *testing.T) {
	for _, args := range [][]string{
		{"0"},
		{"0.0"},
		{"0s"},
		{"0m"},
		{"0h"},
		{"0d"},
		{"0", "0", "0"},
		{"0.001"},
	} {
		start := time.Now()
		out, errb, code := runTool(t, context.Background(), args...)
		if code != 0 || out != "" || errb != "" {
			t.Errorf("sleep %q = (%q, %q, %d), want clean 0", args, out, errb, code)
		}
		if time.Since(start) > 2*time.Second {
			t.Errorf("sleep %q took too long", args)
		}
	}
}

func TestSleepSuffixMath(t *testing.T) {
	// 0.05 + 0.05 seconds; verifies summing and fractional values
	// actually pause (bounded loosely to stay robust on CI).
	start := time.Now()
	_, _, code := runTool(t, context.Background(), "0.05", "0.05s")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if elapsed := time.Since(start); elapsed < 90*time.Millisecond {
		t.Errorf("slept %v, want >= ~100ms", elapsed)
	}
}

func TestSleepIssue7IntegralDuration(t *testing.T) {
	start := time.Now()
	out, errb, code := runTool(t, context.Background(), "1")
	if code != 0 || out != "" || errb != "" {
		t.Fatalf("sleep 1 = (%q, %q, %d), want clean success", out, errb, code)
	}
	if elapsed := time.Since(start); elapsed < time.Second {
		t.Fatalf("sleep 1 returned early after %v", elapsed)
	}
}

func TestSleepCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	out, errb, code := runTool(t, ctx, "1h")
	if code != 1 || out != "" || errb != "" {
		t.Errorf("cancelled: (%q, %q, %d), want quiet exit 1", out, errb, code)
	}
	if time.Since(start) > 2*time.Second {
		t.Error("cancelled sleep did not return promptly")
	}
}

// TestSleepEndOfOptions pins the "--" Utility Syntax Guideline 10 special
// token. POSIX sleep defines no options, so "--" merely ends option
// parsing: a "--" before a valid time still sleeps, and a "--" before an
// option-like operand routes it to time-operand parsing (where a negative
// value is an invalid interval), never to flag parsing.
func TestSleepEndOfOptions(t *testing.T) {
	// "--" then a valid zero time: clean, immediate success.
	out, errb, code := runTool(t, context.Background(), "--", "0")
	if code != 0 || out != "" || errb != "" {
		t.Errorf(`sleep -- 0 = (%q, %q, %d), want clean 0`, out, errb, code)
	}
	// "--" then an option-like operand: parsed as a (negative) time interval.
	_, errb, code = runTool(t, context.Background(), "--", "-1")
	if code != 2 || !strings.Contains(errb, "invalid time interval") {
		t.Errorf(`sleep -- -1 = (%q, %d), want invalid-interval usage error`, errb, code)
	}
}

// panicReader fails the test if standard input is ever read: POSIX sleep
// does not use stdin.
type panicReader struct{ t *testing.T }

func (r panicReader) Read([]byte) (int, error) {
	r.t.Helper()
	r.t.Fatal("sleep must not read standard input")
	return 0, io.EOF
}

// TestSleepDoesNotConsumeStdin pins the STDIN clause ("Not used.") across
// a successful suspension and a usage-error run.
func TestSleepDoesNotConsumeStdin(t *testing.T) {
	for _, args := range [][]string{{"0"}, {"abc"}} {
		var out, errb bytes.Buffer
		rc := &tool.RunContext{
			Ctx:   context.Background(),
			Dir:   t.TempDir(),
			Stdio: tool.Stdio{In: panicReader{t}, Out: &out, Err: &errb},
		}
		cmd.Run(rc, args)
	}
}

func TestSleepErrors(t *testing.T) {
	cases := [][]string{
		{},             // missing operand
		{"abc"},        // not a number
		{"-1"},         // negative
		{"1x"},         // bad suffix
		{"1S"},         // GNU suffixes are lowercase only
		{"s"},          // suffix without number
		{"1", "bogus"}, // any bad operand fails
		{"1_000"},      // Go-only digit-separator syntax; not valid strtod/POSIX input
		{"1_0s"},       // digit separator before a suffix
		{"Inf"},        // infinity is not a valid POSIX decimal integer
		{"inf"},        // case variant of infinity
		{"+Inf"},       // signed infinity
		{"infinity"},   // GNU extension rejected under POSIX-overrides policy
	}
	for _, args := range cases {
		_, errb, code := runTool(t, context.Background(), args...)
		if code != 2 || errb == "" {
			t.Errorf("sleep %q: code=%d err=%q, want usage error 2", args, code, errb)
		}
	}
	_, errb, code := runTool(t, context.Background(), "--frobnicate")
	if code != 2 || !strings.Contains(errb, "frobnicate") {
		t.Errorf("unknown flag: code=%d err=%q", code, errb)
	}
}

func TestSleepHelp(t *testing.T) {
	out, _, code := runTool(t, context.Background(), "--help")
	if code != 0 || !strings.Contains(out, "Usage: sleep") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
}

// TestSleepPOSIXStdoutNotUsed verifies that sleep produces no standard output,
// matching the POSIX Issue 7 STDOUT requirement ("Not used").
func TestSleepPOSIXStdoutNotUsed(t *testing.T) {
	out, errb, code := runTool(t, context.Background(), "0")
	if code != 0 || out != "" || errb != "" {
		t.Errorf("sleep 0: out=%q err=%q code=%d", out, errb, code)
	}
}

// TestSleepPOSIXLargeIntegerAccepted verifies that sleep accepts the POSIX-
// required minimum range of 2^31-1 (2147483647) seconds without a parse
// error. The actual sleep is cancelled immediately; we verify only that
// the operand is accepted.
func TestSleepPOSIXLargeIntegerAccepted(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, errb, code := runTool(t, ctx, "2147483647")
	// Exit code 1 from context cancellation is expected; a parse error
	// would exit 2 with a diagnostic on stderr.
	if code == 2 || strings.Contains(errb, "invalid time interval") {
		t.Errorf("sleep 2147483647: code=%d err=%q, want accepted operand", code, errb)
	}
}

// TestSleepPOSIXInfinityRejected verifies that Inf/inf/infinity values are
// rejected as invalid POSIX operands. POSIX requires a non-negative decimal
// integer; infinity is a GNU extension.
func TestSleepPOSIXInfinityRejected(t *testing.T) {
	for _, op := range []string{"Inf", "inf", "+Inf", "infinity", "Infinity"} {
		_, errb, code := runTool(t, context.Background(), op)
		if code != 2 || !strings.Contains(errb, "invalid time interval") {
			t.Errorf("sleep %q: code=%d err=%q, want usage error 2", op, code, errb)
		}
	}
}

// TestSleepPOSIXNaNRejected verifies NaN is rejected.
func TestSleepPOSIXNaNRejected(t *testing.T) {
	for _, op := range []string{"NaN", "nan"} {
		_, errb, code := runTool(t, context.Background(), op)
		if code != 2 || !strings.Contains(errb, "invalid time interval") {
			t.Errorf("sleep %q: code=%d err=%q, want usage error 2", op, code, errb)
		}
	}
}
