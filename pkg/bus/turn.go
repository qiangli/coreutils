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
	items, err := UnreadPending(sub)
	if err != nil || len(items) == 0 {
		return ""
	}
	block := FormatPending(items)
	// Mark only what was read, and only after it has been rendered: the sidecar
	// may append while we are here, and truncating wholesale would discard a
	// notification the agent never learns existed.
	_ = MarkRead(sub, items[len(items)-1].Seq)
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

// LaunchPreamble is the "unread on login" block: everything addressed to agent
// that it has not yet been shown, rendered once and cleared.
//
// It exists because TurnPreamble cannot serve a LAUNCH. That one keys on the
// control socket, and at launch there is no socket yet — it is created by the
// very call that needs this. An agent is known by NAME before it is known by
// address, so the launch path resolves by name.
//
// It RESOLVES before reading, so a cold agent — one that was not running when
// the message was sent, and for which no sidecar was watching — picks up its
// mail on the way in. That is the whole point: the common case on a real host
// is that nobody was watching, and an agent that only ever learns of messages
// sent while it happened to be up is not reachable in any useful sense.
//
// Empty when there is nothing, so the caller can concatenate unconditionally.
func LaunchPreamble(agent string) string {
	agent = strings.TrimSpace(agent)
	if agent == "" {
		return ""
	}
	// Best-effort: a resolve failure still lets whatever is already buffered
	// through. Delivering some mail beats delivering none because the timeline
	// was briefly unreadable.
	_, _ = ResolveFor(agent)

	items, err := UnreadPending(agent)
	if err != nil || len(items) == 0 {
		return ""
	}
	block := FormatPending(items)
	// Mark only what was rendered, and only after rendering — the same ordering
	// TurnPreamble uses, for the same reason: anything appended in between must
	// survive to the next read rather than being truncated away unseen.
	_ = MarkRead(agent, items[len(items)-1].Seq)
	return block
}

// PrependForAgent puts an agent's unread mail in front of the text it is about
// to be given. Returns text unchanged when there is nothing.
func PrependForAgent(agent, text string) string {
	block := LaunchPreamble(agent)
	if block == "" {
		return text
	}
	return block + "\n" + text
}
