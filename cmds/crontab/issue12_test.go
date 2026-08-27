//go:build !windows

package crontabcmd

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/coreutils/pkg/schedule"
)

// POSIX crontab INPUT FILES: when both the day-of-month and day-of-week
// fields are restricted (neither is an asterisk), a day matching EITHER field
// matches; when only one is restricted, only that field governs. Day of week
// 0 is Sunday, and 7 is not a valid value. These mandatory semantics are
// delivered by the schedule engine's cron parser, so pin them here where the
// crontab applet depends on them.
func TestIssue12DayOfMonthDayOfWeekEitherMatch(t *testing.T) {
	setupCronState(t)
	if _, stderr, code := runCron(t, context.Background(), "0 0 1 * 1 echo either\n"); code != 0 {
		t.Fatalf("install: code=%d stderr=%q", code, stderr)
	}
	jobs, err := schedule.LoadJobs()
	if err != nil || len(jobs) != 1 {
		t.Fatalf("jobs=%v err=%v", jobs, err)
	}
	job := jobs[0]

	// From Wednesday 2026-03-11, "0 0 1 * 1" must fire on Monday 2026-03-16,
	// not wait for the 1st of April: both day fields are restricted, so either
	// may match.
	wednesday := time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC)
	next, err := schedule.ComputeNext(job, wednesday)
	if err != nil || !next.Equal(time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("either-match next=%v err=%v; want 2026-03-16 00:00 UTC", next, err)
	}
	// From Monday 2026-03-16 (after the midnight firing), the next match is
	// Monday 2026-03-23 — again the day-of-week arm, not the day-of-month arm.
	monday := time.Date(2026, 3, 16, 0, 0, 1, 0, time.UTC)
	next, err = schedule.ComputeNext(job, monday)
	if err != nil || !next.Equal(time.Date(2026, 3, 23, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("either-match next=%v err=%v; want 2026-03-23 00:00 UTC", next, err)
	}
	// From Tuesday 2026-06-30, the day-of-month arm wins: Wednesday July 1st.
	juneEnd := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	next, err = schedule.ComputeNext(job, juneEnd)
	if err != nil || !next.Equal(time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("either-match next=%v err=%v; want 2026-07-01 00:00 UTC", next, err)
	}

	// A restricted day-of-week with an unrestricted day-of-month governs
	// alone, and Sunday is day 0.
	if _, stderr, code := runCron(t, context.Background(), "0 0 * * 0 echo sunday\n"); code != 0 {
		t.Fatalf("reinstall: code=%d stderr=%q", code, stderr)
	}
	jobs, err = schedule.LoadJobs()
	if err != nil || len(jobs) != 1 {
		t.Fatalf("jobs=%v err=%v", jobs, err)
	}
	next, err = schedule.ComputeNext(jobs[0], wednesday)
	if err != nil || !next.Equal(time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("sunday next=%v err=%v; want 2026-03-15 00:00 UTC", next, err)
	}
}

// POSIX defines the field ranges as minute [0,59], hour [0,23], day of month
// [1,31], month [1,12], day of week [0,6]. Out-of-range values must reject
// the whole table without changing the stored crontab.
func TestIssue12FieldRangeViolationsRejectAtomically(t *testing.T) {
	setupCronState(t)
	if _, stderr, code := runCron(t, context.Background(), "0 0 * * * echo keep\n"); code != 0 {
		t.Fatalf("baseline install: code=%d stderr=%q", code, stderr)
	}
	for _, table := range []string{
		"60 0 * * * echo bad-minute\n",
		"0 24 * * * echo bad-hour\n",
		"0 0 0 * * echo bad-dom\n",
		"0 0 32 * * echo bad-dom\n",
		"0 0 * 13 * echo bad-month\n",
		"0 0 * * 7 echo bad-dow\n",
	} {
		_, stderr, code := runCron(t, context.Background(), table)
		if code != 1 {
			t.Errorf("table %q: code=%d stderr=%q; want 1", table, code, stderr)
		}
	}
	out, stderr, code := runCronNoStdin(t, context.Background(), "-l")
	if code != 0 || !strings.Contains(out, "echo keep") {
		t.Fatalf("baseline table must be untouched: code=%d out=%q stderr=%q", code, out, stderr)
	}
}
