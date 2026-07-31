package weave

// DRAINING — stopping a sprint so the next one can START FROM WHERE IT LEFT OFF.
//
// The reason this exists is preemption: something more urgent arrives, the
// current work must yield, and it must yield in a state somebody can pick up
// cold. That is a different act from `abort` (kill everything, salvage nothing)
// and from a bare `stop` (close the clock and hope).
//
// A stop is not graceful because it was polite. It is graceful because three
// things are TRUE when it finishes:
//
//  1. No worker is still running. Wrappers are stopped the way `weave pause`
//     stops them — workspace and branch preserved — so the work is parked, not
//     destroyed.
//  2. The tree COMPILES AND ITS TESTS PASS. This is the minimum bar, and it is
//     the one that makes resumption possible at all: inheriting a broken tree
//     means the next sprint spends its first hour on damage from the last one,
//     without knowing which damage is theirs.
//  3. There is a continuity record. "Where it left off" is a written sentence,
//     not an inference from a diff.
//
// # A red gate REFUSES to close the sprint
//
// This is the hard rule, and it is deliberately inconvenient. If the gate
// fails, the sprint stays open in DRAINING: workers are already parked, so
// nothing is burning, and the conductor's remaining job is narrow and clear —
// fix the regression, run `sprint stop` again. Closing over a red gate would
// file the sprint as done and hand the next one a mess whose origin is no
// longer visible, which is the exact failure this feature was asked to prevent.
//
// # No gate is NOT a pass
//
// `gate.Outcome.Ran` exists for precisely this. A drain with no gate command
// cannot certify anything, so it does not: the box records that it closed
// unverified, and says so out loud. Absence of evidence is never success —
// the same rule the fleet evidence invariant states for runs.
//
// # WHY THE WORST CASE IS SAFE: sprint work goes through weave, never direct
//
// Every task a sprint plans is executed by `weave` — an isolated workspace on
// its own branch. No agent edits a repo's working tree directly. That single
// rule is what makes a hard stop survivable, and it is worth stating because
// the whole refusal above rests on it:
//
//   - Unmerged work CANNOT break the tree. A branch that was never merged has
//     no bearing on whether main builds, so parking it costs nothing and risks
//     nothing.
//   - Therefore a RED GATE MEANS SOMETHING ALREADY LANDED. The regression is
//     in merged code, which is exactly the case that must be fixed before the
//     sprint closes — it is the next sprint's inheritance either way.
//   - And the worst case has an exit that is not "close over a broken tree":
//     leave the weave unmerged (it is already parked), or `weave abandon` it
//     and lose only that branch. Main is untouched in both.
//
// So the escape hatch for a failing gate is never --force. It is to drop the
// offending work, which weave makes a local and reversible decision rather than
// a repo-wide one.
//
// # What draining honestly cannot promise
//
// Stopping a wrapper stops the PROCESS. Whether the agent inside had reached a
// tidy internal moment is not observable from here, and pretending otherwise
// would be the kind of claim this package exists to avoid making. What is
// guaranteed is the part that survives a process: the branch, the workspace,
// the gate verdict, and the continuity note.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/qiangli/coreutils/pkg/gate"
)

// drainReport is what one drain attempt observed, per linked repo and overall.
type drainReport struct {
	Paused     []string // "repo (2 workers)" — what was parked
	GateRan    bool
	GatePassed bool
	GateCmd    string
	GateOutput string
	Failures   []string // repos whose gate failed
	Repos      []repoState
}

// Clean reports a drain that satisfies every condition for a good handoff: a
// gate ran, and it passed. Anything else is either unverified or broken, and
// neither may be recorded as a clean stop.
func (r *drainReport) Clean() bool { return r.GateRan && r.GatePassed }

