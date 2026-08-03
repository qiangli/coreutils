package weave

// Conductors are addresses on the board, the same way the steward seat is.
//
//	bashy mb send conductor:22 "ready for an assignment"
//
// The address is the SPRINT, never the agent conducting it. A lease changes
// hands; sent to the holder's name, mail would follow the agent rather than the
// responsibility and a handover would silently lose it. sprintTopic already
// derives the address from the sprint id for exactly this reason.

import (
	"strconv"
	"time"

	"github.com/qiangli/coreutils/pkg/bus"
)

func init() {
	// RegisterHostRoles COMPOSES — pkg/steward registers the seat from its own
	// init, and a bare assignment here would silently replace it depending on
	// link order.
	bus.RegisterHostRoles(conductorRoles)
}

// conductorRoles lists the sprints on this repo that currently have a conductor.
//
// Only LEASED sprints. An unleased sprint has nobody accountable for it, so an
// address would accept mail on behalf of no one — the opposite of the steward
// seat, which is a standing address for a host whether or not it is claimed.
// The distinction is real: there is always exactly one steward seat per host,
// while sprints come and go by the dozen, and minting an address per backlog
// item would bury the ones that mean something.
//
// Read-only and failure-tolerant. Resolving an address must never be the thing
// that reports a broken queue: with no queue, or outside a repo, there are
// simply no conductor addresses here.
func conductorRoles() []bus.HostRole {
	dir, err := weaveQueueDirForCwd()
	if err != nil {
		return nil
	}
	q, err := loadWeaveQueue(dir)
	if err != nil || q == nil {
		return nil
	}
	now := time.Now()
	var out []bus.HostRole
	for _, s := range q.Stories {
		if s == nil || s.Lease == nil || s.Lease.Holder == "" {
			continue
		}
		// An expired lease is not a conductor. Accepting mail for one would
		// promise an owner that walked away half an hour ago.
		if now.Sub(s.Lease.At) > sprintLeaseTTL {
			continue
		}
		out = append(out, bus.HostRole{
			Label: "conductor:" + strconv.FormatInt(s.ID, 10),
			Topic: sprintTopic(s.ID),
		})
	}
	return out
}
