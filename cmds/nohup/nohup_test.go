package nohupcmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty/v2"
	"github.com/qiangli/coreutils/tool"
)

// TestNohupMissing pins Issue 7's nohup EXIT STATUS: "127  An error occurred
// in the nohup utility or the utility could not be found." A missing operand
// is an error in nohup itself, so the status is 127 — not GNU's 125.
func TestNohupMissing(t *testing.T) {
	var out, errb bytes.Buffer
	code := run(&tool.RunContext{Ctx: context.Background(), Dir: t.TempDir(), Stdio: tool.Stdio{Out: &out, Err: &errb, In: strings.NewReader("")}}, nil)
	if code != 127 {
		t.Fatalf("code=%d, want 127", code)
	}
}

// TestNohupInternalErrorStatusIsUnconditional pins that the required 127 does
// not depend on POSIXLY_CORRECT: Issue 7 lists 126, 127, and the utility's own
// status as the only nohup exit values, with no environment condition on them.
func TestNohupInternalErrorStatusIsUnconditional(t *testing.T) {
	for _, env := range [][]string{nil, {}, {"POSIXLY_CORRECT=1"}, {"POSIXLY_CORRECT="}} {
		var out, errb bytes.Buffer
		rc := &tool.RunContext{
			Ctx:   context.Background(),
			Dir:   t.TempDir(),
			Env:   env,
			Stdio: tool.Stdio{Out: &out, Err: &errb, In: strings.NewReader("")},
		}
		if code := run(rc, nil); code != 127 {
			t.Errorf("env=%v: code=%d, want 127", env, code)
		}
	}
}

func TestNohupRunsCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell command differs on windows")
	}
	var out, errb bytes.Buffer
	code := run(&tool.RunContext{Ctx: context.Background(), Dir: t.TempDir(), Env: []string{"PATH=/bin:/usr/bin"}, Stdio: tool.Stdio{Out: &out, Err: &errb, In: strings.NewReader("")}}, []string{"sh", "-c", "printf ok"})
	if code != 0 || out.String() != "ok" {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), errb.String())
	}

	t.Run("posix_help_and_version_are_utility_operands", func(t *testing.T) {
		dir := t.TempDir()
		for _, name := range []string{"--help", "--version"} {
			if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\nprintf 'utility:"+name+"'\n"), 0o755); err != nil {
				t.Fatal(err)
			}
			for _, value := range []string{"1", ""} {
				var out, errb bytes.Buffer
				rc := &tool.RunContext{
					Ctx: context.Background(), Dir: dir,
					Env:   []string{"PATH=.", "POSIXLY_CORRECT=" + value},
					Stdio: tool.Stdio{Out: &out, Err: &errb, In: strings.NewReader("")},
				}
				if code := run(rc, []string{name}); code != 0 || out.String() != "utility:"+name || errb.Len() != 0 {
					t.Errorf("utility=%q POSIXLY_CORRECT=%q: code=%d out=%q err=%q", name, value, code, out.String(), errb.String())
				}
			}
		}
	})

	t.Run("posix_double_dash_ends_options", func(t *testing.T) {
		var out, errb bytes.Buffer
		rc := &tool.RunContext{
			Ctx: context.Background(), Dir: t.TempDir(),
			Env:   []string{"PATH=/bin:/usr/bin", "POSIXLY_CORRECT=1"},
			Stdio: tool.Stdio{Out: &out, Err: &errb, In: strings.NewReader("")},
		}
		if code := run(rc, []string{"--", "sh", "-c", "printf posix-double-dash"}); code != 0 || out.String() != "posix-double-dash" || errb.Len() != 0 {
			t.Fatalf("POSIX --: code=%d out=%q err=%q", code, out.String(), errb.String())
		}
	})

	t.Run("gnu_help_extension_has_explicit_operand_disambiguation", func(t *testing.T) {
		dir := t.TempDir()
		name := "--help"
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\nprintf utility-help\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		var out, errb bytes.Buffer
		rc := &tool.RunContext{
			Ctx: context.Background(), Dir: dir, Env: []string{"PATH=."},
			Stdio: tool.Stdio{Out: &out, Err: &errb, In: strings.NewReader("")},
		}
		if code := run(rc, []string{"--help"}); code != 0 || !strings.Contains(out.String(), "Usage: nohup") {
			t.Fatalf("extension help: code=%d out=%q err=%q", code, out.String(), errb.String())
		}
		out.Reset()
		errb.Reset()
		if code := run(rc, []string{"--", "--help"}); code != 0 || out.String() != "utility-help" || errb.Len() != 0 {
			t.Fatalf("disambiguated utility: code=%d out=%q err=%q", code, out.String(), errb.String())
		}
	})
}

