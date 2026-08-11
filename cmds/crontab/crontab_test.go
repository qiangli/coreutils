package crontabcmd

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/pkg/schedule"
	"github.com/qiangli/coreutils/tool"
)

func runTool(tb testing.TB, ctx context.Context, stdin string, args ...string) (string, string, int) {
	tb.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   ctx,
		Dir:   tb.TempDir(),
		Stdio: tool.Stdio{In: strings.NewReader(stdin), Out: &out, Err: &errb},
	}
	code := cmd.Run(rc, args)
	return out.String(), errb.String(), code
}

func runCron(t *testing.T, ctx context.Context, stdin string, args ...string) (string, string, int) {
	return runTool(t, ctx, stdin, args...)
}

func runCronNoStdin(t *testing.T, ctx context.Context, args ...string) (string, string, int) {
	return runCron(t, ctx, "", args...)
}

func setupCronState(t *testing.T) string {
	t.Helper()
	p := t.TempDir() + "/schedule.json"
	t.Setenv("BASHY_SCHEDULE_STATE", p)
	return p
}

func TestCrontabHelp(t *testing.T) {
	out, _, code := runCronNoStdin(t, context.Background(), "--help")
	if code != 0 || !strings.Contains(out, "Usage: crontab") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
}

func TestCrontabListEmpty(t *testing.T) {
	setupCronState(t)
	out, _, code := runCronNoStdin(t, context.Background(), "-l")
	if code != 0 {
		t.Fatalf("crontab -l: code=%d", code)
	}
	if out != "" {
		t.Errorf("expected empty output, got %q", out)
	}
}

func TestCrontabInstallListRemove(t *testing.T) {
	setupCronState(t)
	cronContent := "*/15 * * * * echo hello\n0 9 * * * date\n"

	// POSIX crontab is silent on success: stdout must be empty.
	out, _, code := runCron(t, context.Background(), cronContent, "-")
	if code != 0 {
		t.Fatalf("crontab - (install from stdin): code=%d", code)
	}
	if out != "" {
		t.Errorf("stdout must be empty on successful install, got %q", out)
	}

	out, _, code = runCronNoStdin(t, context.Background(), "-l")
	if code != 0 {
		t.Fatalf("crontab -l: code=%d", code)
	}
	if !strings.Contains(out, "echo hello") {
		t.Errorf("expected 'echo hello' in list, got %q", out)
	}
	if !strings.Contains(out, "date") {
		t.Errorf("expected 'date' in list, got %q", out)
	}

	_, _, code = runCronNoStdin(t, context.Background(), "-r")
	if code != 0 {
		t.Fatalf("crontab -r: code=%d", code)
	}

	out, _, code = runCronNoStdin(t, context.Background(), "-l")
	if code != 0 {
		t.Fatalf("crontab -l after -r: code=%d", code)
	}
	if out != "" {
		t.Errorf("expected empty after -r, got %q", out)
	}
}

func TestCrontabRoundTrip(t *testing.T) {
	setupCronState(t)
	cronContent := "0 9 * * * echo hello world\n30 18 * * 1 date\n"

	_, _, code := runCron(t, context.Background(), cronContent, "-")
	if code != 0 {
		t.Fatalf("install: code=%d", code)
	}

	out, _, code := runCronNoStdin(t, context.Background(), "-l")
	if code != 0 {
		t.Fatalf("list: code=%d", code)
	}

	_, _, code = runCronNoStdin(t, context.Background(), "-r")
	if code != 0 {
		t.Fatalf("remove: code=%d", code)
	}

	_, _, code = runCron(t, context.Background(), out, "-")
	if code != 0 {
		t.Fatalf("reinstall from -l output: code=%d", code)
	}

	out2, _, code := runCronNoStdin(t, context.Background(), "-l")
	if code != 0 {
		t.Fatalf("list after round-trip: code=%d", code)
	}

	if out != out2 {
		t.Errorf("round-trip mismatch:\ninstall: %q\nlist after round-trip: %q", out, out2)
	}
}

func TestCrontabPersistsShellProgramAndContext(t *testing.T) {
	setupCronState(t)
	_, _, code := runCron(t, context.Background(), "* * * * * echo ok > result &\n", "-")
	if code != 0 {
		t.Fatalf("install: code=%d", code)
	}
	jobs, err := schedule.LoadJobs()
	if err != nil || len(jobs) != 1 {
		t.Fatalf("LoadJobs: jobs=%d err=%v", len(jobs), err)
	}
	if got, want := jobs[0].Command, []string{"sh", "-c", "echo ok > result &"}; !slices.Equal(got, want) {
		t.Errorf("job command=%q, want shell program %q", got, want)
	}
	if !jobs[0].EnvSet || !jobs[0].UmaskSet {
		t.Errorf("submission context not captured: env_set=%v umask_set=%v", jobs[0].EnvSet, jobs[0].UmaskSet)
	}
}

