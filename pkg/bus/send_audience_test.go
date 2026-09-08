package bus

import (
	"errors"
	"strings"
	"testing"
)

// Send's own contract: "an unresolvable target writes NOTHING and fails with
// choices — a post to a name nobody answers was a receipt indistinguishable
// from a real delivery." That held for --to but NOT for a selector: the
// audience was resolved AFTER the durable append and its error was discarded,
// so `mb send --role reviewer` posted to the board and reported success while
// reaching nobody.
func TestSendAudienceWritesNothingWhenTheSelectorCannotResolve(t *testing.T) {
	t.Setenv("BASHY_ROOM_DIR", t.TempDir())

	prev := FleetSelect
	t.Cleanup(func() { FleetSelect = prev })
	FleetSelect = func(Audience) ([]string, error) {
		return nil, errors.New("unknown role \"reviewer\"")
	}

	before := boardLen(t)
	_, err := Send(SendRequest{From: "tester", Audience: &Audience{Role: "reviewer"}, Body: "x"})
	if err == nil {
		t.Fatal("Send succeeded on an unresolvable selector; a receipt nobody can act on is the defect")
	}
	if !strings.Contains(err.Error(), "reviewer") {
		t.Errorf("error should name the unresolvable selector, got %v", err)
	}
	if after := boardLen(t); after != before {
		t.Errorf("board grew from %d to %d — a failed send must write NOTHING", before, after)
	}
}

// The resolving case still posts once and reports the recipients it reached.
func TestSendAudienceStillPostsWhenTheSelectorResolves(t *testing.T) {
	t.Setenv("BASHY_ROOM_DIR", t.TempDir())

	prev := FleetSelect
	t.Cleanup(func() { FleetSelect = prev })
	FleetSelect = func(Audience) ([]string, error) { return []string{"zoe"}, nil }

	res, err := Send(SendRequest{From: "tester", Audience: &Audience{Role: "conductor"}, Body: "peers only"})
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	if len(res.Deliveries) != 1 || res.Deliveries[0].To != "zoe" {
		t.Fatalf("deliveries = %+v, want one to zoe", res.Deliveries)
	}
	if !strings.Contains(res.Label, "conductor") {
		t.Errorf("label = %q, should name the selector so the receipt is readable", res.Label)
	}
}

// Role participates in Empty, or a role-only selector would be treated as no
// selector at all and silently broadcast to everyone.
func TestRoleCountsAsASelector(t *testing.T) {
	if (Audience{Role: "conductor"}).Empty() {
		t.Fatal("a role-only Audience reports Empty, so it would fall through to a broadcast")
	}
}

func boardLen(t *testing.T) int {
	t.Helper()
	posts, err := Posts()
	if err != nil {
		return 0
	}
	return len(posts)
}
