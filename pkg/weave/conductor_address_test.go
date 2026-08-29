package weave

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/qiangli/coreutils/pkg/bus"
)

// A conductor address exists only while somebody is ACCOUNTABLE for the sprint.
//
// The distinction against the steward seat is deliberate and worth pinning:
// there is exactly one steward seat per host and it is a standing address
// whether or not it is claimed, because "nobody is stewarding this host" is
// itself a message worth delivering. Sprints come and go by the dozen, so an
// address per backlog item would bury the ones that mean something — and an
// address for an unleased sprint would accept mail on behalf of no one.
func TestConductorRoles_OnlyLiveLeasesGetAnAddress(t *testing.T) {
	now := time.Now()
	q := &weaveQueue{Stories: []*weaveStory{
		{ID: 1, Lease: &weaveStoryLease{Holder: "codex-gpt5.6-sol", At: now}},
		{ID: 2}, // no lease at all
		{ID: 3, Lease: &weaveStoryLease{Holder: "", At: now}}, // lease with no holder
		{ID: 4, Lease: &weaveStoryLease{ // EXPIRED
			Holder: "claude-opus5", At: now.Add(-2 * sprintLeaseTTL)}},
	}}

	got := rolesFromQueue(q, now)
	if len(got) != 1 {
		t.Fatalf("got %d addresses, want exactly the one live lease: %+v", len(got), got)
	}
	if got[0].Label != "conductor:1" {
		t.Errorf("label = %q, want the name a person types", got[0].Label)
	}
	if got[0].Topic != sprintTopic(1) {
		t.Errorf("topic = %q, want the sprint's stable address", got[0].Topic)
	}
	if got[0].Holder != "codex-gpt5.6-sol" {
		t.Errorf("holder = %q, want the live lease holder", got[0].Holder)
	}
}

// The address is the SPRINT, never the agent conducting it. A lease changes
// hands; sent to the holder's name, mail would follow the agent rather than the
// responsibility and a handover would silently lose it.
func TestConductorRoles_AddressIsTheSprintNotItsHolder(t *testing.T) {
	now := time.Now()
	first := rolesFromQueue(&weaveQueue{Stories: []*weaveStory{
		{ID: 7, Lease: &weaveStoryLease{Holder: "codex-gpt5.6-sol", At: now}},
	}}, now)
	second := rolesFromQueue(&weaveQueue{Stories: []*weaveStory{
		{ID: 7, Lease: &weaveStoryLease{Holder: "claude-opus5", At: now}},
	}}, now)

	if len(first) != 1 || len(second) != 1 {
		t.Fatalf("expected one address each, got %d and %d", len(first), len(second))
	}
	if first[0].Topic != second[0].Topic {
		t.Fatalf("the address changed with the holder (%q -> %q) — mail would not survive a handover",
			first[0].Topic, second[0].Topic)
	}
}

// No queue, or no sprints, is not an error: resolving an address must never be
// the thing that reports a broken queue.
func TestConductorRoles_EmptyIsNotAnError(t *testing.T) {
	if got := rolesFromQueue(nil, time.Now()); got != nil {
		t.Errorf("nil queue produced %+v", got)
	}
	if got := rolesFromQueue(&weaveQueue{}, time.Now()); got != nil {
		t.Errorf("empty queue produced %+v", got)
	}
}

// Role discovery is part of every unified-inbox snapshot. Looking for a role
// in a repository that has never used weave must not create the repository's
// queue tag (or even the top-level weave state directory).
func TestConductorRoles_AbsentQueueIsSideEffectFree(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := filepath.Join(t.TempDir(), "repo")
	if err := os.Mkdir(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "init", "-q", repo).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	t.Chdir(repo)
	root, err := weaveRepoRoot(repo)
	if err != nil {
		t.Fatal(err)
	}

	tag, _ := weaveQueueNames(root)
	wantAbsent := filepath.Join(weaveStateRoot(home), tag)
	for i := 0; i < 3; i++ {
		if got := conductorRoles(); got != nil {
			t.Fatalf("conductorRoles() = %+v, want no roles", got)
		}
		if bus.HostRoles != nil {
			_ = bus.HostRoles()
		}
		_ = bus.RoleLabelFor("conductor.999")
		_ = bus.AddressedToRole("conductor:999")
	}

	if _, err := os.Stat(wantAbsent); !os.IsNotExist(err) {
		t.Fatalf("read-only role lookup created queue tag %s (stat err %v)", wantAbsent, err)
	}
	if _, err := os.Stat(weaveStateRoot(home)); !os.IsNotExist(err) {
		t.Fatalf("read-only role lookup created weave root %s (stat err %v)", weaveStateRoot(home), err)
	}
}

func TestConductorRoles_ReadOnlyResolverFindsExistingQueue(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := filepath.Join(t.TempDir(), "repo")
	if err := os.Mkdir(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "init", "-q", repo).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	t.Chdir(repo)
	root, err := weaveRepoRoot(repo)
	if err != nil {
		t.Fatal(err)
	}
	dir, err := weaveQueueDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := saveWeaveQueue(dir, &weaveQueue{Stories: []*weaveStory{{
		ID: 9, Lease: &weaveStoryLease{Holder: "codex", At: time.Now()},
	}}}); err != nil {
		t.Fatal(err)
	}

	got := conductorRoles()
	if len(got) != 1 || got[0].Label != "conductor:9" || got[0].Holder != "codex" {
		t.Fatalf("conductorRoles() = %+v, want existing live queue role", got)
	}
}
