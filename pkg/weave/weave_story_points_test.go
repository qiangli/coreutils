package weave

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSprintLinkRequiresValidPointsBeforeStoryMutation(t *testing.T) {
	root := setupIsolationFixture(t)
	t.Chdir(root)
	if out, code := runSprint(t, "add", "bounded sprint", "--json"); code != 0 {
		t.Fatalf("sprint add failed (exit %d): %s", code, out)
	}
	if out, code := runWeave(t, "add", "candidate", "--json"); code != 0 {
		t.Fatalf("weave add failed (exit %d): %s", code, out)
	}
	repo := filepath.Base(root)
	assertUnlinked := func(stage string) {
		t.Helper()
		storyDir, err := sprintStoreDir()
		if err != nil {
			t.Fatal(err)
		}
		q, err := loadWeaveQueue(storyDir)
		if err != nil {
			t.Fatal(err)
		}
		s := findWeaveStory(q, 1)
		if s == nil || len(s.Runs) != 0 {
			t.Fatalf("%s mutated sprint links: %+v", stage, s)
		}
	}
	if err := validateSprintRunLink(repo, 99); err == nil || !strings.Contains(err.Error(), "no weave run #99") {
		t.Fatalf("missing-run validation: %v", err)
	}
	out, code := runSprint(t, "link", "1", "--repo", repo, "--task", "99")
	if code == 0 || !strings.Contains(out, "no weave run #99") {
		t.Fatalf("missing-run link exit=%d output=%q", code, out)
	}
	assertUnlinked("missing-run rejection")

	if err := validateSprintRunLink(repo, 1); err == nil || !strings.Contains(err.Error(), "no story points") {
		t.Fatalf("unpointed validation: %v", err)
	}
	out, code = runSprint(t, "link", "1", "--repo", repo, "--task", "1")
	if code == 0 || !strings.Contains(out, "no story points") {
		t.Fatalf("unpointed link exit=%d output=%q", code, out)
	}
	assertUnlinked("unpointed rejection")

	dir, _ := weaveQueueDir(root)
	if err := withWeaveQueueLock(dir, func(q *weaveQueue) error {
		findWeaveItem(q, 1).Points = 4 // simulate corrupt legacy state
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := validateSprintRunLink(repo, 1); err == nil || !strings.Contains(err.Error(), "invalid story points 4") {
		t.Fatalf("invalid-point validation: %v", err)
	}
	out, code = runSprint(t, "link", "1", "--repo", repo, "--task", "1")
	if code == 0 || !strings.Contains(out, "invalid story points 4") {
		t.Fatalf("invalid-point link exit=%d output=%q", code, out)
	}
	assertUnlinked("invalid-point rejection")

	if err := withWeaveQueueLock(dir, func(q *weaveQueue) error {
		findWeaveItem(q, 1).Points = 2
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if out, code := runSprint(t, "link", "1", "--repo", repo, "--task", "1"); code != 0 {
		t.Fatalf("valid pointed link failed (exit %d): %s", code, out)
	}
	storyDir, _ := sprintStoreDir()
	q, _ := loadWeaveQueue(storyDir)
	s := findWeaveStory(q, 1)
	if s == nil || len(s.Runs) != 1 || s.Runs[0] != (sprintRun{Repo: repo, ID: 1}) {
		t.Fatalf("valid link missing: %+v", s)
	}
}
