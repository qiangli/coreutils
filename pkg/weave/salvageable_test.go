package weave

import (
	"bytes"
	"strings"
	"testing"
)

// A run that died WITH GOOD WORK INSIDE IT must say so.
//
// weaveTerminalState is correctly conservative: `submitted` requires a zero exit
// AND commits, so a crashed wrapper can never claim success. That asymmetry is
// right. But the inverse costs real work.
//
// Observed live: an opencode worker implemented a whole feature, committed it,
// and then crashed on the way out (its storage layer blows the filename limit on
// exit). weave marked the run `failed` — with a building, tested, complete diff
// sitting on the branch, and nothing anywhere saying so. It read exactly like a
// run that achieved nothing.
//
// The obvious response to a failed run is to run it again. Doing that would have
// thrown away eight minutes of finished work and paid for it twice.
func TestSalvageableFooterNamesRunsThatDiedWithCommits(t *testing.T) {
	var b bytes.Buffer
	weavePrintSalvageableFooter(&b, []int64{3, 7})
	out := b.String()

	if !strings.Contains(out, "#3") || !strings.Contains(out, "#7") {
		t.Errorf("footer must name the runs: %q", out)
	}
	if !strings.Contains(out, "salvage") {
		t.Errorf("footer must say how to keep the work: %q", out)
	}
	// The whole point: stop the reader from re-running it.
	if !strings.Contains(strings.ToLower(out), "do not re-run") {
		t.Errorf("footer must warn against re-running — that is the loss this exists to prevent: %q", out)
	}
}

// Silence when there is nothing to salvage. A footer that always fires is a
// footer nobody reads.
func TestSalvageableFooterIsSilentWhenThereIsNothing(t *testing.T) {
	var b bytes.Buffer
	weavePrintSalvageableFooter(&b, nil)
	if b.Len() != 0 {
		t.Errorf("expected no footer, got %q", b.String())
	}
}

// The state itself must NOT be loosened. A crash is a crash: `submitted` still
// requires a clean exit AND commits. Surfacing the evidence is not the same as
// asserting success on it, and confusing the two would recreate the exact bug
// the fleet-evidence rule exists to prevent.
func TestACrashNeverClaimsSubmittedEvenWithCommits(t *testing.T) {
	ev := weaveTerminalEvidence{CommitsAhead: 5}
	if got := weaveTerminalState(1, nil, "", ev); got != "failed" {
		t.Errorf("non-zero exit with commits = %q, want failed — a crash must never claim success", got)
	}
	if got := weaveTerminalState(0, nil, "sigkill", ev); got != "killed" {
		t.Errorf("killed with commits = %q, want killed", got)
	}
	if got := weaveTerminalState(0, nil, "", ev); got != "submitted" {
		t.Errorf("clean exit with commits = %q, want submitted", got)
	}
	// And the load-bearing half: no commits means no success, whatever the exit.
	if got := weaveTerminalState(0, nil, "", weaveTerminalEvidence{CommitsAhead: 0}); got != "failed" {
		t.Errorf("clean exit with NO commits = %q, want failed — success may not be reached by an absence of work", got)
	}
}
