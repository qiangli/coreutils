//go:build unix

package batchcmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/pkg/schedule"
)

func setBatchAccessDirs(t *testing.T, dirs ...string) {
	t.Helper()
	old := batchAccessDirs
	batchAccessDirs = dirs
	t.Cleanup(func() { batchAccessDirs = old })
}

func TestIssue743BatchAccessMalformedDenyFailsClosed(t *testing.T) {
	setupBatchState(t)
	dir := t.TempDir()
	// The user is not listed, but the malformed whitespace-bearing line makes
	// the policy ambiguous: batch must deny instead of scheduling.
	if err := os.WriteFile(filepath.Join(dir, "at.deny"), []byte("someone else\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	setBatchAccessDirs(t, dir)
	_, stderr, code := runBatch(t, context.Background(), "true\n")
	if code != 1 || !strings.Contains(stderr, "not authorized") {
		t.Fatalf("malformed deny: code=%d stderr=%q", code, stderr)
	}
	if jobs, err := schedule.LoadJobs(); err != nil || len(jobs) != 0 {
		t.Fatalf("denied submission scheduled jobs=%v err=%v", jobs, err)
	}
}

func TestIssue743BatchAccessEmptyDenyPermits(t *testing.T) {
	setupBatchState(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "at.deny"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	setBatchAccessDirs(t, dir)
	_, stderr, code := runBatch(t, context.Background(), "true\n")
	if code != 0 {
		t.Fatalf("empty deny must permit submission: code=%d stderr=%q", code, stderr)
	}
	if jobs, err := schedule.LoadJobs(); err != nil || len(jobs) != 1 {
		t.Fatalf("jobs=%v err=%v", jobs, err)
	}
}
