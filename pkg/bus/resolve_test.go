package bus

import (
	"testing"

	"github.com/qiangli/coreutils/pkg/room"
)

// THE GAP THIS CLOSES. Before R0 there were no subscriptions; after R0 there
// were inboxes but nothing resolved into them, because no sidecar process
// existed on any host. A DM sat in the timeline and `pending` truthfully
// reported an empty buffer while the message existed.
func TestResolveFor_DeliversWithNoSidecarRunning(t *testing.T) {
	busInTempHome(t)
	if _, err := EnsureSubscription("claude-opus5"); err != nil {
		t.Fatal(err)
	}
	if err := room.Emit(room.Event{
		Type: room.EventNotify, To: "claude-opus5", Topic: "cert", Body: "gate is red",
	}); err != nil {
		t.Fatal(err)
	}

	// No Sidecar is constructed anywhere in this test — that is the point.
	n, err := ResolveFor("claude-opus5")
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("resolved %d, want 1", n)
	}
	items, err := ReadPending("claude-opus5")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Body != "gate is red" {
		t.Fatalf("pending = %+v", items)
	}
	if items[0].Delivery != DeliveryQueued {
		t.Fatalf("delivery = %q, want queued — the agent is reading, so an interrupt would break the turn that came to collect it", items[0].Delivery)
	}
}

// Resolving twice must not deliver twice: the offset advances after the append.
func TestResolveFor_IsIdempotent(t *testing.T) {
	busInTempHome(t)
	if _, err := EnsureSubscription("claude-opus5"); err != nil {
		t.Fatal(err)
	}
	if err := room.Emit(room.Event{Type: room.EventNotify, To: "claude-opus5", Body: "once"}); err != nil {
		t.Fatal(err)
	}
	if n, err := ResolveFor("claude-opus5"); err != nil || n != 1 {
		t.Fatalf("first pass: n=%d err=%v", n, err)
	}
	if n, err := ResolveFor("claude-opus5"); err != nil || n != 0 {
		t.Fatalf("second pass must resolve nothing: n=%d err=%v", n, err)
	}
}

// A message for somebody else must never land in this agent's buffer.
func TestResolveFor_OnlyItsOwnMail(t *testing.T) {
	busInTempHome(t)
	for _, who := range []string{"claude-opus5", "codex-gpt5.6-sol"} {
		if _, err := EnsureSubscription(who); err != nil {
			t.Fatal(err)
		}
	}
	if err := room.Emit(room.Event{Type: room.EventNotify, To: "codex-gpt5.6-sol", Body: "not yours"}); err != nil {
		t.Fatal(err)
	}
	if n, _ := ResolveFor("claude-opus5"); n != 0 {
		t.Fatalf("resolved %d for the wrong subscriber", n)
	}
	if n, _ := ResolveFor("codex-gpt5.6-sol"); n != 1 {
		t.Fatalf("the addressee resolved %d, want 1", n)
	}
}

// A name outside the address book resolves nothing and is not an error —
// refusing to print an empty buffer for it would help nobody.
func TestResolveFor_UnknownSubscriberIsSilent(t *testing.T) {
	busInTempHome(t)
	n, err := ResolveFor("not-an-agent")
	if err != nil {
		t.Fatalf("an unknown subscriber must not error: %v", err)
	}
	if n != 0 {
		t.Fatalf("resolved %d, want 0", n)
	}
}

// A cold agent — one that was not running when the message was sent — gets it
// on the next read. That is the whole (c) half of the delivery model.
func TestResolveFor_ColdAgentGetsItLater(t *testing.T) {
	busInTempHome(t)
	if _, err := EnsureSubscription("agy-gemini3.1"); err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{"first", "second"} {
		if err := room.Emit(room.Event{Type: room.EventNotify, To: "agy-gemini3.1", Body: body}); err != nil {
			t.Fatal(err)
		}
	}
	if n, _ := ResolveFor("agy-gemini3.1"); n != 2 {
		t.Fatalf("a cold agent must receive everything sent while it was away, got %d", n)
	}
}
