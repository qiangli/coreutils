package atrmcmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/pkg/schedule"
	"github.com/qiangli/coreutils/tool"
)

func runATRM(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   t.TempDir(),
		Env:   []string{"BASHY_SCHEDULE_STATE=" + os.Getenv("BASHY_SCHEDULE_STATE")},
		Stdio: tool.Stdio{Out: &out, Err: &errb, In: strings.NewReader("")},
	}
	code := cmd.Run(rc, args)
	return out.String(), errb.String(), code
}

func TestATRMRemovesByIDAndName(t *testing.T) {
	t.Setenv("BASHY_SCHEDULE_STATE", filepath.Join(t.TempDir(), "schedule.json"))
	err := schedule.UpdateJobs(func([]*schedule.Job) ([]*schedule.Job, error) {
		return []*schedule.Job{{ID: "one", Name: "first"}, {ID: "two", Name: "second"}, {ID: "keep"}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	out, errb, code := runATRM(t, "one", "second")
	if code != 0 || out != "" || errb != "" {
		t.Fatalf("atrm: code=%d stdout=%q stderr=%q", code, out, errb)
	}
	jobs, err := schedule.LoadJobs()
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].ID != "keep" {
		t.Fatalf("remaining jobs=%+v, want only keep", jobs)
	}
}

func TestATRMMissingOperandAndUnknownJob(t *testing.T) {
	t.Setenv("BASHY_SCHEDULE_STATE", filepath.Join(t.TempDir(), "schedule.json"))
	_, errb, code := runATRM(t)
	if code != 2 || !strings.Contains(errb, "missing job ID") {
		t.Fatalf("atrm no operand: code=%d stderr=%q", code, errb)
	}
	out, errb, code := runATRM(t, "missing")
	if code != 0 || out != "" || !strings.Contains(errb, `no job "missing"`) {
		t.Fatalf("atrm missing job: code=%d stdout=%q stderr=%q", code, out, errb)
	}
}
