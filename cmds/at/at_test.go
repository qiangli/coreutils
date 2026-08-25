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
	allowAtForTest(t)
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

func TestAtRelativeFileFailsBeforeProcessCWDLookup(t *testing.T) {
	allowAtForTest(t)
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(),
		Env: []string{"BASHY_SCHEDULE_STATE=" + filepath.Join(t.TempDir(), "schedule.json")},
		Stdio: tool.Stdio{
			In: strings.NewReader(""), Out: &out, Err: &errb,
		},
	}
	code := cmd.Run(rc, []string{"-f", "relative-job", "now", "+", "1", "hour"})
	if code != 2 || !strings.Contains(errb.String(), "invocation working directory") {
		t.Fatalf("code=%d stderr=%q", code, errb.String())
	}
	if strings.Contains(errb.String(), "cannot read") {
		t.Fatalf("relative operand consulted process cwd: %q", errb.String())
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

func TestAtQueueSubmissionAndListFiltering(t *testing.T) {
	setupATState(t)
	for _, queue := range []string{"a", "c"} {
		if _, stderr, code := runAT(t, context.Background(), "true\n", "-q", queue, "now", "+", "1", "day"); code != 0 {
			t.Fatalf("submit queue %s: code=%d stderr=%q", queue, code, stderr)
		}
	}
	jobs, err := schedule.LoadJobs()
	if err != nil || len(jobs) != 2 || jobs[0].Queue != "a" || jobs[1].Queue != "c" {
		t.Fatalf("queued jobs=%v err=%v", jobs, err)
	}
	out, stderr, code := runATNoStdin(t, context.Background(), "-lq", "c")
	if code != 0 || stderr != "" || !strings.Contains(out, jobs[1].ID) || strings.Contains(out, jobs[0].ID) {
		t.Fatalf("filtered list: code=%d stdout=%q stderr=%q", code, out, stderr)
	}
	out, _, code = runATNoStdin(t, context.Background(), "-l")
	if code != 0 || !strings.Contains(out, jobs[0].ID) || !strings.Contains(out, jobs[1].ID) {
		t.Fatalf("unfiltered list: code=%d stdout=%q", code, out)
	}
}

func TestAtTouchTimeAndInvalidQueue(t *testing.T) {
	setupATState(t)
	_, stderr, code := runAT(t, context.Background(), "true\n", "-t202901051015.00")
	if code != 0 {
		t.Fatalf("-t: code=%d stderr=%q", code, stderr)
	}
	jobs, _ := schedule.LoadJobs()
	want := time.Date(2029, time.January, 5, 10, 15, 0, 0, time.Local)
	if len(jobs) != 1 || !jobs[0].NextRun.Equal(want) {
		t.Fatalf("-t next=%v, want %v", jobs[0].NextRun, want)
	}
	_, stderr, code = runAT(t, context.Background(), "true\n", "-q", "AA", "now")
	if code != 2 || !strings.Contains(stderr, "invalid queue") {
		t.Fatalf("invalid queue: code=%d stderr=%q", code, stderr)
	}
}

func TestAtRemoveNonexistent(t *testing.T) {
	setupATState(t)
	_, errb, code := runATNoStdin(t, context.Background(), "-r", "nonexistent123")
	if code == 0 {
		t.Errorf("at -r nonexistent: code=%d want nonzero", code)
	}
	if !strings.Contains(errb, "no job") {
		t.Errorf("expected 'no job' error: %q", errb)
	}
}

func TestAtAcceptsEmptyAndBlankStdin(t *testing.T) {
	setupATState(t)
	for _, stdin := range []string{"", "   \n\t"} {
		_, errb, code := runAT(t, context.Background(), stdin, "now", "+", "1", "hour")
		if code != 0 {
			t.Fatalf("stdin %q: code=%d err=%q", stdin, code, errb)
		}
	}
	jobs, err := schedule.LoadJobs()
	if err != nil || len(jobs) != 2 {
		t.Fatalf("jobs=%v err=%v", jobs, err)
	}
	if got := jobs[0].Command; len(got) != 3 || got[2] != "" {
		t.Fatalf("empty command=%q", got)
	}
	if got := jobs[1].Command; len(got) != 3 || got[2] != "   \n\t" {
		t.Fatalf("blank command=%q", got)
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
	state := setupATState(t)
	dir := t.TempDir()
	program := "printf '%s\\n' 'hello world' > marker\n"
	var stdout, stderr bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Env:   []string{"PATH=/usr/bin:/bin", "SHELL=sh", "MBI_ENV=garp", "BASHY_SCHEDULE_STATE=" + state},
		Umask: 0o027, UmaskSet: true,
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
	wantCommand := []string{"sh", "-c", program}
	if !reflect.DeepEqual(jobs[0].Command, wantCommand) {
		t.Fatalf("stored command=%q, want %q", jobs[0].Command, wantCommand)
	}
	if jobs[0].Dir != dir {
		t.Fatalf("stored dir=%q, want %q", jobs[0].Dir, dir)
	}
	if !jobs[0].EnvSet || !reflect.DeepEqual(jobs[0].Env, rc.Env) {
		t.Fatalf("stored env=%q (set=%v), want %q", jobs[0].Env, jobs[0].EnvSet, rc.Env)
	}
	if !jobs[0].UmaskSet || jobs[0].Umask != 0o027 {
		t.Fatalf("stored umask=%04o (set=%v), want 0027", jobs[0].Umask, jobs[0].UmaskSet)
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

func TestAtRejectsCrossFamilySynopsisCombinations(t *testing.T) {
	setupATState(t)
	cases := [][]string{
		{"-l", "-m"},
		{"-l", "-q", "b", "some-job"},
		{"-r", "-q", "a", "some-job"},
		{"-r", "-t", "202901051015", "some-job"},
		{"-l", "-r", "some-job"},
	}
	for _, args := range cases {
		_, stderr, code := runATNoStdin(t, context.Background(), args...)
		if code != 2 {
			t.Fatalf("at %v: code=%d stderr=%q", args, code, stderr)
		}
	}
	if jobs, err := schedule.LoadJobs(); err != nil || len(jobs) != 0 {
		t.Fatalf("invalid synopsis scheduled jobs=%v err=%v", jobs, err)
	}
}

func TestAtMailCompletionState(t *testing.T) {
	setupATState(t)
	_, stderr, code := runAT(t, context.Background(), "true\n", "-m", "now", "+", "1", "hour")
	if code != 0 {
		t.Fatalf("at -m: code=%d stderr=%q", code, stderr)
	}
	jobs, err := schedule.LoadJobs()
	if err != nil || len(jobs) != 1 {
		t.Fatalf("jobs=%v err=%v", jobs, err)
	}
	if !jobs[0].MailOutput || !jobs[0].MailCompletion {
		t.Fatalf("mail state: output=%v completion=%v", jobs[0].MailOutput, jobs[0].MailCompletion)
	}
}

func TestAtListUsesInvocationTZAndLCTIME(t *testing.T) {
	setupATState(t)
	when := time.Date(2029, time.March, 1, 2, 3, 4, 0, time.UTC)
	if err := schedule.SaveJobs([]*schedule.Job{{
		ID: "tz1", Kind: "at", Queue: "a", Enabled: true, NextRun: when,
	}}); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(),
		Dir: t.TempDir(),
		Env: []string{
			"BASHY_SCHEDULE_STATE=" + os.Getenv("BASHY_SCHEDULE_STATE"),
			"TZ=Europe/Berlin",
			"LC_TIME=de_DE.UTF-8",
		},
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
	}
	if code := cmd.Run(rc, []string{"-l"}); code != 0 {
		t.Fatalf("at -l: code=%d stderr=%q", code, errb.String())
	}
	if got := out.String(); !strings.Contains(got, "tz1\tDo Mär  1 03:03:04 2029") {
		t.Fatalf("localized listing=%q", got)
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

func TestParseLicensedAtGrammar(t *testing.T) {
	now := time.Date(2026, time.June, 1, 12, 30, 45, 0, time.UTC) // Monday
	cases := []struct {
		input string
		want  time.Time
	}{
		{"10:15 Jan 5, 2035 + 2 years", time.Date(2037, 1, 5, 10, 15, 0, 0, time.UTC)},
		{"10:15 Jan 5, 2035 + 2 weeks", time.Date(2035, 1, 19, 10, 15, 0, 0, time.UTC)},
		{"10:15 Jan 5, 2035 + 2 months", time.Date(2035, 3, 5, 10, 15, 0, 0, time.UTC)},
		{"9", time.Date(2026, 6, 2, 9, 0, 0, 0, time.UTC)},
		{"2359", time.Date(2026, 6, 1, 23, 59, 0, 0, time.UTC)},
		{"9:0", time.Date(2026, 6, 2, 9, 0, 0, 0, time.UTC)},
		{"9 pm", time.Date(2026, 6, 1, 21, 0, 0, 0, time.UTC)},
		{"12 am", time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC)},
		{"9 utc", time.Date(2026, 6, 2, 9, 0, 0, 0, time.UTC)},
		{"17 utc+ 30minutes", time.Date(2026, 6, 1, 17, 30, 0, 0, time.UTC)},
		{"17 utc Jan 24", time.Date(2027, 1, 24, 17, 0, 0, 0, time.UTC)},
		{"8:15amjan24", time.Date(2027, 1, 24, 8, 15, 0, 0, time.UTC)},
		{"now next hour", time.Date(2026, 6, 1, 13, 30, 45, 0, time.UTC)},
		{"1:00 Tuesday", time.Date(2026, 6, 2, 1, 0, 0, 0, time.UTC)},
		{"23:59 today", time.Date(2026, 6, 1, 23, 59, 0, 0, time.UTC)},
		{"1:00 tomorrow", time.Date(2026, 6, 2, 1, 0, 0, 0, time.UTC)},
	}
	for _, tc := range cases {
		got, err := schedule.ParseAtTimespecInLocation(tc.input, now, time.UTC)
		if err != nil || !got.Equal(tc.want) {
			t.Errorf("%q = %v, %v; want %v", tc.input, got, err, tc.want)
		}
	}
	for _, input := range []string{"24:00", "12:60", "13 pm", "900", "10:15 Feb 30, 2035", "now + 0 days", "10:15 + bananas"} {
		if got, err := schedule.ParseAtTimespecInLocation(input, now, time.UTC); err == nil {
			t.Errorf("invalid %q parsed as %v", input, got)
		}
	}
}

func TestParseAtTouchTime(t *testing.T) {
	now := time.Date(2026, 8, 3, 1, 2, 3, 0, time.UTC)
	for input, want := range map[string]time.Time{
		"202901051015.00": time.Date(2029, 1, 5, 10, 15, 0, 0, time.UTC),
		"2901051015":      time.Date(2029, 1, 5, 10, 15, 0, 0, time.UTC),
		"01051015":        time.Date(2026, 1, 5, 10, 15, 0, 0, time.UTC),
	} {
		got, err := schedule.ParseAtTouchTime(input, now, time.UTC)
		if err != nil || !got.Equal(want) {
			t.Errorf("-t %q = %v, %v; want %v", input, got, err, want)
		}
	}
	for _, input := range []string{"", "202902301015", "202901052415", "202901051015.99", "2029x1051015"} {
		if _, err := schedule.ParseAtTouchTime(input, now, time.UTC); err == nil {
			t.Errorf("invalid -t %q accepted", input)
		}
	}
}

// POSIX defines the at -t time_arg format as exactly touch -t's, which
// accepts a seconds field of 60 for a leap second and carries it into the
// following minute (this repo's touch -t already implements that carry —
// cmds/touch/touch.go's parseISODate). at -t must accept the same input.
func TestParseAtTouchTimeLeapSecond(t *testing.T) {
	now := time.Date(2026, 8, 3, 1, 2, 3, 0, time.UTC)
	for input, want := range map[string]time.Time{
		"202901051015.60": time.Date(2029, 1, 5, 10, 16, 0, 0, time.UTC),
		"202901052359.60": time.Date(2029, 1, 6, 0, 0, 0, 0, time.UTC),
	} {
		got, err := schedule.ParseAtTouchTime(input, now, time.UTC)
		if err != nil || !got.Equal(want) {
			t.Errorf("-t %q = %v, %v; want %v", input, got, err, want)
		}
	}
}
