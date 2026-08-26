// Multi-byte control-string parsing. XCU:tr:OPERANDS makes every escape
// sequence a single-byte binary value — "Multi-byte characters require
// multiple, concatenated escape sequences of this type, including the
// leading <backslash> for each byte" — so escapes are resolved to bytes
// first and only then decoded as characters. Literal text in the control
// string is already a character sequence and is decoded directly.

package trcmd

import (
	"fmt"
	"strconv"
	"unicode/utf8"
)

// setElem is one lexed element of a control string. escaped marks an
// element produced by a backslash escape, which can never act as one of
// the structural tokens '[', ']', '-', '*', ':' or '='.
type setElem struct {
	r       rune
	escaped bool
}

// lexSet resolves escapes to bytes, decodes maximal runs of those bytes as
// characters, and decodes the surrounding literal text as characters.
func lexSet(s string) []setElem {
	var elems []setElem
	var pending []byte
	flushPending := func() {
		for len(pending) > 0 {
			r, size := utf8.DecodeRune(pending)
			if r == utf8.RuneError && size <= 1 {
				r, size = escapeRune(pending[0]), 1
			}
			elems = append(elems, setElem{r: r, escaped: true})
			pending = pending[size:]
		}
		pending = nil
	}
	for i := 0; i < len(s); {
		if s[i] != '\\' {
			flushPending()
			r, size := decodeChar(s, i)
			elems = append(elems, setElem{r: r})
			i += size
			continue
		}
		if i+1 >= len(s) {
			// A trailing backslash is a literal backslash.
			pending = append(pending, '\\')
			i++
			continue
		}
		switch c := s[i+1]; c {
		case 'a', 'b', 'f', 'n', 'r', 't', 'v', '\\':
			pending = append(pending, escapeByte(c))
			i += 2
		case '0', '1', '2', '3', '4', '5', '6', '7':
			val, n, j := 0, 0, i+1
			for n < 3 && j < len(s) && s[j] >= '0' && s[j] <= '7' {
				nv := val*8 + int(s[j]-'0')
				if nv > 255 {
					break
				}
				val, j, n = nv, j+1, n+1
			}
			pending = append(pending, byte(val))
			i = j
		default:
			// \X with no special meaning is X itself, one whole character.
			flushPending()
			r, size := decodeChar(s, i+1)
			elems = append(elems, setElem{r: r, escaped: true})
			i += 1 + size
		}
	}
	flushPending()
	return elems
}

// escapeByte maps a recognised escape letter to the byte it denotes.
func escapeByte(c byte) byte {
	switch c {
	case 'a':
		return '\a'
	case 'b':
		return '\b'
	case 'f':
		return '\f'
	case 'n':
		return '\n'
	case 'r':
		return '\r'
	case 't':
		return '\t'
	case 'v':
		return '\v'
	default:
		return c // '\\'
	}
}

// structural reports whether elem is an unescaped occurrence of r.
func structural(e setElem, r rune) bool { return !e.escaped && e.r == r }

func elemString(elems []setElem) string {
	out := make([]rune, len(elems))
	for i, e := range elems {
		out[i] = e.r
	}
	return string(out)
}

