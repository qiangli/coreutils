// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package issue

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	return New(t.TempDir())
}

// The register is COMMITTED SOURCE, not scratch. A requirement must travel with the
// clone, show up in a diff, and survive the machine it was typed on — which is exactly
// what `sdlc issue` got wrong by writing into .bashy/GENERATED/.
func TestRegisterLivesInTheRepoNotInScratch(t *testing.T) {
	if strings.Contains(Dir, "generated") {
		t.Fatalf("the register is at %q — a generated/ path is derived scratch, and a requirement is source", Dir)
	}
	s := newStore(t)
	it := &Issue{Kind: KindBug, Title: "trap prints the wrong name"}
	path, err := s.Add(it)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(path, filepath.Join(s.Root, Dir)) {
		t.Fatalf("issue written to %q, want inside %q", path, Dir)
	}
	// Readable with `cat`, reviewable in a diff, editable with any editor.
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(b), "---\n") || !strings.Contains(string(b), "kind: bug") {
		t.Fatalf("an issue must be plain markdown + frontmatter; got:\n%s", b)
	}
}

// IDs are content-free hashes, NOT #1/#2 — because the register is committed, so it
// MERGES. A monotonic counter is a merge-conflict generator: two branches both file
// "#7", one gets renumbered, and every reference to it breaks. This is why git-bug and
// Fossil use hashes and GitHub can use integers (a server hands them out).
func TestIDsDoNotCollideAcrossBranches(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 500; i++ {
		id := NewID()
		if seen[id] {
			t.Fatalf("id %q minted twice — a register that reuses ids silently merges two issues into one", id)
		}
		seen[id] = true
	}
}

// Referring to an issue by a unique prefix, exactly like a git commit.
func TestResolveByPrefixAndAmbiguity(t *testing.T) {
	s := newStore(t)
	a := &Issue{ID: "aaaa1111", Kind: KindBug, Title: "first"}
	b := &Issue{ID: "aaaa2222", Kind: KindBug, Title: "second"}
	c := &Issue{ID: "bbbb3333", Kind: KindBug, Title: "third"}
	for _, it := range []*Issue{a, b, c} {
		if _, err := s.Add(it); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.Resolve("bbbb")
	if err != nil || got.ID != "bbbb3333" {
		t.Fatalf("Resolve(bbbb) = %v, %v; want bbbb3333", got, err)
	}
	// An exact id always wins, even when it is also a prefix of nothing else.
	if got, err := s.Resolve("aaaa1111"); err != nil || got.ID != "aaaa1111" {
		t.Fatalf("an exact id did not resolve: %v %v", got, err)
	}
	// An ambiguous prefix must NAME the candidates, not pick one at random.
	_, err = s.Resolve("aaaa")
	if err == nil {
		t.Fatal("an ambiguous prefix resolved to one issue — silently picking is how the wrong issue gets closed")
	}
	if !strings.Contains(err.Error(), "aaaa1111") || !strings.Contains(err.Error(), "aaaa2222") {
		t.Fatalf("the ambiguity error does not name the candidates: %v", err)
	}
}

// An issue is FILED in one repo but may be ABOUT several. A bug whose fix spans a
// library and its consumer is one issue, not two.
func TestIssueSpansReposViaRefs(t *testing.T) {
	s := newStore(t)
	it := &Issue{Kind: KindBug, Title: "trap name", Refs: []string{"../sh", "../coreutils"}}
	if _, err := s.Add(it); err != nil {
		t.Fatal(err)
	}
	got, err := s.Resolve(it.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Refs) != 2 || got.Refs[0] != "../sh" {
		t.Fatalf("refs = %v, want the two repos the fix spans", got.Refs)
	}
}

// A new issue is born OPEN — filed, not yet accepted. A register that cannot hold an
// untriaged thought is a register nobody files into, and the thought goes back to
// being a bullet in a document nobody greps.
func TestIssueIsBornUntriaged(t *testing.T) {
	s := newStore(t)
	it := &Issue{Kind: KindFeature, Title: "render the graph", Status: StatusTriaged} // even if asked for
	if _, err := s.Add(it); err != nil {
		t.Fatal(err)
	}
	if it.Status != StatusOpen {
		t.Fatalf("a newly filed issue has status %q; it must be %q — accepting it is a separate, deliberate act",
			it.Status, StatusOpen)
	}
}

func TestUnknownKindIsRefused(t *testing.T) {
	s := newStore(t)
	if _, err := s.Add(&Issue{Kind: "epic", Title: "x"}); err == nil {
		t.Fatal("an unknown kind was accepted; the vocabulary must be closed or it is not a vocabulary")
	}
	if _, err := s.Add(&Issue{Kind: KindBug}); err == nil {
		t.Fatal("a titleless issue was filed — an issue nobody can identify is a note, not a record")
	}
}

// Retitling must not leave the old file behind as a second, stale record of the same
// issue.
func TestRetitleDoesNotDuplicateTheRecord(t *testing.T) {
	s := newStore(t)
	it := &Issue{Kind: KindBug, Title: "old title"}
	if _, err := s.Add(it); err != nil {
		t.Fatal(err)
	}
	it.Title = "a much better title"
	if _, err := s.Save(it); err != nil {
		t.Fatal(err)
	}
	all, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("the register holds %d records after a retitle, want 1 — the old file was left behind", len(all))
	}
	if all[0].Title != "a much better title" {
		t.Fatalf("title = %q", all[0].Title)
	}
}

// A round trip through disk must preserve every field: the file IS the record.
func TestRoundTrip(t *testing.T) {
	s := newStore(t)
	it := &Issue{
		Kind: KindRequirement, Title: "POSIX cert must not regress",
		Priority: "p0", Refs: []string{"../sh"}, Labels: []string{"cert"},
		Reporter: "qiangli", Body: "86/86 is the floor.",
	}
	if _, err := s.Add(it); err != nil {
		t.Fatal(err)
	}
	got, err := s.Resolve(it.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != KindRequirement || got.Priority != "p0" || got.Body != "86/86 is the floor." ||
		got.Reporter != "qiangli" || len(got.Labels) != 1 {
		t.Fatalf("round trip lost data: %+v", got)
	}
}

// One malformed file must not blind the whole register. A hand-edited issue with a YAML
// typo should not hide the other forty — the store is meant to be edited by hand.
func TestOneBadFileDoesNotHideTheRest(t *testing.T) {
	s := newStore(t)
	if _, err := s.Add(&Issue{Kind: KindBug, Title: "good"}); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(s.Root, Dir, "deadbeef-broken.md")
	if err := os.WriteFile(bad, []byte("not frontmatter at all\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	all, err := s.List()
	if err != nil {
		t.Fatalf("one malformed file made the whole register unreadable: %v", err)
	}
	if len(all) != 1 || all[0].Title != "good" {
		t.Fatalf("List = %v, want the one good issue", all)
	}
}

// An empty register is a new project, not an error.
func TestEmptyRegisterIsNotAnError(t *testing.T) {
	all, err := New(t.TempDir()).List()
	if err != nil || len(all) != 0 {
		t.Fatalf("List on a fresh repo = %v, %v; want empty, nil", all, err)
	}
}
