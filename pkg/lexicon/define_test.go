package lexicon

import (
	"slices"
	"strings"
	"testing"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	s := &Store{byTerm: map[string]int{}, byTermAll: map[string][]int{}}
	s.AddStandardTools([]string{"ls", "grep"}, Overlay{})
	s.AddSystem(SystemInventory{
		EnvVars:      []string{"WEAVE_AGENT"},
		Commands:     []string{"outpost", "dragon"},
		PathSegments: []string{"dhnt", "dragon"}, // dragon is BOTH — the ambiguous case
	}, Overlay{})
	return s
}

// The gap that made `define` wrong on its own terms: a verb called define that
// answers "unknown" for `ls`. Standard tools are not jargon, but they ARE
// applicable to this system, and an agent asking deserves the right pointer.
func TestDefine_AnswersForStandardTools(t *testing.T) {
	d := testStore(t).Define("ls")
	if !d.Found {
		t.Fatalf("ls was not found: %+v", d)
	}
	if d.Concept.Kind != KindStandardTool {
		t.Errorf("kind = %s, want %s", d.Concept.Kind, KindStandardTool)
	}
	if !strings.Contains(d.Concept.ScopeNote, "not local jargon") {
		t.Errorf("the scope note should distinguish standard from local: %q", d.Concept.ScopeNote)
	}
}

// One string genuinely IS several things on a real host. Reporting only the
// first is how a caller confidently acts in the wrong world.
func TestDefine_ReportsEveryReading(t *testing.T) {
	d := testStore(t).Define("dragon")
	if !d.Found {
		t.Fatal("dragon was not found")
	}
	if len(d.Concepts) != 2 {
		t.Fatalf("got %d readings, want 2 (a command AND a path segment): %+v", len(d.Concepts), d.Concepts)
	}
	var kinds []string
	for _, c := range d.Concepts {
		kinds = append(kinds, string(c.Kind))
	}
	slices.Sort(kinds)
	if !slices.Equal(kinds, []string{"command", "path-segment"}) {
		t.Errorf("kinds = %v", kinds)
	}
	// The primary reading is the first, so a caller wanting one answer can take
	// the head without losing the fact that there were others.
	if d.Concept != d.Concepts[0] {
		t.Error("Concept is not the head of Concepts")
	}
}

// An unambiguous term still reports exactly one reading.
func TestDefine_SingleReading(t *testing.T) {
	d := testStore(t).Define("WEAVE_AGENT")
	if len(d.Concepts) != 1 {
		t.Errorf("got %d readings, want 1", len(d.Concepts))
	}
}

// A filter NARROWS; it never invents.
func TestDefineKinds_Filter(t *testing.T) {
	s := testStore(t)

	only := s.DefineKinds("dragon", []Kind{KindCommand})
	if len(only.Concepts) != 1 || only.Concepts[0].Kind != KindCommand {
		t.Fatalf("filtered result = %+v, want exactly the command reading", only.Concepts)
	}

	// Asking for a kind the term does not have is "not known AS THAT" — a
	// different and more useful statement than "not known".
	none := s.DefineKinds("dragon", []Kind{KindVerb})
	if none.Found {
		t.Error("a filter invented a reading the term does not have")
	}
	if !strings.Contains(none.Advice, "not known as verb") {
		t.Errorf("advice should name the filter that failed: %q", none.Advice)
	}
	if !strings.Contains(none.Advice, "Drop the filter") {
		t.Errorf("advice should offer the way forward: %q", none.Advice)
	}
}

// A shape classification is about the STRING, so a namespace filter must not
// produce one — answering "that is an IP" when the caller asked about commands
// answers a question nobody asked.
func TestDefineKinds_FilterSuppressesShapeAnswer(t *testing.T) {
	s := testStore(t)
	if d := s.Define("10.1.2.3"); d.Classification == "" {
		t.Error("unfiltered lookup should classify an address by shape")
	}
	if d := s.DefineKinds("10.1.2.3", []Kind{KindCommand}); d.Classification != "" {
		t.Errorf("a kind filter still produced a shape answer: %q", d.Classification)
	}
}

// The credential check runs before ANY lookup, and the term never comes back.
func TestDefine_CredentialNeverEchoed(t *testing.T) {
	const key = "sk-proj-Ab3xK9mQ7zR2vN8pL4wT6yH1jF5dG0sE"
	d := testStore(t).Define(key)

	if !d.Sensitive {
		t.Fatal("a vendor-prefixed key was not flagged sensitive")
	}
	if d.Term != "" {
		t.Errorf("the term was echoed back: %q", d.Term)
	}
	if d.Found || d.Concept != nil || len(d.Concepts) != 0 {
		t.Error("a sensitive term was looked up; the check must precede resolution")
	}
	if !strings.Contains(d.Advice, "rotate") {
		t.Errorf("advice should say what to do: %q", d.Advice)
	}
}

// Git shas are ubiquitous; calling every one a secret trains people to ignore
// the warning that matters.
func TestDefine_HexDigestIsNotACredential(t *testing.T) {
	d := testStore(t).Define("7420470a1b2c3d4e5f60718293a4b5c6d7e8f900")
	if d.Sensitive {
		t.Error("a hex digest was flagged as a credential")
	}
	if !strings.Contains(d.Classification, "hex digest") {
		t.Errorf("classification = %q", d.Classification)
	}
}

func TestDefine_Unknown(t *testing.T) {
	d := testStore(t).Define("frobnicate")
	if d.Found || d.Sensitive || d.Classification != "" {
		t.Errorf("an unknown word produced an answer: %+v", d)
	}
	if d.Advice == "" {
		t.Error("an unknown word should still say where to look")
	}
}

func TestDefine_Empty(t *testing.T) {
	if d := (&Store{}).Define("   "); d.Found || d.Advice == "" {
		t.Errorf("empty input = %+v", d)
	}
}

// Kinds is derived from what is projected, so a --kind filter's vocabulary
// cannot drift from what the host can actually answer for.
func TestStore_Kinds(t *testing.T) {
	got := testStore(t).Kinds()
	for _, want := range []string{"command", "env-var", "path-segment", "standard-tool"} {
		if !slices.Contains(got, want) {
			t.Errorf("Kinds() = %v, missing %q", got, want)
		}
	}
	if !slices.IsSorted(got) {
		t.Errorf("Kinds() is not sorted: %v", got)
	}
}

// `define` must never gain a subcommand.
//
// Its argument is an arbitrary user token, so every subcommand name permanently
// removes a word from the definable vocabulary: mount `study` and
// `bashy define study` stops meaning "what is the word study". The failure is
// invisible until somebody asks about that exact word and gets a help screen
// instead of an answer — which is why this is a build-failing ratchet rather
// than a note in a doc.
//
// Actions belong under `lexicon`, whose arguments are a closed set.
func TestDefineCmd_HasNoSubcommands(t *testing.T) {
	cmd := NewDefineCmd()
	if subs := cmd.Commands(); len(subs) != 0 {
		names := make([]string, 0, len(subs))
		for _, c := range subs {
			names = append(names, c.Name())
		}
		t.Fatalf("define has subcommands %v — each one steals that word from the vocabulary; "+
			"put the action under `lexicon` instead", names)
	}
}
