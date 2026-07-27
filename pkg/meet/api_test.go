package meet

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

// api.go is the ONE implementation both surfaces call. These tests hold that
// claim two ways: the exported funcs behave, and the cobra commands reach them
// rather than keeping a private copy of the same act.

// Every ref-taking verb accepts what a human types at `meet list`: a room number,
// a full id, or an unambiguous prefix. That is resolveMeeting's job, reused —
// a second resolver would drift on which spellings work, and the failure mode is
// acting on the wrong meeting.
func TestAPIAcceptsEveryRoomReference(t *testing.T) {
	st := newRoom(t)
	st.Room = 1
	if err := st.save(); err != nil {
		t.Fatal(err)
	}

	for _, ref := range []string{"1", "#1", st.ID, st.ID[:12]} {
		got, _, err := Room(ref)
		if err != nil {
			t.Fatalf("Room(%q): %v", ref, err)
		}
		if got.ID != st.ID {
			t.Errorf("Room(%q) = %s, want %s", ref, got.ID, st.ID)
		}
		if _, err := Post(ref, "qiangli", "by "+ref); err != nil {
			t.Errorf("Post(%q): %v", ref, err)
		}
	}

	events, _ := readTranscript(st.ID)
	if len(events) != 4 {
		t.Fatalf("want one post per reference spelling: %+v", events)
	}
}

// A miss is ErrNoRoom, whatever the spelling — the sentinel a transport
// classifies on. The human message is preserved, because it is the one that says
// what to do about it.
func TestAPIUnknownReferenceIsErrNoRoom(t *testing.T) {
	newRoom(t)

	for _, ref := range []string{"nope", "2026-01-01-nothing-here-0000", "99"} {
		if _, _, err := Room(ref); !errors.Is(err, ErrNoRoom) {
			t.Errorf("Room(%q) = %v, want ErrNoRoom", ref, err)
		}
		if _, err := Post(ref, "qiangli", "hi"); !errors.Is(err, ErrNoRoom) {
			t.Errorf("Post(%q) = %v, want ErrNoRoom", ref, err)
		}
		if err := Close(ref, "qiangli"); !errors.Is(err, ErrNoRoom) {
			t.Errorf("Close(%q) = %v, want ErrNoRoom", ref, err)
		}
	}

	// The message a human reads still points at the fix.
	_, _, err := Room("nope")
	if !strings.Contains(err.Error(), "meet list") {
		t.Errorf("the miss must name the recovery: %v", err)
	}
}

