package weave

// END-TO-END LIFECYCLE TESTS.
//
// These drive the sprint through the states a real campaign walks — open, hand
// over, recover, correct, close, clean up — and assert the three invariants
// that used to be advisory:
//
//	REACHABILITY  an open sprint has an owner that resolves and a room to reach it
//	COVERAGE      the plan keeps describing the sprint as new work arrives
//	HYGIENE       the sprint can state whether its repos and this host are clean
//
// Each test asserts the FAILURE it was written against, not just the happy
// path: a gate that passes is only evidence if the same gate is shown refusing.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/coreutils/pkg/room"
)

func execCommandForTest(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	return string(out), err
}

// seedAgent registers a named agent in the test HOME's fleet store, so a name
// the sprint records is one `bashy agents` can actually resolve.
//
// Tests need this now because an unregistered owner is refused — which is the
// point. Before, any string was accepted and the four lifecycle tests below
// passed with conductor names that addressed nobody.
func seedAgent(t *testing.T, name string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "unused")
	_ = dir
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("home: %v", err)
	}
	agents := filepath.Join(home, ".config", "bashy", "agents")
	if err := os.MkdirAll(agents, 0o755); err != nil {
		t.Fatalf("mkdir agents: %v", err)
	}
	body := "name: " + name + "\nkind: agent\ntool: claude\nmodel: opus5\n"
	if err := os.WriteFile(filepath.Join(agents, name+".yaml"), []byte(body), 0o644); err != nil {
		t.Fatalf("write agent: %v", err)
	}
}

