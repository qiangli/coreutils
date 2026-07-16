// Package bre matches POSIX Basic Regular Expressions (plus the common GNU
// extensions), shared by the pure-Go tools that default to BRE (grep, sed).
// Patterns without back-references are translated to Go RE2 syntax; patterns
// with \1..\9 or word-edge anchors use a bounded backtracking matcher.
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
//	                              ']' first member, [:class:], and (in the
//	                              C locale) [.x.] collating symbols and
//	                              [=x=] equivalence classes → their literal
//	                              member; backslash is literal inside (POSIX)
//	\( \)  \|  \{m,n\}            grouping, alternation (GNU), intervals
//	\+ \?                         GNU one-or-more / zero-or-one
//	\w \W \s \S \b \B             GNU character-class / word-boundary
//	                              escapes (same meaning in RE2)
//	\<meta>                       escaped metacharacter = literal
//	( ) { } + ? |                 unescaped = literal (BRE rule)
//
// Rejected with a clear error: any alphanumeric escape with no defined
// translation.
//
// Matching is leftmost-first by default and leftmost-longest — the semantics
// POSIX specifies — after (*Regexp).Longest; see its doc comment for why that
// is opt-in rather than the default. SedEscapes supplies the character escapes
// (\n, \t, …) that a sed regex, but not a grep one, gives meaning to.
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
				return "", fmt.Errorf("\\%c word-edge anchor requires the BRE matcher", n)
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

// sedEscape maps the escapes a sed script may use to name a character it cannot
// write literally, to that character.
var sedEscape = map[byte]byte{
	'n': '\n', 't': '\t', 'r': '\r', 'f': '\f', 'v': '\v', 'a': '\a',
}

// SedEscapes rewrites the character escapes a sed regex gives meaning to but a
// bare BRE does not, replacing each with the literal character it denotes so
// that the rest of the engine sees an ordinary literal.
//
// POSIX (XCU sed) requires \n: "the escape sequence '\n' shall match a
// <newline> embedded in the pattern space" — a script cannot contain a literal
// newline inside a BRE, so the escape is the only way to spell one. GNU sed
// adds \t \r \f \v \a. Every other escape is passed through byte-for-byte,
// including \1..\9, \( \) \{ \} \| \+ \? \< \> \w \s \b and the escaped
// metacharacters, so this is safe to run ahead of ToGo / ToGoERE / Compile. A
// preceding \\ is consumed as a unit, so \\n stays "escaped backslash, then n".
//
// grep deliberately does not get this: GNU grep gives \n no such meaning (a
// grep subject line never contains a newline), and inventing one would be a
// behavior no upstream spelling licenses.
func SedEscapes(p string) string {
	if !strings.Contains(p, `\`) {
		return p
	}
	var b strings.Builder
	b.Grow(len(p))
	for i := 0; i < len(p); i++ {
		if p[i] != '\\' || i+1 >= len(p) {
			b.WriteByte(p[i])
			continue
		}
		if c, ok := sedEscape[p[i+1]]; ok {
			b.WriteByte(c)
		} else {
			b.WriteByte('\\')
			b.WriteByte(p[i+1])
		}
		i++
	}
	return b.String()
}

// normalizeInterval validates the inside of \{...\} and maps GNU grep's
// RE2-invalid "{,n}" extension to "{0,n}".
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
	if strings.Contains(hi, ",") || (lo == "" && hi == "") {
		return "", false
	}
	if lo == "" {
		lo = "0"
	}
	if !digits(lo) || (hi != "" && !digits(hi)) {
		return "", false
	}
	if hi != "" && atoi(lo) > atoi(hi) {
		return "", false
	}
	return lo + "," + hi, true
}

func atoi(s string) int {
	n := 0
	for i := 0; i < len(s); i++ {
		n = n*10 + int(s[i]-'0')
	}
	return n
}

// escapeClassRune escapes a rune for safe inclusion as a literal member of an RE2
// character class ([...]): the class metacharacters are \ ] ^ -.
func escapeClassRune(r rune) string {
	switch r {
	case '\\', ']', '^', '-':
		return "\\" + string(r)
	}
	return string(r)
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
			// Collating symbol [.x.] / equivalence class [=x=]. Under LC_ALL=C
			// (the coreutils locale) the only collating elements are the single
			// characters and no character has equivalents, so both reduce to
			// their literal member(s). Emit them into the class, escaped.
			delim := s[i+1]
			end := strings.Index(s[i+2:], string(delim)+"]")
			if end < 0 {
				return "", 0, fmt.Errorf("unmatched [%c in bracket expression", delim)
			}
			content := s[i+2 : i+2+end]
			if content == "" {
				return "", 0, fmt.Errorf("empty %s", map[byte]string{'.': "collating symbol [..]", '=': "equivalence class [==]"}[delim])
			}
			for _, r := range content {
				b.WriteString(escapeClassRune(r))
			}
			i += 2 + end + 2
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

// ToGoERE translates the small GNU ERE extension set that RE2 does not parse
// under the same spelling. ERE back-references are rejected here because RE2
// cannot express them; CompileEREWithFlags routes those patterns through the
// bounded backtracking matcher instead.
func ToGoERE(p string) (string, error) {
	var b strings.Builder
	for i := 0; i < len(p); {
		if p[i] == '[' {
			cls, n, err := translateBracket(p[i:])
			if err != nil {
				return "", err
			}
			b.WriteString(cls)
			i += n
			continue
		}
		if p[i] != '\\' {
			b.WriteByte(p[i])
			i++
			continue
		}
		if i+1 >= len(p) {
			return "", fmt.Errorf("trailing backslash (\\)")
		}
		n := p[i+1]
		if n >= '1' && n <= '9' {
			return "", fmt.Errorf("back-reference \\%c is not supported (RE2 has no back-references)", n)
		}
		if n == '<' || n == '>' {
			b.WriteString(`\b`)
			i += 2
			continue
		}
		b.WriteByte('\\')
		b.WriteByte(n)
		i += 2
	}
	return b.String(), nil
}

// ValidateERE rejects the POSIX/GNU ERE constructs RE2 cannot express;
// everything else passes through to Go regexp unchanged.
func ValidateERE(p string) error {
	_, err := ToGoERE(p)
	return err
}

func isAlnumByte(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}
