package patch

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var edCommandRE = regexp.MustCompile(`^(?:(\d+)(?:,(\d+))?)?([acdi])$`)

// edScriptStartRE recognizes the first command of a diff -e script. Unlike
// edCommandRE it requires an address, because diff -e always emits one and a
// bare "a", "c", "d", or "i" is an ordinary line of context in the other three
// Issue 7 input forms. Without the address requirement a unified diff whose
// context happens to contain such a line is taken for an ed script and the
// whole patch is misapplied.
var edScriptStartRE = regexp.MustCompile(`^\d+(?:,\d+)?[acdi]$`)

// normalDiffCommandRE recognizes a normal-difference command ("2c2", "0a1",
// "5,7d4"), which identifies the input as a normal diff rather than an ed
// script.
var normalDiffCommandRE = regexp.MustCompile(`^\d+(?:,\d+)?[acd]\d+(?:,\d+)?$`)

// looksLikeDiffListing reports whether a line identifies the input as one of
// the three textual diff forms. POSIX has patch determine the input type from
// the format of the information it contains; once a copied-context, unified,
// or normal listing has announced itself, later lines that resemble ed
// commands are that listing's data, not a script.
func looksLikeDiffListing(trimmed string) bool {
	for _, marker := range []string{"--- ", "+++ ", "*** ", "@@ "} {
		if strings.HasPrefix(trimmed, marker) {
			return true
		}
	}
	if trimmed == "***************" {
		return true
	}
	return normalDiffCommandRE.MatchString(trimmed)
}

// EdRejectError reports syntactically valid diff -e portions whose addresses
// could not be placed. Result bytes returned alongside this error include all
// other portions that could be applied.
type EdRejectError struct {
	Hunks   []Hunk
	Applied bool
}

func (e *EdRejectError) Error() string {
	return fmt.Sprintf("patch: %d ed-style difference(s) could not be placed", len(e.Hunks))
}

// ExtractEdScript finds the first command emitted by diff -e after optional
// patch header material. It returns the script proper and the nearest Index:
// pathname so the CLI can perform normal filename determination.
//
// Detection is deliberately conservative: an addressed ed command is required,
// and any line that announces a copied-context, unified, or normal difference
// listing ends the search with no match, so only real diff -e output is taken
// for an ed script.
func ExtractEdScript(data []byte) (script []byte, indexName string, ok bool) {
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		line = strings.TrimSuffix(line, "\r")
		trimmed := strings.TrimLeft(line, " \t")
		if strings.HasPrefix(trimmed, "Index:") {
			indexName = strings.TrimSpace(strings.TrimPrefix(trimmed, "Index:"))
			continue
		}
		if trimmed == "" {
			continue
		}
		if looksLikeDiffListing(trimmed) {
			return nil, indexName, false
		}
		if edScriptStartRE.MatchString(trimmed) || trimmed == "s/.//" {
			prefix := line[:len(line)-len(trimmed)]
			scriptLines := append([]string(nil), lines[i:]...)
			if prefix != "" {
				for j := range scriptLines {
					if strings.HasPrefix(scriptLines[j], prefix) {
						scriptLines[j] = scriptLines[j][len(prefix):]
					}
				}
			}
			return []byte(strings.Join(scriptLines, "\n")), indexName, true
		}
	}
	return nil, indexName, false
}

// LooksLikeEdScript is the format-only convenience wrapper.
func LooksLikeEdScript(script []byte) bool {
	_, _, ok := ExtractEdScript(script)
	return ok
}

// ApplyEd applies the command subset emitted by POSIX diff -e. The commands
// are deliberately executed in input order: diff emits them from the bottom
// of the file upward, so earlier edits cannot invalidate later addresses.
func ApplyEd(content, script []byte) ([]byte, error) {
	return applyEd(content, script, "")
}

// ApplyEdIfdef applies a diff -e script while retaining both sides under the
// Issue 7 -D preprocessor contract.
func ApplyEdIfdef(content, script []byte, define string) ([]byte, error) {
	return applyEd(content, script, define)
}

