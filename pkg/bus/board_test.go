package bus

import (
	"os"
	"testing"
)

func boardInTempHome(t *testing.T) {
	t.Helper()
	t.Setenv("BASHY_MB_DIR", t.TempDir())
	t.Setenv("BASHY_PRINCIPAL", "")
	t.Setenv("USER", "tester")
}

// THE BUG THIS STORE FIXES. Posts are addressed to the FLEET NAME, but a
// reader's environment carries something else — a bashy-launched agent has
// BASHY_PRINCIPAL=dhnt:agent/<Nick>. Resolving both to one name is what lets a
// bare `bashy mb` work instead of every agent being told its own identity.
func TestBoardIdentity_ResolvesAPrincipalToTheFleetName(t *testing.T) {
	boardInTempHome(t)
	FleetResolveName = func(s string) string {
		if s == "Omar" {
			return "codex-gpt5.6-sol"
		}
		return ""
	}
	t.Cleanup(func() { FleetResolveName = nil })

	t.Setenv("BASHY_PRINCIPAL", "dhnt:agent/Omar")
	if got := BoardIdentity(""); got != "codex-gpt5.6-sol" {
		t.Fatalf("identity = %q, want the fleet name", got)
	}
	// An explicit --as still wins, and also resolves.
	if got := BoardIdentity("Omar"); got != "codex-gpt5.6-sol" {
		t.Fatalf("--as identity = %q", got)
	}
	// A human at a terminal is a legitimate participant under their login name,
	// not an agent to be resolved into one.
	t.Setenv("BASHY_PRINCIPAL", "")
	if got := BoardIdentity(""); got != "tester" {
		t.Fatalf("a non-agent must be used as itself, got %q", got)
	}
}

// PUBLIC BY CONSTRUCTION: addressing says who should ACT, never who may read.
func TestBoard_EveryoneCanReadEverything(t *testing.T) {
	boardInTempHome(t)
	for _, p := range []Post{
		{From: "a", Body: "to everyone"},
		{From: "a", To: "agent-x", Body: "for x"},
		{From: "a", To: "agent-y", Body: "for y"},
	} {
		if err := PostMessage(p); err != nil {
			t.Fatal(err)
		}
	}
	// A reader's default view is what it should act on...
	mine, err := Unseen("agent-x")
	if err != nil {
		t.Fatal(err)
	}
	if len(mine) != 2 { // the broadcast + its own
		t.Fatalf("agent-x sees %d, want 2 (broadcast + directed)", len(mine))
	}
	// ...but the whole board is readable by anyone. That is not an escalation:
	// it is the point of a public board.
	all, err := Posts()
	if err != nil || len(all) != 3 {
		t.Fatalf("Posts() = %d (err %v), want 3", len(all), err)
	}
}

// A first-time reader sees the WHOLE board, not nothing — the opposite of the
// private-inbox rule that opens a new mailbox at the head. Public means the
// history was always yours to read.
func TestBoard_FirstReadSeesTheHistory(t *testing.T) {
	boardInTempHome(t)
	for range 3 {
		if err := PostMessage(Post{From: "a", Body: "old news"}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := Unseen("brand-new-reader")
	if err != nil || len(got) != 3 {
		t.Fatalf("first read = %d (err %v), want the whole board", len(got), err)
	}
}

func TestBoard_CursorAdvancesAndNeverGoesBackwards(t *testing.T) {
	boardInTempHome(t)
	for range 2 {
		if err := PostMessage(Post{From: "a", Body: "x"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := MarkSeen("r", 2); err != nil {
		t.Fatal(err)
	}
	if n, _ := Unseen("r"); len(n) != 0 {
		t.Fatalf("after marking, %d unseen", len(n))
	}
	// Re-reading an older view must not un-see what was already shown.
	if err := MarkSeen("r", 1); err != nil {
		t.Fatal(err)
	}
	if SeenSeq("r") != 2 {
		t.Fatalf("cursor went backwards to %d", SeenSeq("r"))
	}
}

// An unattributable post is worse than none: nobody can ask the sender what
// they meant.
func TestBoard_PostNeedsASender(t *testing.T) {
	boardInTempHome(t)
	if err := PostMessage(Post{Body: "who said this?"}); err == nil {
		t.Fatal("a post with no sender must be refused")
	}
}

func TestBoard_AbsentBoardIsEmptyNotAnError(t *testing.T) {
	t.Setenv("BASHY_MB_DIR", "/nonexistent-xyz")
	posts, err := Posts()
	if err != nil {
		t.Fatalf("an absent board is a state, not a failure: %v", err)
	}
	if len(posts) != 0 {
		t.Fatalf("got %d posts", len(posts))
	}
	_ = os.Getenv("HOME")
}
