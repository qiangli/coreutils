package atcmd

import (
	"context"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/pkg/schedule"
)

func TestIssue743TouchTimePastDiagnosticNamesArgument(t *testing.T) {
	setupATState(t)
	_, stderr, code := runAT(t, context.Background(), "true\n", "-t", "200001010000")
	if code != 2 {
		t.Fatalf("past -t time: code=%d stderr=%q", code, stderr)
	}
	if !strings.Contains(stderr, `time "200001010000" is in the past`) {
		t.Fatalf("past -t diagnostic must name the -t argument: %q", stderr)
	}
	if jobs, err := schedule.LoadJobs(); err != nil || len(jobs) != 0 {
		t.Fatalf("past -t time scheduled jobs=%v err=%v", jobs, err)
	}
}

func TestIssue743ListUnknownJobIDFails(t *testing.T) {
	setupATState(t)
	if _, stderr, code := runAT(t, context.Background(), "true\n", "now", "+", "1", "hour"); code != 0 {
		t.Fatalf("submit: code=%d stderr=%q", code, stderr)
	}
	jobs, err := schedule.LoadJobs()
	if err != nil || len(jobs) != 1 {
		t.Fatalf("jobs=%v err=%v", jobs, err)
	}
	id := jobs[0].ID

	out, stderr, code := runATNoStdin(t, context.Background(), "-l", id, "no-such-job")
	if code != 1 {
		t.Fatalf("-l with unknown id: code=%d stdout=%q stderr=%q", code, out, stderr)
	}
	if !strings.Contains(out, id+"\t") {
		t.Errorf("existing job must still be listed: stdout=%q", out)
	}
	if !strings.Contains(stderr, `no job "no-such-job"`) {
		t.Errorf("unknown id must be diagnosed: stderr=%q", stderr)
	}

	out, stderr, code = runATNoStdin(t, context.Background(), "-l", "ghost")
	if code != 1 || out != "" || !strings.Contains(stderr, `no job "ghost"`) {
		t.Fatalf("-l ghost: code=%d stdout=%q stderr=%q", code, out, stderr)
	}

	// A known id alone still lists successfully.
	out, stderr, code = runATNoStdin(t, context.Background(), "-l", id)
	if code != 0 || stderr != "" || !strings.Contains(out, id+"\t") {
		t.Fatalf("-l known id: code=%d stdout=%q stderr=%q", code, out, stderr)
	}
}
