package weave

import (
	"strings"
	"testing"
	"time"

	"github.com/qiangli/coreutils/pkg/room"
)

func TestOwnerNoticeIsDurableDeduplicatedAndRedacted(t *testing.T) {
	dir, root := newQueueInTempRepo(t)
	q := &weaveQueue{Root: root, Items: []*weaveItem{{ID: 7, Owner: "worker", State: "submitted", Head: "abc123"}}}
	if err := saveConductorLock(dir, &ConductorLock{Holder: "accountable-conductor", HeartbeatAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	weaveQueueOwnerNotice(dir, q, q.Items[0], "run-terminal")
	if len(q.OwnerNotices) != 1 || q.OwnerNotices[0].Owner != "accountable-conductor" {
		t.Fatalf("notice owner = %#v", q.OwnerNotices)
	}
	if err := saveWeaveQueue(dir, q); err != nil {
		t.Fatal(err)
	}
	weaveDeliverOwnerNotices(dir)
	weaveDeliverOwnerNotices(dir) // replay must not duplicate the durable event.
	events, err := room.Timeline(0)
	if err != nil {
		t.Fatal(err)
	}
	var notices []string
	for _, e := range events {
		if strings.Contains(e.Body, "event_id=") {
			notices = append(notices, e.Body)
		}
	}
	if len(notices) != 1 {
		t.Fatalf("owner notices = %d, want one: %#v", len(notices), notices)
	}
	got := notices[0]
	for _, want := range []string{"weave run #7 submitted", "schema=bashy-owner-notice-v1", "event=run-terminal", "owner=accountable-conductor", "terminal_state=submitted", "evidence=abc123"} {
		if !strings.Contains(got, want) {
			t.Errorf("notice missing %q: %s", want, got)
		}
	}
	if strings.Contains(got, "secret prompt") || strings.Contains(got, "VerifyOutput") {
		t.Fatalf("notice leaked body/log material: %s", got)
	}
}
