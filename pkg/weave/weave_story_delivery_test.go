package weave

import (
	"os"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/pkg/bus"
	"github.com/qiangli/coreutils/pkg/room"
)

func TestStoryLifecycleNotificationsReachManagerInbox(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("BASHY_ROOM_DIR", t.TempDir())

	sprint := &weaveStory{ID: 120, Owner: "manager"}
	for _, body := range []string{
		"worker claimed story abc",
		"worker yielded story abc",
		"worker submitted story abc",
	} {
		if err := notifySprintOwner(sprint, "worker", body); err != nil {
			t.Fatalf("notify manager: %v", err)
		}
	}

	snapshot, err := bus.SnapshotInbox("manager")
	if err != nil {
		t.Fatalf("manager inbox: %v", err)
	}
	if len(snapshot.Items) != 3 {
		t.Fatalf("manager received %d lifecycle notifications, want 3: %+v", len(snapshot.Items), snapshot.Items)
	}
	for i, verb := range []string{"claimed", "yielded", "submitted"} {
		if !strings.Contains(snapshot.Items[i].Body, verb) {
			t.Errorf("notification %d body = %q, want %q", i, snapshot.Items[i].Body, verb)
		}
		if snapshot.Items[i].Delivery != bus.DeliveryQueued {
			t.Errorf("notification %d delivery = %q, want %q", i, snapshot.Items[i].Delivery, bus.DeliveryQueued)
		}
	}
}

func TestSprintDeliveryAcceptsAttachedExternalStream(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("BASHY_ROOM_DIR", t.TempDir())
	const owner = "external-manager"
	if err := room.Join(room.Card{
		ID: room.AgentClaimID(owner), Nick: owner, Mode: "sprint-inbox",
		Tool: "agy", Binding: "agy:test", PID: os.Getpid(),
		Caps: []string{room.CapInboxStream},
	}); err != nil {
		t.Fatal(err)
	}
	defer room.Leave(room.AgentClaimID(owner))
	if !sprintInboxDeliveryLive(owner) {
		t.Fatal("live attached external inbox stream was not accepted")
	}
}

// THE BEHAVIOUR THAT CHANGED when sprintInboxDeliveryLive became a projection of
// room.OwnerTransportFor, asserted so the change is a decision with a test
// rather than something inherited from a refactor.
//
// The old private copy accepted CapInboxStream only when Mode was
// "sprint-inbox". A live `bashy inbox --watch --as X` publishes Mode "inbox",
// so it did NOT count as reachable — an agent holding its own inbox open,
// reported unreachable. It counts now: holding your inbox open IS the whole
// content of the attached rung.
func TestSprintDeliveryAcceptsAPlainInboxWatch(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("BASHY_ROOM_DIR", t.TempDir())
	const owner = "watching-manager"
	if err := room.Join(room.Card{
		ID: room.AgentClaimID(owner), Nick: owner, Mode: "inbox",
		Tool: "claude", Binding: "claude:test", PID: os.Getpid(),
		Caps: []string{room.CapInboxStream},
	}); err != nil {
		t.Fatal(err)
	}
	defer room.Leave(room.AgentClaimID(owner))
	if !sprintInboxDeliveryLive(owner) {
		t.Fatal("a live `inbox --watch` (Mode \"inbox\") was reported unreachable;\n" +
			"holding your own inbox open is exactly what the attached rung means")
	}
}

// And the refusal still holds where it must: a live card that promises nothing
// is not a delivery path, whatever its mode. This is the loophole
// `agents track start` produces.
func TestSprintDeliveryRefusesACardThatPromisesNothing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("BASHY_ROOM_DIR", t.TempDir())
	const owner = "tracked-only"
	if err := room.Join(room.Card{
		ID: room.AgentClaimID(owner), Nick: owner, Mode: "weave",
		Tool: "codex", Binding: "codex:test", PID: os.Getpid(),
	}); err != nil {
		t.Fatal(err)
	}
	defer room.Leave(room.AgentClaimID(owner))
	if sprintInboxDeliveryLive(owner) {
		t.Fatal("a tracked work record with no delivery capability was accepted as reachable")
	}
}
