package patch

import "strings"

// ApplyOptions controls how Apply matches and places hunks. It is the
// pure-logic counterpart of the CLI's -R/-f/-F/-l flags; cmds/patch
// translates flags into this struct.
type ApplyOptions struct {
	Reverse          bool // -R: swap the old/new sides of every hunk first
	Fuzz             int  // -F NUM: context lines that may be dropped from a hunk's edges before it is given up on
	IgnoreWhitespace bool // -l: collapse whitespace runs when comparing context/old lines
	Force            bool // -f: never treat a mismatch as "already applied"; only try forward matches
}

// HunkOutcome is what happened when Apply tried to place one hunk.
type HunkOutcome int

const (
	HunkApplied HunkOutcome = iota
	HunkAppliedFuzzy
	HunkAlreadyApplied
	HunkFailed
)

// HunkReport is one line of the per-hunk narration a real patch(1)
// prints ("Hunk #2 succeeded at 37 with fuzz 1", "Hunk #3 FAILED").
type HunkReport struct {
	Index    int // 1-based
	Outcome  HunkOutcome
	FuzzUsed int
	At       int // 0-based line the hunk was placed at (meaningful for Applied/AppliedFuzzy)
}

// Result is everything Apply learned about applying one file's hunks.
type Result struct {
	Lines          []string
	NoFinalNewline bool
	Reports        []HunkReport
	// Rejects holds, in the orientation actually attempted (already
	// reverse-swapped when ApplyOptions.Reverse is set), every hunk that
	// could not be placed — for WriteReject.
	Rejects []Hunk
}

// AllApplied reports whether every hunk succeeded (applied, fuzzy, or
// recognized as already applied) with nothing rejected.
func (r Result) AllApplied() bool {
	for _, rep := range r.Reports {
		if rep.Outcome == HunkFailed {
			return false
		}
	}
	return true
}

// Apply places hunks against oldLines (the file's current content, one
// element per line, no trailing newlines) in order, honoring opts, and
// returns the resulting content.
//
// Matching, in order, for each hunk: an exact match of its full context at
// its expected position (carried forward from the net size change of every
// earlier successfully placed hunk); failing that, the same exact match
// searched outward through the rest of the file (handles drift from edits
// elsewhere); failing that, up to opts.Fuzz rounds with that many lines of
// leading/trailing context peeled off both ends of the search (GNU's fuzz
// factor, simplified — see docs/patch-continuation-ledger.md for exactly
// how this differs from GNU's own fuzz algorithm); and finally, unless
// opts.Force, a check for whether the hunk's NEW content is already present
// (an idempotent re-application), which is reported rather than treated as
// a failure. Anything left over is a reject: the file is left untouched at
// that hunk's location and the hunk is recorded in Result.Rejects.
func Apply(oldLines []string, oldNoFinalNewline bool, hunks []Hunk, opts ApplyOptions) Result {
	work := make([]Hunk, len(hunks))
	for i, h := range hunks {
		if opts.Reverse {
			work[i] = reverseHunk(h)
		} else {
			work[i] = h
		}
	}

	var out []string
	finalNoEOL := oldNoFinalNewline
	cur := 0
	offset := 0

	appendVerbatim := func(from, to int) {
		if to <= from {
			return
		}
		out = append(out, oldLines[from:to]...)
		finalNoEOL = oldNoFinalNewline && to == len(oldLines)
	}

	var reports []HunkReport
	var rejects []Hunk

	for idx, h := range work {
		expected := h.OldStart + offset
		matchStart, peelUsed, outcome := locateHunk(oldLines, h, expected, opts)

		switch outcome {
		case HunkFailed:
			end := matchStart
			if matchStart < cur {
				// A malformed or out-of-order hunk can point back into
				// content already emitted for an earlier hunk. Reject it
				// without moving cur backwards or consuming a different
				// range in its place.
				matchStart = cur
				end = cur
			} else {
				end = min(matchStart+h.OldCount, len(oldLines))
			}
			appendVerbatim(cur, matchStart)
			appendVerbatim(matchStart, end)
			cur = end
			rejects = append(rejects, h)
			reports = append(reports, HunkReport{Index: idx + 1, Outcome: HunkFailed})

		case HunkAlreadyApplied:
			end := matchStart + h.NewCount
			if end > len(oldLines) || matchStart < cur {
				// Can't safely skip past content already emitted; treat
				// as a plain failure instead of corrupting output.
				reports = append(reports, HunkReport{Index: idx + 1, Outcome: HunkFailed})
				rejects = append(rejects, h)
				continue
			}
			appendVerbatim(cur, matchStart)
			appendVerbatim(matchStart, end)
			cur = end
			// Hunk coordinates are all expressed against the original side of
			// the patch. Carry only placement drift from the stated location;
			// size changes from earlier hunks must not be applied to indices in
			// oldLines, which deliberately remains the original target image.
			offset = matchStart - h.OldStart
			reports = append(reports, HunkReport{Index: idx + 1, Outcome: HunkAlreadyApplied, At: matchStart})

		default: // HunkApplied / HunkAppliedFuzzy
			if matchStart < cur {
				end := max(min(matchStart+h.OldCount, len(oldLines)), cur)
				appendVerbatim(cur, end)
				cur = end
				rejects = append(rejects, h)
				reports = append(reports, HunkReport{Index: idx + 1, Outcome: HunkFailed})
				continue
			}
			appendVerbatim(cur, matchStart)
			repl, lastNoEOL, ok := buildReplacement(oldLines, matchStart, h.Lines, oldNoFinalNewline)
			out = append(out, repl...)
			cur = matchStart + h.OldCount
			if cur == len(oldLines) && ok {
				finalNoEOL = lastNoEOL
			} else if cur >= len(oldLines) {
				finalNoEOL = false
			}
			offset = matchStart - h.OldStart
			reports = append(reports, HunkReport{Index: idx + 1, Outcome: outcome, FuzzUsed: peelUsed, At: matchStart})
		}
	}
	appendVerbatim(cur, len(oldLines))

	return Result{Lines: out, NoFinalNewline: finalNoEOL, Reports: reports, Rejects: rejects}
}

