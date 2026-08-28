package editor

import (
	"fmt"
	"strconv"
	"strings"
)

func (e *Engine) parseAddresses(s string) ([]int, int, bool, error) {
	pos := skipBlank(s, 0)
	var out []int
	a, next, ok, err := e.parseAddress(s, pos)
	if err != nil {
		return nil, pos, false, err
	}
	if ok {
		out = append(out, a)
		pos = next
	}
	for {
		p := skipBlank(s, pos)
		if p >= len(s) || (s[p] != ',' && s[p] != ';') {
			return out, p, len(out) > 0, nil
		}
		sep := s[p]
		firstOmitted := len(out) == 0
		if len(out) == 0 {
			if sep == ',' {
				out = append(out, 1)
			} else {
				out = append(out, e.Buffer.Current)
			}
		}
		if sep == ';' {
			e.Buffer.Current = out[len(out)-1]
		}
		p = skipBlank(s, p+1)
		b, n, found, err := e.parseAddress(s, p)
		if err != nil {
			return nil, p, true, err
		}
		if !found {
			if firstOmitted {
				b = e.Buffer.Last()
			} else {
				b = out[len(out)-1]
			}
			n = p
		}
		out = append(out, b)
		pos = n
	}
}

func (e *Engine) parseAddress(s string, pos int) (int, int, bool, error) {
	pos = skipBlank(s, pos)
	if pos >= len(s) {
		return 0, pos, false, nil
	}
	value := 0
	switch s[pos] {
	case '\'':
		if pos+1 >= len(s) || s[pos+1] < 'a' || s[pos+1] > 'z' {
			return 0, pos, false, fmt.Errorf("invalid mark address")
		}
		if e.marks == nil {
			return 0, pos, false, fmt.Errorf("invalid address")
		}
		var ok bool
		value, ok = e.marks[s[pos+1]]
		if !ok {
			return 0, pos, false, fmt.Errorf("invalid address")
		}
		pos += 2
	case '.':
		value, pos = e.Buffer.Current, pos+1
	case '$':
		value, pos = e.Buffer.Last(), pos+1
	case '/', '?':
		delim := s[pos]
		pattern, rest, closed := scanRE(s[pos+1:], string(delim))
		consumed := len(s[pos+1:]) - len(rest)
		if closed {
			pos += 1 + consumed
		} else {
			pos = len(s)
		}
		if pattern == "" {
			pattern = e.lastRE
		}
		if pattern == "" {
			return 0, pos, false, fmt.Errorf("no previous regular expression")
		}
		re, err := e.compileRE(pattern)
		if err != nil {
			return 0, pos, false, err
		}
		e.lastRE = pattern
		value, err = e.search(re, delim == '/')
		if err != nil {
			return 0, pos, false, err
		}
	case '+', '-':
		value = e.Buffer.Current
	default:
		if s[pos] < '0' || s[pos] > '9' {
			return 0, pos, false, nil
		}
		start := pos
		for pos < len(s) && s[pos] >= '0' && s[pos] <= '9' {
			pos++
		}
		value, _ = strconv.Atoi(s[start:pos])
	}
	// Address offsets: "+n"/"-n" add or subtract n, a bare "+"/"-" adds or
	// subtracts 1, and a bare decimal number adds that many lines. Offsets
	// may be blank-separated from each other, but a number binds to a sign
	// only when it immediately follows it: "3 ---- 2" is line one.
	for {
		p := skipBlank(s, pos)
		if p >= len(s) {
			pos = p
			break
		}
		sign := 0
		if s[p] == '+' {
			sign, p = 1, p+1
		} else if s[p] == '-' {
			sign, p = -1, p+1
		} else if s[p] >= '0' && s[p] <= '9' {
			sign = 1
		} else {
			pos = p
			break
		}
		start := p
		for p < len(s) && s[p] >= '0' && s[p] <= '9' {
			p++
		}
		delta := 1
		if p > start {
			delta, _ = strconv.Atoi(s[start:p])
		}
		value += sign * delta
		pos = p
	}
	if value < 0 || value > e.Buffer.Last() {
		return 0, pos, false, fmt.Errorf("invalid address")
	}
	return value, pos, true, nil
}

func (e *Engine) search(re matcher, forward bool) (int, error) {
	n := e.Buffer.Last()
	if n == 0 {
		return 0, fmt.Errorf("no match")
	}
	for step := 1; step <= n; step++ {
		idx := 0
		if forward {
			idx = (e.Buffer.Current + step - 1) % n
		} else {
			idx = (e.Buffer.Current - step - 1 + n*2) % n
		}
		ok, err := re.MatchString(e.Buffer.Lines[idx])
		if err != nil {
			return 0, err
		}
		if ok {
			return idx + 1, nil
		}
	}
	return 0, fmt.Errorf("no match")
}

func skipBlank(s string, p int) int {
	for p < len(s) && strings.ContainsRune(" \t", rune(s[p])) {
		p++
	}
	return p
}

func trimBlank(s string) string { return strings.Trim(s, " \t") }

// commandLetter returns the command character of a command line after a
// purely syntactic skip over its addresses. It exists so a global command
// list can tell an s command, whose escaped newlines continue the
// replacement, from the other commands, whose trailing backslash only
// continues the list. It never evaluates a search or a mark.
func commandLetter(line string) byte {
	for i := 0; i < len(line); {
		c := line[i]
		switch {
		case c == ' ' || c == '\t' || c == ',' || c == ';' || c == '+' || c == '-' ||
			c == '.' || c == '$' || (c >= '0' && c <= '9'):
			i++
		case c == '\'':
			i += 2
		case c == '/' || c == '?':
			j := i + 1
			for j < len(line) && line[j] != c {
				if line[j] == '\\' {
					j++
				}
				j++
			}
			i = j + 1
		default:
			return c
		}
	}
	return 0
}
