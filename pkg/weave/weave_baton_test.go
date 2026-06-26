package weave

import (
	"strings"
	"testing"
)

func TestBatonRoundTripAndRender(t *testing.T) {
	dir := t.TempDir()
	bt := &Baton{Goal: "close the bash-gap", Stage: "sprint 1 of 2",
		Done: []string{"#258 cd merged"}, NextActions: []string{"reassign #259 to claude"},
		Lessons: []string{"codex not steerable"}, WrittenBy: "claude"}
	if err := saveBaton(dir, bt); err != nil {
		t.Fatal(err)
	}
	got, ok := loadBaton(dir)
	if !ok || got.Goal != bt.Goal || len(got.NextActions) != 1 {
		t.Fatalf("round-trip failed: %+v", got)
	}
	md := renderBaton(got)
	for _, want := range []string{"close the bash-gap", "sprint 1 of 2", "reassign #259", "Reconcile with live state"} {
		if !strings.Contains(md, want) {
			t.Fatalf("render missing %q", want)
		}
	}
}