func reverseHunk(h Hunk) Hunk {
	lines := make([]HunkLine, len(h.Lines))
	for i, l := range h.Lines {
		nl := l
		switch l.Kind {
		case LineDelete:
			nl.Kind = LineAdd
		case LineAdd:
			nl.Kind = LineDelete
		}
		lines[i] = nl
	}
	return Hunk{OldStart: h.NewStart, OldCount: h.NewCount, NewStart: h.OldStart, NewCount: h.OldCount, Lines: lines}
}

// buildReplacement walks exactly h.OldCount lines of oldLines starting at
// matchStart, emitting target's own text for context, patch text for
// additions, and nothing for deletions. Using the target's actual text for
// context (rather than the patch's recorded text) is what makes -l and
// fuzzy matches not silently rewrite whitespace on lines the hunk didn't
// touch.
func buildReplacement(oldLines []string, matchStart int, lines []HunkLine, oldNoFinalNewline bool) (repl []string, lastNoEOL bool, ok bool) {
	ti := matchStart
	lastNoEOL = oldNoFinalNewline
	sawAny := false
	for _, l := range lines {
		switch l.Kind {
		case LineContext:
			repl = append(repl, oldLines[ti])
			lastNoEOL = l.NoEOL
			sawAny = true
			ti++
		case LineDelete:
			ti++
		case LineAdd:
			repl = append(repl, l.Text)
			lastNoEOL = l.NoEOL
			sawAny = true
		}
	}
	return repl, lastNoEOL, sawAny
}

