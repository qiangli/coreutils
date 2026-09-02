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
