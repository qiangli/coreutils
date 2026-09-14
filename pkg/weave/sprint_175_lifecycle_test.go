package weave

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/coreutils/pkg/issue"
	todopkg "github.com/qiangli/coreutils/pkg/todo"
)

func TestSprintSubmitRefusesUnclaimedStoryWithoutMutation(t *testing.T) {
	home := t.TempDir()
	repo := t.TempDir()
	t.Setenv("BASHY_SPRINT_DIR", home)
	t.Setenv("WEAVE_CONDUCTOR", "worker")
	st := todopkg.RepoStore(repo)
	it, err := todopkg.Add(st, "deliver", "", "p0", nil, "", "")
	if err != nil {
		t.Fatal(err)
	}
	it.Sprint = 1
	if _, err := st.Save(it); err != nil {
		t.Fatal(err)
	}
	q := &weaveQueue{NextStoryID: 2, Stories: []*weaveStory{{ID: 1, Title: "neutral", PrimaryGoal: "deliver", SpecRef: "docs/plan.md", Column: "doing", StoryRoots: []string{repo}, Created: time.Now().UTC()}}}
	if err := saveWeaveQueue(home, q); err != nil {
		t.Fatal(err)
	}

	cmd := NewSprintCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"submit", "1", it.ID, "--repo", repo, "--as", "worker", "-m", "commit abc; tests pass"})
	if err := cmd.Execute(); err == nil || !strings.Contains(out.String(), "sprint claim 1") {
		t.Fatalf("unclaimed submit error=%v output=%q", err, out.String())
	}
	got, _ := todopkg.ResolveRef(st, it.ID)
	if got.Assignee != "" || got.Status != todopkg.StatusTodo {
		t.Fatalf("refusal mutated story: %+v", got)
	}
}

func TestSprintStoryAuditRejectsClosedStoryWithoutAcceptanceEvidence(t *testing.T) {
	repo := t.TempDir()
	st := todopkg.RepoStore(repo)
	now := time.Now().UTC()
	it := &issue.Issue{ID: issue.NewID(), Kind: issue.KindTask, Title: "edited closure", Status: todopkg.StatusDone, Sprint: 9, Assignee: "worker", Closed: &now, Created: now}
	if _, err := st.Save(it); err != nil {
		t.Fatal(err)
	}
	s := &weaveStory{ID: 9, StoryRoots: []string{repo}}
	if err := sprintStoryAcceptanceAudit(s); err == nil || !strings.Contains(err.Error(), "acceptance evidence") {
		t.Fatalf("audit error = %v", err)
	}
}

func TestSprintManagerAcceptIsTheOnlyClosePath(t *testing.T) {
	home := t.TempDir()
	repo := t.TempDir()
	t.Setenv("BASHY_SPRINT_DIR", home)
	t.Setenv("WEAVE_CONDUCTOR", "manager")
	st := todopkg.RepoStore(repo)
	it, err := todopkg.Add(st, "delivered", "", "p0", nil, "", "")
	if err != nil {
		t.Fatal(err)
	}
	it.Sprint = 1
	it.Assignee = "worker"
	it.Status = todopkg.StatusAssigned
	if _, err := st.Save(it); err != nil {
		t.Fatal(err)
	}
	s := &weaveStory{ID: 1, Title: "neutral", PrimaryGoal: "deliver", SpecRef: "docs/plan.md", Column: "doing", Owner: "manager", Lease: &weaveStoryLease{Holder: "manager", At: time.Now().UTC()}, StoryRoots: []string{repo}, Created: time.Now().UTC()}
	weaveStoryAppend(s, "worker", "decision", "worker submitted story "+shortSprintStoryID(it.ID)+" for review/merge: commit abc; tests pass")
	if err := saveWeaveQueue(home, &weaveQueue{NextStoryID: 2, Stories: []*weaveStory{s}}); err != nil {
		t.Fatal(err)
	}

	cmd := NewSprintCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"accept", "1", it.ID, "--repo", repo, "-m", "independent gate passed"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("accept: %v\n%s", err, out.String())
	}
	got, _ := todopkg.ResolveRef(st, it.ID)
	if got.Status != todopkg.StatusDone || got.Closed == nil || got.ClosedBy != "manager" {
		t.Fatalf("accepted story = %+v", got)
	}
	q, err := loadWeaveQueue(home)
	if err != nil {
		t.Fatal(err)
	}
	if !sprintAcceptanceEvidence(q.Stories[0], it.ID) {
		t.Fatal("manager acceptance evidence was not persisted")
	}
}
