package morecmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

var testIsTerminal = true

func init() {
	// Override the variable from more.go
	isTerminal = func(w io.Writer) bool {
		return testIsTerminal
	}
}

func runMore(t *testing.T, dir, stdin string, args ...string) (string, string, int) {
	t.Helper()
	return runMoreEnv(t, dir, stdin, nil, args...)
}

func runMoreEnv(t *testing.T, dir, stdin string, env []string, args ...string) (string, string, int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Env:   env,
		Stdio: tool.Stdio{In: strings.NewReader(stdin), Out: &out, Err: &errb},
	}
	testIsTerminal = true
	code := cmd.Run(rc, append([]string{"-e"}, args...))
	return out.String(), errb.String(), code
}

func runMoreNonTerminal(t *testing.T, dir, stdin string, args ...string) (string, string, int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Stdio: tool.Stdio{In: strings.NewReader(stdin), Out: &out, Err: &errb},
	}
	testIsTerminal = false
	code := cmd.Run(rc, args)
	return out.String(), errb.String(), code
}

func TestMoreReadsStdin(t *testing.T) {
	out, errb, code := runMore(t, t.TempDir(), "a\nb\n")
	if out != "a\nb\n" || errb != "" || code != 0 {
		t.Fatalf("more stdin = (%q, %q, %d)", out, errb, code)
	}
}

func TestMoreConcatenatesFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b"), []byte("b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, code := runMore(t, dir, "", "a", "b")
	if out != "a\nb\n" || code != 0 {
		t.Fatalf("more files = (%q, %d)", out, code)
	}
}

func TestMoreSqueezeAndFromLine(t *testing.T) {
	out, errb, code := runMore(t, t.TempDir(), "one\n\n\ntwo\n", "-s", "-F", "2")
	if want := "\ntwo\n"; out != want || errb != "" || code != 0 {
		t.Fatalf("more squeeze/from-line = (%q, %q, %d), want (%q, \"\", 0)", out, errb, code, want)
	}
}

func TestMoreAcceptsSupportedDisplayFlags(t *testing.T) {
	out, errb, code := runMore(t, t.TempDir(), "a\nb\n", "-c", "-n", "5")
	if out != "a\nb\n" || !strings.HasPrefix(errb, "\x1b[H\x1b[2J") || code != 0 {
		t.Fatalf("more display flags = (%q, %q, %d)", out, errb, code)
	}

	out, errb, code = runMore(t, t.TempDir(), "a\nb\n", "-e", "-u", "--number", "5")
	if out != "a\nb\n" || errb != "" || code != 0 {
		t.Fatalf("more alias flags = (%q, %q, %d)", out, errb, code)
	}

	out, errb, code = runMore(t, t.TempDir(), "a\n", "-10")
	if out != "a\n" || errb != "" || code != 0 {
		t.Fatalf("more numeric screen size = (%q, %q, %d)", out, errb, code)
	}
}

func TestMoreRejectsInteractiveFlags(t *testing.T) {
	for _, args := range [][]string{
		{"-d"}, {"-l"}, {"-f"}, {"-i"}, {"-t", "tag"}, {"--tag="},
	} {
		_, errb, code := runMore(t, t.TempDir(), "a\n", args...)
		if code == 0 || !strings.Contains(errb, "not supported") {
			t.Fatalf("expected %v to fail with not supported, got code %d err %q", args, code, errb)
		}
	}
	for _, args := range [][]string{{"-t"}} {
		_, errb, code := runMore(t, t.TempDir(), "a\n", args...)
		if code != 2 || !strings.Contains(errb, "needs an argument") {
			t.Fatalf("expected missing argument for %v, got code %d err %q", args, code, errb)
		}
	}
}

func TestMoreCommandOptionSupportsOnlySpaceAndLowercaseQ(t *testing.T) {
	out, errb, code := runMore(t, t.TempDir(), "a\n", "-p", " ")
	if code != 0 || out != "a\n" || errb != "" {
		t.Fatalf("-p space = (%q, %q, %d)", out, errb, code)
	}
	out, errb, code = runMore(t, t.TempDir(), "a\n", "-p", "q")
	if code != 0 || out != "" || errb != "" {
		t.Fatalf("-p q = (%q, %q, %d)", out, errb, code)
	}
	for _, unsupported := range []string{"b", "Q", "\n"} {
		_, errb, code = runMore(t, t.TempDir(), "a\n", "-p", unsupported)
		if code == 0 || !strings.Contains(errb, "not supported") {
			t.Fatalf("-p %q = code %d err %q", unsupported, code, errb)
		}
	}
}

