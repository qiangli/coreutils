package envcmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/coreutils/tool"
)

// COMMAND execution is exercised against this test binary re-invoked as a
// helper process, so the assertions are byte-exact on every platform and do
// not depend on a system shell being present (the point of the whole repo).
const helperEnvKey = "COREUTILS_ENV_TEST_HELPER"

// TestEnvHelperProcess is not a test: when the child marker is set in the
// environment env built for it, it acts as the COMMAND the other tests run.
func TestEnvHelperProcess(t *testing.T) {
	if os.Getenv(helperEnvKey) != "1" {
		return
	}
	// Exit before the testing framework writes anything, so the parent sees
	// only what the helper itself printed.
	os.Exit(helperMain(helperArgs()))
}

func helperArgs() []string {
	for i, a := range os.Args {
		if a == "--" {
			return os.Args[i+1:]
		}
	}
	return nil
}

func helperMain(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "helper: missing mode")
		return 2
	}
	switch args[0] {
	case "argv":
		// Bracketed so an empty or whitespace-only argument is still visible.
		for _, a := range args[1:] {
			fmt.Printf("[%s]\n", a)
		}
	case "environ":
		for _, e := range os.Environ() {
			fmt.Printf("%s\n", e)
		}
	case "cat":
		if _, err := io.Copy(os.Stdout, os.Stdin); err != nil {
			return 1
		}
	case "quiet":
	case "sleep":
		d, _ := strconv.Atoi(args[1])
		time.Sleep(time.Duration(d) * time.Second)
	case "exit":
		n, _ := strconv.Atoi(args[1])
		return n
	default:
		fmt.Fprintf(os.Stderr, "helper: unknown mode %q\n", args[0])
		return 2
	}
	return 0
}

// helperArgv is the leading argv that re-enters this binary as the helper.
func helperArgv(t *testing.T) []string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Skipf("os.Executable unavailable: %v", err)
	}
	return []string{exe, "-test.run=^TestEnvHelperProcess$", "--"}
}

// runExec invokes env with the helper marker already in the inherited
// environment, so the invocation carries zero NAME=VALUE operands.
func runExec(t *testing.T, ctx context.Context, dir string, env []string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   ctx,
		Dir:   dir,
		Env:   env,
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
	}
	code = cmd.Run(rc, args)
	return out.String(), errb.String(), code
}

// Arguments reach COMMAND as data: no shell, no concatenation, no re-quoting,
// so metacharacters cannot become syntax. Also covers an explicit (absolute)
// COMMAND path and the zero-assignment form.
func TestEnvExecPassesArgvVerbatimWithoutShell(t *testing.T) {
	hostile := []string{
		"plain",
		"two words",
		"; touch pwned",
		"$(touch pwned)",
		"`touch pwned`",
		"a|b&&c",
		"*",
		"'single' \"double\"",
		"new\nline",
		"tab\there",
		"$HOME",
		"",
		"--not-an-env-flag",
	}
	args := append(helperArgv(t), "argv")
	args = append(args, hostile...)

	dir := t.TempDir()
	out, errb, code := runExec(t, context.Background(), dir, []string{helperEnvKey + "=1"}, args...)
	if code != 0 || errb != "" {
		t.Fatalf("verbatim argv = (%q, %d), want (\"\", 0)", errb, code)
	}
	var want strings.Builder
	for _, a := range hostile {
		fmt.Fprintf(&want, "[%s]\n", a)
	}
	if out != want.String() {
		t.Errorf("argv round-trip:\n got %q\nwant %q", out, want.String())
	}
	// Nothing may have been interpreted: no side effect in the workdir.
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
		t.Errorf("COMMAND arguments were interpreted: dir entries %v (err %v)", entries, err)
	}
}

