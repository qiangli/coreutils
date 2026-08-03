package bus

// RESOLVE ON READ — the half of delivery that needs no daemon.
//
// The sidecar pre-resolves notifications off an agent's critical path, which is
// the right design for INTERRUPTS: breaking into a running turn has to happen
// while the turn is running, so somebody must be watching. But nothing was
// watching. On a real host `bus subscriptions` was empty and no sidecar process
// existed, so a notification sat in the timeline matching nothing, and even
// after R0 gave every identity an inbox it stayed unresolved until a human
// happened to run `bus sidecar --once`.
//
// The observation that closes the gap cheaply: QUEUEING NEEDS NO SOCKET. Only
// an interrupt does. Everything else is "append to the agent's buffer", and the
// agent asking for its buffer is a perfectly good moment to do it.
//
// So `bus pending` resolves for its own subscriber before reading. That is the
// (c) half of the agreed (a)+(c) model, and it covers the cases (a) cannot:
//
//	cold          not running when the message was sent; resolves on next read
//	shell-only    running under bashy but not bashy-LAUNCHED, so it has no
//	              control socket and no sidecar could ever push to it
//	no sidecar    nobody started one, which is the state every host is in
//
// The sidecar remains the optimisation, not the requirement. A host with one
// gets interrupts; a host without one still gets every message, one turn later.
//
// # Why an interrupt is not attempted here
//
// The agent is READING RIGHT NOW. Urgency has already been satisfied by the act
// of asking, so re-delivering as an interrupt would break into the very turn
// that came to collect it. Entries resolved on this path are therefore recorded
// as queued — which is what actually happened, and a record that says what
// happened is the whole point.

import (
	"github.com/qiangli/coreutils/pkg/room"
)

// ResolveFor delivers everything subscriber has not yet been given, into its
// pending buffer. Returns how many were queued.
//
// Ordering is the sidecar's rule, and it is not negotiable: the subscription's
// offset advances only AFTER the buffer has been written. Advancing first and
// then failing to append would consume a notification the agent never learns
// existed — a silent drop, which leaves it acting on stale assumptions. A
// duplicate is recoverable; a loss is not.
//
// A subscriber with no subscription resolves nothing and is not an error: after
// R0 that means a name outside the address book, and refusing to print an
// empty buffer for it would help nobody.
func ResolveFor(subscriber string) (int, error) {
	sub, err := LoadSubscription(subscriber)
	if err != nil || sub.Subscriber == "" {
		return 0, nil
	}
	events, err := room.Timeline(0)
	if err != nil {
		return 0, err
	}

	var high int64
	queued := 0
	for _, e := range events {
		if e.Seq > high {
			high = e.Seq
		}
		if e.Seq <= sub.Since || !sub.Matches(e) {
			continue
		}
		// DeliveryQueued unconditionally: see the package comment. The caller is
		// reading, so an interrupt would break into the turn that came to collect
		// the message.
		if err := AppendPending(subscriber, Pending{
			SchemaVersion: SchemaVersion,
			Seq:           e.Seq, TS: e.TS,
			Principal: e.Principal, Topic: e.Topic, To: e.To, Room: e.Room,
			Body:     e.Body,
			Delivery: DeliveryQueued,
		}); err != nil {
			return queued, err
		}
		queued++
	}

	if queued > 0 || high > sub.Since {
		sub.Since = high
		if err := SaveSubscription(sub); err != nil {
			return queued, err
		}
	}
	return queued, nil
}
