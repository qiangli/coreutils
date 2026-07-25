package bus

import (
	"strings"

	"github.com/qiangli/coreutils/pkg/room"
)

// TurnPreamble returns the pending-notification block for the live session
// reachable at ctlSock, and CLEARS what it returns.
//
// This is the turn-boundary inject point, called by whoever is about to hand an
// agent a turn. It closes the last gap in the bus: `bus pending` is a channel an
// agent must choose to read, and the whole premise of the sidecar is that an
// agent cannot reliably choose. So the harness reads it on the agent's behalf, at
// the one moment the agent is guaranteed to be listening — the instant it is
// being given something to do.
//
// Sessions are matched by CONTROL SOCKET rather than by name. A subscription
// already names the instance it belongs to (so the sidecar knows where to send an
// interrupt), and resolving that instance to its live room card yields the socket
// — so the same link serves both directions and no new identity field is needed.
// Matching on a name would also be wrong: names are reused across runs, and a
// stale subscription would hand one session another's notifications.
//
// Best-effort throughout, and deliberately so: an unreadable bus must never block
// a steer. The same discipline kb uses — a missing store costs nothing and stops
// nothing.
func TurnPreamble(ctlSock string) string {
	if strings.TrimSpace(ctlSock) == "" {
		return ""
	}
	sub, ok := subscriberForCtlSock(ctlSock)
	if !ok {
		return ""
	}
	items, err := ReadPending(sub)
	if err != nil || len(items) == 0 {
		return ""
	}
	block := FormatPending(items)
	// Clear only what was read, and only after it has been rendered: the sidecar
	// may append while we are here, and truncating wholesale would discard a
	// notification the agent never learns existed.
	_ = ClearPending(sub, items[len(items)-1].Seq)
	return block
}

// subscriberForCtlSock finds the subscription whose instance is the live session
// on this control socket.
func subscriberForCtlSock(ctlSock string) (string, bool) {
	subs, err := Subscriptions()
	if err != nil {
		return "", false
	}
	for _, s := range subs {
		if s.Instance == "" {
			continue
		}
		card, found, ferr := room.Find(s.Instance)
		if ferr != nil || !found {
			continue
		}
		if card.CtlSock != "" && card.CtlSock == ctlSock {
			return s.Subscriber, true
		}
	}
	return "", false
}

// Prepend puts any pending notifications in front of the text about to be sent
// to a session.
//
// The block goes FIRST, ahead of the caller's own message, because a
// notification is context for the instruction that follows — "Foo was renamed"
// changes how the agent should read "now fix Foo". Appending it after would have
// the agent commit to an approach and only then learn the ground moved.
func Prepend(ctlSock, text string) string {
	block := TurnPreamble(ctlSock)
	if block == "" {
		return text
	}
	return block + "\n" + text
}
