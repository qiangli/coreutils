package weave

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSprintResourceInventorySeparatesRecycledRunsAndKeepsCompetitors(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	store := filepath.Join(home, "sprint")
	t.Setenv("BASHY_SPRINT_DIR", store)
	write := func(path string, q weaveQueue) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		b, _ := json.Marshal(q)
		if err := os.WriteFile(path, b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	born := time.Now().UTC()
	write(filepath.Join(store, "queue.json"), weaveQueue{Stories: []*weaveStory{{ID: 138, Owner: "manager", Runs: []sprintRun{{Repo: "repo", Queue: "repo-hash", ID: 1, Born: born}, {Repo: "repo", Queue: "repo-hash", ID: 2, Born: born.Add(-time.Hour)}}}}})
	write(filepath.Join(weaveStateRoot(home), "repo-hash", "queue.json"), weaveQueue{Root: "/repo", Items: []*weaveItem{{ID: 1, Created: born, State: "working", Owner: "worker", WrapperPid: 123, WrapperStartID: "fixture:old"}, {ID: 2, Created: born, State: "working", Owner: "competitor", WrapperPid: 456}}})
	cache := filepath.Join(home, "cache")
	got, err := ReadSprintInventory(context.Background(), cache)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Complete || len(got.Workloads) != 2 {
		t.Fatalf("inventory=%+v", got)
	}
	if got.Workloads[0].Sprint != 138 || got.Workloads[0].StartID != "fixture:old" || got.Workloads[1].Sprint != 0 || got.Workloads[0].ID == got.Workloads[1].ID {
		t.Fatalf("run generations/competitors lost: %+v", got.Workloads)
	}
	before, err := os.Stat(filepath.Join(cache, "sprint-inventory.json"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := ReadSprintInventory(context.Background(), cache)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := os.Stat(filepath.Join(cache, "sprint-inventory.json"))
	if !second.At.Equal(got.At) || !after.ModTime().Equal(before.ModTime()) {
		t.Fatal("unchanged observation rewrote shared cache")
	}
	if _, err := os.Stat(filepath.Join(store, "queue.lock")); !os.IsNotExist(err) {
		t.Fatalf("observation touched queue mutation lock: %v", err)
	}
}

func TestSprintResourceOwnerFenceRefusesFormerOwner(t *testing.T) {
	lease := seedSprintLease(t, "current")
	before := lease()
	sent := false
	err := WithSprintObservationOwner(context.Background(), 98, "former", func() error { sent = true; return nil })
	if err == nil || sent {
		t.Fatal("former owner received a resource notice")
	}
	if after := lease(); after != before {
		t.Fatal("observation changed lease")
	}
}
