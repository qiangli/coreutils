//go:build !windows

package crontabcmd

import (
	"context"
	"slices"
	"testing"

	"github.com/qiangli/coreutils/pkg/schedule"
)

func TestCrontabUsesBuildShellReference(t *testing.T) {
	setupCronState(t)
	shell, err := cronShellPath()
	if err != nil || shell != schedule.DefaultShellPath() {
		t.Fatalf("default shell=%q err=%v", shell, err)
	}
	_, stderr, code := runCron(t, context.Background(), "* * * * * printf ok\n")
	if code != 0 {
		t.Fatalf("install exit=%d: %s", code, stderr)
	}
	jobs, err := schedule.LoadJobs()
	if err != nil || len(jobs) != 1 {
		t.Fatalf("jobs=%v error=%v", jobs, err)
	}
	if !slices.Equal(jobs[0].Command, []string{shell, "-c", "printf ok"}) || !slices.Contains(jobs[0].Env, "SHELL="+shell) {
		t.Fatalf("shell reference lost: command=%q env=%q", jobs[0].Command, jobs[0].Env)
	}
}
