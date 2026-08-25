//go:build windows

package crontabcmd

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/pkg/schedule"
	"github.com/qiangli/coreutils/tool"
)

func TestWindowsInstallFailsClosedWithoutMutation(t *testing.T) {
	state := filepath.Join(t.TempDir(), "schedule.json")
	store := schedule.NewStore(state)
	if err := store.SaveJobs([]*schedule.Job{{ID: "at-keep", Kind: "at"}}); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: t.TempDir(), Env: []string{"BASHY_SCHEDULE_STATE=" + state}, Stdio: tool.Stdio{In: strings.NewReader("* * * * * echo no\n"), Out: &out, Err: &errOut}}
	cfg := runConfig{currentUser: func() (cronIdentity, error) { return cronIdentity{name: "user", home: rc.Dir}, nil }, euid: func() int { return 0 }}
	if code := runWithConfig(rc, nil, cfg); code != 1 || !strings.Contains(errOut.String(), "unsupported on Windows") {
		t.Fatalf("code=%d stderr=%q", code, errOut.String())
	}
	jobs, err := store.LoadJobs()
	if err != nil || len(jobs) != 1 || jobs[0].ID != "at-keep" {
		t.Fatalf("store mutated: jobs=%+v err=%v", jobs, err)
	}
}