// parseSetMultibyte expands a control string into its character sequence
// in the multi-byte universe. It recognises the same constructs as the
// single-byte parser: literal and escaped characters, ranges, [:class:],
// [=c=], and — in SET2 only — [c*n] / [c*].
func parseSetMultibyte(s string, isSet2 bool, tables *charTables) (*setSpec, string) {
	elems := lexSet(s)
	sp := &setSpec{fillPos: -1, fillTokenPos: -1}
	for i := 0; i < len(elems); {
		if structural(elems[i], '[') {
			if cls, adv, ok := matchClassMB(elems[i:]); ok {
				expanded, known := tables.classChars(cls)
				if !known {
					return nil, fmt.Sprintf("invalid character class '%s'", cls)
				}
				sp.addClass(cls, expanded)
				i += adv
				continue
			}
			if eqc, adv, ok, errMsg := matchEquivMB(elems[i:]); errMsg != "" {
				return nil, errMsg
			} else if ok {
				sp.appendOrdinary(eqc)
				sp.lastIsClass = false
				i += adv
				continue
			}
			if rc, count, fill, adv, ok, errMsg := matchRepeatMB(elems[i:]); errMsg != "" {
				return nil, errMsg
			} else if ok {
				if !isSet2 {
					return nil, "the [c*] repeat construct may not appear in string1"
				}
				if fill {
					if sp.fillPos >= 0 {
						return nil, "only one [c*] repeat construct may appear in string2"
					}
					sp.fillPos = len(sp.chars)
					sp.fillTokenPos = len(sp.tokens)
					sp.fillChar = rc
				} else {
					for k := 0; k < count; k++ {
						sp.appendOrdinary(rc)
					}
				}
				sp.lastIsClass = false
				i += adv
				continue
			}
		}
		lo := elems[i].r
		if i+2 < len(elems) && structural(elems[i+1], '-') {
			hi := elems[i+2].r
			if hi < lo {
				return nil, fmt.Sprintf("range-endpoints of '%c-%c' are in reverse collating sequence order", lo, hi)
			}
			for r := lo; r <= hi; r++ {
				if r >= 0xD800 && r <= 0xDFFF {
					continue // surrogates are not characters
				}
				sp.appendOrdinary(r)
			}
			i += 3
			sp.lastIsClass = false
			continue
		}
		sp.appendOrdinary(lo)
		i++
		sp.lastIsClass = false
	}
	return sp, ""
}

// matchClassMB matches a leading "[:name:]". A malformed construct is not
// an error — the characters are then taken literally, as GNU does.
func matchClassMB(elems []setElem) (string, int, bool) {
	if len(elems) < 4 || !structural(elems[1], ':') {
		return "", 0, false
	}
	for k := 2; k+1 < len(elems); k++ {
		if structural(elems[k], ':') && structural(elems[k+1], ']') {
			return elemString(elems[2:k]), k + 2, true
		}
	}
	return "", 0, false
}

// matchEquivMB matches a leading "[=c=]". Without a collation provider an
// equivalence class contains exactly its own character, which is the
// complete answer in the POSIX locale.
func matchEquivMB(elems []setElem) (rune, int, bool, string) {
	if len(elems) < 4 || !structural(elems[1], '=') {
		return 0, 0, false, ""
	}
	for k := 2; k+1 < len(elems); k++ {
		if structural(elems[k], '=') && structural(elems[k+1], ']') {
			inner := elems[2:k]
			if len(inner) != 1 {
				return 0, 0, false, fmt.Sprintf("%s: equivalence class operand must be a single character", elemString(inner))
			}
			return inner[0].r, k + 2, true, ""
		}
	}
	return 0, 0, false, ""
}

// matchRepeatMB matches a leading "[c*n]" / "[c*]". n is decimal, or octal
// with a leading 0; n omitted means "pad SET2 to the length of SET1".
func matchRepeatMB(elems []setElem) (c rune, count int, fill bool, adv int, ok bool, errMsg string) {
	if len(elems) < 2 {
		return
	}
	c = elems[1].r
	j := 2
	if j >= len(elems) || !structural(elems[j], '*') {
		return 0, 0, false, 0, false, ""
	}
	j++
	digStart := j
	for j < len(elems) && !elems[j].escaped && elems[j].r >= '0' && elems[j].r <= '9' {
		j++
	}
	if j >= len(elems) || !structural(elems[j], ']') {
		return 0, 0, false, 0, false, ""
	}
	digits := elemString(elems[digStart:j])
	n := 0
	if digits != "" {
		base := 10
		if digits[0] == '0' {
			base = 8
		}
		v, err := strconv.ParseInt(digits, base, 32)
		if err != nil {
			return 0, 0, false, 0, false, fmt.Sprintf("invalid repeat count '%s' in [c*n] construct", digits)
		}
		n = int(v)
	}
	return c, n, digits == "", j + 1, true, ""
}
