package recall

import (
	"fmt"
	"os"
	"strings"
)

// EnvKnowledge switches knowledge injection for a launched agent.
//
//	off (default)  the agent is launched exactly as before
//	on             a recalled preamble is prepended to its prompt
//
// DEFAULT OFF IS DELIBERATE AND TEMPORARY. Injection is the treatment arm of an
// experiment that has not returned yet: the composition claim has no supporting
// measurement at either orchestrating seat
// (dhnt/docs/composition-validation-experiment.md §6d/§6e). Turning it on for
// every user before it is measured would (a) change every agent's behaviour on an
// unproven feature and (b) destroy the control arm — once injection is the
// default, there is no clean B to compare against.
//
// Flip the default only when the leaderboard shows a positive lift with
// non-overlapping intervals. Until then this env var IS the experiment.
const EnvKnowledge = "BASHY_KNOWLEDGE"

// Enabled reports whether knowledge injection is on for this process.
func Enabled() bool { return enabledIn(os.Getenv(EnvKnowledge)) }

func enabledIn(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "on", "1", "true", "yes":
		return true
	default:
		return false
	}
}

// PreambleBudget caps the injected text. Small on purpose: measured retrieval
// showed k=1 beating k=4, which matches ~4-chunk human working memory, and an
// agent whose context is half preamble has less room for the actual task.
const PreambleBudget = 700

// Preamble renders what is known about `goal` as a prompt prefix, or "" when
// nothing is known or injection is off.
//
// Shape follows pkg/foreman's kbPreamble, which is the pattern already proven in
// this tree — a labelled block naming its source and the verb to dig further, so
// the agent can tell recalled knowledge from the operator's instruction and can
// go read the original. Two rules it inherits:
//
//   - NEVER present a hit as an instruction. It is labelled as prior knowledge
//     that may be stale, because a recalled note asserted as policy is exactly
//     how a `candidate` becomes a false constraint.
//   - Cite, do not summarise. Every line carries an id the agent can open. A
//     summarising preamble is a confabulation surface.
func Preamble(goal string, readers ...Reader) string {
	if !Enabled() || strings.TrimSpace(goal) == "" {
		return ""
	}
	return preamble(goal, readers...)
}

// preamble is Preamble without the env check, so tests and callers that have
// already decided can use it directly.
func preamble(goal string, readers ...Reader) string {
	res := Recall(Query{Text: goal, K: 2, Budget: PreambleBudget}, readers...)
	if len(res.Hits) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Prior knowledge recalled from this host (may be stale — verify before relying on it;\n")
	b.WriteString("`bashy recall <topic>` for more, `bashy kb show <slug>` for a full page):\n")
	for _, h := range res.Hits {
		fmt.Fprintf(&b, "- [%s/%s] %s", h.Ring, h.Status, h.Cue)
		if h.Gist != "" {
			fmt.Fprintf(&b, " — %s", h.Gist)
		}
		fmt.Fprintf(&b, "  (%s)\n", h.ID)
	}
	b.WriteString("\n")
	return b.String()
}

// PreambleForHost is the convenience the launcher uses: open this host's rings and
// render. Returns "" on any problem — injection is best-effort by construction,
// because a memory lookup must never be able to stop an agent from starting.
func PreambleForHost(goal string) string {
	if !Enabled() || strings.TrimSpace(goal) == "" {
		return ""
	}
	rs := openRings("")
	if len(rs) == 0 {
		return ""
	}
	return preamble(goal, rs...)
}