// Create builds through sessionFlags.newState, so a room opened from a browser is
// held to exactly the invariants `meet start` enforces.
func TestCreateHoldsTheRoleInvariants(t *testing.T) {
	newRoom(t)

	if _, err := Create(CreateOptions{Participants: []string{"codex"}}); err == nil {
		t.Error("a room needs a topic")
	}
	// The secretary records the decisions and must not have a stake in them —
	// enforced by Validate, not re-checked here.
	_, err := Create(CreateOptions{
		Topic: "x", Participants: []string{"codex"}, Secretary: "codex",
	})
	if err == nil {
		t.Error("the secretary must not also be a participant")
	}

	st, err := Create(CreateOptions{
		Topic:  "Should the cache be write-through?",
		Agenda: []string{"the write path"}, Participants: []string{"codex"},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if st.Room < 1 {
		t.Error("a created room needs a door to attach to")
	}
	if st.Status != "open" || st.Out != "docs" || st.TurnTimeout == "" {
		t.Errorf("defaults did not land: %+v", st)
	}
	// The agenda is recorded as well as stored — a round reads the header, and a
	// reader of the minutes reads the transcript.
	events, _ := readTranscript(st.ID)
	if countKind(events, "agenda") != 1 {
		t.Errorf("agenda events = %+v", events)
	}
}

// Close is organizer-only and must never reach for stdin: an HTTP handler has no
// terminal, and confirmConclusion would either error or block on a closed pipe.
func TestCloseIsOrganizerOnlyAndNeverPrompts(t *testing.T) {
	st := newRoom(t)

	if err := Close(st.ID, "codex"); !errors.Is(err, ErrNotOrganizer) {
		t.Fatalf("a member's close = %v, want ErrNotOrganizer", err)
	}
	if reloaded, _ := loadState(st.ID); reloaded.Status != "open" {
		t.Fatal("a refused close must not close the room")
	}

	if err := Close(st.ID, "qiangli"); err != nil {
		t.Fatalf("the organizer's close: %v", err)
	}
	reloaded, _ := loadState(st.ID)
	if reloaded.Status != "closed" {
		t.Errorf("status = %q", reloaded.Status)
	}
	// Yes is recorded rather than silent: the confirm event is what distinguishes
	// a room concluded deliberately from one abandoned.
	events, _ := readTranscript(st.ID)
	if countKind(events, "confirm") != 1 {
		t.Errorf("want one confirm event: %+v", events)
	}
}

// Address takes the room lease, because a turn appends to the transcript a
// concurrent round is also writing.
func TestAddressIsLeaseGuarded(t *testing.T) {
	st := newRoom(t)
	lease, err := acquireRunLease(st.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Release()

	if _, err := Address(context.Background(), st.ID, "codex", "hi"); !errors.Is(err, ErrMeetingBusy) {
		t.Fatalf("Address while the floor is held = %v, want ErrMeetingBusy", err)
	}
	// A post is not: that is what lets a room feel like chat.
	if _, err := Post(st.ID, "qiangli", "meanwhile"); err != nil {
		t.Fatalf("Post while the floor is held: %v", err)
	}
}

func TestMarkKinds(t *testing.T) {
	st := newRoom(t)

	for _, kind := range []string{"decision", "action", "note"} {
		ev, err := Mark(st.ID, kind, kind+" text")
		if err != nil {
			t.Fatalf("Mark(%q): %v", kind, err)
		}
		if ev.Kind != kind || ev.Speaker != st.Human {
			t.Errorf("Mark(%q) = %+v", kind, ev)
		}
	}
	// Case is not the point; the closed set is.
	if _, err := Mark(st.ID, "DECISION", "shouty"); err != nil {
		t.Errorf("kind should be case-insensitive: %v", err)
	}
	if _, err := Mark(st.ID, "proclamation", "hear ye"); err == nil {
		t.Error("an invented kind is an event nothing renders; it must be refused")
	}
	if _, err := Mark(st.ID, "note", "  "); err == nil {
		t.Error("an empty marker is not a marker")
	}
}

// The cobra tree goes THROUGH api.go — one implementation, not two. `tell` is the
// one write verb that runs no agent, so it is the one this can prove end to end
// without spawning a CLI.
func TestCobraTellGoesThroughPost(t *testing.T) {
	st := newRoom(t)
	st.Room = 1
	if err := st.save(); err != nil {
		t.Fatal(err)
	}

	cmd := NewMeetCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	// A room NUMBER, which the old body could not accept: it called loadState on
	// the argument directly. That it works now is the visible consequence of the
	// command reaching the room through Post → roomOf → resolveMeeting.
	cmd.SetArgs([]string{"tell", "1", "the", "cache", "should", "be", "write-through"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("meet tell: %v", err)
	}

	events, _ := readTranscript(st.ID)
	if len(events) != 1 {
		t.Fatalf("events = %+v", events)
	}
	if events[0].Kind != "human" || events[0].Speaker != st.Human {
		t.Errorf("event = %+v", events[0])
	}
	if events[0].Text != "the cache should be write-through" {
		t.Errorf("text = %q", events[0].Text)
	}
}

// `meet close` shares closeRoom with Close, and keeps its own attended semantics:
// the terminal prompt is the check there, so --yes is what an unattended CLI
// close needs and the organizer check does not apply to whoever typed it.
func TestCobraCloseSharesTheBody(t *testing.T) {
	st := newRoom(t)

	cmd := NewMeetCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader(""))
	cmd.SetArgs([]string{"close", st.ID, "--yes"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("meet close: %v", err)
	}
	if !strings.Contains(out.String(), "wrote ") {
		t.Errorf("close must report where the minutes went:\n%s", out.String())
	}
	reloaded, _ := loadState(st.ID)
	if reloaded.Status != "closed" {
		t.Errorf("status = %q", reloaded.Status)
	}
}

// Rooms is the list a channel sidebar renders, and it backfills a door for an
// open room that has none — a number that appeared only after some other command
// ran would be a door that is sometimes there.
func TestRoomsBackfillsADoor(t *testing.T) {
	st := newRoom(t)
	st.Room = 0
	if err := st.save(); err != nil {
		t.Fatal(err)
	}

	rooms, err := Rooms()
	if err != nil {
		t.Fatal(err)
	}
	if len(rooms) != 1 {
		t.Fatalf("rooms = %+v", rooms)
	}
	if rooms[0].Room != 1 {
		t.Errorf("room = %d, want the lowest free door", rooms[0].Room)
	}
	if rooms[0].Updated.IsZero() {
		t.Error("a room with no transcript still reports when it was created")
	}
	reloaded, _ := loadState(st.ID)
	if reloaded.Room != 1 {
		t.Error("the backfilled door must be persisted, or the number renumbers under the reader")
	}
}
