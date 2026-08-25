//go:build !windows

package batchcmd

import (
	"context"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/pkg/schedule"
)

func allowBatchForTest(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	current, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "at.allow"), []byte(current.Username+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := batchAccessDirs
	batchAccessDirs = []string{dir}
	t.Cleanup(func() { batchAccessDirs = old })
}

func TestBatchAccessDenyRejectsWithoutScheduling(t *testing.T) {
	state := t.TempDir() + "/schedule.json"
	t.Setenv("BASHY_SCHEDULE_STATE", state)
	current, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	old := batchAccessDirs
	batchAccessDirs = []string{dir}
	t.Cleanup(func() { batchAccessDirs = old })
	if err := os.WriteFile(filepath.Join(dir, "at.deny"), []byte(current.Username+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, stderr, code := runBatch(t, context.Background(), "true\n")
	if code != 1 || !strings.Contains(stderr, "not authorized") {
		t.Fatalf("deny: code=%d stderr=%q", code, stderr)
	}
	if jobs, err := schedule.LoadJobs(); err != nil || len(jobs) != 0 {
		t.Fatalf("denied submission scheduled jobs=%v err=%v", jobs, err)
	}
}
