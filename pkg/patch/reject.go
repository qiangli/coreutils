package patch

import "fmt"

// WriteReject renders failed hunks as a unified-format patch fragment,
// suitable for writing to a ".rej" file. GNU patch mirrors the ORIGINAL
// input's notation (context in, context reject); this package always
// emits unified — a documented simplification, not a silent one, tracked
// in docs/patch-continuation-ledger.md.
func WriteReject(oldName, newName string, hunks []Hunk) []byte {
	var out []byte
	out = append(out, fmt.Appendf(nil, "--- %s\n", oldName)...)
	out = append(out, fmt.Appendf(nil, "+++ %s\n", newName)...)
	for _, h := range hunks {
		out = append(out, hunkHeader(h)...)
		for _, l := range h.Lines {
			var mark byte
			switch l.Kind {
			case LineContext:
				mark = ' '
			case LineDelete:
				mark = '-'
			case LineAdd:
				mark = '+'
			}
			out = append(out, mark)
			out = append(out, l.Text...)
			out = append(out, '\n')
			if l.NoEOL {
				out = append(out, "\\ No newline at end of file\n"...)
			}
		}
	}
	return out
}

func hunkHeader(h Hunk) []byte {
	oldRange := unifiedRangeText(h.OldStart, h.OldCount)
	newRange := unifiedRangeText(h.NewStart, h.NewCount)
	return fmt.Appendf(nil, "@@ -%s +%s @@\n", oldRange, newRange)
}

func unifiedRangeText(start0, count int) string {
	switch count {
	case 1:
		return fmt.Sprintf("%d", start0+1)
	case 0:
		return fmt.Sprintf("%d,0", start0)
	default:
		return fmt.Sprintf("%d,%d", start0+1, count)
	}
}
