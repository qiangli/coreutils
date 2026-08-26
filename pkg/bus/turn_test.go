package bus

import (
	"os"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/pkg/room"
)

// joinLive publishes a room card so a subscription's Instance resolves to a
// reachable control socket, the way a live agent session does.
// Uses our OWN pid: room.Members prunes a card whose pid is not alive, and the
// liveness probe is signal 0 — which returns EPERM (not success) for a process we
// do not own. A card claiming pid 1 is therefore pruned as dead on macOS, and the
// test would fail for a reason that has nothing to do with the bus.
func joinLive(t *testing.T, id, ctlSock string) {
	t.Helper()
	if err := room.Join(room.Card{ID: id, CtlSock: ctlSock, PID: os.Getpid()}); err != nil {
		t.Fatal(err)
	}
}

// The turn-boundary hook: the harness reads the buffer on the agent's behalf, at
// the one moment the agent is guaranteed to be listening. This is what closes the
// gap the sidecar exists for — `bus pending` is a channel an agent must CHOOSE to
// read, and the premise is that it cannot reliably choose.
func TestTurnPreambleDeliversAndClears(t *testing.T) {
	isolate(t)
	joinLive(t, "dev-b-session", "/tmp/dev-b.sock")
	subscribe(t, Subscription{
		Subscriber: "dev-b", Topics: []string{"code.api.*"}, Instance: "dev-b-session",
	})
	publish(t, "--topic", "code.api.Foo", "--as", "dev-a", "Foo was renamed")

	if _, err := testSidecar(t, &recorder{}).Once(); err != nil {
		t.Fatal(err)
	}

	block := TurnPreamble("/tmp/dev-b.sock")
	if !strings.Contains(block, "Foo was renamed") {
		t.Fatalf("the turn preamble did not carry the notification:\n%s", block)
	}
	// Cleared, or the agent would see the same notification at every turn forever.
	if again := TurnPreamble("/tmp/dev-b.sock"); again != "" {
		t.Errorf("a second turn re-delivered the notification:\n%s", again)
	}
}

// A session nobody subscribed for must get nothing. Subscription is opt-in per
// agent — the design is explicit that auto-subscribing everything to everything
// is how a bus makes a fleet worse.
func TestTurnPreambleIsEmptyWithoutASubscription(t *testing.T) {
	isolate(t)
	joinLive(t, "lonely-session", "/tmp/lonely.sock")
	publish(t, "--topic", "code.api.Foo", "--as", "dev-a", "nobody asked")
	if _, err := testSidecar(t, &recorder{}).Once(); err != nil {
		t.Fatal(err)
	}
	if block := TurnPreamble("/tmp/lonely.sock"); block != "" {
		t.Errorf("an unsubscribed session was handed notifications:\n%s", block)
	}
}

// Sessions are matched by control socket, so one session must never be handed
// another's notifications — the failure a name-based match invites when a name is
// reused across runs.
func TestTurnPreambleDoesNotCrossSessions(t *testing.T) {
	isolate(t)
	joinLive(t, "a-session", "/tmp/a.sock")
	joinLive(t, "b-session", "/tmp/b.sock")
	subscribe(t, Subscription{Subscriber: "a", Topics: []string{"*"}, Instance: "a-session"})
	subscribe(t, Subscription{Subscriber: "b", Topics: []string{"*"}, Instance: "b-session"})
	publish(t, "--topic", "x", "--as", "p", "for both")
	if _, err := testSidecar(t, &recorder{}).Once(); err != nil {
		t.Fatal(err)
	}

	if a := TurnPreamble("/tmp/a.sock"); !strings.Contains(a, "for both") {
		t.Errorf("session a got nothing:\n%s", a)
	}
	// b has its own copy — per-subscriber buffers, so a's read did not consume it.
	if b := TurnPreamble("/tmp/b.sock"); !strings.Contains(b, "for both") {
		t.Errorf("session a's read consumed session b's copy:\n%s", b)
	}
	// An unrelated socket matches nothing.
	if c := TurnPreamble("/tmp/nobody.sock"); c != "" {
		t.Errorf("an unknown socket was handed notifications:\n%s", c)
	}
}

// The block goes FIRST: a notification is context for the instruction that
// follows, and appending it would have the agent commit to an approach and only
// then learn the ground moved.
func TestPrependPutsNotificationsBeforeTheMessage(t *testing.T) {
	isolate(t)
	joinLive(t, "s", "/tmp/s.sock")
	subscribe(t, Subscription{Subscriber: "dev", Topics: []string{"*"}, Instance: "s"})
	publish(t, "--topic", "code.api.Foo", "--as", "dev-a", "Foo was renamed")
	if _, err := testSidecar(t, &recorder{}).Once(); err != nil {
		t.Fatal(err)
	}

	got := Prepend("/tmp/s.sock", "now fix Foo")
	if !strings.HasSuffix(got, "now fix Foo") {
		t.Errorf("the caller's message must come last:\n%s", got)
	}
	if strings.Index(got, "Foo was renamed") > strings.Index(got, "now fix Foo") {
		t.Errorf("the notification must come first:\n%s", got)
	}
}

// An empty or unmatched socket must return the text untouched — the hook is on
// the hot path of every steer and may never mangle a message or block one.
func TestPrependIsInertWithoutNotifications(t *testing.T) {
	isolate(t)
	for _, sock := range []string{"", "/tmp/unknown.sock"} {
		if got := Prepend(sock, "just the message"); got != "just the message" {
			t.Errorf("Prepend(%q) altered the text: %q", sock, got)
		}
	}
}
