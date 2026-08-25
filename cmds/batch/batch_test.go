package batchcmd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/coreutils/pkg/schedule"
	"github.com/qiangli/coreutils/tool"
)

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

func runBatch(t *testing.T, ctx context.Context, stdin string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   ctx,
		Dir:   t.TempDir(),
		Env:   []string{"BASHY_SCHEDULE_STATE=" + os.Getenv("BASHY_SCHEDULE_STATE")},
		Stdio: tool.Stdio{In: strings.NewReader(stdin), Out: &out, Err: &errb},
	}
	code = cmd.Run(rc, args)
	return out.String(), errb.String(), code
}

func setupBatchState(t *testing.T) string {
	t.Helper()
	p := t.TempDir() + "/schedule.json"
	t.Setenv("BASHY_SCHEDULE_STATE", p)
	allowBatchForTest(t)
	return p
}

func TestBatchHelp(t *testing.T) {
	out, _, code := runBatch(t, context.Background(), "", "--help")
	if code != 0 || !strings.Contains(out, "Usage: batch") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
}

func TestBatchRelativeFileFailsBeforeProcessCWDLookup(t *testing.T) {
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(),
		Env: []string{"BASHY_SCHEDULE_STATE=" + filepath.Join(t.TempDir(), "schedule.json")},
		Stdio: tool.Stdio{
			In: strings.NewReader(""), Out: &out, Err: &errb,
		},
	}
	code := cmd.Run(rc, []string{"-f", "relative-job"})
	if code != 2 || !strings.Contains(errb.String(), "unknown shorthand") {
		t.Fatalf("code=%d stderr=%q", code, errb.String())
	}
	if strings.Contains(errb.String(), "cannot read") || strings.Contains(errb.String(), "invocation working directory") {
		t.Fatalf("relative operand consulted process cwd: %q", errb.String())
	}
}

// TestBatchSubmissionStdoutEmpty proves the POSIX invariant: a successful
// batch submission writes nothing to stdout. The job diagnostic belongs on
// stderr only.
func TestBatchSubmissionStdoutEmpty(t *testing.T) {
	setupBatchState(t)
	out, _, code := runBatch(t, context.Background(), "echo hello\n")
	if code != 0 {
		t.Fatalf("batch: code=%d", code)
	}
	if out != "" {
		t.Errorf("stdout must be empty on success, got %q", out)
	}
}

func TestBatchAuthenticatedRecipientAndWriteError(t *testing.T) {
	setupBatchState(t)
	identity, err := schedule.AuthenticatedIdentity()
	if err != nil {
		t.Fatal(err)
	}
	rc := &tool.RunContext{
		Ctx: context.Background(), Dir: t.TempDir(),
		Env:   []string{"BASHY_SCHEDULE_STATE=" + os.Getenv("BASHY_SCHEDULE_STATE"), "LOGNAME=attacker"},
		Stdio: tool.Stdio{In: strings.NewReader("true\n"), Out: &bytes.Buffer{}, Err: failingWriter{}},
	}
	if code := cmd.Run(rc, nil); code == 0 {
		t.Fatal("batch succeeded despite failed confirmation write")
	}
	jobs, loadErr := schedule.LoadJobs()
	if loadErr != nil || len(jobs) != 0 {
		t.Fatalf("failed confirmation scheduled jobs=%v err=%v", jobs, loadErr)
	}
	rc.Stdio.Err = &bytes.Buffer{}
	if code := cmd.Run(rc, nil); code != 0 {
		t.Fatalf("successful batch submission code=%d", code)
	}
	jobs, loadErr = schedule.LoadJobs()
	if loadErr != nil || len(jobs) != 1 || jobs[0].OwnerUID != identity.UID || jobs[0].MailTo != identity.Name || jobs[0].MailTo == "attacker" {
		t.Fatalf("trusted identity: jobs=%v err=%v", jobs, loadErr)
	}
}

// TestBatchDiagnosticOnStderr proves the job confirmation appears on stderr
// in the traditional at/batch format ("job <id> at <date>"), not RFC3339.
func TestBatchDiagnosticOnStderr(t *testing.T) {
	setupBatchState(t)
	out, errb, code := runBatch(t, context.Background(), "echo hello\n")
	if code != 0 {
		t.Fatalf("batch: code=%d", code)
	}
	if out != "" {
		t.Errorf("stdout must be empty, got %q", out)
	}
	if !strings.HasPrefix(errb, "job ") {
		t.Errorf("stderr must start with 'job ', got %q", errb)
	}
	if !strings.Contains(errb, " at ") {
		t.Errorf("stderr must contain ' at ' separator, got %q", errb)
	}
	// Traditional format is "Mon Jan _2 15:04:05 2006". Verify the
	// timestamp portion is NOT RFC3339 (which contains a 'T' separator
	// between date and time and a +/--offset) and DOES parse as the
	// traditional layout.
	parts := strings.SplitN(errb, " at ", 2)
	if len(parts) != 2 {
		t.Fatalf("cannot split ' at ' in %q", errb)
	}
	ts := strings.TrimSpace(parts[1])
	if _, err := time.Parse(time.RFC3339, ts); err == nil {
		t.Errorf("stderr time %q parsed as RFC3339 — should be traditional format", ts)
	}
	if _, err := time.Parse("Mon Jan _2 15:04:05 2006", ts); err != nil {
		t.Errorf("stderr time %q is not traditional at/batch format: %v", ts, err)
	}
}

