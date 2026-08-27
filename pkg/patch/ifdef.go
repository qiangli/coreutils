package patch

// ApplyIfdef applies hunks while retaining both versions under C preprocessor
// conditionals named by define. It uses the same exact/fuzz/whitespace matcher
// as ordinary application, so -D never guesses at a location.
func ApplyIfdef(oldLines []string, oldNoFinalNewline bool, hunks []Hunk, opts ApplyOptions, define string) Result {
	work := make([]Hunk, len(hunks))
	for i, h := range hunks {
		if opts.Reverse {
			work[i] = reverseHunk(h)
		} else {
			work[i] = h
		}
	}
	var out []string
	cur := 0
	result := Result{NoFinalNewline: oldNoFinalNewline}
	for i, h := range work {
		at, fuzz, outcome := locateHunk(oldLines, h, h.OldStart, opts)
		if outcome == HunkFailed || outcome == HunkAlreadyApplied || at < cur {
			result.Reports = append(result.Reports, HunkReport{Index: i + 1, Outcome: HunkFailed})
			result.Rejects = append(result.Rejects, h)
			continue
		}
		out = append(out, oldLines[cur:at]...)
		ti := at
		for p := 0; p < len(h.Lines); {
			if h.Lines[p].Kind == LineContext {
				out = append(out, oldLines[ti])
				ti++
				p++
				continue
			}
			var oldPart, newPart []string
			for p < len(h.Lines) && h.Lines[p].Kind != LineContext {
				switch h.Lines[p].Kind {
				case LineDelete:
					oldPart = append(oldPart, oldLines[ti])
					ti++
				case LineAdd:
					newPart = append(newPart, h.Lines[p].Text)
				}
				p++
			}
			switch {
			case len(oldPart) > 0 && len(newPart) > 0:
				out = append(out, "#ifndef "+define)
				out = append(out, oldPart...)
				out = append(out, "#else")
				out = append(out, newPart...)
				out = append(out, "#endif")
			case len(oldPart) > 0:
				out = append(out, "#ifndef "+define)
				out = append(out, oldPart...)
				out = append(out, "#endif")
			case len(newPart) > 0:
				out = append(out, "#ifdef "+define)
				out = append(out, newPart...)
				out = append(out, "#endif")
			}
		}
		cur = at + h.OldCount
		if cur == len(oldLines) {
			result.NoFinalNewline = false
			if len(h.Lines) > 0 && h.Lines[len(h.Lines)-1].Kind == LineContext {
				result.NoFinalNewline = h.Lines[len(h.Lines)-1].NoEOL
			}
		}
		repOutcome := HunkApplied
		if fuzz > 0 {
			repOutcome = HunkAppliedFuzzy
		}
		result.Reports = append(result.Reports, HunkReport{Index: i + 1, Outcome: repOutcome, FuzzUsed: fuzz, At: at})
	}
	out = append(out, oldLines[cur:]...)
	if cur < len(oldLines) {
		result.NoFinalNewline = oldNoFinalNewline
	}
	result.Lines = out
	return result
}