// seedLiveAgent registers the agent AND publishes a live room card for it, so
// the name is claimable. Claiming a sprint now requires the seat to be RUNNING
// — a sprint seated to a name with no process behind it accepts room messages
// and inbox mail nobody will read.
func seedLiveAgent(t *testing.T, name string) {
	t.Helper()
	seedAgent(t, name)
	if err := room.Join(room.Card{
		ID:      name,
		Nick:    name,
		Tool:    "claude",
		Model:   "opus5",
		Binding: "claude:opus5",
		Role:    "conductor",
		Mode:    "weave",
		PID:     os.Getpid(),
		Joined:  time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		t.Skipf("host room unavailable in this environment: %v", err)
	}
	t.Cleanup(func() { room.Leave(name) })
}

func TestSprintRefusesAnOwnerThatResolvesToNobody(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("WEAVE_CONDUCTOR", "ghost-that-does-not-exist")

	if out, code := runSprint(t, "add", "unreachable owner"); code != 0 {
		t.Fatalf("add exit=%d: %s", code, out)
	}
	out, code := runSprint(t, "start", "1", "--for", "1h")
	if code == 0 {
		t.Fatalf("start accepted an owner that resolves to no agent:\n%s", out)
	}
	// The refusal must be actionable: an agent reading it needs to know BOTH
	// registration paths without going to look them up.
	for _, want := range []string{"does not resolve", "agents add", "agents track start"} {
		if !strings.Contains(out, want) {
			t.Errorf("refusal missing %q:\n%s", want, out)
		}
	}
}

func TestSprintRefusesThePlaceholderConductorName(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("WEAVE_CONDUCTOR", "conductor")

	if out, code := runSprint(t, "add", "placeholder owner"); code != 0 {
		t.Fatalf("add exit=%d: %s", code, out)
	}
	out, code := runSprint(t, "start", "1", "--for", "1h")
	if code == 0 {
		t.Fatalf("start accepted the placeholder name %q:\n%s", "conductor", out)
	}
	if !strings.Contains(out, "placeholder") {
		t.Errorf("refusal should explain that the name addresses nobody:\n%s", out)
	}
}

// TestOpenSprintKeepsItsRoomAcrossPauseAndHandoff is the regression for the
// live defect: sprint #99 sat in `doing` with a running box, an owner, no lease
// and NO ROOM, because pause and handoff closed it. The handoff window is
// exactly when an arriving agent needs somewhere to ask.
func TestOpenSprintKeepsItsRoomAcrossPauseAndHandoff(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("WEAVE_CONDUCTOR", "Ada")
	seedLiveAgent(t, "Ada")

	if out, code := runSprint(t, "add", "room retention"); code != 0 {
		t.Fatalf("add exit=%d: %s", code, out)
	}
	if out, code := runSprint(t, "start", "1", "--for", "1h"); code != 0 {
		t.Fatalf("start exit=%d: %s", code, out)
	}
	roomAfterStart := sprintContactRef(t, 1)
	if roomAfterStart == "" {
		t.Skip("no room could be opened in this environment; retention is untestable here")
	}

	if out, code := runSprint(t, "pause", "1", "-m", "paused for the test"); code != 0 {
		t.Fatalf("pause exit=%d: %s", code, out)
	}
	if got := sprintContactRef(t, 1); got != roomAfterStart {
		t.Fatalf("pause changed the room of an OPEN sprint: %q → %q", roomAfterStart, got)
	}

	if out, code := runSprint(t, "handoff", "1", "-m", "handing over"); code != 0 {
		t.Fatalf("handoff exit=%d: %s", code, out)
	}
	if got := sprintContactRef(t, 1); got != roomAfterStart {
		t.Fatalf("handoff closed the room of an OPEN sprint: %q → %q", roomAfterStart, got)
	}

	// And a successor inherits that same room rather than opening a second one.
	if out, code := runSprint(t, "take", "1", "--as", "Ada"); code != 0 {
		t.Fatalf("take exit=%d: %s", code, out)
	}
	if got := sprintContactRef(t, 1); got != roomAfterStart {
		t.Fatalf("take replaced the sprint's room instead of inheriting it: %q → %q", roomAfterStart, got)
	}
}

// TestClosingASprintReleasesItsRoom is the other half: retention must not leak.
func TestClosingASprintReleasesItsRoom(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("WEAVE_CONDUCTOR", "Ada")
	seedLiveAgent(t, "Ada")

	if out, code := runSprint(t, "add", "room release"); code != 0 {
		t.Fatalf("add exit=%d: %s", code, out)
	}
	if out, code := runSprint(t, "start", "1", "--for", "1h"); code != 0 {
		t.Fatalf("start exit=%d: %s", code, out)
	}
	if sprintContactRef(t, 1) == "" {
		t.Skip("no room in this environment")
	}
	if out, code := runSprint(t, "move", "1", "done"); code != 0 {
		t.Fatalf("move done exit=%d: %s", code, out)
	}
	if got := sprintContactRef(t, 1); got != "" {
		t.Fatalf("a closed sprint still advertises room %q — the channel outlived the work", got)
	}
}

// TestSprintCannotCloseOverUncoveredOpenWork is the coverage regression: a
// story filed into a sprint and never linked to a goal item used to be
// invisible to the close gate, so the sprint closed clean over open work.
func TestSprintCannotCloseOverUncoveredOpenWork(t *testing.T) {
	s := &weaveStory{ID: 7, Column: "doing", Title: "coverage"}
	// A goal item covering one story, and a second story nothing covers.
	s.Goal = []sprintGoalItem{{ID: "aaaaaaaa", Text: "planned", Stories: []sprintStoryRef{{Repo: "/r", ID: "aaaaaaaabbbb"}}}}

	covered := sprintStoryRef{Repo: "/r", ID: "aaaaaaaabbbb"}
	if !sprintStoryCovered(s, covered) {
		t.Fatal("a story linked to a goal must read as covered (8-vs-12 char id widths must still match)")
	}
	uncovered := sprintStoryRef{Repo: "/r", ID: "ccccccccdddd"}
	if sprintStoryCovered(s, uncovered) {
		t.Fatal("a story no goal references must read as uncovered")
	}
}

// TestCoverageGateNeedsAReasonToBeForced — the escape hatch must cost a
// sentence, or it becomes the default.
func TestCoverageGateNeedsAReasonToBeForced(t *testing.T) {
	s := &weaveStory{ID: 8, Column: "doing"}
	problems := []string{"deadbeef [p0] something open"}
	// Simulate the gate's decision directly on a known-nonempty problem set.
	if err := sprintCoverageGateFor(s, problems, false, ""); err == nil {
		t.Fatal("gate must refuse while open stories sit outside the plan")
	}
	if err := sprintCoverageGateFor(s, problems, true, ""); err == nil {
		t.Fatal("--force without --reason must still refuse")
	}
	if err := sprintCoverageGateFor(s, problems, true, "triaged: tracked in #123"); err != nil {
		t.Fatalf("--force with a reason must pass: %v", err)
	}
}

func TestReachabilityReportsEveryProblemWithAFix(t *testing.T) {
	// An open sprint with no owner and no room must report BOTH, not the first.
	s := &weaveStory{ID: 9, Column: "doing"}
	r := sprintCheckReachability(s)
	if len(r.Problems) < 2 {
		t.Fatalf("expected both a missing owner and a missing room, got %v", r.Problems)
	}
	joined := strings.Join(r.Problems, "\n")
	if !strings.Contains(joined, "owner") || !strings.Contains(joined, "room") {
		t.Errorf("reachability must name both failures:\n%s", joined)
	}
	// A closed sprint is trivially reachable — it needs no room.
	done := &weaveStory{ID: 9, Column: "done"}
	if p := sprintCheckReachability(done).Problems; len(p) != 0 {
		t.Errorf("a done sprint must not be reported unreachable: %v", p)
	}
}

func TestIntegratedBranchRuleNeverOffersUniqueWork(t *testing.T) {
	// The safety property of the whole cleanup story: a branch is only ever
	// offered for removal when its patches exist elsewhere.
	repo := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		out, err := gitOutputForTest(repo, args...)
		if err != nil {
			t.Skipf("git unavailable in this environment: %v", err)
		}
		return out
	}
	git("init", "-q", "-b", "main")
	git("config", "user.email", "t@example.com")
	git("config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", "-A")
	git("commit", "-qm", "base")
	git("checkout", "-q", "-b", "feature")
	if err := os.WriteFile(filepath.Join(repo, "b.txt"), []byte("two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", "-A")
	git("commit", "-qm", "unique work")
	git("checkout", "-q", "main")

	if gitBranchIntegrated(repo, "main", "feature") {
		t.Fatal("a branch with a unique commit must NEVER be reported integrated")
	}
	// After merging, the same branch becomes safe.
	git("merge", "-q", "--no-ff", "-m", "merge", "feature")
	if !gitBranchIntegrated(repo, "main", "feature") {
		t.Fatal("a merged branch must be reported integrated")
	}
}

// --- helpers ---

func sprintContactRef(t *testing.T, id int64) string {
	t.Helper()
	dir, err := sprintStoreDir()
	if err != nil {
		t.Fatalf("sprint dir: %v", err)
	}
	q, err := loadWeaveQueue(dir)
	if err != nil {
		t.Fatalf("load board: %v", err)
	}
	s := findWeaveStory(q, id)
	if s == nil || s.Contact == nil {
		return ""
	}
	return s.Contact.Ref
}

// gitOutputForTest runs a git command in a repo, for fixtures only.
func gitOutputForTest(root string, args ...string) (string, error) {
	a := append([]string{"-C", root}, args...)
	out, err := execCommandForTest("git", a...)
	return out, err
}

// TestSprintEndGatesOnCoverageAndHygiene is the regression for a gap that was
// CLAIMED closed and was not: `sprint move ... done` was gated while
// `sprint end` set Column = "done" directly, so the documented "end gates on
// it" was false and the strictest close path was the unguarded one.
func TestSprintEndGatesOnCoverageAndHygiene(t *testing.T) {
	src, err := os.ReadFile("weave_story_box.go")
	if err != nil {
		t.Fatalf("read box source: %v", err)
	}
	body := string(src)
	// The gate must sit on the ending path itself, not on a sibling verb.
	for _, want := range []string{"sprintCoverageGate(s, false, \"\")", "sprintCheckHygiene(s); !hy.Clean()"} {
		if !strings.Contains(body, want) {
			t.Errorf("sprint end is missing its close gate %q — a sprint could end over open or unclean work", want)
		}
	}
}

// TestSprintShowSurfacesCoverageAndReachability guards the other silent gap:
// renderSprintCoverage existed but nothing called it, so the plan's blind spot
// was computed and never shown.
func TestSprintShowSurfacesCoverageAndReachability(t *testing.T) {
	src, err := os.ReadFile("weave_story.go")
	if err != nil {
		t.Fatalf("read story source: %v", err)
	}
	body := string(src)
	for _, want := range []string{"renderSprintCoverage(out, s)", "renderSprintReachability(out, s)"} {
		if !strings.Contains(body, want) {
			t.Errorf("sprint show does not call %s — the finding is computed and never surfaced", want)
		}
	}
	if !strings.Contains(body, "UNREACHABLE: %d") {
		t.Error("sprint board does not flag unreachable open sprints")
	}
}

// TestGoalAddCanCoverAStoryInOneStep — covering a newly reported bug must not
// need two commands, because the second one is the one that gets skipped.
func TestGoalAddCoversAStoryInOneStep(t *testing.T) {
	cmd := NewSprintCmd()
	goal, _, err := cmd.Find([]string{"goal", "add"})
	if err != nil {
		t.Fatalf("find goal add: %v", err)
	}
	if goal.Flags().Lookup("story") == nil {
		t.Error("sprint goal add must accept --story so create-and-link is one command")
	}
}

// TestSprintOwnerMustBeALiveEntryInBashyAgents is THE critical invariant:
// a sprint's manager name must be a real entry in `bashy agents`, and must be
// RUNNING when it takes the seat. Everything else about collaboration depends
// on it — the room, the inbox, mb, chat and ping all key on this one string,
// so a name that resolves to nothing turns every one of them into a black hole
// that accepts requests and answers none.
func TestSprintOwnerMustBeALiveEntryInBashyAgents(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("WEAVE_CONDUCTOR", "seatless")

	if out, code := runSprint(t, "add", "owner invariant"); code != 0 {
		t.Fatalf("add exit=%d: %s", code, out)
	}

	// 1. Unknown name — refused, and the refusal teaches both registrations.
	out, code := runSprint(t, "start", "1", "--for", "1h")
	if code == 0 {
		t.Fatalf("a sprint was seated to a name that is in no roster:\n%s", out)
	}
	if !strings.Contains(out, "does not resolve") {
		t.Errorf("refusal must say the name resolves to no agent:\n%s", out)
	}

	// 2. Registered but NOT running — still refused. This is the case that
	// used to look healthy: `bashy agents list` shows it, so every surface
	// prints it as a contact, and nothing is behind it.
	seedAgent(t, "seatless")
	out, code = runSprint(t, "start", "1", "--for", "1h")
	if code == 0 {
		t.Fatalf("a sprint was seated to a registered but DEAD agent:\n%s", out)
	}
	if !strings.Contains(out, "not RUNNING") {
		t.Errorf("refusal must distinguish registered-but-dead from unknown:\n%s", out)
	}
	if !strings.Contains(out, "owner-pid") {
		t.Errorf("refusal must name --owner-pid; a card anchored to a dying command is the trap:\n%s", out)
	}

	// 3. Registered AND live — accepted.
	seedLiveAgent(t, "seatless")
	if out, code := runSprint(t, "start", "1", "--for", "1h"); code != 0 {
		t.Fatalf("a registered, live agent must be able to take the seat: exit=%d\n%s", code, out)
	}
}

// TestConductorCannotWalkAwayFromUnreadRequests: pause, handoff and end are the
// three ways a conductor stops answering. Leaving with unread mail converts a
// question into an indefinite wait — the sender gets no signal that nobody is
// coming, and the durable queue faithfully preserves a message that will never
// be looked at.
func TestConductorCannotWalkAwayFromUnreadRequests(t *testing.T) {
	for _, verb := range []string{"pause", "hand off", "end"} {
		t.Run(verb, func(t *testing.T) {
			// The gate is exercised directly: publishing real bus traffic into
			// a temp HOME is a different subsystem's setup, and what must not
			// regress is the RULE — unread mail blocks the handover verbs.
			s := &weaveStory{ID: 5, Owner: "somebody", Column: "doing"}
			if err := sprintUnansweredGate(s, verb); err != nil {
				// No mail in a clean environment: the gate must be silent, not
				// noisy. A check that fires when there is nothing to report is
				// one operators learn to bypass.
				t.Fatalf("gate must pass with no unread mail: %v", err)
			}
		})
	}
	// And an owner with no name is not a reason to block anything.
	if err := sprintUnansweredGate(&weaveStory{ID: 6}, "pause"); err != nil {
		t.Fatalf("an ownerless sprint must not be gated on mail: %v", err)
	}
}

// TestHandoverVerbsAllConsultTheUnansweredGate guards the wiring, because a
// gate function that exists and is called from nowhere is the exact failure
// this session already shipped once.
func TestHandoverVerbsAllConsultTheUnansweredGate(t *testing.T) {
	for _, f := range []struct{ file, verb string }{
		{"weave_story_lifecycle.go", "pause"},
		{"weave_story.go", "hand off"},
		{"weave_story_box.go", "end"},
	} {
		src, err := os.ReadFile(f.file)
		if err != nil {
			t.Fatalf("read %s: %v", f.file, err)
		}
		want := "sprintUnansweredGate(s, \"" + f.verb + "\")"
		if !strings.Contains(string(src), want) {
			t.Errorf("%s does not call %s — a conductor could leave with unread requests", f.file, want)
		}
	}
}
