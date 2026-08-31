package weave

import (
	"testing"

	todopkg "github.com/qiangli/coreutils/pkg/todo"
)

func sprintTestStory(t *testing.T, root string, sprint int64, title, priority, status string) string {
	t.Helper()
	st := todopkg.RepoStore(root)
	it, err := todopkg.Add(st, title, "", priority, nil, "", "")
	if err != nil {
		t.Fatal(err)
	}
	it.Sprint = sprint
	it.Status = status
	if _, err := st.Save(it); err != nil {
		t.Fatal(err)
	}
	return it.ID
}

func TestSprintNextDerivesPriorityFromStories(t *testing.T) {
	root := t.TempDir()
	s := &weaveStory{ID: 99123, StoryRoots: []string{root}}
	low := sprintTestStory(t, root, s.ID, "low", "p2", todopkg.StatusTodo)
	high := sprintTestStory(t, root, s.ID, "high", "p0", todopkg.StatusTodo)
	next, err := nextSprintStory(s)
	if err != nil {
		t.Fatal(err)
	}
	if next == nil || next.Ref.ID != high {
		t.Fatalf("next = %#v, want p0 %s ahead of p2 %s", next, high, low)
	}
	// Priority remains authoritative on the story: editing it changes the index
	// without rewriting the sprint card.
	st := todopkg.RepoStore(root)
	it, _ := todopkg.ResolveRef(st, low)
	it.Priority = "p0"
	if _, err := st.Save(it); err != nil {
		t.Fatal(err)
	}
	next, err = nextSprintStory(s)
	if err != nil || next == nil || next.Ref.ID != low {
		t.Fatalf("next after priority edit = %#v, err=%v, want %s", next, err, low)
	}
}

func TestSprintGoalCompletionFollowsStoryClosureAndReopen(t *testing.T) {
	root := t.TempDir()
	s := &weaveStory{ID: 99124, StoryRoots: []string{root}}
	id := sprintTestStory(t, root, s.ID, "deliver", "p0", todopkg.StatusTodo)
	g := sprintGoalItem{ID: "delivery", Text: "deliver it", Stories: []sprintStoryRef{{Repo: root, ID: id}}}
	if sprintGoalDone(g) {
		t.Fatal("open story checked the goal")
	}
	st := todopkg.RepoStore(root)
	if _, err := todopkg.SetStatus(st, id, todopkg.StatusDone); err != nil {
		t.Fatal(err)
	}
	if !sprintGoalDone(g) {
		t.Fatal("closed story did not check the goal")
	}
	if _, err := todopkg.SetStatus(st, id, todopkg.StatusTodo); err != nil {
		t.Fatal(err)
	}
	if sprintGoalDone(g) {
		t.Fatal("reopened story did not uncheck the goal")
	}
}

func TestSprintGateEvidenceAndTakeoverIdentity(t *testing.T) {
	root := t.TempDir()
	id := sprintTestStory(t, root, 99125, "gated", "p0", todopkg.StatusDone)
	g := sprintGoalItem{ID: "gate", Stories: []sprintStoryRef{{Repo: root, ID: id}}, GateRequired: true}
	if sprintGoalDone(g) {
		t.Fatal("gate-required goal checked without evidence")
	}
	g.Evidence = "go test ./... PASS"
	if !sprintGoalDone(g) {
		t.Fatal("closed story plus evidence did not check goal")
	}
	s := &weaveStory{Lease: &weaveStoryLease{Holder: "established-owner"}}
	if got := sprintTakeoverIdentity(s, ""); got != "established-owner" {
		t.Fatalf("implicit takeover identity = %q", got)
	}
	if got := sprintTakeoverIdentity(s, "preferred-owner"); got != "preferred-owner" {
		t.Fatalf("explicit rename = %q", got)
	}
}