func TestMorePatternStartsAtMatch(t *testing.T) {
	out, errb, code := runMore(t, t.TempDir(), "alpha\nbeta\ngamma\n", "-P", "bet")
	if want := "beta\ngamma\n"; out != want || errb != "" || code != 0 {
		t.Fatalf("more pattern = (%q, %q, %d), want (%q, \"\", 0)", out, errb, code, want)
	}
}

func TestMorePatternIsLiteral(t *testing.T) {
	out, errb, code := runMore(t, t.TempDir(), "alpha\nx^bety\ngamma\n", "-P", "^bet")
	if want := "x^bety\ngamma\n"; out != want || errb != "" || code != 0 {
		t.Fatalf("more literal pattern = (%q, %q, %d), want (%q, \"\", 0)", out, errb, code, want)
	}

	out, errb, code = runMore(t, t.TempDir(), "a\n[b\nc\n", "-P", "[")
	if want := "[b\nc\n"; out != want || errb != "" || code != 0 {
		t.Fatalf("more bracket pattern = (%q, %q, %d), want (%q, \"\", 0)", out, errb, code, want)
	}
}

func TestMorePatternNotFound(t *testing.T) {
	out, errb, code := runMore(t, t.TempDir(), "alpha\nbeta\n", "-P", "zzz")
	if out != "alpha\nbeta\n" || code != 0 {
		t.Fatalf("more pattern miss = (%q, %d), want display from start", out, code)
	}
	if !strings.Contains(errb, "Pattern not found") {
		t.Fatalf("more pattern miss stderr = %q, want Pattern not found", errb)
	}
}

func TestMoreRejectsBadLineCounts(t *testing.T) {
	_, errb, code := runMore(t, t.TempDir(), "", "-F", "0")
	if code != 2 || !strings.Contains(errb, "invalid starting line") {
		t.Fatalf("more bad from-line code=%d err=%q", code, errb)
	}
}

func TestMoreValidatesLineCountsBeforeOpeningTTY(t *testing.T) {
	origOpen := openTTY
	opens, closes := 0, 0
	openTTY = func(*tool.RunContext) (*ttyChannel, error) {
		opens++
		return &ttyChannel{
			readCommand: func(context.Context) (byte, error) { return 'q', nil },
			close:       func() error { closes++; return nil },
		}, nil
	}
	t.Cleanup(func() { openTTY = origOpen })

	for _, args := range [][]string{{"-n=-1"}, {"--number=-1"}, {"-F", "0"}} {
		_, errb, code := runMore(t, t.TempDir(), "", args...)
		if code != 2 || !strings.Contains(errb, "invalid") {
			t.Fatalf("more %v: code=%d err=%q", args, code, errb)
		}
	}
	if opens != 0 || closes != 0 {
		t.Fatalf("invalid values opened/closed controlling terminal %d/%d times; want 0/0", opens, closes)
	}
}

func TestMoreNonTerminalIgnoresFAndP(t *testing.T) {
	// -F and -P should be ignored when output is non-terminal.
	out, errb, code := runMoreNonTerminal(t, t.TempDir(), "alpha\nbeta\ngamma\n", "-F", "2", "-P", "bet")
	if want := "alpha\nbeta\ngamma\n"; out != want || errb != "" || code != 0 {
		t.Fatalf("more non-terminal -F -P = (%q, %q, %d), want (%q, \"\", 0)", out, errb, code, want)
	}
}

