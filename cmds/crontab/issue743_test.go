//go:build !windows

package crontabcmd

import (
	"os"
	"testing"

	"github.com/qiangli/coreutils/pkg/schedule"
)

func TestIssue743BackslashOnlyEscapesPercent(t *testing.T) {
	setupCronState(t)
	// Backslash escapes only a following percent-sign. Before any other byte
	// it is command data (printf 'a\tb\n' is a canonical crontab command) and
	// must reach the shell unchanged, in both the command and stdin portions.
	source := "0 1 * * * printf 'a\\tb\\n' \\% x\\y%data\\%keep%z\n"
	rc, _, errOut := cronTestContext(t, source)
	if code := runWithConfig(rc, nil, allowedConfig(rc.Dir)); code != 0 {
		t.Fatalf("install code=%d stderr=%q", code, errOut)
	}
	jobs, err := schedule.NewStore(os.Getenv("BASHY_SCHEDULE_STATE")).LoadJobs()
	if err != nil || len(jobs) != 1 {
		t.Fatalf("jobs=%v err=%v", jobs, err)
	}
	j := jobs[0]
	if got, want := j.Command[2], "printf 'a\\tb\\n' % x\\y"; got != want {
		t.Errorf("command=%q, want %q", got, want)
	}
	if got, want := j.Stdin, "data%keep\nz\n"; got != want {
		t.Errorf("stdin=%q, want %q", got, want)
	}
}

func TestIssue743TrailingBackslashIsLiteral(t *testing.T) {
	command, stdin := splitCronCommand(`echo tail\`)
	if command != `echo tail\` || stdin != "" {
		t.Fatalf("command=%q stdin=%q", command, stdin)
	}
	// A doubled backslash before a percent-sign: the left-to-right scan emits
	// the first backslash literally and the second escapes the percent.
	command, stdin = splitCronCommand(`echo a\\%b`)
	if command != `echo a\%b` || stdin != "" {
		t.Fatalf("command=%q stdin=%q", command, stdin)
	}
}