// pauseLinkedRepos stops running workers in every repo this sprint links,
// reusing weave's own pause semantics so a drained worker is parked exactly the
// way a paused one is — same preserved workspace, same preserved branch.
//
// A repo whose queue cannot be found is REPORTED, not skipped silently: a
// sprint that thinks it parked three repos and parked two is worse than one
// that admits it, because the unparked worker keeps writing to a tree the next
// sprint believes is quiet.
func pauseLinkedRepos(s *weaveStory) (paused []string, problems []string) {
	seen := map[string]bool{}
	for _, r := range s.Runs {
		if r.Repo == "" || seen[r.Repo] {
			continue
		}
		seen[r.Repo] = true
		dir, ok := queueDirForRepoName(r.Repo)
		if !ok {
			problems = append(problems, fmt.Sprintf("%s: no unique queue on this host", r.Repo))
			continue
		}
		// ONLY THIS SPRINT'S RUNS. Parking is per-queue at the weave layer, so
		// pausing a whole repo would stop ANOTHER running sprint's workers —
		// the exact cross-sprint damage the shared-repo exemption exists to
		// avoid one check later. A sprint may stop its own work and nobody
		// else's.
		mine := map[int64]bool{}
		for _, rr := range s.Runs {
			if rr.Repo == r.Repo {
				mine[rr.ID] = true
			}
		}
		n, err := pauseWorkersIn(dir, mine)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", r.Repo, err))
			continue
		}
		paused = append(paused, fmt.Sprintf("%s (%d worker(s))", r.Repo, n))
	}
	return paused, problems
}

// queueDirForRepoName finds a repo's weave queue by its basename.
//
// The queue tag is "<basename>-<hash of the repo path>", and the hash exists so
// the directory name never spells out where the repo lives. That means a name
// alone cannot be turned back into a path — it has to be matched against the
// queues that exist. Two checkouts of the same-named repo are ambiguous, and
// this reports no match rather than guessing at one.
func queueDirForRepoName(repo string) (string, bool) {
	want := strings.TrimSpace(repo)
	if want == "" {
		return "", false
	}
	var hits []string
	for _, dir := range weaveAllQueueDirs() {
		tag := filepath.Base(dir)
		// tag is "<basename>-<8 hex>"; split off the hash suffix.
		if i := strings.LastIndex(tag, "-"); i > 0 && tag[:i] == want {
			hits = append(hits, dir)
		}
	}
	// Exactly one match, or none. Two checkouts of the same-named repo are
	// genuinely ambiguous, and pausing the wrong one would stop work nobody
	// asked to stop while leaving the intended worker running — the worst of
	// both outcomes.
	if len(hits) != 1 {
		return "", false
	}
	return hits[0], true
}

// pauseWorkersIn stops the running wrappers for the GIVEN runs in one queue,
// leaving every other worker in that repo alone.
//
// The filter is the whole point. Two sprints can share a repo, and a drain that
// paused the queue would stop work its sprint does not own — silently, since a
// paused worker looks the same however it got there.
func pauseWorkersIn(dir string, only map[int64]bool) (int, error) {
	var stopped int
	q, err := loadWeaveQueue(dir)
	if err != nil {
		return 0, err
	}
	for _, it := range q.Items {
		if it.State != "working" || !only[it.ID] {
			continue
		}
		if it.WrapperPid > 0 && pidAlive(it.WrapperPid) {
			weaveStopWrapper(it.WrapperPid)
			stopped++
		}
	}
	return stopped, nil
}

// runDrainGate settles the minimum bar: does the tree still build and pass.
//
// The command is the caller's, because only they know what "compiles and
// tests" means for their tree. When none is given nothing is run and Ran stays
// false — which the caller must treat as UNVERIFIED, never as a pass.
func runDrainGate(ctx context.Context, dir, command string) gate.Outcome {
	if strings.TrimSpace(command) == "" {
		return gate.Outcome{Ran: false}
	}
	if dir == "" {
		dir, _ = os.Getwd()
	}
	return gate.RunLocal(ctx, dir, command, "")
}

// drainSummary renders what happened, in the order a reader needs it: the
// blocking problem first, the evidence second.
func drainSummary(r *drainReport, elapsed, planned time.Duration) string {
	var b strings.Builder
	if len(r.Paused) > 0 {
		// The parked branches ARE the resumption pointer — "where it left off"
		// is a workspace and a branch per repo, not a feeling.
		fmt.Fprintf(&b, "parked %s (unmerged, resumable); ", strings.Join(r.Paused, ", "))
	}
	switch {
	case !r.GateRan:
		b.WriteString("NO GATE RAN — the stop is unverified")
	case r.GatePassed:
		b.WriteString("gate green")
	default:
		b.WriteString("GATE FAILED")
	}
	if n := len(r.Repos); n > 0 {
		clean := 0
		for i := range r.Repos {
			if r.Repos[i].OK() {
				clean++
			}
		}
		fmt.Fprintf(&b, "; %d/%d repo(s) wrapped up", clean, n)
	}
	fmt.Fprintf(&b, "; ran %s of %s", roundDur(elapsed), roundDur(planned))
	return b.String()
}
