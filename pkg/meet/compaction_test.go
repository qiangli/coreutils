package meet

import (
	"strings"
	"testing"
)

func TestPreviewOffloadsLongTurns(t *testing.T) {
	short := Event{Kind: "turn", Speaker: "claude", Text: "a concise point.", File: "/tmp/x.txt"}
	if got := preview(short); strings.Contains(got, "file://") {
		t.Errorf("short turn should inline in full, got offloaded: %q", got)
	}
	long := Event{Kind: "turn", Speaker: "claude", Text: strings.Repeat("word ", 400), File: "/tmp/full.txt"}
	got := preview(long)
	if !strings.Contains(got, "file:///tmp/full.txt") {
		t.Errorf("long turn should carry a file:// link, got %q", got)
	}
	if !strings.Contains(got, "chars — full:") || len(got) > previewFull+200 {
		t.Errorf("long turn should be a bounded head/tail preview, got len=%d", len(got))
	}
}

func TestTranscriptContextCollapsesOldTurns(t *testing.T) {
	var ev []Event
	for i := 0; i < 20; i++ {
		ev = append(ev, Event{Kind: "turn", Speaker: "a", Text: strings.Repeat("x", 800), File: "/tmp/f.txt"})
	}
	ctx := transcriptContext(ev)
	// Older turns collapse to a one-line ref ("[full: file://…]"); recent ones get
	// the richer "…chars — full:" preview. Total stays bounded well under 20×800.
	if len(ctx) > 12000 {
		t.Errorf("context not bounded: len=%d", len(ctx))
	}
	if !strings.Contains(ctx, "[full: file://") {
		t.Errorf("expected collapsed older-turn references")
	}
}

func TestParseConverge(t *testing.T) {
	out := `DECISIONS:
- ship the P0 verbs
- keep secretary notes-only
ACTIONS:
- claude: file the minutes
OPEN QUESTIONS:
- blind vs sequential default?
SUMMARY:
The group agreed on the P0 scope.
It will iterate from there.`
	syn := parseConverge(out)
	if len(syn.Decisions) != 2 || len(syn.Actions) != 1 || len(syn.OpenQuestions) != 1 {
		t.Fatalf("parse counts: dec=%d act=%d oq=%d", len(syn.Decisions), len(syn.Actions), len(syn.OpenQuestions))
	}
	if syn.Decisions[0].Text != "ship the P0 verbs" || syn.Actions[0] != "claude: file the minutes" {
		t.Errorf("bad items: %v %v", syn.Decisions, syn.Actions)
	}
	if !strings.HasPrefix(syn.Summary, "The group agreed") || !strings.Contains(syn.Summary, "iterate") {
		t.Errorf("summary joined wrong: %q", syn.Summary)
	}
}

func TestParseConvergeNone(t *testing.T) {
	syn := parseConverge("DECISIONS:\nnone\nACTIONS:\nnone\nOPEN QUESTIONS:\nnone\nSUMMARY:\nNothing decided.")
	if len(syn.Decisions)+len(syn.Actions)+len(syn.OpenQuestions) != 0 {
		t.Errorf("'none' should yield no items")
	}
	if syn.Summary != "Nothing decided." {
		t.Errorf("summary=%q", syn.Summary)
	}
}

// The secretary may INFER a decision from consensus, but the reader must always
// be able to tell an inferred decision from a stated one — the label is the guard
// against hallucinated consensus, not the mode.
func TestParseConvergeMarksInferredDecisions(t *testing.T) {
	syn := parseConverge("DECISIONS:\n- ship the P0 verbs\n- (inferred) cert bypasses the atomizer\n" +
		"RISKS:\n- the fd race is unfixed\nCORRECTIONS:\n- chunks=1 is not unchunked\nSUMMARY:\nok.")
	if len(syn.Decisions) != 2 {
		t.Fatalf("want 2 decisions, got %d", len(syn.Decisions))
	}
	if syn.Decisions[0].Inferred {
		t.Error("stated decision must not be marked inferred")
	}
	if !syn.Decisions[1].Inferred || syn.Decisions[1].Text != "cert bypasses the atomizer" {
		t.Errorf("inferred decision mis-parsed: %+v", syn.Decisions[1])
	}
	if len(syn.Risks) != 1 || len(syn.Corrections) != 1 {
		t.Errorf("risks=%v corrections=%v", syn.Risks, syn.Corrections)
	}
}
