package editor

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/qiangli/coreutils/pkg/bre"
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
			b, n = out[len(out)-1], p
		}
		out = append(out, b)
		if len(out) > 2 {
			out = out[len(out)-2:]
		}
		pos = n
	}
}

func (e *Engine) parseAddress(s string, pos int) (int, int, bool, error) {
	pos = skipBlank(s, pos)
	if pos >= len(s) {
		return 0, pos, false, nil
	}
	value, found := 0, true
	switch s[pos] {
	case '.':
		value, pos = e.Buffer.Current, pos+1
	case '$':
		value, pos = e.Buffer.Last(), pos+1
	case '/', '?':
		delim := s[pos]
		pattern, rest, closed, err := delimited(s[pos+1:], delim)
		if err != nil {
			return 0, pos, false, err
		}
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
		re, err := bre.Compile(pattern)
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
	if !found {
		return 0, pos, false, nil
	}
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
		p = skipBlank(s, p)
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

func (e *Engine) search(re *bre.Regexp, forward bool) (int, error) {
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
		if re.MatchString(e.Buffer.Lines[idx]) {
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
