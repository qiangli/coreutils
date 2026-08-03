package atcmd

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/coreutils/pkg/schedule"
	"github.com/qiangli/coreutils/tool"
)

func runAT(t *testing.T, ctx context.Context, stdin string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	return runTool(t, ctx, stdin, args...)
}

func runATNoStdin(t *testing.T, ctx context.Context, args ...string) (stdout, stderr string, code int) {
	return runAT(t, ctx, "", args...)
}

func setupATState(t *testing.T) string {
	t.Helper()
	p := t.TempDir() + "/schedule.json"
	t.Setenv("BASHY_SCHEDULE_STATE", p)
	return p
}

func TestAtHelp(t *testing.T) {
	out, _, code := runATNoStdin(t, context.Background(), "--help")
	if code != 0 || !strings.Contains(out, "Usage: at") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
}

func TestAtMissingTimespec(t *testing.T) {
	_, errb, code := runATNoStdin(t, context.Background())
	if code != 2 || !strings.Contains(errb, "missing timespec") {
		t.Errorf("missing timespec: code=%d err=%q", code, errb)
	}
}

func TestAtInvalidTimespec(t *testing.T) {
	_, errb, code := runATNoStdin(t, context.Background(), "bogus")
	if code != 2 || !strings.Contains(errb, "invalid timespec") {
		t.Errorf("invalid timespec: code=%d err=%q", code, errb)
	}
}

func TestAtCreateAndListAndRemove(t *testing.T) {
	setupATState(t)
	stdin := "echo hello world\n"

	out, errb, code := runAT(t, context.Background(), stdin, "now", "+", "1", "hour")
	if code != 0 {
		t.Fatalf("at now + 1 hour: code=%d", code)
	}
	if out != "" || !strings.Contains(errb, "job ") || !strings.Contains(errb, " at ") {
		t.Errorf("at submission streams: stdout=%q stderr=%q", out, errb)
	}

	out, _, code = runATNoStdin(t, context.Background(), "-l")
	if code != 0 {
		t.Fatalf("at -l: code=%d", code)
	}
	if !strings.Contains(out, "\t") {
		t.Errorf("at -l missing job/date fields: %q", out)
	}

	jobs, _ := schedule.LoadJobs()
	for _, j := range jobs {
		if j.Kind == "at" && j.Enabled {
			_, _, code = runATNoStdin(t, context.Background(), "-r", j.ID)
			if code != 0 {
				t.Errorf("at -r %s: code=%d", j.ID, code)
			}
		}
	}

	out, _, code = runATNoStdin(t, context.Background(), "-l")
	if code != 0 {
		t.Fatalf("at -l after remove: code=%d", code)
	}
	if out != "" {
		t.Errorf("expected no output after removing all jobs: %q", out)
	}
}

func TestAtRemoveNonexistent(t *testing.T) {
	setupATState(t)
	_, errb, code := runATNoStdin(t, context.Background(), "-r", "nonexistent123")
	if code != 0 {
		t.Errorf("at -r nonexistent: code=%d want 0", code)
	}
	if !strings.Contains(errb, "no job") {
		t.Errorf("expected 'no job' error: %q", errb)
	}
}

func TestAtEmptyStdin(t *testing.T) {
	setupATState(t)
	_, errb, code := runAT(t, context.Background(), "", "now", "+", "1", "hour")
	if code != 2 || !strings.Contains(errb, "no command given") {
		t.Errorf("empty stdin: code=%d err=%q", code, errb)
	}
}

func TestAtHHMM(t *testing.T) {
	setupATState(t)
	stdin := "true\n"

	out, errb, code := runAT(t, context.Background(), stdin, "23:59")
	if code != 0 {
		t.Fatalf("at 23:59: code=%d", code)
	}
	if out != "" || !strings.Contains(errb, "job ") {
		t.Errorf("submission streams: stdout=%q stderr=%q", out, errb)
	}
}

func TestAtMidnight(t *testing.T) {
	setupATState(t)
	stdin := "true\n"

	out, errb, code := runAT(t, context.Background(), stdin, "midnight")
	if code != 0 {
		t.Fatalf("at midnight: code=%d", code)
	}
	if out != "" || !strings.Contains(errb, "job ") {
		t.Errorf("submission streams: stdout=%q stderr=%q", out, errb)
	}
}

func TestAtNoon(t *testing.T) {
	setupATState(t)
	stdin := "true\n"

	out, errb, code := runAT(t, context.Background(), stdin, "noon")
	if code != 0 {
		t.Fatalf("at noon: code=%d", code)
	}
	if out != "" || !strings.Contains(errb, "job ") {
		t.Errorf("submission streams: stdout=%q stderr=%q", out, errb)
	}
}

