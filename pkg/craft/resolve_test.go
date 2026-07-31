package craft

import (
	"slices"
	"testing"
)

func testIndex(t *testing.T) *Index {
	t.Helper()
	return NewIndex([]Implementation{
		impl(t, "go-repo-health", goBuildTest, "Verify a Go repository builds and its tests pass",
			map[string]string{"check-build": "go build ./...", "check-tests": "go test ./..."}),
		impl(t, "rust-repo-health", rustBuildTest, "Verify a Rust crate compiles and its tests pass", nil),
		impl(t, "force-agent-shell", wiredForced, "Check agent CLIs route their shell through bashy",
			map[string]string{"check-wireda": "bashy install-agent --check"}),
	})
}

// Two implementations of ONE guarantee are one capability, so a query returns
// one result with an alternative — not two rows competing for selection. That
// competition is the shadowing failure this design removes.
func TestIndex_GroupsByCapability(t *testing.T) {
	ix := testIndex(t)
	if ix.Len() != 2 {
		t.Fatalf("indexed %d capabilities, want 2 (build+test is ONE)", ix.Len())
	}

	got := ix.Resolve(Query{Text: "repo health"})
	if len(got) != 1 {
		t.Fatalf("got %d matches, want 1: %+v", len(got), got)
	}
	if got[0].Alternatives != 1 {
		t.Errorf("alternatives = %d, want 1 — the other implementation is reachable, not a rival", got[0].Alternatives)
	}
}

// An exact name outranks everything, which is what keeps bare-name dispatch
// working while the primary interface becomes a question.
func TestIndex_ExactNameWins(t *testing.T) {
	got := testIndex(t).Resolve(Query{Text: "force-agent-shell"})
	if len(got) == 0 {
		t.Fatal("an exact name did not resolve")
	}
	if got[0].Name != "force-agent-shell" {
		t.Errorf("top match = %q, want the exact name", got[0].Name)
	}
	if !slices.Contains(got[0].Why, "exact name match") {
		t.Errorf("why = %v, should name the signal that fired", got[0].Why)
	}
}

// GRAPH EXPANSION: a capability is described by what it GUARANTEES, so the
// contract is searchable text. This is what lets a query find a skill whose
// prose never uses the query's words.
func TestIndex_MatchesOnContractPredicate(t *testing.T) {
	ix := NewIndex([]Implementation{
		impl(t, "opaque-name", goBuildTest, "does a thing", nil),
	})
	got := ix.Resolve(Query{Text: "builida"})
	if len(got) == 0 {
		t.Fatal("a contract predicate did not match; graph expansion is not working")
	}
	if !slices.Contains(got[0].Why, "contract") {
		t.Errorf("why = %v, want the contract signal", got[0].Why)
	}
}

// Natural-language phrasing must not be defeated by its own filler.
func TestIndex_IgnoresStopWords(t *testing.T) {
	ix := testIndex(t)
	plain := ix.Resolve(Query{Text: "rust crate compiles"})
	wordy := ix.Resolve(Query{Text: "how can you check that the rust crate compiles"})
	if len(plain) == 0 || len(wordy) == 0 {
		t.Fatal("one of the phrasings resolved to nothing")
	}
	if plain[0].Key != wordy[0].Key {
		t.Errorf("phrasing changed the top result: %s vs %s", plain[0].Name, wordy[0].Name)
	}
}

// A query that matches nothing returns nothing — it does not return the
// least-bad row. A confident wrong answer propagates.
func TestIndex_NoMatchIsEmpty(t *testing.T) {
	if got := testIndex(t).Resolve(Query{Text: "provision a kubernetes cluster"}); len(got) != 0 {
		t.Errorf("an unrelated query returned %d matches: %+v", len(got), got)
	}
}

// The index is derived, so two identical queries must agree exactly.
func TestIndex_Deterministic(t *testing.T) {
	ix := testIndex(t)
	a := ix.Resolve(Query{Text: "verify tests pass"})
	b := ix.Resolve(Query{Text: "verify tests pass"})
	if len(a) != len(b) {
		t.Fatalf("two identical queries returned %d and %d matches", len(a), len(b))
	}
	for i := range a {
		if a[i].Key != b[i].Key || a[i].Score != b[i].Score {
			t.Fatalf("result %d differed between runs", i)
		}
	}
}

// Election prefers the implementation needing the least model — the one with
// the most bindings — so the default answer is the cheapest to run.
func TestIndex_ElectsMostBoundImplementation(t *testing.T) {
	got := testIndex(t).Resolve(Query{Text: "repository tests pass"})
	if len(got) == 0 {
		t.Fatal("no match")
	}
	if got[0].Primary.Name != "go-repo-health" {
		t.Errorf("elected %q; the bound implementation should win — it needs no model",
			got[0].Primary.Name)
	}
}

// A contract-less implementation states no guarantee, so there is nothing for a
// query about guarantees to match. It is dropped rather than given a synthetic
// capability that would merge it with unrelated skills.
func TestNewIndex_DropsContractlessImplementations(t *testing.T) {
	ix := NewIndex([]Implementation{
		impl(t, "nothing", noContract, "a skill with no contract", nil),
	})
	if ix.Len() != 0 {
		t.Errorf("indexed %d capabilities, want 0", ix.Len())
	}
}

func TestIndex_LimitRespected(t *testing.T) {
	got := testIndex(t).Resolve(Query{Text: "verify tests pass repository shell", Limit: 1})
	if len(got) != 1 {
		t.Errorf("got %d matches, want 1", len(got))
	}
}