// locateHunk finds where h's old-side sequence sits in oldLines, trying
// exact/nearby matches at increasing fuzz before falling back to an
// already-applied check.
func locateHunk(oldLines []string, h Hunk, expected int, opts ApplyOptions) (matchStart, fuzzUsed int, outcome HunkOutcome) {
	leadCtx, trailCtx := edgeContextCounts(h.Lines)
	maxFuzz := max(opts.Fuzz, 0)
	for f := 0; f <= maxFuzz; f++ {
		peelLead := min(f, leadCtx)
		peelTrail := min(f, trailCtx)
		core := h.Lines[peelLead : len(h.Lines)-peelTrail]
		subOld := oldSeq(core)
		// Fuzz relaxes comparison of edge context; it does not make those
		// lines cease to exist. Restrict the core match so the complete
		// old side still fits in oldLines before buildReplacement consumes
		// it. Without these margins a trailing-context peel could match at
		// EOF and make buildReplacement index past the slice.
		minCoreStart := peelLead
		maxCoreStart := len(oldLines) - h.OldCount + peelLead
		pos, found := searchWithin(oldLines, subOld, expected+peelLead, minCoreStart, maxCoreStart, opts.IgnoreWhitespace)
		if found {
			outcome = HunkApplied
			if f > 0 {
				outcome = HunkAppliedFuzzy
			}
			return pos - peelLead, f, outcome
		}
		if peelLead == leadCtx && peelTrail == trailCtx && f < maxFuzz {
			// No more context left to peel; further fuzz rounds are
			// identical, stop early.
			break
		}
	}
	if !opts.Force {
		wantNew := newSeq(h.Lines)
		// An empty new side (a deletion hunk) contains no evidence that the
		// deletion was already applied: an empty sequence matches every
		// file. Treat a failed deletion as a real reject instead of falsely
		// reporting HunkAlreadyApplied.
		if len(wantNew) > 0 {
			if pos, found := search(oldLines, wantNew, expected, opts.IgnoreWhitespace); found {
				return pos, 0, HunkAlreadyApplied
			}
		}
	}
	return clampInt(expected, 0, len(oldLines)), 0, HunkFailed
}

func edgeContextCounts(lines []HunkLine) (lead, trail int) {
	for _, l := range lines {
		if l.Kind != LineContext {
			break
		}
		lead++
	}
	for i := len(lines) - 1; i >= 0; i-- {
		if lines[i].Kind != LineContext {
			break
		}
		trail++
	}
	if lead+trail > len(lines) {
		trail = len(lines) - lead
	}
	return lead, trail
}

func oldSeq(lines []HunkLine) []string {
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		if l.Kind != LineAdd {
			out = append(out, l.Text)
		}
	}
	return out
}

func newSeq(lines []HunkLine) []string {
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		if l.Kind != LineDelete {
			out = append(out, l.Text)
		}
	}
	return out
}

// search looks for want as a contiguous run inside target, starting at hint
// and expanding outward so the nearest match wins over a farther one.
func search(target, want []string, hint int, ignoreWS bool) (int, bool) {
	return searchWithin(target, want, hint, 0, len(target)-len(want), ignoreWS)
}

// searchWithin is search constrained to candidate start positions in the
// inclusive [minStart, maxStart] range. Fuzzy hunk matching uses this to
// reserve space for peeled leading and trailing context.
func searchWithin(target, want []string, hint, minStart, maxStart int, ignoreWS bool) (int, bool) {
	l := len(want)
	sequenceMax := len(target) - l
	if minStart < 0 {
		minStart = 0
	}
	if maxStart > sequenceMax {
		maxStart = sequenceMax
	}
	if minStart > maxStart {
		return 0, false
	}
	if l == 0 {
		return clampInt(hint, minStart, maxStart), true
	}
	start := clampInt(hint, minStart, maxStart)
	for _, cand := range searchOrderWithin(start, minStart, maxStart) {
		if matchAt(target, want, cand, ignoreWS) {
			return cand, true
		}
	}
	return 0, false
}

func searchOrder(start, maxStart int) []int {
	return searchOrderWithin(start, 0, maxStart)
}

func searchOrderWithin(start, minStart, maxStart int) []int {
	order := []int{start}
	for d := 1; start-d >= minStart || start+d <= maxStart; d++ {
		if start-d >= minStart {
			order = append(order, start-d)
		}
		if start+d <= maxStart {
			order = append(order, start+d)
		}
	}
	return order
}

func matchAt(target, want []string, pos int, ignoreWS bool) bool {
	for i, w := range want {
		t := target[pos+i]
		if ignoreWS {
			t, w = normalizeWS(t), normalizeWS(w)
		}
		if t != w {
			return false
		}
	}
	return true
}

func normalizeWS(s string) string {
	var out strings.Builder
	inBlank := false
	for i := 0; i < len(s); i++ {
		if s[i] == ' ' || s[i] == '\t' {
			if !inBlank {
				out.WriteByte(' ')
				inBlank = true
			}
			continue
		}
		inBlank = false
		out.WriteByte(s[i])
	}
	return out.String()
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
