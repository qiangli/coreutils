package crontabcmd

import (
	"bytes"
	"context"
	"strings"
	"testing"

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

	_, _, code := runCron(t, context.Background(), cronContent, "-")
	if code != 0 {
		t.Fatalf("crontab - (install from stdin): code=%d", code)
	}

	out, _, code := runCronNoStdin(t, context.Background(), "-l")
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
	cronContent := "0 9 * * * echo ok\njust three fields\n* * * * * cmd\n"

	_, errb, code := runCron(t, context.Background(), cronContent, "-")
	if code != 0 {
		t.Fatalf("install with bad line: code=%d", code)
	}
	if !strings.Contains(errb, "not enough fields") {
		t.Errorf("expected error about not enough fields, got %q", errb)
	}

	out, _, code := runCronNoStdin(t, context.Background(), "-l")
	if code != 0 {
		t.Fatalf("list: code=%d", code)
	}
	if !strings.Contains(out, "echo ok") {
		t.Errorf("valid line should be installed, got %q", out)
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
