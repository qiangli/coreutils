// Package meetroom implements pkg/role's rooms on top of `bashy meet`.
//
// It is separate from pkg/role because meet transitively pulls the shell
// interpreter, and pkg/steward — which needs the role VOCABULARY — sits in the
// cross-OS build canary that interpreter cannot satisfy. Keeping the words in
// one package and the transport in another means naming a role costs nothing.
package meetroom

import (
	"fmt"
	"strings"

	"github.com/qiangli/coreutils/pkg/meet"
	"github.com/qiangli/coreutils/pkg/role"
)

// Assume opens the room for a newly-held role.
//
// The holder joins as a PARTICIPANT rather than as the chair: meet requires a
// chair to have someone to call on, and this room deliberately has no roster
// when it opens — existing before anyone needs it is the entire point.
func Assume(a role.Assignment, holder string) (*role.Contact, error) {
	if strings.TrimSpace(a.Ref) == "" {
		return nil, fmt.Errorf("role: assignment has no ref")
	}
	subject := fmt.Sprintf("%s %s", a.Kind, a.Ref)
	if t := strings.TrimSpace(a.Title); t != "" {
		subject += ": " + t
	}
	st, err := meet.Create(meet.CreateOptions{
		Topic:        subject,
		Participants: []string{holder},
		Initiator:    holder,
		Agenda:       agendaFor(a.Kind),
	})
	if err != nil {
		return nil, err
	}
	if st == nil || st.ID == "" {
		return nil, fmt.Errorf("role: meet returned no room")
	}
	return &role.Contact{Kind: "meet", Ref: st.ID, Room: st.Room, Topic: a.Topic()}, nil
}

// Release closes a role's room on a graceful handoff.
//
// A room that is already gone is NOT an error: the point of releasing is that
// the room ends up closed, and a successor sweeping a dead holder's room may
// have closed it first. Reporting that as a failure would make the honest path
// look broken.
func Release(c *role.Contact, holder string) error {
	if c == nil || c.Ref == "" {
		return nil
	}
	err := meet.Close(c.Ref, holder)
	if err != nil && isGone(err) {
		return nil
	}
	return err
}

// Sweep closes the rooms of roles whose holder is no longer live.
//
// This is the half no verb can cover. A holder that died runs nothing, so
// without a sweep its room stays open forever, advertising a channel to
// somebody who will never read it — and an unanswered room is more damaging
// than an absent one, because it consumes the time of whoever trusted it.
func Sweep(open []role.Occupied, actor string) role.SweepResult {
	res := role.SweepResult{Failed: map[string]error{}}
	for _, o := range open {
		if o.Live || o.Contact == nil || o.Contact.Ref == "" {
			continue
		}
		if err := Release(o.Contact, actor); err != nil {
			res.Failed[o.Label] = err
			continue
		}
		res.Closed = append(res.Closed, o.Label)
	}
	return res
}

func agendaFor(k role.Kind) []string {
	switch k {
	case role.Steward:
		return []string{"what is running on this host", "contention", "handoff"}
	default:
		return []string{"delivery", "blockers", "handoff"}
	}
}

// isGone reports an error that means the room is already closed or missing —
// the desired end state either way.
func isGone(err error) bool {
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "not found") || strings.Contains(s, "closed") ||
		strings.Contains(s, "no such")
}
