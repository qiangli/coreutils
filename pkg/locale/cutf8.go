package locale

import (
	"fmt"
	"strings"
	"sync"
	"unicode"
)

// CUTF8Class reports membership in glibc's C.UTF-8 POSIX character classes.
// It intentionally is not a direct mapping to Go's Unicode predicates.  In
// particular glibc treats non-ASCII decimal numbers as alpha (while POSIX
// digit remains exactly 0-9), and classifies non-breaking spaces as printable
// punctuation rather than white space.
func CUTF8Class(name string, r rune) bool {
	switch name {
	case "alpha":
		return unicode.IsLetter(r) || r > unicode.MaxASCII && unicode.IsNumber(r)
	case "digit":
		return r >= '0' && r <= '9'
	case "alnum":
		return CUTF8Class("alpha", r) || CUTF8Class("digit", r)
	case "upper":
		return unicode.IsUpper(r)
	case "lower":
		return unicode.IsLower(r)
	case "space":
		return cutf8Space(r)
	case "blank":
		return r == '\t' || r == ' ' || unicode.In(r, unicode.Zs) && !cutf8NoBreak(r)
	case "cntrl":
		return unicode.IsControl(r)
	case "graph":
		return cutf8Print(r) && !cutf8Space(r)
	case "print":
		return cutf8Print(r)
	case "punct":
		return CUTF8Class("graph", r) && !CUTF8Class("alnum", r)
	case "xdigit":
		return r >= '0' && r <= '9' || r >= 'a' && r <= 'f' || r >= 'A' && r <= 'F'
	default:
		return false
	}
}

func cutf8NoBreak(r rune) bool {
	return r == '\u00a0' || r == '\u2007' || r == '\u202f'
}

func cutf8Space(r rune) bool {
	return unicode.IsSpace(r) && !cutf8NoBreak(r)
}

func cutf8Print(r rune) bool {
	return unicode.IsGraphic(r)
}

var cutf8RE2Classes sync.Map

// CUTF8RE2ClassContent returns the contents (without the surrounding square
// brackets) of an RE2 class with the same membership as CUTF8Class.  Building
// ranges from the predicate avoids subtly widening alpha to ASCII digits or
// space to NBSP through a convenient but incompatible Unicode property.
func CUTF8RE2ClassContent(name string) (string, bool) {
	if !knownCUTF8Class(name) {
		return "", false
	}
	if cached, ok := cutf8RE2Classes.Load(name); ok {
		return cached.(string), true
	}
	var b strings.Builder
	for lo := rune(0); lo <= unicode.MaxRune; {
		for lo <= unicode.MaxRune && !CUTF8Class(name, lo) {
			lo++
		}
		if lo > unicode.MaxRune {
			break
		}
		hi := lo
		for hi < unicode.MaxRune && CUTF8Class(name, hi+1) {
			hi++
		}
		writeRE2Rune(&b, lo)
		if hi != lo {
			b.WriteByte('-')
			writeRE2Rune(&b, hi)
		}
		lo = hi + 1
	}
	result := b.String()
	cutf8RE2Classes.Store(name, result)
	return result, true
}

func knownCUTF8Class(name string) bool {
	switch name {
	case "alpha", "digit", "alnum", "upper", "lower", "space", "blank", "cntrl", "graph", "print", "punct", "xdigit":
		return true
	default:
		return false
	}
}

func writeRE2Rune(b *strings.Builder, r rune) {
	fmt.Fprintf(b, `\x{%X}`, r)
}