func TestNohupSearchesPATHRelativeToRunContextDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell command differs on windows")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	command := filepath.Join(bin, "helper")
	if err := os.WriteFile(command, []byte("#!/bin/sh\nprintf relative\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Env:   []string{"PATH=bin"},
		Stdio: tool.Stdio{Out: &out, Err: &errb, In: strings.NewReader("")},
	}
	if code := run(rc, []string{"helper"}); code != 0 || out.String() != "relative" {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), errb.String())
	}
}

func TestNohupFoundButNotExecutableReturns126(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission semantics differ on windows")
	}
	dir := t.TempDir()
	command := filepath.Join(dir, "blocked")
	if err := os.WriteFile(command, []byte("#!/bin/sh\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Env: []string{"PATH=."}, Stdio: tool.Stdio{Out: &out, Err: &errb}}
	if code := run(rc, []string{"blocked"}); code != 126 {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), errb.String())
	}
}

func TestNohupNotFoundReturns127(t *testing.T) {
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: t.TempDir(), Env: []string{"PATH=" + t.TempDir()}, Stdio: tool.Stdio{Out: &out, Err: &errb}}
	if code := run(rc, []string{"missing-command"}); code != 127 {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), errb.String())
	}
}

func TestNohupRedirectsTerminalEquivalentOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell command differs on windows")
	}
	dir := t.TempDir()
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Env:   []string{"PATH=/bin:/usr/bin"},
		Stdio: tool.Stdio{In: strings.NewReader("")},
	}
	if code := run(rc, []string{"sh", "-c", "printf out; printf err >&2; exit 17"}); code != 17 {
		t.Fatalf("code=%d", code)
	}
	data, err := os.ReadFile(filepath.Join(dir, "nohup.out"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "outerr" {
		t.Fatalf("nohup.out=%q", data)
	}
}

func TestNohupRedirectsStderrToStdoutWhenErrIsNil(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell command differs on windows")
	}
	var out bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   t.TempDir(),
		Env:   []string{"PATH=/bin:/usr/bin"},
		Stdio: tool.Stdio{Out: &out, In: strings.NewReader("")},
	}
	if code := run(rc, []string{"sh", "-c", "printf out; printf err >&2"}); code != 0 {
		t.Fatalf("code=%d out=%q", code, out.String())
	}
	if out.String() != "outerr" {
		t.Fatalf("stdout=%q", out.String())
	}
}

func TestNohupClosedStdoutAndTerminalStderrRequiresNohupOut(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pty not supported in this test on windows")
	}
	ptm, pts, err := pty.Open()
	if err != nil {
		t.Skipf("pty.Open failed: %v", err)
	}
	defer ptm.Close()
	defer pts.Close()

	closedOut, err := os.CreateTemp(t.TempDir(), "closed-stdout")
	if err != nil {
		t.Fatal(err)
	}
	if err := closedOut.Close(); err != nil {
		t.Fatal(err)
	}

	wantErr := errors.New("both nohup.out locations are unavailable")
	oldOpener := nohupOutputOpener
	openerCalled := false
	nohupOutputOpener = func(*tool.RunContext) (*os.File, string, error) {
		openerCalled = true
		return nil, "", wantErr
	}
	t.Cleanup(func() { nohupOutputOpener = oldOpener })

	rc := &tool.RunContext{
		Ctx: context.Background(), Dir: t.TempDir(),
		Env:   []string{"PATH=/bin:/usr/bin", "POSIXLY_CORRECT=1"},
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: closedOut, Err: pts},
	}
	if code := run(rc, []string{"sh", "-c", "exit 0"}); code != 127 {
		t.Fatalf("closed stdout redirect failure code=%d, want 127", code)
	}
	if !openerCalled {
		t.Fatal("nohup did not try nohup.out when stdout was closed and stderr was a terminal")
	}
}

func TestNohupOutputHelper(t *testing.T) {
	if os.Getenv("GO_WANT_NOHUP_OUTPUT_HELPER") != "1" {
		return
	}
	fmt.Fprint(os.Stdout, "home")
	os.Exit(0)
}

func TestNohupInputHelper(t *testing.T) {
	if os.Getenv("GO_WANT_NOHUP_INPUT_HELPER") != "1" {
		return
	}
	var buf [1024]byte
	_, err := os.Stdin.Read(buf[:])
	if err != nil && (errors.Is(err, syscall.EBADF) || strings.Contains(err.Error(), "bad file descriptor")) {
		fmt.Fprint(os.Stdout, "read_ebadf")
		os.Exit(0)
	}
	fmt.Fprintf(os.Stdout, "err:%v", err)
	os.Exit(2)
}

