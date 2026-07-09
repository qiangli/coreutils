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
	dec, act, oq, sum := parseConverge(out)
	if len(dec) != 2 || len(act) != 1 || len(oq) != 1 {
		t.Fatalf("parse counts: dec=%d act=%d oq=%d", len(dec), len(act), len(oq))
	}
	if dec[0] != "ship the P0 verbs" || act[0] != "claude: file the minutes" {
		t.Errorf("bad items: %v %v", dec, act)
	}
	if !strings.HasPrefix(sum, "The group agreed") || !strings.Contains(sum, "iterate") {
		t.Errorf("summary joined wrong: %q", sum)
	}
}

func TestParseConvergeNone(t *testing.T) {
	dec, act, oq, sum := parseConverge("DECISIONS:\nnone\nACTIONS:\nnone\nOPEN QUESTIONS:\nnone\nSUMMARY:\nNothing decided.")
	if len(dec)+len(act)+len(oq) != 0 {
		t.Errorf("'none' should yield no items")
	}
	if sum != "Nothing decided." {
		t.Errorf("summary=%q", sum)
	}
}
