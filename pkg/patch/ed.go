package patch

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var edCommandRE = regexp.MustCompile(`^(?:(\d+)(?:,(\d+))?)?([acd])$`)

// ApplyEd applies the command subset emitted by POSIX diff -e. The commands
// are deliberately executed in input order: diff emits them from the bottom
// of the file upward, so earlier edits cannot invalidate later addresses.
func ApplyEd(content, script []byte) ([]byte, error) {
	if len(script) == 0 {
		return append([]byte(nil), content...), nil
	}
	lines, noEOL := bytesToLines(content)
	physical := strings.Split(strings.TrimSuffix(string(script), "\n"), "\n")
	current := 0 // ed's current line address, one-based; zero is before line 1.
	for i := 0; i < len(physical); {
		command := strings.TrimSuffix(physical[i], "\r")
		i++
		// diff -e protects a data line consisting solely of "." by inserting
		// ".." and then emitting this substitution against the current line.
		if command == "s/.//" {
			if current < 1 || current > len(lines) || lines[current-1] == "" {
				return nil, fmt.Errorf("patch: ed substitution has no non-empty current line")
			}
			lines[current-1] = lines[current-1][1:]
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
		if m[3] == "a" || m[3] == "c" {
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
				return nil, fmt.Errorf("patch: ed address %d exceeds file length %d", last, len(lines))
			}
			lines = spliceLines(lines, last, last, text)
			current = last + len(text)
			if last == oldLen {
				noEOL = false
			}
		case "d":
			if first < 1 || last > len(lines) {
				return nil, fmt.Errorf("patch: ed range %d,%d exceeds file length %d", first, last, len(lines))
			}
			lines = spliceLines(lines, first-1, last, nil)
			current = min(first, len(lines))
			if last == oldLen {
				noEOL = false
			}
		case "c":
			if first < 1 || last > len(lines) {
				return nil, fmt.Errorf("patch: ed range %d,%d exceeds file length %d", first, last, len(lines))
			}
			lines = spliceLines(lines, first-1, last, text)
			current = first - 1 + len(text)
			if last == oldLen {
				noEOL = false
			}
		}
	}
	return linesToBytes(lines, noEOL), nil
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
