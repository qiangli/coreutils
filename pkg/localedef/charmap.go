package localedef

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"
)

// ParseCharmap expands decimal symbolic-name ranges (and the common two-dot
// hexadecimal extension). WIDTH sections are deliberately ignored.
func ParseCharmap(in io.Reader) (*Charmap, error) {
	return ParseCharmapTarget(in, "")
}

// ErrCodeset marks unsupported encodings and positions outside the target
// repertoire. localedef reports these implementation limits with status 2.
var ErrCodeset = errors.New("unsupported codeset or character encoding")

// TargetCodeset returns the canonical name and maximum encoded character size.
// Aliases are case-insensitive. An empty name leaves the charmap unchanged.
func TargetCodeset(name string) (string, int, error) {
	switch strings.ToUpper(name) {
	case "":
		return "", 0, nil
	case "UTF-8", "UTF8":
		return "UTF-8", 4, nil
	case "ASCII", "US-ASCII", "ANSI_X3.4-1968", "ISO646-US":
		return "ASCII", 1, nil
	default:
		return "", 0, fmt.Errorf("%w: %q", ErrCodeset, name)
	}
}

// ParseCharmapTarget maps UCS position constants (<Uhhhh> or <Uhhhhhhhh>,
// also accepted without brackets) into target. Numeric byte encodings remain
// literal target bytes; they are checked, never reinterpreted as UCS positions.
func ParseCharmapTarget(in io.Reader, target string) (*Charmap, error) {
	target, maxBytes, err := TargetCodeset(target)
	if err != nil {
		return nil, err
	}

	m := &Charmap{MinBytes: 1, MaxBytes: 1, Symbols: map[string][]byte{}}
	r := newLines(in)
	state := "header"
	start := 1
	seen := false
	for {
		s, line, err := r.next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if ok, err := r.directive(s, line); ok {
			if err != nil {
				return nil, err
			}
			continue
		}
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		vs, err := lex(s, r.escape, r.comment)
		if err != nil {
			return nil, atLine(line, err)
		}
		if len(vs) == 0 {
			continue
		}
		if state == "width" {
			if len(vs) == 2 && vs[0].Text == "END" && vs[1].Text == "WIDTH" {
				state = "done"
			}
			continue
		}
		if len(vs) == 1 && vs[0].Text == "CHARMAP" {
			if seen {
				return nil, problem(line, "duplicate CHARMAP")
			}
			seen = true
			state = "map"
			start = line
			continue
		}
		if len(vs) == 2 && vs[0].Text == "END" && vs[1].Text == "CHARMAP" {
			if state != "map" {
				return nil, problem(line, "unexpected END CHARMAP")
			}
			state = "done"
			continue
		}
		if len(vs) == 1 && vs[0].Text == "WIDTH" && state == "done" {
			state = "width"
			start = line
			continue
		}
		if state == "header" {
			if len(vs) != 2 || vs[0].Kind != Symbol {
				return nil, problem(line, "expected charmap header or CHARMAP")
			}
			switch vs[0].Text {
			case "code_set_name":
				m.CodeSet = vs[1].Text
			case "mb_cur_min", "mb_cur_max":
				n, err := strconv.Atoi(vs[1].Text)
				if err != nil || n < 1 || n > 16 {
					return nil, problem(line, "invalid %s", vs[0].Text)
				}
				if vs[0].Text == "mb_cur_min" {
					m.MinBytes = n
				} else {
					m.MaxBytes = n
				}
			default:
				return nil, problem(line, "unknown charmap header %q", vs[0].Text)
			}
			continue
		}
		if state != "map" {
			return nil, problem(line, "unexpected data outside CHARMAP")
		}
		if len(vs) < 2 || vs[0].Kind != Symbol {
			return nil, problem(line, "expected symbolic name and byte encoding")
		}
		names := []string{vs[0].Text}
		idx := 1
		if vs[1].Kind == Ellipsis {
			if len(vs) < 4 || vs[2].Kind != Symbol {
				return nil, problem(line, "incomplete symbolic range")
			}
			names, err = rangeNames(vs[0].Text, vs[2].Text, vs[1].Text)
			if err != nil {
				return nil, problem(line, "%v", err)
			}
			idx = 3
		}
		v := vs[idx]
		ucs := (v.Kind == Symbol || v.Kind == Word) && strings.HasPrefix(v.Text, "U")
		var position uint64
		if ucs {
			if target == "" {
				return nil, fmt.Errorf("%w: UCS position requires -u", ErrCodeset)
			}
			if len(v.Text) != 5 && len(v.Text) != 9 {
				return nil, problem(line, "invalid UCS position constant %q", v.Text)
			}
			position, err = strconv.ParseUint(v.Text[1:], 16, 32)
			if err != nil {
				return nil, problem(line, "invalid UCS position constant %q", v.Text)
			}
		} else if v.Kind != Bytes {
			return nil, problem(line, "expected numeric byte encoding or UCS position constant")
		}
		b := []byte(v.Text)
		for n, name := range names {
			if _, ok := m.Symbols[name]; ok {
				return nil, problem(line, "duplicate symbol <%s>", name)
			}
			if ucs {
				pos := position + uint64(n)
				if pos > utf8.MaxRune || !utf8.ValidRune(rune(pos)) || (target == "ASCII" && pos > 127) {
					return nil, fmt.Errorf("%w: line %d: U%08X is outside %s", ErrCodeset, line, pos, target)
				}
				b = []byte(string(rune(pos)))
			}
			if target != "" {
				valid := utf8.Valid(b)
				if target == "ASCII" {
					for _, ch := range b {
						valid = valid && ch < 128
					}
				}
				if !valid || len(b) == 0 || len(b) > maxBytes {
					return nil, fmt.Errorf("%w: line %d: invalid %s bytes", ErrCodeset, line, target)
				}
			} else if len(b) < m.MinBytes || len(b) > m.MaxBytes {
				return nil, problem(line, "encoding length outside mb_cur_min/mb_cur_max")
			}
			m.Symbols[name] = append([]byte(nil), b...)
			if !ucs && n+1 < len(names) {
				carry := true
				for j := len(b) - 1; j >= 0; j-- {
					b[j]++
					if b[j] != 0 {
						carry = false
						break
					}
				}
				if carry {
					return nil, problem(line, "range encoding overflow")
				}
			}
		}
	}
	if !seen || state == "map" || state == "width" {
		return nil, problem(start, "missing CHARMAP or END %s", strings.ToUpper(state))
	}
	if target != "" {
		m.CodeSet, m.MinBytes, m.MaxBytes = target, 1, maxBytes
	}
	if m.MinBytes > m.MaxBytes {
		return nil, problem(1, "mb_cur_min exceeds mb_cur_max")
	}
	return m, nil
}
func rangeNames(a, b, dots string) ([]string, error) {
	base := 10
	digits := "0123456789"
	if dots == ".." {
		base = 16
		digits += "abcdefABCDEF"
	}
	i := len(a)
	for i > 0 && strings.ContainsRune(digits, rune(a[i-1])) {
		i--
	}
	if i == len(a) || len(a) != len(b) || a[:i] != b[:i] {
		return nil, fmt.Errorf("invalid symbolic range")
	}
	lo, e1 := strconv.ParseUint(a[i:], base, 32)
	hi, e2 := strconv.ParseUint(b[i:], base, 32)
	if e1 != nil || e2 != nil || hi < lo || hi-lo > 1<<20 {
		return nil, fmt.Errorf("invalid or excessive symbolic range")
	}
	names := make([]string, 0, hi-lo+1)
	for n := lo; n <= hi; n++ {
		suffix := strconv.FormatUint(n, base)
		if base == 16 && strings.ToUpper(a) == a {
			suffix = strings.ToUpper(suffix)
		}
		names = append(names, a[:i]+strings.Repeat("0", len(a)-i-len(suffix))+suffix)
	}
	return names, nil
}
