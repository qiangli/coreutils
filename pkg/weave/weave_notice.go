package weave

// Owner notices are lifecycle facts, not another coordination channel. The
// queue journals an event before bus delivery, so restart can replay a missed
// delivery without inferring a result from a disappeared process.

import (
	"fmt"
	"strings"
	"time"

	"github.com/qiangli/coreutils/pkg/bus"
	"github.com/qiangli/coreutils/pkg/room"
)

const weaveOwnerNoticeSchema = "bashy-owner-notice-v1"

type weaveOwnerNotice struct {
	ID            int64     `json:"id"`
	Key           string    `json:"key"`
	Type          string    `json:"type"`
	Source        string    `json:"source"`
	Repo          string    `json:"repo"`
	Run           int64     `json:"run"`
	Actor         string    `json:"actor"`
	Owner         string    `json:"owner"`
	Timestamp     time.Time `json:"timestamp"`
	TerminalState string    `json:"terminal_state,omitempty"`
	Evidence      string    `json:"evidence,omitempty"`
	Delivered     bool      `json:"delivered,omitempty"`
}

// weaveOwnerFor resolves only durable ownership. It never picks an arbitrary
// active worker merely because that worker happens to be reachable.
func weaveOwnerFor(dir string, q *weaveQueue, it *weaveItem) string {
	if l, ok := loadConductorLock(dir); ok && strings.TrimSpace(l.Holder) != "" {
		return strings.TrimSpace(l.Holder)
	}
	if q != nil && it != nil {
		for _, s := range q.Stories {
			for _, r := range s.Runs {
				if r.ID == it.ID && s.Lease != nil && strings.TrimSpace(s.Lease.Holder) != "" {
					return strings.TrimSpace(s.Lease.Holder)
				}
			}
		}
	}
	if it != nil {
		return strings.TrimSpace(it.Owner)
	}
	return ""
}

func weaveQueueOwnerNotice(dir string, q *weaveQueue, it *weaveItem, typ string) {
	if q == nil || it == nil || strings.TrimSpace(typ) == "" {
		return
	}
	owner := weaveOwnerFor(dir, q, it)
	if owner == "" {
		return
	} // no explicit accountable owner: do not guess.
	key := fmt.Sprintf("weave:%s:%d:%s:%s", q.Root, it.ID, typ, it.State)
	for _, e := range q.OwnerNotices {
		if e.Key == key {
			return
		}
	}
	q.NextOwnerNoticeID++
	evidence := it.Head
	if evidence == "" {
		evidence = it.Completion
	}
	q.OwnerNotices = append(q.OwnerNotices, weaveOwnerNotice{ID: q.NextOwnerNoticeID, Key: key, Type: typ, Source: "weave", Repo: q.Root, Run: it.ID, Actor: it.Owner, Owner: owner, Timestamp: time.Now().UTC(), TerminalState: it.State, Evidence: evidence})
}

// weaveDeliverOwnerNotices runs after the queue write. It is safe on every
// recovery path: a stable key is checked in the durable timeline before
// publishing, covering a crash after publish but before its delivery receipt.
func weaveDeliverOwnerNotices(dir string) {
	q, err := loadWeaveQueue(dir)
	if err != nil {
		return
	}
	for _, e := range q.OwnerNotices {
		if e.Delivered || e.Owner == "" {
			continue
		}
		_, _ = bus.EnsureSubscription(e.Owner) // offline owner gets a durable inbox.
		subject := weaveOwnerNoticeSubject(e)
		if !weaveOwnerNoticeOnTimeline(e.Key) {
			if err := bus.Publish(bus.Notification{Principal: "weave", To: e.Owner, Body: subject, Priority: bus.DeliveryQueued}); err != nil {
				continue
			}
			// A live addressed conversation is woken; a watcher cursor alone is
			// not delivery and does not consume this durable event.
			_ = bus.SteerLive(e.Owner, subject)
		}
		_ = withWeaveQueueLock(dir, func(fresh *weaveQueue) error {
			for i := range fresh.OwnerNotices {
				if fresh.OwnerNotices[i].ID == e.ID {
					fresh.OwnerNotices[i].Delivered = true
					break
				}
			}
			return nil
		})
	}
}

func weaveOwnerNoticeSubject(e weaveOwnerNotice) string {
	state := e.TerminalState
	if state == "" {
		state = e.Type
	}
	line := fmt.Sprintf("weave run #%d %s", e.Run, state)
	return fmt.Sprintf("%s [schema=%s event_id=%d event=%s source=%s repo=%s run=%d actor=%s owner=%s timestamp=%s terminal_state=%s evidence=%s key=%s]", line, weaveOwnerNoticeSchema, e.ID, e.Type, e.Source, e.Repo, e.Run, e.Actor, e.Owner, e.Timestamp.Format(time.RFC3339Nano), state, e.Evidence, e.Key)
}

func weaveOwnerNoticeOnTimeline(key string) bool {
	events, err := room.Timeline(0)
	if err != nil {
		return false
	}
	for _, e := range events {
		if strings.Contains(e.Body, "key="+key+"]") {
			return true
		}
	}
	return false
}
