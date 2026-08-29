package schedule

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	mailxpkg "github.com/qiangli/coreutils/pkg/mailx"
)

func TestUpdateJobsConcurrentWritersDoNotLoseEntries(t *testing.T) {
	withState(t)
	const writers = 24
	var wg sync.WaitGroup
	errs := make(chan error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			errs <- UpdateJobs(func(jobs []*Job) ([]*Job, error) {
				return append(jobs, &Job{ID: strconv.Itoa(id)}), nil
			})
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	jobs, err := LoadJobs()
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != writers {
		t.Fatalf("stored jobs=%d, want %d", len(jobs), writers)
	}
}

func TestFailedSubmissionConfirmationCannotRaceImmediateExecution(t *testing.T) {
	state := withState(t)
	result := filepath.Join(t.TempDir(), "executed")
	job := &Job{
		ID: "unconfirmed", Kind: "at", Enabled: true, NextRun: time.Now().Add(-time.Second),
		Command: []string{"/bin/sh", "-c", "printf ran > " + result},
	}
	confirmEntered := make(chan struct{})
	releaseConfirm := make(chan struct{})
	submitDone := make(chan error, 1)
	go func() {
		submitDone <- NewStore(state).SubmitJobWithConfirmation(job, func() error {
			close(confirmEntered)
			<-releaseConfirm
			return errors.New("confirmation failed")
		})
	}()
	<-confirmEntered
	tickDone := make(chan error, 1)
	go func() {
		_, err := TickOnceWithProviders(time.Now(), io.Discard, nil, func() (float64, error) { return 0, nil })
		tickDone <- err
	}()
	select {
	case err := <-tickDone:
		t.Fatalf("tick bypassed in-flight submission transaction: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(releaseConfirm)
	if err := <-submitDone; err == nil || !strings.Contains(err.Error(), "confirmation failed") {
		t.Fatalf("submission error=%v", err)
	}
	if err := <-tickDone; err != nil {
		t.Fatal(err)
	}
	jobs, err := LoadJobs()
	if err != nil || len(jobs) != 0 {
		t.Fatalf("failed submission remained stored: jobs=%v err=%v", jobs, err)
	}
	if _, err := os.Stat(result); !os.IsNotExist(err) {
		t.Fatalf("failed submission executed: stat err=%v", err)
	}
}

func TestScheduleOutputModeEscape(t *testing.T) {
	withState(t)
	t.Setenv("BASHY_AGENTIC", "1")

	// Under agent mode, `list` emits the JSON envelope (consistent with weave/dag).
	cmd := NewScheduleCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"list"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "schema_version") {
		t.Errorf("BASHY_AGENTIC list should be JSON, got %q", out.String())
	}

	// --json=false escapes back to prose even under agent mode.
	cmd = NewScheduleCmd()
	var out2 bytes.Buffer
	cmd.SetOut(&out2)
	cmd.SetErr(&out2)
	cmd.SetArgs([]string{"list", "--json=false"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out2.String(), "schema_version") {
		t.Errorf("--json=false should force prose, got %q", out2.String())
	}
}

func withState(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "schedule.json")
	t.Setenv("BASHY_SCHEDULE_STATE", p)
	return p
}

func TestComputeNext(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.Local)
	cases := []struct {
		kind, spec string
		ok         bool
	}{
		{"cron", "*/15 * * * *", true},
		{"every", "30m", true},
		{"every", "bogus", false},
		{"at", "2099-01-02 03:04", true},
		{"cron", "not a cron", false},
	}
	for _, c := range cases {
		j := &Job{Kind: c.kind, Spec: c.spec, CreatedAt: now}
		next, err := j.computeNext(now)
		if c.ok && err != nil {
			t.Errorf("%s %q: unexpected err %v", c.kind, c.spec, err)
		}
		if !c.ok && err == nil {
			t.Errorf("%s %q: expected error", c.kind, c.spec)
		}
		if c.ok && !next.After(now) {
			t.Errorf("%s %q: next %v not after now", c.kind, c.spec, next)
		}
	}
}

func TestAddListRemoveRoundTrip(t *testing.T) {
	withState(t)
	s, _ := load()
	s.Jobs = append(s.Jobs, &Job{ID: "j1", Kind: "every", Spec: "1h", Command: []string{"true"}, Enabled: true, CreatedAt: time.Now(), NextRun: time.Now().Add(time.Hour)})
	if err := s.save(); err != nil {
		t.Fatal(err)
	}
	s2, err := load()
	if err != nil || len(s2.Jobs) != 1 || s2.Jobs[0].ID != "j1" {
		t.Fatalf("round-trip failed: %+v %v", s2, err)
	}
}

func TestJSONListRedactsCapturedEnvironment(t *testing.T) {
	state := withState(t)
	s, _ := load()
	s.Jobs = append(s.Jobs, &Job{
		ID: "private-at", Kind: "at", Spec: time.Now().Add(time.Hour).Format(time.RFC3339),
		Command: []string{"sh", "-c", "true"}, Env: []string{"TOKEN=top-secret"}, EnvSet: true,
		Enabled: true, CreatedAt: time.Now(), NextRun: time.Now().Add(time.Hour),
	})
	if err := s.save(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(state)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("state mode=%v, want 0600", info.Mode().Perm())
	}

	cmd := NewScheduleCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"list", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "top-secret") || strings.Contains(out.String(), "TOKEN=") {
		t.Fatalf("JSON listing disclosed captured environment: %s", out.String())
	}
	loaded, err := load()
	if err != nil || len(loaded.Jobs) != 1 || !reflect.DeepEqual(loaded.Jobs[0].Env, []string{"TOKEN=top-secret"}) {
		t.Fatalf("private state did not retain environment: jobs=%v err=%v", loaded.Jobs, err)
	}
}

