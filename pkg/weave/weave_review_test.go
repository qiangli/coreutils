package weave

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func weaveReviewFixture(t *testing.T, verify string, commitWork bool) (root, dir string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	root = weaveTestRepo(t)
	var err error
	dir, err = weaveQueueDir(root)
	if err != nil {
		t.Fatal(err)
	}
	workspace := filepath.Join(dir, "workspaces", "issue-1")
	if err := os.MkdirAll(filepath.Dir(workspace), 0o755); err != nil {
		t.Fatal(err)
	}
	weaveTestGit(t, root, "clone", "--local", "--no-hardlinks", root, workspace)
	weaveTestGit(t, workspace, "checkout", "-qb", "agent/weave-issue-1")
	if commitWork {
		if err := os.WriteFile(filepath.Join(workspace, "tracked.txt"), []byte("changed\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		weaveTestGit(t, workspace, "add", ".")
		weaveTestGit(t, workspace, "commit", "-qm", "agent work")
	}
	commitsAhead := 0
	if commitWork {
		commitsAhead = 1
	}
	q := &weaveQueue{
		NextID: 2,
		Root:   root,
		Items: []*weaveItem{{
			ID:            1,
			Title:         "review me",
			State:         "submitted",
			Workspace:     workspace,
			Branch:        "agent/weave-issue-1",
			Created:       time.Now().UTC(),
			CommitsAhead:  commitsAhead,
			VerifyCommand: verify,
		}},
	}
	if err := saveWeaveQueue(dir, q); err != nil {
		t.Fatal(err)
	}
	return root, dir
}

func runReviewInRoot(t *testing.T, root string) {
	t.Helper()
	oldWD, _ := os.Getwd()
	defer func() { _ = os.Chdir(oldWD) }()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	cmd := &cobra.Command{}
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := runWeaveReview(cmd, 1, &weaveOutputFlags{}); err != nil {
		t.Fatalf("review: %v out=%s", err, buf.String())
	}
}

func TestRunWeaveReviewPassingVerifyPersistsPass(t *testing.T) {
	root, dir := weaveReviewFixture(t, "true", true)
	runReviewInRoot(t, root)

	q, err := loadWeaveQueue(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := findWeaveItem(q, 1)
	if got == nil {
		t.Fatal("issue disappeared")
	}
	if got.ReviewVerdict != "pass" || got.ReviewBlocking || got.ReviewExit != 0 || got.ReviewBy != "weave review" || got.ReviewAt.IsZero() {
		t.Fatalf("reviewed item = %+v, want passing persisted review", got)
	}
}

func TestRunWeaveReviewFailingVerifyPersistsBlocked(t *testing.T) {
	root, dir := weaveReviewFixture(t, "false", true)
	runReviewInRoot(t, root)

	q, err := loadWeaveQueue(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := findWeaveItem(q, 1)
	if got == nil {
		t.Fatal("issue disappeared")
	}
	if got.ReviewVerdict != "blocked" || !got.ReviewBlocking || got.ReviewExit == 0 {
		t.Fatalf("reviewed item = %+v, want blocked failing review", got)
	}
}

func TestRunWeaveReviewNoCommitsPersistsBlocked(t *testing.T) {
	root, dir := weaveReviewFixture(t, "true", false)
	runReviewInRoot(t, root)

	q, err := loadWeaveQueue(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := findWeaveItem(q, 1)
	if got == nil {
		t.Fatal("issue disappeared")
	}
	if got.ReviewVerdict != "blocked" || !got.ReviewBlocking || got.ReviewExit != 0 {
		t.Fatalf("reviewed item = %+v, want blocked no-commits review", got)
	}
}

func TestRequireReviewGateBlocksUnreviewedAndBlockedAllowsPass(t *testing.T) {
	for _, tc := range []struct {
		name    string
		item    *weaveItem
		wantErr bool
	}{
		{name: "unreviewed", item: &weaveItem{ID: 1}, wantErr: true},
		{name: "blocked", item: &weaveItem{ID: 1, ReviewVerdict: "blocked", ReviewBlocking: true}, wantErr: true},
		{name: "pass", item: &weaveItem{ID: 1, ReviewVerdict: "pass"}, wantErr: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := weaveRequireReviewGate(tc.item)
			if (err != nil) != tc.wantErr {
				t.Fatalf("weaveRequireReviewGate() err=%v wantErr=%v", err, tc.wantErr)
			}
		})
	}
}
