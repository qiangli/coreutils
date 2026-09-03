package weave

import (
	"os"
	"testing"

	"github.com/qiangli/coreutils/pkg/room"
)

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