func applyEd(content, script []byte, define string) ([]byte, error) {
	// Keep diff -e conditional output byte-for-byte consistent with the
	// POSIX -D form used for the other supported difference listings.
	endif := "#endif /* " + define + " */"
	if len(script) == 0 {
		return append([]byte(nil), content...), nil
	}
	lines, noEOL := bytesToLines(content)
	physical := strings.Split(strings.TrimSuffix(string(script), "\n"), "\n")
	current := 0 // ed's current line address, one-based; zero is before line 1.
	var rejects []Hunk
	applied := false
	for i := 0; i < len(physical); {
		command := strings.TrimSuffix(physical[i], "\r")
		i++
		// diff -e protects a data line consisting solely of "." by inserting
		// ".." and then emitting this substitution against the current line.
		if command == "s/.//" {
			if current < 1 || current > len(lines) || lines[current-1] == "" {
				rejects = append(rejects, edRejectHunk(current, current, "s", nil))
				continue
			}
			lines[current-1] = lines[current-1][1:]
			applied = true
			continue
		}
		m := edCommandRE.FindStringSubmatch(command)
		if m == nil {
			return nil, fmt.Errorf("patch: malformed ed command %q", command)
		}
		first := current
		if m[1] != "" {
			first, _ = strconv.Atoi(m[1])
		}
		last := first
		if m[2] != "" {
			last, _ = strconv.Atoi(m[2])
		}
		if first < 0 || last < first {
			return nil, fmt.Errorf("patch: invalid ed address %q", command)
		}
		var text []string
		if m[3] == "a" || m[3] == "c" || m[3] == "i" {
			terminated := false
			for i < len(physical) {
				line := strings.TrimSuffix(physical[i], "\r")
				i++
				if line == "." {
					terminated = true
					break
				}
				text = append(text, line)
			}
			if !terminated {
				return nil, fmt.Errorf("patch: unterminated ed text after %q", command)
			}
		}
		oldLen := len(lines)
		switch m[3] {
		case "a":
			if last > len(lines) {
				rejects = append(rejects, edRejectHunk(first, last, m[3], text))
				continue
			}
			inserted := text
			if define != "" {
				inserted = conditionalAddition(text, define)
			}
			lines = spliceLines(lines, last, last, inserted)
			current = last + len(inserted)
			if define != "" && len(text) > 0 {
				current-- // Keep dot-unquoting on the final payload line, not #endif.
			}
			if last == oldLen {
				noEOL = false
			}
			applied = true
		case "d":
			if first < 1 || last > len(lines) {
				rejects = append(rejects, edRejectHunk(first, last, m[3], text))
				continue
			}
			replacement := []string(nil)
			if define != "" {
				replacement = append([]string{"#ifndef " + define}, lines[first-1:last]...)
				replacement = append(replacement, endif)
			}
			lines = spliceLines(lines, first-1, last, replacement)
			current = min(first, len(lines))
			if last == oldLen {
				noEOL = false
			}
			applied = true
		case "c":
			if first == 0 {
				first, last = 1, max(last, 1)
			}
			if first < 1 || last > len(lines) {
				rejects = append(rejects, edRejectHunk(first, last, m[3], text))
				continue
			}
			replacement := text
			if define != "" {
				replacement = append([]string{"#ifndef " + define}, lines[first-1:last]...)
				replacement = append(replacement, "#else")
				replacement = append(replacement, text...)
				replacement = append(replacement, endif)
			}
			lines = spliceLines(lines, first-1, last, replacement)
			current = first - 1 + len(replacement)
			if define != "" && len(text) > 0 {
				current--
			}
			if last == oldLen {
				noEOL = false
			}
			applied = true
		case "i":
			if first == 0 {
				first = 1
			}
			if first < 1 || first > len(lines) {
				rejects = append(rejects, edRejectHunk(first, first, m[3], text))
				continue
			}
			inserted := text
			if define != "" {
				inserted = conditionalAddition(text, define)
			}
			lines = spliceLines(lines, first-1, first-1, inserted)
			current = first - 1 + len(inserted)
			if define != "" && len(text) > 0 {
				current--
			}
			applied = true
		}
	}
	result := linesToBytes(lines, noEOL)
	if len(rejects) != 0 {
		return result, &EdRejectError{Hunks: rejects, Applied: applied}
	}
	return result, nil
}

func edRejectHunk(first, last int, operation string, text []string) Hunk {
	start := max(first-1, 0)
	h := Hunk{OldStart: start, NewStart: start}
	switch operation {
	case "a":
		h.OldStart, h.NewStart = max(last, 0), max(last, 0)
	case "i":
		// Insertion is before the addressed line.
	case "c":
		h.OldCount = max(last-first+1, 0)
	case "d", "s":
		h.OldCount = max(last-first+1, 1)
	}
	if operation == "a" || operation == "i" || operation == "c" {
		h.NewCount = len(text)
		for _, line := range text {
			h.Lines = append(h.Lines, HunkLine{Kind: LineAdd, Text: line})
		}
	}
	return h
}

func conditionalAddition(text []string, define string) []string {
	out := make([]string, 0, len(text)+2)
	out = append(out, "#ifdef "+define)
	out = append(out, text...)
	out = append(out, "#endif /* "+define+" */")
	return out
}

func spliceLines(lines []string, from, to int, replacement []string) []string {
	out := make([]string, 0, len(lines)-(to-from)+len(replacement))
	out = append(out, lines[:from]...)
	out = append(out, replacement...)
	out = append(out, lines[to:]...)
	return out
}

func bytesToLines(content []byte) ([]string, bool) {
	if len(content) == 0 {
		return nil, false
	}
	s := string(content)
	noEOL := !strings.HasSuffix(s, "\n")
	parts := strings.Split(s, "\n")
	if !noEOL {
		parts = parts[:len(parts)-1]
	}
	return parts, noEOL
}

func linesToBytes(lines []string, noEOL bool) []byte {
	if len(lines) == 0 {
		return nil
	}
	s := strings.Join(lines, "\n")
	if !noEOL {
		s += "\n"
	}
	return []byte(s)
}
