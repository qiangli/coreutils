package bus

// R0 — every identity has an inbox, always.
//
// `Subscription.Matches` delivers a 1:1 notification only when a STORED
// subscription carries `To: <name>`. Subscriptions were opt-in, created by
// `bus subscribe` and nothing else — so on a real host there were NONE, and a
// notification addressed to an agent matched nothing and reached nobody. The
// message was durable and undelivered, which is the same failure as a meeting
// room nobody polls, on better bones.
//
// Opt-in addressing is the shape of an opt-in smoke alarm. The address book
// (`bashy agents list`) enumerates the fleet, so the fleet is exactly the set
// that needs inboxes, and creating them is a reconciliation rather than a
// decision.
//
// # An inbox, never a doorbell
//
// The default subscription grants the power to be REACHED and nothing else:
//
//	To             the agent's own name — the DM address, and the whole point
//	Topics         EMPTY. An inbox is not a subscription to the firehose;
//	               broadcast interest stays something an operator asks for.
//	InterruptFrom  EMPTY, which means NOBODY. Auto-subscribe must never hand
//	               out the power to break into a turn — that is the governance
//	               boundary the subscription type defaults closed on purpose,
//	               and widening it as a side effect of "make agents reachable"
//	               would turn an inbox into an attack surface.
//
// So a message to a freshly-reconciled agent is QUEUED and read at a turn
// boundary. Interruption remains granted by name, per the report/author split
// the fleet uses everywhere else.
//
// # It never overwrites
//
// An existing subscription is left exactly as it is. An operator who tuned
// InterruptFrom or MaxPerMin has expressed a policy, and a reconciliation that
// silently reset it would be a security regression disguised as a repair.
//
// # A new inbox starts at the HEAD, not at zero
//
// Since is the sidecar's read offset, and the sidecar delivers everything after
// it. A subscription created with Since = 0 would hand its agent the entire
// timeline the moment it first connects — every unrelated notification the host
// has ever emitted, as if it were new. So a new inbox opens at the current head:
// it receives what arrives from now on, which is what "you now have an address"
// means. Nothing addressed to the agent is lost, because before this there was
// no address to lose it from.

import (
	"fmt"
	"strings"

	"github.com/qiangli/coreutils/pkg/room"
)

// FleetNames is the seam to the agent catalog, injected by the host.
//
// pkg/bus must not import pkg/fleet: the bus is the transport and the catalog
// is policy, and a transport that knows the roster is one you cannot test
// without one. This mirrors steward.OpenRoom and fleet's own LiveProbe — the
// host wires the real thing, and a nil hook simply means the fleet is not
// reconcilable here rather than an error.
var FleetNames func() []string

// EnsureSubscription gives subscriber a default inbox if it has none.
//
// Returns true when one was created. Idempotent: an existing subscription is
// returned untouched, so this is safe to call on every catalog load.
func EnsureSubscription(subscriber string) (bool, error) {
	subscriber = strings.TrimSpace(subscriber)
	if subscriber == "" {
		return false, fmt.Errorf("bus: an inbox needs a subscriber name")
	}
	if existing, err := LoadSubscription(subscriber); err == nil && existing.Subscriber != "" {
		return false, nil
	}
	return true, SaveSubscription(Subscription{
		Subscriber: subscriber,
		To:         subscriber,
		// Topics, InterruptFrom and MaxPerMin are deliberately left at their
		// zero values — see the package comment. An inbox, not a doorbell.
		Since: timelineHead(),
	})
}

// ReconcileSubscriptions ensures an inbox for every name, and reports which
// ones it had to create.
//
// The report is the point: it makes "every identity is addressable" a checkable
// claim rather than an assumption, which is what the address book rests on.
// A name that fails is collected and reported rather than aborting the sweep —
// one bad name must not leave the rest of the fleet unaddressable.
func ReconcileSubscriptions(names []string) (created []string, err error) {
	var failed []string
	for _, n := range names {
		switch made, e := EnsureSubscription(n); {
		case e != nil:
			failed = append(failed, n)
		case made:
			created = append(created, n)
		}
	}
	if len(failed) > 0 {
		return created, fmt.Errorf("bus: could not open an inbox for %s", strings.Join(failed, ", "))
	}
	return created, nil
}

// timelineHead is the sequence a new inbox starts from.
//
// A read failure yields 0, and that is the safe direction here: the cost is a
// replay of history the agent did not need, which is noise. The opposite
// default — skipping ahead past events on an unreadable timeline — would drop
// messages, and DEMOTE-NEVER-DROP forbids trading a delivery for tidiness.
func timelineHead() int64 {
	events, err := room.Timeline(1)
	if err != nil || len(events) == 0 {
		return 0
	}
	return events[len(events)-1].Seq
}
