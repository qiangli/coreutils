// Package bre translates POSIX Basic Regular Expressions (plus the common GNU
// extensions) into Go RE2 syntax, shared by the pure-Go tools that default to
// BRE (grep, sed). RE2 cannot express back-references or word-edge anchors, so
// those are rejected with a clear error rather than silently mis-matched.
//
// Supported BRE constructs (translated to the equivalent Go regexp):
//
//	literal characters            unchanged
//	.                             any character
//	*                             quantifier after an atom; literal at
//	                              the start of the pattern or after
//	                              \( \| ^ (POSIX/GNU rule)
//	^ $                           anchors only in their BRE anchor
//	                              positions (^ at start or after \( \|;
//	                              $ at end or before \) \|); literal
//	                              elsewhere
//	[...] [^...]                  bracket expressions, incl. ranges,
//	                              ']' first member, and [:class:];
//	                              backslash is literal inside (POSIX)
//	\( \)  \|  \{m,n\}            grouping, alternation (GNU), intervals
//	\+ \?                         GNU one-or-more / zero-or-one
//	\w \W \s \S \b \B             GNU character-class / word-boundary
//	                              escapes (same meaning in RE2)
//	\<meta>                       escaped metacharacter = literal
//	( ) { } + ? |                 unescaped = literal (BRE rule)
//
// Rejected with a clear error: back-references \1..\9, word-edge anchors \< \>,
// collating symbols [. .], equivalence classes [= =], and any alphanumeric
// escape with no defined translation.
package bre

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	posStart  = iota // start of pattern or just after \( or \|
	posAtom          // after a complete atom (a quantifier may follow)
	posAnchor        // after ^ or $ (quantifier would be literal)
)

// ToGo translates one POSIX basic regular expression (plus the GNU extensions
// listed in the package comment) into Go RE2 syntax.
func ToGo(p string) (string, error) {
	var b strings.Builder
	state := posStart
	i := 0
	for i < len(p) {
		c := p[i]
		switch c {
		case '\\':
			if i+1 >= len(p) {
				return "", fmt.Errorf("trailing backslash (\\)")
			}
			n := p[i+1]
			switch {
			case n == '(':
				b.WriteByte('(')
				state = posStart
				i += 2
			case n == ')':
				b.WriteByte(')')
				state = posAtom
				i += 2
			case n == '|':
				b.WriteByte('|')
				state = posStart
				i += 2
			case n == '{':
				if state != posAtom {
					return "", fmt.Errorf("\\{ with nothing to repeat")
				}
				end := strings.Index(p[i+2:], `\}`)
				if end < 0 {
					return "", fmt.Errorf("unmatched \\{")
				}
				inner := p[i+2 : i+2+end]
				norm, ok := normalizeInterval(inner)
				if !ok {
					return "", fmt.Errorf("invalid interval \\{%s\\}", inner)
				}
				b.WriteString("{" + norm + "}")
				state = posAtom
				i += 2 + end + 2
			case n == '}':
				return "", fmt.Errorf("unmatched \\}")
			case n == '+' || n == '?':
				if state != posAtom {
					return "", fmt.Errorf("\\%c with nothing to repeat", n)
				}
				b.WriteByte(n)
				i += 2
			case n >= '1' && n <= '9':
				return "", fmt.Errorf("back-reference \\%c is not supported (RE2 has no back-references)", n)
			case n == 'w' || n == 'W' || n == 's' || n == 'S' || n == 'b' || n == 'B':
				b.WriteByte('\\')
				b.WriteByte(n)
				state = posAtom
				i += 2
			case n == '<' || n == '>':
				return "", fmt.Errorf("\\%c word-edge anchor is not supported (use \\b)", n)
			case n >= utf8.RuneSelf:
				r, sz := utf8.DecodeRuneInString(p[i+1:])
				b.WriteRune(r)
				state = posAtom
				i += 1 + sz
			case isAlnumByte(n):
				return "", fmt.Errorf("unsupported escape \\%c", n)
			default:
				b.WriteString(regexp.QuoteMeta(string(n)))
				state = posAtom
				i += 2
			}
		case '*':
			if state == posAtom {
				b.WriteByte('*')
			} else {
				b.WriteString(`\*`)
				state = posAtom
			}
			i++
		case '^':
			if state == posStart {
				b.WriteByte('^')
				state = posAnchor
			} else {
				b.WriteString(`\^`)
				state = posAtom
			}
			i++
		case '$':
			anchor := i == len(p)-1 ||
				(i+2 < len(p) && p[i+1] == '\\' && (p[i+2] == ')' || p[i+2] == '|'))
			if anchor {
				b.WriteByte('$')
				state = posAnchor
			} else {
				b.WriteString(`\$`)
				state = posAtom
			}
			i++
		case '[':
			cls, n, err := translateBracket(p[i:])
			if err != nil {
				return "", err
			}
			b.WriteString(cls)
			state = posAtom
			i += n
		case '.':
			b.WriteByte('.')
			state = posAtom
			i++
		case '(', ')', '{', '}', '+', '?', '|':
			// unescaped = literal in BRE
			b.WriteString(regexp.QuoteMeta(string(c)))
			state = posAtom
			i++
		default:
			b.WriteByte(c)
			state = posAtom
			i++
		}
	}
	return b.String(), nil
}

