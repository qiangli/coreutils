package patch

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// rawLine is one physical line of patch input with its trailing-newline
// state preserved (only the very last line of the whole input can lack
// one).
type rawLine struct {
	text  string
	noEOL bool
}

func splitRawLines(data []byte) []rawLine {
	s := string(data)
	trailingNL := strings.HasSuffix(s, "\n")
	if s == "" {
		return nil
	}
	parts := strings.Split(s, "\n")
	if trailingNL {
		parts = parts[:len(parts)-1]
	}
	out := make([]rawLine, len(parts))
	for i, p := range parts {
		out[i].text = strings.TrimSuffix(p, "\r")
	}
	if !trailingNL && len(out) > 0 {
		out[len(out)-1].noEOL = true
	}
	return stripCommonIndent(out)
}

// stripCommonIndent accepts the historical indented-patch transport form.
// Once the first recognizable control line is indented, the same exact byte
// prefix is removed from every physical line that carries it. Patch payload
// indentation after the diff marker is therefore preserved.
func stripCommonIndent(lines []rawLine) []rawLine {
	for _, line := range lines {
		trimmed := strings.TrimLeft(line.text, " \t")
		if trimmed == line.text {
			if strings.HasPrefix(trimmed, "Index:") || strings.HasPrefix(trimmed, "diff --git ") ||
				strings.HasPrefix(trimmed, "--- ") || strings.HasPrefix(trimmed, "*** ") ||
				normalHunkRe.MatchString(trimmed) {
				return lines
			}
			continue
		}
		if strings.HasPrefix(trimmed, "Index:") || strings.HasPrefix(trimmed, "diff --git ") ||
			strings.HasPrefix(trimmed, "--- ") || strings.HasPrefix(trimmed, "*** ") ||
			normalHunkRe.MatchString(trimmed) {
			prefix := line.text[:len(line.text)-len(trimmed)]
			out := append([]rawLine(nil), lines...)
			for i := range out {
				if strings.HasPrefix(out[i].text, prefix) {
					out[i].text = out[i].text[len(prefix):]
				}
			}
			return out
		}
	}
	return lines
}

var (
	unifiedHunkRe = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)
	contextSepRe  = regexp.MustCompile(`^\*{9,}\s*$`)
	contextOldRe  = regexp.MustCompile(`^\*\*\* (\d+)(?:,(\d+))? \*\*\*\*\s*$`)
	contextNewRe  = regexp.MustCompile(`^--- (\d+)(?:,(\d+))? ----\s*$`)
	normalHunkRe  = regexp.MustCompile(`^(\d+)(?:,(\d+))?([acd])(\d+)(?:,(\d+))?$`)
	gitDiffRe     = regexp.MustCompile(`^diff --git a/(.+) b/(.+)$`)
)

// Parse scans data for one or more per-file patches, auto-detecting each
// section's notation (unified, context, or normal) from its own headers.
// Lines that precede or separate recognized sections (Index: lines, cvs/svn
// banners, blank lines, and so on) are skipped rather than rejected, the
// same leniency POSIX patch(1p) documents ("garbage" before the diff
// proper is ignored).
func Parse(data []byte) (*Patch, error) {
	lines := splitRawLines(data)
	n := len(lines)
	var files []FilePatch
	var indexName string
	for i := 0; i < n; {
		switch {
		case strings.HasPrefix(lines[i].text, "Index:"):
			indexName = strings.TrimSpace(strings.TrimPrefix(lines[i].text, "Index:"))
			i++
		case strings.HasPrefix(lines[i].text, "diff --git "):
			fp, ni, err := parseGitWrappedFile(lines, i)
			if err != nil {
				return nil, err
			}
			fp.IndexName = indexName
			files = append(files, fp)
			indexName = ""
			i = ni
		case strings.HasPrefix(lines[i].text, "--- ") && i+1 < n && strings.HasPrefix(lines[i+1].text, "+++ "):
			fp, ni, err := parseUnifiedFile(lines, i)
			if err != nil {
				return nil, err
			}
			fp.IndexName = indexName
			files = append(files, fp)
			indexName = ""
			i = ni
		case strings.HasPrefix(lines[i].text, "*** ") && i+1 < n && strings.HasPrefix(lines[i+1].text, "--- "):
			fp, ni, err := parseContextFile(lines, i)
			if err != nil {
				return nil, err
			}
			fp.IndexName = indexName
			files = append(files, fp)
			indexName = ""
			i = ni
		case normalHunkRe.MatchString(lines[i].text):
			fp, ni, err := parseNormalFile(lines, i)
			if err != nil {
				return nil, err
			}
			fp.IndexName = indexName
			files = append(files, fp)
			indexName = ""
			i = ni
		default:
			i++
		}
	}
	if len(files) == 0 {
		return nil, errors.New("no recognizable patch hunks found in input")
	}
	return &Patch{Files: files}, nil
}