// The child's environment is exactly what env computed, in environ order:
// inherited entries first (first-assignment slot kept), then new names in
// operand order, with -u removals and -i honored.
func TestEnvExecChildEnvironmentOrdering(t *testing.T) {
	base := []string{"KEEP=1", "REPLACED=old", "DROPPED=1", helperEnvKey + "=1"}
	args := append([]string{"-u", "DROPPED", "REPLACED=new", "ADDED=2"}, helperArgv(t)...)
	args = append(args, "environ")

	out, errb, code := runExec(t, context.Background(), t.TempDir(), base, args...)
	if code != 0 || errb != "" {
		t.Fatalf("child environ = (%q, %d)", errb, code)
	}
	want := []string{"KEEP=1", "REPLACED=new", helperEnvKey + "=1", "ADDED=2"}
	got := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if runtime.GOOS == "windows" {
		// Windows normalizes the environment block; assert membership only.
		for _, w := range want {
			if !strings.Contains(out, w) {
				t.Errorf("child environment missing %q:\n%s", w, out)
			}
		}
		if strings.Contains(out, "DROPPED=") || strings.Contains(out, "REPLACED=old") {
			t.Errorf("child environment kept a removed/stale entry:\n%s", out)
		}
		return
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("child environ order:\n got %q\nwant %q", got, want)
	}

	// -i starts the child from an empty environment; only the operands survive.
	args = append([]string{"-i", helperEnvKey + "=1", "ONLY=me"}, helperArgv(t)...)
	args = append(args, "environ")
	out, errb, code = runExec(t, context.Background(), t.TempDir(), base, args...)
	if code != 0 || errb != "" {
		t.Fatalf("-i child environ = (%q, %d)", errb, code)
	}
	if want := helperEnvKey + "=1\nONLY=me\n"; out != want {
		t.Errorf("-i child environ = %q, want %q", out, want)
	}
}

// With a COMMAND, env contributes nothing of its own to either stream, and
// the child inherits the invocation's stdin.
func TestEnvExecStdioPassthroughAndSilence(t *testing.T) {
	base := []string{helperEnvKey + "=1"}

	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   t.TempDir(),
		Env:   base,
		Stdio: tool.Stdio{In: strings.NewReader("piped payload"), Out: &out, Err: &errb},
	}
	if code := cmd.Run(rc, append(helperArgv(t), "cat")); code != 0 {
		t.Fatalf("cat helper code = %d, stderr %q", code, errb.String())
	}
	if out.String() != "piped payload" || errb.String() != "" {
		t.Errorf("stdin passthrough = (%q, %q), want (%q, \"\")", out.String(), errb.String(), "piped payload")
	}

	// A silent COMMAND leaves both streams empty — env announces nothing.
	stdout, stderr, code := runExec(t, context.Background(), t.TempDir(), base, append(helperArgv(t), "quiet")...)
	if stdout != "" || stderr != "" || code != 0 {
		t.Errorf("silent COMMAND = (%q, %q, %d), want (\"\", \"\", 0)", stdout, stderr, code)
	}

	// COMMAND's own status is env's status, unmodified.
	if _, _, code := runExec(t, context.Background(), t.TempDir(), base, append(helperArgv(t), "exit", "42")...); code != 42 {
		t.Errorf("COMMAND exit status = %d, want 42", code)
	}
}

// Cancelling the invocation context terminates COMMAND rather than leaking it.
func TestEnvExecHonorsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	_, _, code := runExec(t, ctx, t.TempDir(), []string{helperEnvKey + "=1"}, append(helperArgv(t), "sleep", "60")...)
	elapsed := time.Since(start)

	if elapsed > 30*time.Second {
		t.Fatalf("cancellation did not stop COMMAND: took %s", elapsed)
	}
	if code == 0 {
		t.Errorf("cancelled COMMAND exit status = 0, want non-zero")
	}
	if runtime.GOOS != "windows" && code != 137 {
		t.Errorf("cancelled COMMAND exit status = %d, want 137 (128+SIGKILL)", code)
	}
}

