package atcmd

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/coreutils/pkg/schedule"
)

// POSIX writes the year date form as month_name day_number "," year_number.
// The comma is its own grammar terminal: historical at tokenizes punctuation
// separately, so it may arrive attached to the day, to the year, to both, or
// blank-separated on either side.
func TestIssue12CommaAdjacentYearForms(t *testing.T) {
	now := time.Date(2026, time.June, 1, 12, 30, 45, 0, time.UTC) // Monday
	want := time.Date(2035, 1, 5, 10, 15, 0, 0, time.UTC)
	for _, input := range []string{
		"10:15 Jan 5, 2035",
		"10:15 Jan 5,2035",
		"10:15 Jan 5 , 2035",
		"10:15 Jan 5 ,2035",
		"10:15 Jan5,2035",
		"10:15 Jan 5 2035",
	} {
		got, err := schedule.ParseAtTimespecInLocation(input, now, time.UTC)
		if err != nil || !got.Equal(want) {
			t.Errorf("%q = %v, %v; want %v", input, got, err, want)
		}
	}
	// A comma with no following year_number violates the grammar, as does
	// trailing garbage after the year.
	for _, input := range []string{
		"10:15 Jan 5,",
		"10:15 Jan 5 ,",
		"10:15 Jan 5, 2035,",
		"10:15 Jan 5, 2035 extra",
	} {
		if got, err := schedule.ParseAtTimespecInLocation(input, now, time.UTC); err == nil {
			t.Errorf("invalid %q parsed as %v", input, got)
		}
	}
}

func TestIssue12CommaYearSubmission(t *testing.T) {
	setupATState(t)
	_, stderr, code := runAT(t, context.Background(), "true\n", "noon", "jan", "5,2036")
	if code != 0 {
		t.Fatalf("submit: code=%d stderr=%q", code, stderr)
	}
	if !strings.Contains(stderr, "job ") {
		t.Fatalf("submission confirmation missing: %q", stderr)
	}
	jobs, err := schedule.LoadJobs()
	if err != nil || len(jobs) != 1 {
		t.Fatalf("jobs=%v err=%v", jobs, err)
	}
	want := time.Date(2036, 1, 5, 12, 0, 0, 0, time.Local)
	if !jobs[0].NextRun.Equal(want) {
		t.Errorf("NextRun=%v want %v", jobs[0].NextRun, want)
	}
}