// extractName reads a "--- "/"+++ "/"*** " header's remainder: the pathname
// up to a tab (GNU/POSIX separate an optional trailing timestamp with a
// tab), or the whole trimmed remainder when no timestamp was written.
func extractName(rest string) string {
	if name, _, ok := strings.Cut(rest, "\t"); ok {
		return name
	}
	return strings.TrimRight(rest, " \t")
}

// unifiedRangeVal inverts cmds/diff's unifiedRange encoding: an omitted
// count means exactly one line at num (1-based); an explicit ",0" count
// means zero lines and num is already the 0-based insertion point; any
// other explicit count means num is the 1-based first line.
func unifiedRangeVal(num int, hasCount bool, count int) (start0, n int) {
	if !hasCount {
		return num - 1, 1
	}
	if count == 0 {
		return num, 0
	}
	return num - 1, count
}

func parseUnifiedFile(lines []rawLine, i int) (FilePatch, int, error) {
	oldName := extractName(lines[i].text[len("--- "):])
	newName := extractName(lines[i+1].text[len("+++ "):])
	i += 2
	n := len(lines)
	var hunks []Hunk
	for i < n && strings.HasPrefix(lines[i].text, "@@ ") {
		m := unifiedHunkRe.FindStringSubmatch(lines[i].text)
		if m == nil {
			return FilePatch{}, 0, fmt.Errorf("patch: malformed unified hunk header %q", lines[i].text)
		}
		oldNum, _ := strconv.Atoi(m[1])
		oldStart0, oldCount := unifiedRangeVal(oldNum, m[2] != "", atoiDefault(m[2], 0))
		newNum, _ := strconv.Atoi(m[3])
		newStart0, newCount := unifiedRangeVal(newNum, m[4] != "", atoiDefault(m[4], 0))
		i++

		var hl []HunkLine
		oldSeen, newSeen := 0, 0
		for oldSeen < oldCount || newSeen < newCount {
			if i >= n {
				return FilePatch{}, 0, fmt.Errorf("patch: unexpected end of input inside a hunk for %q", newName)
			}
			text := lines[i].text
			switch {
			case len(text) > 0 && text[0] == '\\':
				if len(hl) > 0 {
					hl[len(hl)-1].NoEOL = true
				}
				i++
				continue
			case text == "" || text[0] == ' ':
				body := ""
				if len(text) > 0 {
					body = text[1:]
				}
				hl = append(hl, HunkLine{Kind: LineContext, Text: body, NoEOL: lines[i].noEOL})
				oldSeen++
				newSeen++
			case text[0] == '-':
				hl = append(hl, HunkLine{Kind: LineDelete, Text: text[1:], NoEOL: lines[i].noEOL})
				oldSeen++
			case text[0] == '+':
				hl = append(hl, HunkLine{Kind: LineAdd, Text: text[1:], NoEOL: lines[i].noEOL})
				newSeen++
			default:
				return FilePatch{}, 0, fmt.Errorf("patch: unexpected line %q inside a unified hunk", text)
			}
			i++
		}
		// A "\ No newline at end of file" marker for the hunk's very last
		// line arrives only after both counts are already satisfied.
		for i < n && len(lines[i].text) > 0 && lines[i].text[0] == '\\' {
			if len(hl) > 0 {
				hl[len(hl)-1].NoEOL = true
			}
			i++
		}
		hunks = append(hunks, Hunk{OldStart: oldStart0, OldCount: oldCount, NewStart: newStart0, NewCount: newCount, Lines: hl})
	}
	return FilePatch{OldName: oldName, NewName: newName, Format: FormatUnified, Hunks: hunks}, i, nil
}

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return v
}