func TestAtFromFile(t *testing.T) {
	setupATState(t)
	dir := t.TempDir()
	f, err := os.CreateTemp(dir, "at-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("echo from file\n")
	f.Close()

	out, errb, code := runATNoStdin(t, context.Background(), "-f", f.Name(), "midnight")
	if code != 0 {
		t.Fatalf("at -f %s midnight: code=%d", f.Name(), code)
	}
	if out != "" || !strings.Contains(errb, "job ") {
		t.Errorf("submission streams: stdout=%q stderr=%q", out, errb)
	}
}

func TestAtJobRetainsShellProgramAndWorkingDirectory(t *testing.T) {
	setupATState(t)
	dir := t.TempDir()
	program := "printf '%s\\n' 'hello world' > marker\n"
	var stdout, stderr bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(),
		Dir: dir,
		Env: []string{"SHELL=sh"},
		Stdio: tool.Stdio{
			In: strings.NewReader(program), Out: &stdout, Err: &stderr,
		},
	}
	if code := cmd.Run(rc, []string{"now"}); code != 0 {
		t.Fatalf("at now: code=%d stderr=%q", code, stderr.String())
	}
	if stdout.Len() != 0 || !strings.HasPrefix(stderr.String(), "job ") {
		t.Fatalf("submission streams: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	jobs, err := schedule.LoadJobs()
	if err != nil || len(jobs) != 1 {
		t.Fatalf("jobs=%v err=%v", jobs, err)
	}
	wantCommand := []string{"sh", "-c", strings.TrimSpace(program)}
	if !reflect.DeepEqual(jobs[0].Command, wantCommand) {
		t.Fatalf("stored command=%q, want %q", jobs[0].Command, wantCommand)
	}
	if jobs[0].Dir != dir {
		t.Fatalf("stored dir=%q, want %q", jobs[0].Dir, dir)
	}

	job := exec.Command(jobs[0].Command[0], jobs[0].Command[1:]...)
	job.Dir = jobs[0].Dir
	if out, err := job.CombinedOutput(); err != nil {
		t.Fatalf("execute stored shell program: %v output=%q", err, out)
	}
	got, err := os.ReadFile(filepath.Join(dir, "marker"))
	if err != nil || string(got) != "hello world\n" {
		t.Fatalf("marker=%q err=%v", got, err)
	}
}

func TestAtFileNotFound(t *testing.T) {
	setupATState(t)
	_, errb, code := runATNoStdin(t, context.Background(), "-f", "/nonexistent/file", "midnight")
	if code != 2 || !strings.Contains(errb, "cannot read file") {
		t.Errorf("-f nonexistent: code=%d err=%q", code, errb)
	}
}

func TestAtUnknownFlag(t *testing.T) {
	_, errb, code := runATNoStdin(t, context.Background(), "--bogus")
	if code != 2 || !strings.Contains(errb, "bogus") {
		t.Errorf("unknown flag: code=%d err=%q", code, errb)
	}
}

func TestAtPastTime(t *testing.T) {
	setupATState(t)
	stdin := "true\n"

	_, errb, code := runAT(t, context.Background(), stdin, "2000-01-01", "00:00")
	if code != 2 || !strings.Contains(errb, "in the past") {
		t.Errorf("past time: code=%d err=%q want 2 and 'in the past'", code, errb)
	}
}

func TestParseAtTimespec(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.Local)
	cases := []struct {
		input string
		ok    bool
	}{
		{"midnight", true},
		{"noon", true},
		{"now + 5 minutes", true},
		{"now + 1 hour", true},
		{"now + 3 days", true},
		{"now + 2 weeks", true},
		{"now + 1 month", true},
		{"15:04", true},
		{"23:59", true},
		{"2026-06-01 15:04", true},
		{"2026-06-01T15:04:05", true},
		{time.Date(2026, 7, 1, 9, 0, 0, 0, time.Local).Format(time.RFC3339), true},
		{"bogus nonsense", false},
		{"", false},
	}

	for _, c := range cases {
		_, err := schedule.ParseAtTimespec(c.input, now)
		if c.ok && err != nil {
			t.Errorf("ParseAtTimespec(%q) = %v, want nil", c.input, err)
		}
		if !c.ok && err == nil {
			t.Errorf("ParseAtTimespec(%q) = nil, want error", c.input)
		}
	}
}