func TestCrontabParseSkipsComments(t *testing.T) {
	setupCronState(t)
	cronContent := "# This is a comment\n0 9 * * * echo working\n# Another comment\n"

	_, _, code := runCron(t, context.Background(), cronContent, "-")
	if code != 0 {
		t.Fatalf("install with comments: code=%d", code)
	}

	out, _, code := runCronNoStdin(t, context.Background(), "-l")
	if code != 0 {
		t.Fatalf("list: code=%d", code)
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 1 {
		t.Errorf("expected 1 line, got %d: %q", len(lines), out)
	}
	if !strings.Contains(lines[0], "echo working") {
		t.Errorf("expected 'echo working', got %q", lines[0])
	}
}

func TestCrontabReinstallReplaces(t *testing.T) {
	setupCronState(t)
	_, _, code := runCron(t, context.Background(), "0 9 * * * first\n", "-")
	if code != 0 {
		t.Fatalf("first install: code=%d", code)
	}

	_, _, code = runCron(t, context.Background(), "30 18 * * * second\n", "-")
	if code != 0 {
		t.Fatalf("second install: code=%d", code)
	}

	out, _, code := runCronNoStdin(t, context.Background(), "-l")
	if code != 0 {
		t.Fatalf("list: code=%d", code)
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 1 {
		t.Errorf("expected 1 line after replace, got %d: %q", len(lines), out)
	}
	if !strings.Contains(out, "second") {
		t.Errorf("expected 'second', got %q", out)
	}
	if strings.Contains(out, "first") {
		t.Errorf("'first' should have been replaced, got %q", out)
	}
}

func TestCrontabBadLine(t *testing.T) {
	setupCronState(t)
	_, _, code := runCron(t, context.Background(), "0 9 * * * echo original\n", "-")
	if code != 0 {
		t.Fatalf("seed install: code=%d", code)
	}
	cronContent := "0 9 * * * echo replacement\njust three fields\n"

	_, errb, code := runCron(t, context.Background(), cronContent, "-")
	if code != 1 {
		t.Fatalf("install with bad line: code=%d, want 1", code)
	}
	if !strings.Contains(errb, "not enough fields") {
		t.Errorf("expected error about not enough fields, got %q", errb)
	}

	out, _, code := runCronNoStdin(t, context.Background(), "-l")
	if code != 0 {
		t.Fatalf("list: code=%d", code)
	}
	if out != "0 9 * * * echo original\n" {
		t.Errorf("invalid replacement changed existing table: %q", out)
	}
}

func TestCrontabInvalidScheduleIsAtomic(t *testing.T) {
	setupCronState(t)
	if _, _, code := runCron(t, context.Background(), "0 9 * * * echo original\n", "-"); code != 0 {
		t.Fatalf("seed install: code=%d", code)
	}
	_, errb, code := runCron(t, context.Background(), "not-a-minute * * * * echo bad\n", "-")
	if code != 1 || !strings.Contains(errb, "cannot compute next run") {
		t.Fatalf("invalid schedule = (stderr %q, code %d), want diagnostic and 1", errb, code)
	}
	out, _, code := runCronNoStdin(t, context.Background(), "-l")
	if code != 0 || out != "0 9 * * * echo original\n" {
		t.Fatalf("invalid schedule changed table: out=%q code=%d", out, code)
	}
}

func TestCrontabRejectsConflictingModesAndExtraOperands(t *testing.T) {
	setupCronState(t)
	for _, tc := range []struct {
		stdin string
		args  []string
	}{
		{args: []string{"-l", "-r"}},
		{args: []string{"-l", "file"}},
		{args: []string{"-r", "file"}},
		{stdin: "0 9 * * * echo must-not-install\n", args: []string{"-", "extra"}},
	} {
		_, errb, code := runCron(t, context.Background(), tc.stdin, tc.args...)
		if code != 2 || errb == "" {
			t.Errorf("crontab %q = (stderr %q, code %d), want usage error", tc.args, errb, code)
		}
	}
	out, _, code := runCronNoStdin(t, context.Background(), "-l")
	if code != 0 || out != "" {
		t.Fatalf("invalid invocation changed table: out=%q code=%d", out, code)
	}
}

func TestCrontabUserOptionFailsClosed(t *testing.T) {
	setupCronState(t)
	_, errb, code := runCronNoStdin(t, context.Background(), "-u", "someone-else", "-l")
	if code != 2 || !strings.Contains(errb, "not supported") || !strings.Contains(errb, "-u") {
		t.Fatalf("crontab -u = (stderr %q, code %d), want fail-closed contract error", errb, code)
	}
}

func TestCrontabStdinReplace(t *testing.T) {
	setupCronState(t)
	cronContent := "0 9 * * * echo from stdin\n"

	_, _, code := runCron(t, context.Background(), cronContent)
	if code != 0 {
		t.Fatalf("install from stdin (no operands): code=%d", code)
	}

	out, _, code := runCronNoStdin(t, context.Background(), "-l")
	if code != 0 {
		t.Fatalf("list: code=%d", code)
	}
	if !strings.Contains(out, "echo from stdin") {
		t.Errorf("expected 'echo from stdin', got %q", out)
	}
}

// TestCrontabInstallSilent proves POSIX crontab is silent on success:
// neither stdin-replace, file-install, nor reinstall produce any stdout.
func TestCrontabInstallSilent(t *testing.T) {
	setupCronState(t)

	// stdin replace (crontab -)
	out, _, code := runCron(t, context.Background(), "0 9 * * * echo one\n", "-")
	if code != 0 {
		t.Fatalf("crontab -: code=%d", code)
	}
	if out != "" {
		t.Errorf("crontab - stdout must be empty, got %q", out)
	}

	// stdin replace (no operands, reads stdin)
	out, _, code = runCron(t, context.Background(), "0 9 * * * echo two\n")
	if code != 0 {
		t.Fatalf("crontab (stdin): code=%d", code)
	}
	if out != "" {
		t.Errorf("crontab (stdin) stdout must be empty, got %q", out)
	}

	// reinstall (replacement) also silent
	out, _, code = runCron(t, context.Background(), "30 18 * * * echo three\n", "-")
	if code != 0 {
		t.Fatalf("crontab reinstall: code=%d", code)
	}
	if out != "" {
		t.Errorf("crontab reinstall stdout must be empty, got %q", out)
	}
}