func TestTickFiresDueJobAndReschedules(t *testing.T) {
	withState(t)
	marker := filepath.Join(t.TempDir(), "fired")
	now := time.Now()
	s, _ := load()
	// Due job (NextRun in the past) that creates a marker file when it fires.
	s.Jobs = append(s.Jobs, &Job{
		ID: "due", Kind: "every", Spec: "1h",
		Command: []string{"sh", "-c", "echo $BASHY_SCHEDULE_PROMPT > " + filepath.ToSlash(marker)},
		Prompt:  "hello-prompt", Enabled: true, CreatedAt: now.Add(-2 * time.Hour),
		NextRun: now.Add(-time.Minute),
	})
	// Not-due job.
	s.Jobs = append(s.Jobs, &Job{ID: "future", Kind: "every", Spec: "1h", Command: []string{"true"}, Enabled: true, CreatedAt: now, NextRun: now.Add(time.Hour)})
	if err := s.save(); err != nil {
		t.Fatal(err)
	}

	fired, err := tickOnce(now, os.Stdout)
	if err != nil {
		t.Fatal(err)
	}
	if len(fired) != 1 || fired[0] != "due" {
		t.Fatalf("fired = %v, want [due]", fired)
	}
	// The agentic prompt reached the command's env.
	if b, err := os.ReadFile(marker); err != nil || string(b) != "hello-prompt\n" {
		t.Errorf("prompt not delivered via env: %q %v", b, err)
	}
	// The due job was rescheduled into the future; the other is untouched.
	s3, _ := load()
	due := s3.find("due")
	if due == nil || !due.NextRun.After(now) {
		t.Errorf("due job not rescheduled: %+v", due)
	}
}

func TestTickOneShotAtDisables(t *testing.T) {
	withState(t)
	now := time.Now()
	s, _ := load()
	s.Jobs = append(s.Jobs, &Job{ID: "once", Kind: "at", Spec: "2000-01-01 00:00", Command: []string{"true"}, Enabled: true, CreatedAt: now, NextRun: now.Add(-time.Hour)})
	_ = s.save()
	if _, err := tickOnce(now, os.Stdout); err != nil {
		t.Fatal(err)
	}
	s2, _ := load()
	if j := s2.find("once"); j == nil || j.Enabled {
		t.Errorf("one-shot at job should be disabled after firing: %+v", j)
	}
}

