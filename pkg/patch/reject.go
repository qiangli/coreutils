package patch

import "fmt"

// WriteReject renders failed hunks in the original input notation.
func WriteReject(oldName, newName string, hunks []Hunk) []byte {
	return WriteRejectFormat(oldName, newName, FormatUnified, hunks)
}

func WriteRejectFormat(oldName, newName string, format Format, hunks []Hunk) []byte {
	switch format {
	case FormatContext:
		return writeContextReject(oldName, newName, hunks)
	case FormatNormal:
		return writeContextReject(oldName, newName, hunks)
	default:
		return writeUnifiedReject(oldName, newName, hunks)
	}
}

func writeUnifiedReject(oldName, newName string, hunks []Hunk) []byte {
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

func writeContextReject(oldName, newName string, hunks []Hunk) []byte {
	var out []byte
	out = append(out, fmt.Appendf(nil, "*** %s\n--- %s\n", oldName, newName)...)
	for _, h := range hunks {
		hasDelete, hasAdd := false, false
		for _, l := range h.Lines {
			hasDelete = hasDelete || l.Kind == LineDelete
			hasAdd = hasAdd || l.Kind == LineAdd
		}
		out = append(out, "***************\n"...)
		out = append(out, fmt.Appendf(nil, "*** %s ****\n", contextRangeText(h.OldStart, h.OldCount))...)
		for _, l := range h.Lines {
			if l.Kind == LineAdd {
				continue
			}
			mark := "  "
			if l.Kind == LineDelete {
				mark = "- "
				if hasAdd {
					mark = "! "
				}
			}
			out = appendContextRejectLine(out, mark, l)
		}
		out = append(out, fmt.Appendf(nil, "--- %s ----\n", contextRangeText(h.NewStart, h.NewCount))...)
		for _, l := range h.Lines {
			if l.Kind == LineDelete {
				continue
			}
			mark := "  "
			if l.Kind == LineAdd {
				mark = "+ "
				if hasDelete {
					mark = "! "
				}
			}
			out = appendContextRejectLine(out, mark, l)
		}
	}
	return out
}

func appendContextRejectLine(out []byte, mark string, line HunkLine) []byte {
	out = append(out, mark...)
	out = append(out, line.Text...)
	out = append(out, '\n')
	if line.NoEOL {
		out = append(out, "\\ No newline at end of file\n"...)
	}
	return out
}

func contextRangeText(start0, count int) string {
	if count <= 1 {
		return fmt.Sprintf("%d", start0+count)
	}
	return fmt.Sprintf("%d,%d", start0+1, start0+count)
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