func TestMorePOSIXNonTerminalOnlySqueezeTakesEffect(t *testing.T) {
	input := "one\n\n\ntwo\nthree\n"
	out, errb, code := runMoreNonTerminal(t, t.TempDir(), input,
		"-s", "-n", "1", "-p", "q", "-d", "-l", "-f", "-i", "-t", "tag")
	if want := "one\n\ntwo\nthree\n"; out != want || errb != "" || code != 0 {
		t.Fatalf("more non-terminal POSIX option effects = (%q, %q, %d), want (%q, \"\", 0)", out, errb, code, want)
	}

	var terminalOut, terminalErr bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   t.TempDir(),
		Stdio: tool.Stdio{In: strings.NewReader(input), Out: &terminalOut, Err: &terminalErr},
	}
	withPagerTTY(t, rc.Out, commandTTY(" "), 2, 80)
	if code := cmd.Run(rc, []string{"-s", "-n", "1", "-p", "q"}); code != 0 {
		t.Fatalf("terminal more code=%d stderr=%q", code, terminalErr.String())
	}
	if terminalOut.String() != "" {
		t.Fatalf("terminal -p q should quit before content, got %q", terminalOut.String())
	}
}

func TestMorePOSIXLineCountMustBePositive(t *testing.T) {
	for _, args := range [][]string{{"-n", "0"}, {"-n0"}, {"-0"}} {
		_, errb, code := runMoreNonTerminal(t, t.TempDir(), "input\n", args...)
		if code == 0 || !strings.Contains(errb, "invalid line count") {
			t.Errorf("more %q = stderr %q code %d, want positive-number error", args, errb, code)
		}
	}
}

func TestMoreSqueezeCRLF(t *testing.T) {
	// -s squeezes only repeated empty text lines (\n)
	// CR-containing lines (\r\n) are non-empty.
	out, errb, code := runMore(t, t.TempDir(), "one\n\n\ntwo\n\r\n\r\nthree\n\n\n", "-s")
	if want := "one\n\ntwo\n\r\n\r\nthree\n\n"; out != want || errb != "" || code != 0 {
		t.Fatalf("more squeeze CRLF = (%q, %q, %d), want (%q, \"\", 0)", out, errb, code, want)
	}
}

func TestMoreEnvironmentMORE(t *testing.T) {
	env := []string{"MORE=-s\t-F 2"}
	out, errb, code := runMoreEnv(t, t.TempDir(), "one\n\n\ntwo\n", env)
	if want := "\ntwo\n"; out != want || errb != "" || code != 0 {
		t.Fatalf("more MORE env = (%q, %q, %d), want (%q, \"\", 0)", out, errb, code, want)
	}

	// Precedence: command line should override.
	// -F 3 should override -F 2 from env.
	out, errb, code = runMoreEnv(t, t.TempDir(), "one\n\n\ntwo\nthree\n", env, "-F", "3")
	// line 3 is the second \n
	if want := "\ntwo\nthree\n"; out != want || errb != "" || code != 0 {
		t.Fatalf("more MORE env precedence = (%q, %q, %d), want (%q, \"\", 0)", out, errb, code, want)
	}
}

type errorReader struct{}

func (errorReader) Read(p []byte) (n int, err error) {
	return 0, fmt.Errorf("simulated read error")
}

type shortWriter struct{}

func (shortWriter) Write(p []byte) (n int, err error) {
	// Always short write and error
	return len(p) / 2, fmt.Errorf("simulated short write")
}

func TestMoreReadWriteErrors(t *testing.T) {
	var errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   t.TempDir(),
		Stdio: tool.Stdio{In: errorReader{}, Out: shortWriter{}, Err: &errb},
	}
	testIsTerminal = true
	code := cmd.Run(rc, []string{})
	if code == 0 {
		t.Fatalf("expected non-zero exit code on read error, got %d", code)
	}
	if !strings.Contains(strings.ToLower(errb.String()), "simulated read error") {
		t.Fatalf("expected read error message, got %q", errb.String())
	}

	errb.Reset()
	rc = &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   t.TempDir(),
		Stdio: tool.Stdio{In: strings.NewReader("hello\nworld\n"), Out: shortWriter{}, Err: &errb},
	}
	code = cmd.Run(rc, []string{})
	if code == 0 {
		t.Fatalf("expected non-zero exit code on write error, got %d", code)
	}
	if !strings.Contains(errb.String(), "simulated short write") {
		t.Fatalf("expected write error message, got %q", errb.String())
	}
}
func init() {
	openTTY = func(rc *tool.RunContext) (*ttyChannel, error) {
		return &ttyChannel{
			readCommand: func(context.Context) (byte, error) { return ' ', nil },
			close:       func() error { return nil },
		}, nil
	}
}