func TestCronFireDeliversOutputAndExplicitStdin(t *testing.T) {
	j := &Job{ID: "cron-mail", Kind: "cron", POSIXCron: true,
		Command: []string{"/bin/sh", "-c", `read first; read second; printf 'out:%s/%s\n' "$first" "$second"; printf 'err\n' >&2`},
		Stdin:   "alpha\nbeta\n", StdinSet: true, Env: []string{"PATH=/usr/bin:/bin"}, EnvSet: true,
		Umask: 0o022, UmaskSet: true, MailOutput: true, MailTo: "recipient"}
	var recipient string
	var delivered []byte
	err := FireJob(j, io.Discard, func(to string, body []byte) error {
		recipient, delivered = to, append([]byte(nil), body...)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if recipient != "recipient" || string(delivered) != "out:alpha/beta\nerr\n" {
		t.Fatalf("mail=(%q,%q)", recipient, delivered)
	}
}

func TestAtFireDeliversCompletionMailWithoutOutput(t *testing.T) {
	j := &Job{ID: "at-complete", Kind: "at",
		Command: []string{"/bin/sh", "-c", "true"}, Env: []string{"PATH=/usr/bin:/bin"}, EnvSet: true,
		MailOutput: true, MailCompletion: true, MailTo: "recipient"}
	var recipient string
	var delivered []byte
	err := FireJob(j, io.Discard, func(to string, body []byte) error {
		recipient, delivered = to, append([]byte(nil), body...)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if recipient != "recipient" || !strings.Contains(string(delivered), "at job at-complete completed successfully") {
		t.Fatalf("completion mail=(%q,%q)", recipient, delivered)
	}
}

func TestCronFireWithoutMailProviderReportsUndeliverableOutput(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "must-not-exist")
	j := &Job{ID: "cron-no-mail", Kind: "cron", POSIXCron: true, Command: []string{"/bin/sh", "-c", "echo output; touch " + marker}, MailOutput: true, MailTo: "recipient"}
	err := j.fire(io.Discard)
	if !errors.Is(err, ErrMailDeliveryUnsupported) {
		t.Fatalf("error=%v", err)
	}
	if _, statErr := os.Stat(marker); statErr != nil {
		t.Fatalf("job did not execute before output mail failure: %v", statErr)
	}
}

func TestAtCompletionMailWithoutProviderExecutesBeforeReportingDeliveryFailure(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "executed")
	j := &Job{ID: "at-no-mail", Kind: "at", Command: []string{"/bin/sh", "-c", "touch " + marker}, MailOutput: true, MailCompletion: true, MailTo: "recipient"}
	err := j.fire(io.Discard)
	if !errors.Is(err, ErrMailDeliveryUnsupported) {
		t.Fatalf("error=%v", err)
	}
	if _, statErr := os.Stat(marker); statErr != nil {
		t.Fatalf("job did not execute: %v", statErr)
	}
}

func TestTickWithoutMailProviderClaimsExecutedCronJob(t *testing.T) {
	withState(t)
	now := time.Now()
	j := &Job{ID: "due-cron", Kind: "cron", POSIXCron: true, Spec: "* * * * *", Command: []string{"/bin/sh", "-c", "true"}, Enabled: true, NextRun: now.Add(-time.Minute), MailCompletion: true, MailTo: "recipient"}
	if err := SaveJobs([]*Job{j}); err != nil {
		t.Fatal(err)
	}
	fired, err := tickOnce(now, os.Stdout)
	if !errors.Is(err, ErrMailDeliveryUnsupported) || !reflect.DeepEqual(fired, []string{"due-cron"}) {
		t.Fatalf("tick=(%v,%v)", fired, err)
	}
	jobs, loadErr := LoadJobs()
	if loadErr != nil || len(jobs) != 1 || jobs[0].LastRun.IsZero() || !jobs[0].NextRun.After(now) {
		t.Fatalf("job mutated: %+v err=%v", jobs, loadErr)
	}
}

func TestBatchLoadGatingAndMailFailureDoNotStarveJobs(t *testing.T) {
	state := withState(t)
	now := time.Now()
	dir := t.TempDir()
	batchMarker := filepath.Join(dir, "batch")
	normalMarker := filepath.Join(dir, "normal")
	jobs := []*Job{
		{ID: "batch", Kind: "at", Command: []string{"sh", "-c", "touch " + batchMarker}, Enabled: true, NextRun: now.Add(-time.Minute), BatchLoad: true, MailOutput: true, MailCompletion: true, MailTo: "recipient"},
		{ID: "normal", Kind: "at", Command: []string{"sh", "-c", "touch " + normalMarker}, Enabled: true, NextRun: now.Add(-time.Minute)},
	}
	if err := NewStore(state).SaveJobs(jobs); err != nil {
		t.Fatal(err)
	}
	deliveries := 0
	deliver := func(string, []byte) error { deliveries++; return nil }
	fired, err := TickOnceWithProviders(now, io.Discard, deliver, func() (float64, error) { return BatchLoadLimit + 1, nil })
	if err != nil || !reflect.DeepEqual(fired, []string{"normal"}) {
		t.Fatalf("high-load tick=(%v,%v)", fired, err)
	}
	if _, err := os.Stat(normalMarker); err != nil {
		t.Fatalf("normal job starved: %v", err)
	}
	if _, err := os.Stat(batchMarker); !os.IsNotExist(err) {
		t.Fatalf("batch ran under high load: %v", err)
	}
	fired, err = TickOnceWithProviders(now.Add(time.Second), io.Discard, deliver, func() (float64, error) { return BatchLoadLimit, nil })
	if err != nil || !reflect.DeepEqual(fired, []string{"batch"}) || deliveries != 1 {
		t.Fatalf("low-load tick=(%v,%v) deliveries=%d", fired, err, deliveries)
	}
	if _, err := os.Stat(batchMarker); err != nil {
		t.Fatalf("batch did not run at permitted load: %v", err)
	}
}

func TestUnavailableMailDoesNotStarveUnrelatedDueJob(t *testing.T) {
	withState(t)
	now := time.Now()
	marker := filepath.Join(t.TempDir(), "normal")
	if err := SaveJobs([]*Job{
		{ID: "needs-mail", Kind: "at", Command: []string{"sh", "-c", "exit 99"}, Enabled: true, NextRun: now.Add(-time.Minute), BatchLoad: true, MailOutput: true, MailCompletion: true, MailTo: "recipient"},
		{ID: "normal", Kind: "at", Command: []string{"sh", "-c", "touch " + marker}, Enabled: true, NextRun: now.Add(-time.Minute)},
	}); err != nil {
		t.Fatal(err)
	}
	fired, err := TickOnceWithProviders(now, io.Discard, nil, func() (float64, error) { return 0, nil })
	if !errors.Is(err, ErrMailDeliveryUnsupported) || !reflect.DeepEqual(fired, []string{"needs-mail", "normal"}) {
		t.Fatalf("tick=(%v,%v)", fired, err)
	}
	if _, statErr := os.Stat(marker); statErr != nil {
		t.Fatalf("unrelated job was starved: %v", statErr)
	}
	loaded, _ := LoadJobs()
	if mail := loaded[0]; mail.Enabled || mail.LastRun.IsZero() {
		t.Fatalf("executed undeliverable job was not claimed: %+v", mail)
	}
}

func TestLocalMailDeliveryUsesSubmittedSpool(t *testing.T) {
	dir := t.TempDir()
	spool := filepath.Join(dir, "spool")
	deliver, err := DiscoverLocalMailDelivery([]string{
		"LOGNAME=alice",
		"MAILX_SPOOL=" + spool,
		"MAIL=" + filepath.Join(spool, "alice"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := deliver("alice", []byte("completed\n")); err != nil {
		t.Fatal(err)
	}
	entries, err := mailxpkg.ReadMbox(filepath.Join(spool, "alice"))
	if err != nil || len(entries) != 1 || entries[0].Message == nil || !bytes.Contains(entries[0].Message.Body, []byte("completed")) {
		t.Fatalf("local completion mail entries=%v err=%v", entries, err)
	}
	if err := deliver("../escape", []byte("bad")); err == nil {
		t.Fatal("unsafe local recipient accepted")
	}
}

func TestSendmailDeliveryDiscoveryAndInvocation(t *testing.T) {
	if runtime.GOOS == "windows" || runtime.GOOS == "js" {
		t.Skip("requires a POSIX executable script")
	}
	dir := t.TempDir()
	capture := filepath.Join(dir, "message")
	sendmail := filepath.Join(dir, "sendmail")
	if err := os.WriteFile(sendmail, []byte("#!/bin/sh\n/bin/cat > \"$SENDMAIL_CAPTURE\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("SENDMAIL_CAPTURE", capture)
	deliver, err := DiscoverMailDelivery()
	if err != nil {
		t.Fatal(err)
	}
	if err := deliver("trusted-user", []byte("job output\n")); err != nil {
		t.Fatal(err)
	}
	message, err := os.ReadFile(capture)
	if err != nil || !strings.Contains(string(message), "To: trusted-user\n") || !strings.HasSuffix(string(message), "\n\njob output\n") {
		t.Fatalf("message=%q err=%v", message, err)
	}
}

func TestStatePathForUsesOnlyInvocationContext(t *testing.T) {
	dir := t.TempDir()
	if got, want := StatePathFor(dir, []string{"BASHY_SCHEDULE_STATE=relative/state.json"}), filepath.Join(dir, "relative", "state.json"); got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if got, want := StatePathFor(dir, []string{"HOME=invocation-home"}), filepath.Join(dir, "invocation-home", ".config", "bashy", "schedule.json"); got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	abs := filepath.Join(t.TempDir(), "absolute.json")
	if got := StatePathFor("", []string{"BASHY_SCHEDULE_STATE=" + abs}); got != abs {
		t.Fatalf("absolute invocation state path = %q, want %q", got, abs)
	}
	for _, env := range [][]string{
		nil,
		{"BASHY_SCHEDULE_STATE=relative.json"},
		{"XDG_CONFIG_HOME=relative-config"},
		{"HOME=relative-home"},
	} {
		if got := StatePathFor("", env); got != "" {
			t.Errorf("StatePathFor without an invocation base = %q for env %q, want empty fail-closed path", got, env)
		}
	}
}
