package bc

import (
	"fmt"
	"strings"
	"unicode"
)

type tokenKind int

const (
	tEOF tokenKind = iota
	tError
	tNL
	tNumber
	tIdent
	tString
	tOp
)

type token struct {
	k    tokenKind
	s    string
	line int
}

func lex(src string) ([]token, error) {
	var out []token
	line := 1
	for i := 0; i < len(src); {
		c := src[i]
		if c == ' ' || c == '\t' || c == '\r' {
			i++
			continue
		}
		if c == '\n' || c == ';' {
			out = append(out, token{tNL, string(c), line})
			if c == '\n' {
				line++
			}
			i++
			continue
		}
		if c == '/' && i+1 < len(src) && src[i+1] == '*' {
			startLine := line
			j := i + 2
			for j+1 < len(src) && !(src[j] == '*' && src[j+1] == '/') {
				if src[j] == '\n' {
					line++
				}
				j++
			}
			if j+1 >= len(src) {
				out = append(out, token{tError, fmt.Sprintf("line %d: unterminated comment", startLine), startLine})
				break
			}
			i = j + 2
			continue
		}
		if c == '"' {
			startLine := line
			j := i + 1
			for j < len(src) && src[j] != '"' {
				if src[j] == '\n' {
					line++
				}
				j++
			}
			if j >= len(src) {
				out = append(out, token{tError, fmt.Sprintf("line %d: unterminated string", startLine), startLine})
				break
			}
			if j-i-1 > 1000 { // _POSIX2_BC_STRING_MAX
				out = append(out, token{tError, fmt.Sprintf("line %d: string exceeds BC_STRING_MAX (1000 bytes)", startLine), startLine})
				i = j + 1
				continue
			}
			out = append(out, token{tString, src[i+1 : j], line})
			i = j + 1
			continue
		}
		// POSIX reserves upper-case A-F for input digits; identifiers are
		// lower-case. Recognize those digits before the identifier path.
		if c >= 'A' && c <= 'F' {
			startLine := line
			var value strings.Builder
			j := i
			for j < len(src) {
				d := src[j]
				if unicode.IsDigit(rune(d)) || (d >= 'A' && d <= 'F') || d == '.' {
					value.WriteByte(d)
					j++
					continue
				}
				if d == '\\' && j+1 < len(src) && src[j+1] == '\n' {
					j += 2
					line++
					continue
				}
				break
			}
			out = append(out, token{tNumber, value.String(), startLine})
			i = j
			continue
		}
		if c >= 'a' && c <= 'z' {
			// POSIX has only one-character ordinary, array, and function
			// identifiers.  Its reserved words are the sole multi-character
			// identifiers, and take precedence under the longest-token rule.
			keyword := ""
			for _, candidate := range []string{"define", "return", "length", "break", "while", "scale", "sqrt", "auto", "ibase", "obase", "quit", "for", "if"} {
				if strings.HasPrefix(src[i:], candidate) {
					keyword = candidate
					break
				}
			}
			if keyword != "" {
				out = append(out, token{tIdent, keyword, line})
				i += len(keyword)
			} else {
				out = append(out, token{tIdent, string(c), line})
				i++
			}
			continue
		}
		if unicode.IsDigit(rune(c)) || c == '.' {
			startLine := line
			var value strings.Builder
			j := i
			for j < len(src) {
				d := src[j]
				if unicode.IsDigit(rune(d)) || (d >= 'A' && d <= 'F') || d == '.' {
					value.WriteByte(d)
					j++
					continue
				}
				if d == '\\' && j+1 < len(src) && src[j+1] == '\n' {
					j += 2
					line++
					continue
				}
				break
			}
			if value.String() == "." {
				out = append(out, token{tError, fmt.Sprintf("line %d: invalid number %q", startLine, value.String()), startLine})
			} else {
				out = append(out, token{tNumber, value.String(), startLine})
			}
			i = j
			continue
		}
		matched := ""
		for _, op := range []string{"++", "--", "+=", "-=", "*=", "/=", "%=", "^=", "==", "!=", "<=", ">="} {
			if strings.HasPrefix(src[i:], op) {
				matched = op
				break
			}
		}
		if matched != "" {
			out = append(out, token{tOp, matched, line})
			i += len(matched)
			continue
		}
		if strings.ContainsRune("+-*/%^=<>(),{}[]", rune(c)) {
			out = append(out, token{tOp, string(c), line})
			i++
			continue
		}
		if c == '\\' && i+1 < len(src) && src[i+1] == '\n' {
			i += 2
			line++
			continue
		}
		out = append(out, token{tError, fmt.Sprintf("line %d: invalid character %q", line, c), line})
		i++
	}
	out = append(out, token{tEOF, "", line})
	return out, nil
}