func TestNohupRedirectsTerminalInput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pty not supported in this test on windows")
	}
	ptm, pts, err := pty.Open()
	if err != nil {
		t.Skipf("pty.Open failed: %v", err)
	}
	defer ptm.Close()
	defer pts.Close()

	var out, errb bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rc := &tool.RunContext{
		Ctx:   ctx,
		Dir:   t.TempDir(),
		Env:   append(os.Environ(), "GO_WANT_NOHUP_INPUT_HELPER=1"),
		Stdio: tool.Stdio{In: pts, Out: &out, Err: &errb},
	}
	result := make(chan int, 1)
	go func() {
		result <- run(rc, []string{os.Args[0], "-test.run=^TestNohupInputHelper$"})
	}()

	select {
	case code := <-result:
		if code != 0 || out.String() != "read_ebadf" {
			t.Fatalf("code=%d out=%q err=%q", code, out.String(), errb.String())
		}
	case <-time.After(2 * time.Second):
		cancel()
		<-result
		t.Fatal("nohup left the child reading from terminal input")
	}
}

func TestNohupFallsBackToHomeNohupOut(t *testing.T) {
	dir, home := t.TempDir(), t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "nohup.out"), 0o755); err != nil {
		t.Fatal(err)
	}
	env := append(os.Environ(), "GO_WANT_NOHUP_OUTPUT_HELPER=1", "HOME="+home)
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Env: env, Stdio: tool.Stdio{}}
	if code := run(rc, []string{os.Args[0], "-test.run=^TestNohupOutputHelper$"}); code != 0 {
		t.Fatalf("code=%d", code)
	}
	data, err := os.ReadFile(filepath.Join(home, "nohup.out"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "home" {
		t.Fatalf("home/nohup.out=%q", data)
	}
}

func TestNohupRedirectsTerminalOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pty not supported in this test on windows")
	}
	ptm, pts, err := pty.Open()
	if err != nil {
		t.Skipf("pty.Open failed: %v", err)
	}
	defer ptm.Close()
	defer pts.Close()

	dir := t.TempDir()
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Env:   []string{"PATH=/bin:/usr/bin"},
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: pts, Err: pts},
	}
	if code := run(rc, []string{"sh", "-c", "printf out; printf err >&2"}); code != 0 {
		t.Fatalf("code=%d", code)
	}

	data, err := os.ReadFile(filepath.Join(dir, "nohup.out"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "outerr" {
		t.Fatalf("nohup.out=%q", data)
	}
}

func TestNohupTerminalRedirectionDiagnostics(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pty not supported on windows")
	}

	runWithTty := func(t *testing.T, inTty, outTty, errTty bool, expectedMsg string) {
		t.Helper()
		var ptmIn, ptsIn *os.File
		var ptmOut, ptsOut *os.File
		var ptmErr, ptsErr *os.File
		var err error

		if inTty {
			ptmIn, ptsIn, err = pty.Open()
			if err != nil {
				t.Skipf("pty.Open failed: %v", err)
			}
			defer ptmIn.Close()
			defer ptsIn.Close()
		}
		if outTty {
			ptmOut, ptsOut, err = pty.Open()
			if err != nil {
				t.Skipf("pty.Open failed: %v", err)
			}
			defer ptmOut.Close()
			defer ptsOut.Close()
		}
		if errTty {
			ptmErr, ptsErr, err = pty.Open()
			if err != nil {
				t.Skipf("pty.Open failed: %v", err)
			}
			defer ptmErr.Close()
			defer ptsErr.Close()
		}

		var stdin io.Reader = strings.NewReader("")
		if inTty {
			stdin = ptsIn
		}
		var stdout io.Writer = &bytes.Buffer{}
		if outTty {
			stdout = ptsOut
		}
		var stderr bytes.Buffer

		rc := &tool.RunContext{
			Ctx:   context.Background(),
			Dir:   t.TempDir(),
			Env:   []string{"PATH=/bin:/usr/bin"},
			Stdio: tool.Stdio{In: stdin, Out: stdout, Err: &stderr},
		}
		if errTty {
			rc.Stdio.Err = ptsErr
		}

		result := make(chan int, 1)
		go func() {
			result <- run(rc, []string{"true"})
		}()

		select {
		case code := <-result:
			if code != 0 {
				t.Errorf("expected exit code 0, got %d", code)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timeout waiting for command to exit")
		}

		var gotMsg string
		if errTty {
			buf := make([]byte, 1024)
			_ = ptmErr.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
			n, _ := ptmErr.Read(buf)
			gotMsg = string(buf[:n])
		} else {
			gotMsg = stderr.String()
		}
		gotMsg = strings.ReplaceAll(gotMsg, "\r\n", "\n")

		if gotMsg != expectedMsg {
			t.Errorf("inTty=%v, outTty=%v, errTty=%v: expected diagnostic %q, got %q", inTty, outTty, errTty, expectedMsg, gotMsg)
		}
	}

	t.Run("in_out_err_all_tty", func(t *testing.T) {
		runWithTty(t, true, true, true, "nohup: ignoring input and appending output to 'nohup.out'\n")
	})
	t.Run("in_err_tty", func(t *testing.T) {
		runWithTty(t, true, false, true, "nohup: ignoring input and redirecting stderr to stdout\n")
	})
	t.Run("in_only_tty", func(t *testing.T) {
		runWithTty(t, true, false, false, "nohup: ignoring input\n")
	})
	t.Run("out_only_tty", func(t *testing.T) {
		runWithTty(t, false, true, false, "nohup: appending output to 'nohup.out'\n")
	})
	t.Run("err_only_tty", func(t *testing.T) {
		runWithTty(t, false, false, true, "nohup: redirecting stderr to stdout\n")
	})
	t.Run("no_tty", func(t *testing.T) {
		runWithTty(t, false, false, false, "")
	})
}

func TestNohupDevNullOpenFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pty not supported on windows")
	}

	// A PTY stands in for a terminal stdin so the /dev/null redirect path runs.
	_, pts, err := pty.Open()
	if err != nil {
		t.Skipf("pty.Open failed: %v", err)
	}
	defer pts.Close()

	// Inject an opener that always fails, simulating an environment where
	// /dev/null cannot be opened write-only.
	oldOpener := devNullOpener
	defer func() { devNullOpener = oldOpener }()
	devNullOpener = func() (*os.File, error) {
		return nil, errors.New("permission denied")
	}

	// runNohup must redirect terminal stdin before resolving the command, so a
	// failing opener reports the Issue 7 internal-error status 127 with the
	// redirect diagnostic and never starts a child. Ordering is proven by the
	// diagnostic (and the absent sentinel), since a lookup failure would
	// report "command not found" at the same status.
	const expectedDiag = "nohup: failed to render standard input unusable: permission denied\n"
	runCase := func(t *testing.T, env []string, command string, args ...string) {
		tempDir := t.TempDir()
		var out, errb bytes.Buffer
		// Non-terminal stdout/stderr keep this on the stdin-redirect path only.
		rc := &tool.RunContext{
			Ctx:   context.Background(),
			Dir:   tempDir,
			Env:   env,
			Stdio: tool.Stdio{In: pts, Out: &out, Err: &errb},
		}
		// A sentinel proves no child ever started, even when the command is
		// resolvable and would create it.
		sentinel := filepath.Join(tempDir, "should_not_exist")
		full := append(append([]string{command}, args...), sentinel)

		code := run(rc, full)

		if code != 127 {
			t.Fatalf("expected exit code 127 (nohup-internal redirect failure), got %d err=%q", code, errb.String())
		}
		if got := errb.String(); got != expectedDiag {
			t.Errorf("expected diagnostic %q, got %q", expectedDiag, got)
		}
		// The lookup outcome ("command not found") must not be reached:
		// the redirect runs first and wins.
		if strings.Contains(errb.String(), "command not found") {
			t.Errorf("command lookup ran before redirect: %q", errb.String())
		}
		// No child may have started.
		if _, err := os.Stat(sentinel); err == nil {
			t.Errorf("sentinel %s was created; child executed despite redirect failure", sentinel)
		}
	}

	// The missing-command case is the adversarial ordering proof: if lookup ran
	// first the diagnostic would name the missing command instead of the
	// redirect failure.
	t.Run("missing_command", func(t *testing.T) {
		runCase(t, []string{"PATH=" + filepath.Join(t.TempDir(), "nowhere")}, "definitely-missing-command")
	})
	// touch is resolvable here, yet the opener failure must short-circuit before
	// it can start, leaving the sentinel absent.
	t.Run("found_command", func(t *testing.T) {
		runCase(t, []string{"PATH=/bin:/usr/bin"}, "touch")
	})

	// The same redirect failure reports the required 127 with POSIXLY_CORRECT
	// set as well: the status is unconditional.
	t.Run("posixly_correct", func(t *testing.T) {
		tempDir := t.TempDir()
		var out, errb bytes.Buffer
		rc := &tool.RunContext{
			Ctx:   context.Background(),
			Dir:   tempDir,
			Env:   []string{"PATH=/bin:/usr/bin", "POSIXLY_CORRECT=1"},
			Stdio: tool.Stdio{In: pts, Out: &out, Err: &errb},
		}
		sentinel := filepath.Join(tempDir, "should_not_exist")
		if code := run(rc, []string{"touch", sentinel}); code != 127 {
			t.Fatalf("expected exit code 127 (redirect failure, POSIXLY_CORRECT set), got %d err=%q", code, errb.String())
		}
		if _, err := os.Stat(sentinel); err == nil {
			t.Errorf("sentinel %s was created; child executed despite redirect failure", sentinel)
		}
	})
}
