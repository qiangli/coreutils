package room

import (
	"os"
	"testing"
)

func isolate(t *testing.T) {
	t.Helper()
	t.Setenv("BASHY_ROOM_DIR", t.TempDir())
}

// TestJoinLeaveRoundTrip — join publishes a member and a timeline event; leave
// removes it and records a leave.
func TestJoinLeaveRoundTrip(t *testing.T) {
	isolate(t)
	c := Card{
		ID: "codex-1", Binding: "codex:gpt-5.5", Nick: "Bruno", Tool: "codex",
		Mode: "interactive", PID: os.Getpid(), // alive: this test process
	}
	if err := Join(c); err != nil {
		t.Fatalf("join: %v", err)
	}
	members, err := Members()
	if err != nil {
		t.Fatalf("members: %v", err)
	}
	if len(members) != 1 || members[0].ID != "codex-1" {
		t.Fatalf("members = %+v, want the one card", members)
	}
	if got, ok, _ := Find("Bruno"); !ok || got.ID != "codex-1" {
		t.Fatalf("find by nick = %+v (%v)", got, ok)
	}

	Leave("codex-1")
	members, _ = Members()
	if len(members) != 0 {
		t.Fatalf("after leave, want 0 members, got %d", len(members))
	}

	// The timeline recorded both a join and a leave.
	events, _ := Timeline(0)
	if len(events) != 2 || events[0].Type != EventJoin || events[1].Type != EventLeave {
		t.Fatalf("timeline = %+v, want join then leave", events)
	}
	if events[0].Seq != 1 || events[1].Seq != 2 {
		t.Fatalf("seq should be 1,2 got %d,%d", events[0].Seq, events[1].Seq)
	}
}

// TestMembersPrunesDead — a card whose pid is gone is pruned on read, so the room
// never asserts a dead member is live (absence-of-evidence).
func TestMembersPrunesDead(t *testing.T) {
	isolate(t)
	if err := Join(Card{ID: "codex-dead", Binding: "codex:gpt-5.5", Tool: "codex", PID: 2147483000}); err != nil {
		t.Fatalf("join: %v", err)
	}
	members, err := Members()
	if err != nil {
		t.Fatalf("members: %v", err)
	}
	if len(members) != 0 {
		t.Fatalf("a dead member should be pruned on read, got %d", len(members))
	}
}

// TestTimelineTail — Emit appends; Timeline(n) returns the last n, oldest-first.
func TestTimelineTail(t *testing.T) {
	isolate(t)
	for _, b := range []string{"a", "b", "c"} {
		if err := Emit(Event{Type: EventNote, Body: b}); err != nil {
			t.Fatal(err)
		}
	}
	got, _ := Timeline(2)
	if len(got) != 2 || got[0].Body != "b" || got[1].Body != "c" {
		t.Fatalf("tail(2) = %+v, want [b c]", got)
	}
}