func TestBatchEmptyStdin(t *testing.T) {
	setupBatchState(t)
	_, errb, code := runBatch(t, context.Background(), "")
	if code != 0 {
		t.Errorf("empty stdin: code=%d err=%q", code, errb)
	}
	jobs, err := schedule.LoadJobs()
	if err != nil || len(jobs) != 1 {
		t.Fatalf("jobs=%v err=%v", jobs, err)
	}
	if got := jobs[0].Command; len(got) != 3 || got[2] != "" {
		t.Fatalf("empty job command=%q", got)
	}
}

// TestBatchPersistsJob proves the job is persisted to the schedule store
// with the correct kind and enabled state.
func TestBatchPersistsJob(t *testing.T) {
	setupBatchState(t)
	_, _, code := runBatch(t, context.Background(), "echo persisted > result &\n")
	if code != 0 {
		t.Fatalf("batch: code=%d", code)
	}
	jobs, err := schedule.LoadJobs()
	if err != nil {
		t.Fatalf("LoadJobs: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected 1 persisted job, got %d", len(jobs))
	}
	if jobs[0].Kind != "at" {
		t.Errorf("job kind=%q, want 'at'", jobs[0].Kind)
	}
	if !jobs[0].Enabled {
		t.Errorf("job should be enabled")
	}
	if got, want := jobs[0].Command, []string{"sh", "-c", "echo persisted > result &\n"}; !slices.Equal(got, want) {
		t.Errorf("job command=%q, want shell program %q", got, want)
	}
	if jobs[0].Queue != "b" || !jobs[0].MailOutput || !jobs[0].MailCompletion || !jobs[0].BatchLoad {
		t.Errorf("batch semantics not captured: queue=%q mail_output=%v mail_completion=%v batch_load=%v",
			jobs[0].Queue, jobs[0].MailOutput, jobs[0].MailCompletion, jobs[0].BatchLoad)
	}
	if !jobs[0].EnvSet || !jobs[0].UmaskSet {
		t.Errorf("submission context not captured: env_set=%v umask_set=%v", jobs[0].EnvSet, jobs[0].UmaskSet)
	}
}

func TestBatchIsAtQueueBWithCompletionMailAndLoadMarker(t *testing.T) {
	setupBatchState(t)
	before := time.Now()
	_, stderr, code := runBatch(t, context.Background(), "")
	if code != 0 {
		t.Fatalf("batch: code=%d stderr=%q", code, stderr)
	}
	j := mustOneBatchJob(t)
	if j.Kind != "at" || j.Queue != "b" || !j.MailOutput || !j.MailCompletion || !j.BatchLoad {
		t.Fatalf("batch not represented as at -q b -m with load policy: %+v", j)
	}
	if j.NextRun.Before(before) || j.NextRun.After(time.Now().Add(time.Second)) {
		t.Fatalf("batch next run=%v does not represent now", j.NextRun)
	}
}

func mustOneBatchJob(t *testing.T) *schedule.Job {
	t.Helper()
	jobs, err := schedule.LoadJobs()
	if err != nil || len(jobs) != 1 {
		t.Fatalf("jobs=%v err=%v", jobs, err)
	}
	return jobs[0]
}

func TestBatchUnknownFlag(t *testing.T) {
	setupBatchState(t)
	_, errb, code := runBatch(t, context.Background(), "", "--bogus")
	if code != 2 || !strings.Contains(errb, "bogus") {
		t.Errorf("unknown flag: code=%d err=%q", code, errb)
	}
}

func TestBatchRejectsOperands(t *testing.T) {
	setupBatchState(t)
	_, errb, code := runBatch(t, context.Background(), "", "extra")
	if code != 2 || !strings.Contains(errb, "does not accept operands") {
		t.Errorf("operand: code=%d err=%q", code, errb)
	}
	jobs, err := schedule.LoadJobs()
	if err != nil || len(jobs) != 0 {
		t.Fatalf("operand scheduled jobs=%v err=%v", jobs, err)
	}
}

func TestBatchDiagnosticUsesInvocationTZAndLCTIME(t *testing.T) {
	setupBatchState(t)
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
	if code := cmd.Run(rc, nil); code != 0 {
		t.Fatalf("batch: code=%d stderr=%q", code, errb.String())
	}
	if out.String() != "" {
		t.Fatalf("stdout=%q", out.String())
	}
	if !strings.Contains(errb.String(), " at Di ") {
		t.Fatalf("localized diagnostic=%q", errb.String())
	}
}