// contextRangeVal inverts cmds/diff's contextRange encoding, deferring the
// 0-vs-1-line ambiguity of a single bare number to the caller, which knows
// the true count once it has counted the block's actual body lines.
func contextRangeVal(hasSecond bool, first, second int) (start0Explicit bool, start0 int, countFromHeader int) {
	if hasSecond {
		return true, first - 1, second - first + 1
	}
	return false, 0, first
}

type markedLine struct {
	mark  string
	text  string
	noEOL bool
}

// consumeMarkedBlock reads lines whose first two bytes are one of marks,
// tolerating an interleaved "\ No newline..." marker that tags the
// previously read line.
func consumeMarkedBlock(lines []rawLine, i int, marks map[string]bool) ([]markedLine, int) {
	var out []markedLine
	for i < len(lines) {
		text := lines[i].text
		if len(text) > 0 && text[0] == '\\' {
			if len(out) > 0 {
				out[len(out)-1].noEOL = true
			}
			i++
			continue
		}
		if len(text) < 2 || !marks[text[:2]] {
			break
		}
		out = append(out, markedLine{mark: text[:2], text: text[2:], noEOL: lines[i].noEOL})
		i++
	}
	return out, i
}

var oldBlockMarks = map[string]bool{"  ": true, "- ": true, "! ": true}
var newBlockMarks = map[string]bool{"  ": true, "+ ": true, "! ": true}

func parseContextFile(lines []rawLine, i int) (FilePatch, int, error) {
	oldName := extractName(lines[i].text[len("*** "):])
	newName := extractName(lines[i+1].text[len("--- "):])
	i += 2
	n := len(lines)
	var hunks []Hunk
	for i < n && contextSepRe.MatchString(lines[i].text) {
		i++
		if i >= n {
			return FilePatch{}, 0, errors.New("patch: unexpected end of input after a context hunk separator")
		}
		om := contextOldRe.FindStringSubmatch(lines[i].text)
		if om == nil {
			return FilePatch{}, 0, fmt.Errorf("patch: expected a context old-range header, got %q", lines[i].text)
		}
		oldFirst, _ := strconv.Atoi(om[1])
		oldHasSecond, oldSecondStr := om[2] != "", om[2]
		oldSecond := atoiDefault(oldSecondStr, 0)
		i++

		var oldBlock []markedLine
		if !contextNewRe.MatchString(lines[i].text) {
			oldBlock, i = consumeMarkedBlock(lines, i, oldBlockMarks)
		}
		if i >= n {
			return FilePatch{}, 0, errors.New("patch: expected a context new-range header, got end of input")
		}
		nm := contextNewRe.FindStringSubmatch(lines[i].text)
		if nm == nil {
			return FilePatch{}, 0, fmt.Errorf("patch: expected a context new-range header, got %q", lines[i].text)
		}
		newFirst, _ := strconv.Atoi(nm[1])
		newHasSecond, newSecondStr := nm[2] != "", nm[2]
		newSecond := atoiDefault(newSecondStr, 0)
		i++

		var newBlock []markedLine
		if i < n && !contextSepRe.MatchString(lines[i].text) {
			newBlock, i = consumeMarkedBlock(lines, i, newBlockMarks)
		}

		oldStart0, oldCount := resolveContextStart(oldHasSecond, oldFirst, oldSecond, len(oldBlock))
		newStart0, newCount := resolveContextStart(newHasSecond, newFirst, newSecond, len(newBlock))

		hl := zipContextBlocks(oldBlock, newBlock)
		hunks = append(hunks, Hunk{OldStart: oldStart0, OldCount: oldCount, NewStart: newStart0, NewCount: newCount, Lines: hl})
	}
	return FilePatch{OldName: oldName, NewName: newName, Format: FormatContext, Hunks: hunks}, i, nil
}

