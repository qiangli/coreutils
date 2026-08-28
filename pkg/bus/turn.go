package bus

import (
	"strings"

	"github.com/qiangli/coreutils/pkg/room"
)

// PreparedPreamble is a rendered inbox snapshot whose cursor acknowledgements
// are deliberately deferred until the embedding host has injected Text into a
// live agent successfully.
type PreparedPreamble struct {
	Text string
	ack  []func() error
}

// NewPreparedPreamble creates a host-owned preamble without exposing the
// acknowledgement list as mutable API.
func NewPreparedPreamble(text string, ack func() error) PreparedPreamble {
	p := PreparedPreamble{Text: strings.TrimSpace(text)}
	if ack != nil {
		p.ack = append(p.ack, ack)
	}
	return p
}

// Commit acknowledges the exact source snapshots represented by Text. Call it
// only after successful delivery. Repeated calls are safe because every source
// cursor is monotonic.
func (p PreparedPreamble) Commit() error {
	for _, ack := range p.ack {
		if err := ack(); err != nil {
			return err
		}
	}
	return nil
}

// PrepareTurnInbox is the embedding host's read-through view of communication
// stores that pkg/bus cannot import (for example Meet, which already imports
// bus). It owns no storage and must acknowledge only records it successfully
// rendered. Bashy wires it to its unified inbox; a bare coreutils embedding
// leaves it nil and retains the bus-only behaviour.
var PrepareTurnInbox func(agent string) PreparedPreamble

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
	p := PrepareTurnPreamble(ctlSock)
	_ = p.Commit()
	return p.Text
}

// PrepareTurnPreamble snapshots pending input for a verified live control
// socket but does not advance any cursor. The caller commits after injection.
func PrepareTurnPreamble(ctlSock string) PreparedPreamble {
	if strings.TrimSpace(ctlSock) == "" {
		return PreparedPreamble{}
	}
	sub, ok := subscriberForCtlSock(ctlSock)
	if !ok {
		return PreparedPreamble{}
	}
	// A sidecar is an immediacy optimization, not a correctness prerequisite.
	// Reconcile the timeline and pending view now so a Bashy-owned session still
	// receives Bus input when no sidecar process is running.
	snapshot, err := SnapshotInbox(sub)
	if err != nil {
		return PreparedPreamble{}
	}
	p := PreparedPreamble{}
	if len(snapshot.Items) > 0 {
		p.Text = FormatPending(snapshot.Items)
		p.ack = append(p.ack, snapshot.Commit)
	}
	if PrepareTurnInbox != nil {
		p = mergePrepared(p, PrepareTurnInbox(sub))
	}
	return p
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
	p := PreparePrepend(ctlSock, text)
	_ = p.Commit()
	return p.Text
}

// PreparePrepend is Prepend with deferred cursor acknowledgement.
func PreparePrepend(ctlSock, text string) PreparedPreamble {
	p := PrepareTurnPreamble(ctlSock)
	if p.Text == "" {
		p.Text = text
		return p
	}
	p.Text = p.Text + "\n" + text
	return p
}

// LaunchPreamble is the compatibility immediate-ack form. Bashy chat uses the
// prepared form below so a refused launch cannot consume its mail.
func LaunchPreamble(agent string) string {
	p := PrepareLaunchPreamble(agent)
	_ = p.Commit()
	return p.Text
}

// PrepareLaunchPreamble snapshots everything addressed to agent without
// advancing cursors. The caller commits after the launch accepts the prompt.
func PrepareLaunchPreamble(agent string) PreparedPreamble {
	agent = strings.TrimSpace(agent)
	if agent == "" {
		return PreparedPreamble{}
	}
	snapshot, err := SnapshotInbox(agent)
	if err != nil {
		return PreparedPreamble{}
	}
	p := PreparedPreamble{}
	if len(snapshot.Items) > 0 {
		p.Text = FormatPending(snapshot.Items)
		p.ack = append(p.ack, snapshot.Commit)
	}
	if PrepareTurnInbox != nil {
		p = mergePrepared(p, PrepareTurnInbox(agent))
	}
	return p
}

func mergePrepared(a, b PreparedPreamble) PreparedPreamble {
	a.Text = joinPreambles(a.Text, b.Text)
	a.ack = append(a.ack, b.ack...)
	return a
}

// PrepareForAgent puts a cold agent's unread mail ahead of its opening prompt
// and defers acknowledgement until the prompt is accepted.
func PrepareForAgent(agent, text string) PreparedPreamble {
	p := PrepareLaunchPreamble(agent)
	if p.Text == "" {
		p.Text = text
		return p
	}
	p.Text = p.Text + "\n" + text
	return p
}

func joinPreambles(a, b string) string {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	switch {
	case a == "":
		return b
	case b == "":
		return a
	default:
		return a + "\n\n" + b
	}
}

// PrependForAgent puts an agent's unread mail in front of the text it is about
// to be given. Returns text unchanged when there is nothing.
func PrependForAgent(agent, text string) string {
	p := PrepareForAgent(agent, text)
	_ = p.Commit()
	return p.Text
}
