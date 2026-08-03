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
	directed, other, _, err := Unseen("agent-x", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(directed) != 1 || len(other) != 1 { // its own + the broadcast
		t.Fatalf("agent-x sees %d directed / %d other, want 1/1", len(directed), len(other))
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
	_, other, _, err := Unseen("brand-new-reader", 0)
	if err != nil || len(other) != 3 {
		t.Fatalf("first read = %d (err %v), want the whole board", len(other), err)
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
	if d, o, _, _ := Unseen("r", 0); len(d)+len(o) != 0 {
		t.Fatalf("after marking, %d unseen", len(d)+len(o))
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

// A SELECTOR IS STORED, NOT EXPANDED. Expanding made the board grow with the
// size of the audience — `--band 4` wrote eight identical posts — so --all
// became unreadable and every reader's scan got longer for a line that
// concerned one of them.
func TestBoard_SelectorIsOnePostAndResolvesAtReadTime(t *testing.T) {
	boardInTempHome(t)
	FleetSelect = func(a Audience) ([]string, error) {
		if a.Band == 4 {
			return []string{"claude-opus5", "codex-gpt5.6-sol"}, nil
		}
		return nil, nil
	}
	t.Cleanup(func() { FleetSelect = nil; audienceCache = map[Audience]map[string]bool{} })

	if err := PostMessage(Post{From: "steward", Audience: &Audience{Band: 4}, Body: "L4 only"}); err != nil {
		t.Fatal(err)
	}
	all, _ := Posts()
	if len(all) != 1 {
		t.Fatalf("a selector post must be ONE record, got %d", len(all))
	}
	// In the audience → sees it.
	if _, other, _, _ := Unseen("claude-opus5", 0); len(other) != 1 {
		t.Fatal("an agent in the audience must see a selector post")
	}
	// Outside it → does not.
	if _, other, _, _ := Unseen("agy-gemini3.1", 0); len(other) != 0 {
		t.Fatal("an agent outside the audience must not see it in its default view")
	}
	// But the board is still public.
	if posts, _ := Posts(); len(posts) != 1 {
		t.Fatal("the post must remain readable by anyone via --all")
	}
}

// DIRECTED IS NEVER CAPPED; the rest is, and the overflow is REPORTED. A cap
// that stays quiet is a silent drop, and a reader cannot tell "nothing else"
// from "twelve more" unless it is told.
func TestBoard_DirectedUncappedOtherCappedWithReportedOverflow(t *testing.T) {
	boardInTempHome(t)
	for range 9 {
		if err := PostMessage(Post{From: "a", Body: "fyi"}); err != nil {
			t.Fatal(err)
		}
	}
	for range 7 {
		if err := PostMessage(Post{From: "a", To: "me", Body: "do this"}); err != nil {
			t.Fatal(err)
		}
	}
	directed, other, older, err := Unseen("me", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(directed) != 7 {
		t.Fatalf("directed = %d; an obligation must never be truncated", len(directed))
	}
	if len(other) != 5 {
		t.Fatalf("other = %d, want the cap of 5", len(other))
	}
	if older != 4 {
		t.Fatalf("older = %d, want 4 reported as hidden", older)
	}
	// The newest are kept, not the oldest.
	if other[len(other)-1].Seq != 9 {
		t.Fatalf("the cap must keep the NEWEST, last seq = %d", other[len(other)-1].Seq)
	}
}
