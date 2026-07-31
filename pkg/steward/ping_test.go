package steward

import (
	"strings"
	"testing"
)

// NOTE ON ISOLATION: the seat is a singleton per machine-and-account, and the
// registry REFUSES a second store for one — correctly, since a second store
// would be a second seat with its own empty journal and its own epoch 1, unable
// to see or be fenced by the first. So these tests take a store the same way
// every other test here does, through WithRegistryRoot, rather than pointing
// --dir somewhere fresh and tripping the singleton.

// A ping to an empty seat would queue a message for nobody, and the bus would
// hold it indefinitely. Refusing says the useful thing instead.
func TestPing_VacantSeatRefuses(t *testing.T) {
	dir, reg := t.TempDir(), t.TempDir()
	_, err := ping(dir, "tester", "need the GPU", "", WithRegistryRoot(reg))
	if err == nil {
		t.Fatal("pinging a vacant seat must refuse, not queue for nobody")
	}
	if !strings.Contains(err.Error(), "seat is open") || !strings.Contains(err.Error(), "claim") {
		t.Errorf("the refusal must say a POC can be created: %v", err)
	}
}

// An empty body is a mistake, not a message. Sending it would spend an
// interrupt — the scarcest thing in this system — on nothing.
func TestPing_EmptyBodyRefuses(t *testing.T) {
	dir, reg := t.TempDir(), t.TempDir()
	for _, body := range []string{"", "   ", "\n"} {
		if _, err := ping(dir, "tester", body, "", WithRegistryRoot(reg)); err == nil {
			t.Errorf("an empty body (%q) must refuse", body)
		} else if !strings.Contains(err.Error(), "nothing to send") {
			t.Errorf("the refusal should name the problem: %v", err)
		}
	}
}

// THE ADDRESS SURVIVES A HANDOFF. The topic names the seat, not its holder, so
// a message written an hour ago still reaches whoever is responsible now —
// rather than an agent who has since released it, or nobody.
func TestPing_AddressesTheSeatNotTheHolder(t *testing.T) {
	topic := stewardAssignment().Topic()
	if !strings.HasPrefix(topic, "steward.") {
		t.Fatalf("topic = %q", topic)
	}
	if strings.Contains(topic, seatHolderName()) {
		t.Errorf("topic %q carries the HOLDER — a handoff would strand every "+
			"message already sent to it", topic)
	}
	// And the seat label a human reads is NOT the address: one says which
	// machine, the other has to be unique.
	if seatLabel() == topic {
		t.Error("the human label and the address must not be the same string")
	}
}
