package bus

import (
	"fmt"

	"github.com/qiangli/coreutils/pkg/room"
)

// Notification is one change to announce.
type Notification struct {
	// Principal is who is announcing it. REQUIRED — a notification nobody can be
	// attributed to is not a notification, and a subscriber decides whose
	// interrupts it honours by this name.
	Principal string
	// Addressing: at least one of Topic / To / Room.
	Topic string
	To    string
	Room  string
	Body  string
	// Priority REQUESTS a delivery tier (see DeliveryQueued / DeliveryInterrupt).
	// The subscriber decides whether to honour an interrupt, so this is a request
	// and never a guarantee.
	Priority string
}

// Publish announces a change on the bus.
//
// This is the one enforcement point, and it exists because the invariants used to
// live only inside the cobra handler: anything publishing programmatically —
// the coach, kb, a future conductor — would have bypassed the principal and
// addressing checks entirely and written a notification that could reach nobody.
// A rule enforced in the CLI layer is a rule that holds only for humans.
func Publish(n Notification) error {
	if n.Principal == "" {
		return fmt.Errorf("bus: principal is required (REPORT/AUTHOR invariant)")
	}
	if n.Topic == "" && n.To == "" && n.Room == "" {
		return fmt.Errorf("bus: addressing is required — set at least one of Topic, To, or Room (an unaddressed notification reaches nobody)")
	}
	switch n.Priority {
	case "", DeliveryQueued, DeliveryInterrupt:
	default:
		return fmt.Errorf("bus: unknown priority %q (use %s or %s)", n.Priority, DeliveryQueued, DeliveryInterrupt)
	}
	return room.Notify(room.Event{
		Principal: n.Principal,
		Topic:     n.Topic,
		To:        n.To,
		Room:      n.Room,
		Body:      n.Body,
		Priority:  n.Priority,
	})
}