func resolveContextStart(hasSecond bool, first, second, actualCount int) (start0, count int) {
	explicit, s0, headerCount := contextRangeVal(hasSecond, first, second)
	if explicit {
		// The header's own span is authoritative even when the block
		// body was omitted entirely (a pure insertion/deletion hunk
		// still spans real old/new lines; they just aren't printed
		// because they're all context, reconstructed from the other
		// side's block).
		return s0, headerCount
	}
	// Bare single number: the header collapses count 0 and count 1 to
	// different numbers (see cmds/diff's contextRange), so the actual
	// parsed block length disambiguates which one this was.
	return first - actualCount, actualCount
}

// zipContextBlocks reconstructs one interleaved edit sequence from the
// independently-collected old-file and new-file halves of a context hunk.
// Context lines are the sync points: they appear, with identical text, at
// the same relative position in both halves, so a run of non-context lines
// in one half lines up with the run in the other.
func zipContextBlocks(oldBlock, newBlock []markedLine) []HunkLine {
	var hl []HunkLine
	oi, ni := 0, 0
	for oi < len(oldBlock) || ni < len(newBlock) {
		if oi < len(oldBlock) && oldBlock[oi].mark == "  " {
			hl = append(hl, HunkLine{Kind: LineContext, Text: oldBlock[oi].text, NoEOL: oldBlock[oi].noEOL})
			oi++
			if ni < len(newBlock) && newBlock[ni].mark == "  " {
				ni++
			}
			continue
		}
		for oi < len(oldBlock) && oldBlock[oi].mark != "  " {
			hl = append(hl, HunkLine{Kind: LineDelete, Text: oldBlock[oi].text, NoEOL: oldBlock[oi].noEOL})
			oi++
		}
		for ni < len(newBlock) && newBlock[ni].mark != "  " {
			hl = append(hl, HunkLine{Kind: LineAdd, Text: newBlock[ni].text, NoEOL: newBlock[ni].noEOL})
			ni++
		}
		if oi >= len(oldBlock) && ni < len(newBlock) && newBlock[ni].mark == "  " {
			// Old block was entirely absent (pure insertion): drain
			// remaining new-side context directly.
			hl = append(hl, HunkLine{Kind: LineContext, Text: newBlock[ni].text, NoEOL: newBlock[ni].noEOL})
			ni++
		}
	}
	return hl
}

func parseNormalFile(lines []rawLine, i int) (FilePatch, int, error) {
	n := len(lines)
	var hunks []Hunk
	for i < n {
		m := normalHunkRe.FindStringSubmatch(lines[i].text)
		if m == nil {
			break
		}
		oldFirst, _ := strconv.Atoi(m[1])
		oldHasSecond := m[2] != ""
		oldSecond := atoiDefault(m[2], 0)
		op := m[3]
		newFirst, _ := strconv.Atoi(m[4])
		newHasSecond := m[5] != ""
		newSecond := atoiDefault(m[5], 0)
		i++

		oldCount := 1
		if oldHasSecond {
			oldCount = oldSecond - oldFirst + 1
		}
		newCount := 1
		if newHasSecond {
			newCount = newSecond - newFirst + 1
		}

		var hl []HunkLine
		var oldStart0, newStart0 int
		var err error
		switch op {
		case "a":
			oldStart0, oldCount = oldFirst, 0
			newStart0 = newFirst - 1
			var adds []markedLine
			adds, i, err = consumeFixedMarked(lines, i, "> ", newCount)
			if err != nil {
				return FilePatch{}, 0, err
			}
			for _, l := range adds {
				hl = append(hl, HunkLine{Kind: LineAdd, Text: l.text, NoEOL: l.noEOL})
			}
		case "d":
			oldStart0 = oldFirst - 1
			newStart0, newCount = newFirst, 0
			var dels []markedLine
			dels, i, err = consumeFixedMarked(lines, i, "< ", oldCount)
			if err != nil {
				return FilePatch{}, 0, err
			}
			for _, l := range dels {
				hl = append(hl, HunkLine{Kind: LineDelete, Text: l.text, NoEOL: l.noEOL})
			}
		case "c":
			oldStart0 = oldFirst - 1
			newStart0 = newFirst - 1
			var dels []markedLine
			dels, i, err = consumeFixedMarked(lines, i, "< ", oldCount)
			if err != nil {
				return FilePatch{}, 0, err
			}
			for _, l := range dels {
				hl = append(hl, HunkLine{Kind: LineDelete, Text: l.text, NoEOL: l.noEOL})
			}
			if i >= n || lines[i].text != "---" {
				return FilePatch{}, 0, fmt.Errorf("patch: expected '---' separator in a change hunk, got %q", peekText(lines, i))
			}
			i++
			var adds []markedLine
			adds, i, err = consumeFixedMarked(lines, i, "> ", newCount)
			if err != nil {
				return FilePatch{}, 0, err
			}
			for _, l := range adds {
				hl = append(hl, HunkLine{Kind: LineAdd, Text: l.text, NoEOL: l.noEOL})
			}
		}
		hunks = append(hunks, Hunk{OldStart: oldStart0, OldCount: oldCount, NewStart: newStart0, NewCount: newCount, Lines: hl})
	}
	if len(hunks) == 0 {
		return FilePatch{}, 0, errors.New("patch: no hunks found in normal-format diff")
	}
	return FilePatch{Format: FormatNormal, Hunks: hunks}, i, nil
}

