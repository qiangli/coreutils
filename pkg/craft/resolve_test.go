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

// THE MEASURED FAILURE, pinned. `find "ssh into a machine"` returned the Go
// build-and-test gate with a confident score, because the gate's prose says
// "machine-verified". One incidental word out of two, in the least specific
// half of the question, and nothing in the answer had anything to do with ssh.
//
// Nothing is the correct answer here, and it is not a lesser one: "this host
// has no skill for that" is a state a caller can act on, while a plausible
// wrong row is acted on as if it were right — `compose` renders it as a
// runnable script.
func TestIndex_IncidentalWordIsNotAMatch(t *testing.T) {
	ix := NewIndex([]Implementation{
		impl(t, "go-repo-health", goBuildTest,
			"Verify a Go repository is healthy with one machine-verified, attested command", nil),
	})
	if got := ix.Resolve(Query{Text: "ssh into a machine"}); len(got) != 0 {
		t.Errorf("query about ssh matched %q (score %.0f, %v) — one incidental word is not an answer",
			got[0].Name, got[0].Score, got[0].Why)
	}
	// The control: the same index still answers the question it CAN answer.
	// Without this, the assertion above would pass on an index that matches
	// nothing at all.
	if got := ix.Resolve(Query{Text: "verify the repository is healthy"}); len(got) != 1 {
		t.Fatalf("the on-topic query returned %d matches, want 1", len(got))
	}
}

// Substring matching makes every field a haystack in which short words are
// always found. Matching is per WORD, so a term reaches "repository" from
// "repo" but never reaches "concatenate" from "cat".
func TestIndex_MatchesWordsNotSubstrings(t *testing.T) {
	ix := NewIndex([]Implementation{
		impl(t, "text-joiner", goBuildTest, "concatenate the fragments", nil),
	})
	if got := ix.Resolve(Query{Text: "cat"}); len(got) != 0 {
		t.Errorf("\"cat\" matched %q through a substring of \"concatenate\"", got[0].Name)
	}
	if got := ix.Resolve(Query{Text: "concatenate"}); len(got) != 1 {
		t.Errorf("the whole word did not match; the rule is per-word, not no-match")
	}
}

// A ranking must say how much of the question it answered. A score alone cannot
// distinguish "answered all of it" from "recognised one word", which is exactly
// the distinction that made the wrong answer above look right.
func TestIndex_ReportsTermCoverage(t *testing.T) {
	got := testIndex(t).Resolve(Query{Text: "rust crate compiles"})
	if len(got) == 0 {
		t.Fatal("no match")
	}
	if got[0].Terms != 3 || got[0].Covered != 3 {
		t.Errorf("covered %d/%d terms, want 3/3", got[0].Covered, got[0].Terms)
	}
}

// A capability with three implementations must not outrank a better answer with
// one. Scored per implementation it did — an artefact of how the catalog is
// written rather than of what was asked.
func TestIndex_ScoreIsPerCapabilityNotPerImplementation(t *testing.T) {
	one := NewIndex([]Implementation{
		impl(t, "go-repo-health", goBuildTest, "verify tests pass", nil),
	}).Resolve(Query{Text: "verify tests pass"})
	many := NewIndex([]Implementation{
		impl(t, "go-repo-health", goBuildTest, "verify tests pass", nil),
		impl(t, "rust-repo-health", goBuildTest, "verify tests pass", nil),
		impl(t, "zig-repo-health", goBuildTest, "verify tests pass", nil),
	}).Resolve(Query{Text: "verify tests pass"})
	if len(one) != 1 || len(many) != 1 {
		t.Fatalf("got %d and %d matches, want 1 each", len(one), len(many))
	}
	if one[0].Score != many[0].Score {
		t.Errorf("score %.0f vs %.0f — implementation count changed the ranking",
			one[0].Score, many[0].Score)
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
