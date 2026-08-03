package steward

// REACHING THE STEWARD — the point of contact, contactable.
//
// A steward is the single point of contact for one login on one host: the seat
// anything addressed to that machine-and-login goes through, and the one that
// answers for what happened there. It was the one role in the tree you could
// not actually reach. `sprint ping` could interrupt a conductor; nothing could
// interrupt the person's own POC, which for a messenger is backwards.
//
// # It addresses the SEAT, not the holder
//
// The topic is the scope id, so a message survives a handoff or a takeover: the
// holder changes, the responsibility does not. Addressed to a name, a message
// sent an hour ago would arrive for somebody who has since released the seat —
// delivered to an agent with no context, or to nobody at all.
//
// # A stale seat is still worth sending to, and worth flagging
//
// The bus holds what it cannot deliver (demote, never drop), so a ping to a
// lapsed steward is not lost — it waits. What would be wrong is letting the
// sender believe it was READ. So the send succeeds and says plainly that the
// seat has not heartbeated, which is a different sentence from "delivered".
//
// # Why not just post in the room
//
// The room is where a conversation happens; the bus is what makes somebody look
// at it. An agent mid-turn cannot decide to go and check a room — its sidecar
// holds the subscription off the critical path and hands it a buffer at a turn
// boundary. Posting without pinging is leaving a note on a desk nobody is at.

import (
	"fmt"
	"strings"
	"time"

	"github.com/qiangli/coreutils/pkg/bus"
)

// Ping sends an interrupt to whoever holds this host's steward seat.
//
// It returns the line to show the sender: who it went to, where to expect a
// reply, and — when the seat has not heartbeated — that the message is queued
// rather than read.
func Ping(dir, from, body, priority string) (string, error) {
	return ping(dir, from, body, priority)
}

// ping takes the store options a test needs to stay inside its own registry —
// the seat is a singleton, so a test that opened a second store for one would
// be refused, and rightly.
func ping(dir, from, body, priority string, opts ...Option) (string, error) {
	if strings.TrimSpace(body) == "" {
		return "", fmt.Errorf("nothing to send — `--body \"<what you need>\"`")
	}
	st, err := Open(dir, opts...)
	if err != nil {
		return "", err
	}
	view, err := st.Status(time.Now())
	if err != nil {
		return "", err
	}
	if view.Authority.Vacant {
		// Sending to an empty seat would queue a message for nobody, and the
		// bus would hold it indefinitely. Refusing says the useful thing
		// instead: there is no POC here, and claiming the seat is how one
		// appears.
		// The readable label, not the scope id: the id is what a topic needs,
		// a person needs to know which machine and login.
		return "", fmt.Errorf("%s has no steward to ping — the seat is open (`bashy steward claim`)",
			seatLabel())
	}

	// Address the SEAT, which is what --help has always promised. The holder's
	// name is a TOOL (`codex`), every bus subscriber is an AGENT
	// (`codex-gpt5.6-sol`), and `To: Holder.Name` therefore matched nothing —
	// the ping was published, durable, and unreadable by any read verb. See
	// bus.EnsureRoleInbox.
	topic := stewardAssignment().Topic()

	// Idempotent, and called on the SEND path on purpose: a ping must not depend
	// on a claim having run under an earlier version that did not open the inbox.
	// A failure here is reported rather than swallowed — the whole defect being
	// fixed is a send that looked like it worked.
	if _, ierr := bus.EnsureRoleInbox(topic); ierr != nil {
		return "", fmt.Errorf("steward ping: cannot open the seat inbox: %w", ierr)
	}

	n := bus.Notification{
		Principal: from,
		Topic:     topic,
		To:        topic,
		Body:      body,
		Priority:  priority,
	}
	if c, cerr := loadSeatContact(); cerr == nil && c != nil {
		n.Room = c.Ref
	}
	if err := bus.Publish(n); err != nil {
		return "", err
	}

	line := fmt.Sprintf("pinged %s on %s", view.Authority.Holder, n.Topic)
	if c, cerr := loadSeatContact(); cerr == nil && c != nil {
		line += " — reply in " + c.String()
	}
	if view.Liveness != LivenessLive {
		// Sent, and honest about what that means. The bus holds it; nobody has
		// necessarily read it.
		line += fmt.Sprintf("\n  note: the seat is %s, not live — the ping is QUEUED, not read", view.Liveness)
		if view.Liveness == LivenessLapsed {
			line += "\n  a lapsed seat is takeable: `bashy steward takeover` if this needs an answer now"
		}
	}
	return line, nil
}