// normalizeInterval validates the inside of \{...\} and maps the
// POSIX-valid-but-RE2-invalid "{,n}" to "{0,n}".
func normalizeInterval(s string) (string, bool) {
	digits := func(x string) bool {
		if x == "" {
			return false
		}
		for i := 0; i < len(x); i++ {
			if x[i] < '0' || x[i] > '9' {
				return false
			}
		}
		return true
	}
	comma := strings.IndexByte(s, ',')
	if comma < 0 {
		return s, digits(s)
	}
	lo, hi := s[:comma], s[comma+1:]
	if lo == "" {
		lo = "0"
	}
	if !digits(lo) || (hi != "" && !digits(hi)) {
		return "", false
	}
	return lo + "," + hi, true
}

// translateBracket converts one POSIX bracket expression (s starts at '[') to
// RE2 class syntax, returning the translation and the number of input bytes
// consumed.
func translateBracket(s string) (string, int, error) {
	var b strings.Builder
	b.WriteByte('[')
	i := 1
	if i < len(s) && s[i] == '^' {
		b.WriteByte('^')
		i++
	}
	first := true
	for i < len(s) {
		if s[i] == ']' && !first {
			b.WriteByte(']')
			return b.String(), i + 1, nil
		}
		switch {
		case s[i] == '[' && i+1 < len(s) && s[i+1] == ':':
			end := strings.Index(s[i:], ":]")
			if end < 0 {
				return "", 0, fmt.Errorf("unmatched [: in bracket expression")
			}
			b.WriteString(s[i : i+end+2]) // RE2 supports [:class:] directly
			i += end + 2
		case s[i] == '[' && i+1 < len(s) && (s[i+1] == '.' || s[i+1] == '='):
			return "", 0, fmt.Errorf("collating symbols [. .] / equivalence classes [= =] are not supported")
		case s[i] == '\\':
			// backslash is a literal member in POSIX bracket expressions
			b.WriteString(`\\`)
			i++
		case s[i] == ']':
			// only reachable as the first member: literal
			b.WriteString(`\]`)
			i++
		default:
			b.WriteByte(s[i])
			i++
		}
		first = false
	}
	return "", 0, fmt.Errorf("unmatched [")
}

// ValidateERE rejects the POSIX/GNU ERE constructs RE2 cannot express;
// everything else passes through to Go regexp unchanged.
func ValidateERE(p string) error {
	for i := 0; i+1 < len(p); i++ {
		if p[i] != '\\' {
			continue
		}
		n := p[i+1]
		if n >= '1' && n <= '9' {
			return fmt.Errorf("back-reference \\%c is not supported (RE2 has no back-references)", n)
		}
		if n == '<' || n == '>' {
			return fmt.Errorf("\\%c word-edge anchor is not supported (use \\b)", n)
		}
		i++
	}
	return nil
}

func isAlnumByte(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}