func peekText(lines []rawLine, i int) string {
	if i >= len(lines) {
		return "<end of input>"
	}
	return lines[i].text
}

func consumeFixedMarked(lines []rawLine, i int, prefix string, want int) ([]markedLine, int, error) {
	var out []markedLine
	for len(out) < want {
		if i >= len(lines) {
			return nil, 0, fmt.Errorf("patch: expected %d %q line(s), ran out of input", want, prefix)
		}
		text := lines[i].text
		if len(text) > 0 && text[0] == '\\' {
			if len(out) > 0 {
				out[len(out)-1].noEOL = true
			}
			i++
			continue
		}
		if !strings.HasPrefix(text, prefix) {
			return nil, 0, fmt.Errorf("patch: expected a %q line, got %q", prefix, text)
		}
		out = append(out, markedLine{text: text[len(prefix):], noEOL: lines[i].noEOL})
		i++
	}
	return out, i, nil
}

// parseGitWrappedFile handles a "diff --git a/X b/Y" section: it skips the
// git-specific extended headers (index/mode/similarity/rename lines) and
// hands off to parseUnifiedFile for the textual hunk that normally follows.
// A section with no textual hunk (a pure rename, mode change, or binary
// patch) comes back as an unsupported FilePatch rather than being silently
// dropped — see docs/patch-continuation-ledger.md.
func parseGitWrappedFile(lines []rawLine, i int) (FilePatch, int, error) {
	gm := gitDiffRe.FindStringSubmatch(lines[i].text)
	var gitOld, gitNew string
	if gm != nil {
		gitOld, gitNew = "a/"+gm[1], "b/"+gm[2]
	}
	i++
	n := len(lines)
	var renameFrom, renameTo string
	binary := false
	for i < n {
		text := lines[i].text
		switch {
		case strings.HasPrefix(text, "--- "), strings.HasPrefix(text, "diff --git "):
			goto doneHeaders
		case strings.HasPrefix(text, "rename from "):
			renameFrom = text[len("rename from "):]
		case strings.HasPrefix(text, "rename to "):
			renameTo = text[len("rename to "):]
		case strings.HasPrefix(text, "GIT binary patch"), strings.HasPrefix(text, "Binary files "):
			binary = true
		}
		i++
	}
doneHeaders:
	if i < n && strings.HasPrefix(lines[i].text, "--- ") && i+1 < n && strings.HasPrefix(lines[i+1].text, "+++ ") {
		fp, ni, err := parseUnifiedFile(lines, i)
		if err != nil {
			return FilePatch{}, 0, err
		}
		fp.RenameFrom, fp.RenameTo = renameFrom, renameTo
		return fp, ni, nil
	}
	reason := "git diff section has no textual hunk to apply (metadata-only or unrecognized)"
	if binary {
		reason = "binary patch content is not supported by pure-Go coreutils"
	} else if renameFrom != "" || renameTo != "" {
		reason = "a git rename with no content hunk is not supported by pure-Go coreutils"
	}
	return FilePatch{
		OldName:     gitOld,
		NewName:     gitNew,
		RenameFrom:  renameFrom,
		RenameTo:    renameTo,
		Unsupported: reason,
	}, i, nil
}