// POSIX: a zero-length PATH prefix means the working directory, and a wholly
// empty PATH is one zero-length prefix — so the search is the working
// directory only, never a silent fallback to the built-in default path.
func TestEnvExecEmptyPathSearchesWorkingDirectoryOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs a POSIX executable bit and shebang")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "env-test-local")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf local\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	out, errb, code := runExec(t, context.Background(), dir, []string{"PATH="}, "env-test-local")
	if code != 0 || errb != "" || out != "local" {
		t.Errorf("empty PATH working-directory search = (%q, %q, %d), want (%q, \"\", 0)", out, errb, code, "local")
	}

	// A command that exists only on the default path must NOT be found.
	_, errb, code = runExec(t, context.Background(), dir, []string{"PATH="}, "sh")
	if code != 127 || !strings.Contains(errb, "No such file or directory") {
		t.Errorf("empty PATH fell back to the default path: (%q, %d), want 127", errb, code)
	}

	// An empty element inside a non-empty PATH means the working directory too.
	out, errb, code = runExec(t, context.Background(), dir, []string{"PATH=/nonexistent-env-test-dir:"}, "env-test-local")
	if code != 0 || errb != "" || out != "local" {
		t.Errorf("trailing empty PATH element = (%q, %q, %d)", out, errb, code)
	}

	// An explicit path is used as given — PATH is not consulted at all.
	out, errb, code = runExec(t, context.Background(), dir, []string{"PATH=/nonexistent-env-test-dir"}, "./env-test-local")
	if code != 0 || errb != "" || out != "local" {
		t.Errorf("explicit relative COMMAND path = (%q, %q, %d)", out, errb, code)
	}
	out, errb, code = runExec(t, context.Background(), dir, []string{"PATH="}, script)
	if code != 0 || errb != "" || out != "local" {
		t.Errorf("explicit absolute COMMAND path = (%q, %q, %d)", out, errb, code)
	}

	// A directory matched by name is found-but-not-invokable: 126, not 127.
	if err := os.Mkdir(filepath.Join(dir, "env-test-dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, errb, code = runExec(t, context.Background(), dir, []string{"PATH="}, "env-test-dir")
	if code != 126 {
		t.Errorf("directory as COMMAND = (%q, %d), want 126", errb, code)
	}
}

// Assignment/unset operand validation, and the boundary between assignments
// and COMMAND.
func TestEnvAssignmentOperandValidation(t *testing.T) {
	// -u rejects a NAME that is empty or contains '='.
	for _, bad := range []string{"", "A=B", "=", "A=", "=B"} {
		_, errb, code := runTool(t, []string{"A=1"}, "-u", bad)
		if code != 2 || !strings.Contains(errb, "cannot unset") {
			t.Errorf("-u %q = (%q, %d), want a 'cannot unset' usage error", bad, errb, code)
		}
	}

	// An operand with an empty NAME is still an assignment (putenv semantics),
	// so it must not be mistaken for COMMAND.
	out, errb, code := runTool(t, nil, "=value")
	if code != 0 || errb != "" || out != "=value\n" {
		t.Errorf("empty-name assignment = (%q, %q, %d)", out, errb, code)
	}

	// The first operand without '=' is COMMAND; a later '=' operand is one of
	// its arguments, not an assignment.
	args := append([]string{"KEY=set"}, helperArgv(t)...)
	args = append(args, "argv", "NOT=anassignment")
	out, errb, code = runExec(t, context.Background(), t.TempDir(), []string{helperEnvKey + "=1"}, args...)
	if code != 0 || errb != "" || out != "[NOT=anassignment]\n" {
		t.Errorf("post-COMMAND '=' operand = (%q, %q, %d)", out, errb, code)
	}

	// --argv0 without a COMMAND is a usage error, not a silent no-op.
	if _, errb, code := runTool(t, nil, "--argv0", "x"); code != 2 || !strings.Contains(errb, "requires a COMMAND") {
		t.Errorf("--argv0 without COMMAND = (%q, %d)", errb, code)
	}
}

// '--' ends env's own options: everything after it is operands, even when it
// looks like a flag.
func TestEnvDoubleDashEndsOptions(t *testing.T) {
	out, errb, code := runTool(t, []string{"A=1"}, "--", "B=2")
	if code != 0 || errb != "" || out != "A=1\nB=2\n" {
		t.Errorf("env -- B=2 = (%q, %q, %d)", out, errb, code)
	}

	// A COMMAND spelled like an option is executed, not parsed.
	_, errb, code = runTool(t, []string{"PATH="}, "--", "-i")
	if code != 127 || !strings.Contains(errb, "-i") {
		t.Errorf("env -- -i = (%q, %d), want a 127 lookup failure naming -i", errb, code)
	}

	// Options before '--' still apply.
	out, errb, code = runTool(t, []string{"A=1"}, "-i", "--", "B=2")
	if code != 0 || errb != "" || out != "B=2\n" {
		t.Errorf("env -i -- B=2 = (%q, %q, %d)", out, errb, code)
	}

	args := append([]string{"--", "KEY=v"}, helperArgv(t)...)
	args = append(args, "argv", "-i")
	out, errb, code = runExec(t, context.Background(), t.TempDir(), []string{helperEnvKey + "=1"}, args...)
	if code != 0 || errb != "" || out != "[-i]\n" {
		t.Errorf("COMMAND options are not env options = (%q, %q, %d)", out, errb, code)
	}
}
