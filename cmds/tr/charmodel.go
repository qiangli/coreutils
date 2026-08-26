// LC_CTYPE character model. tr operates on the characters of the
// invocation's locale, which XCU:tr:ENVIRONMENT_VARIABLES defines as
// "the interpretation of sequences of bytes of text data as characters
// ... and the behavior of character classes". Three universes are
// possible and exactly one is selected per invocation:
//
//   - the C/POSIX locale, where a character is a byte (ctypeTables);
//   - another single-byte locale, whose classes and case maps come from
//     the pkg/ctype provider (ctypeTables again);
//   - a UTF-8 codeset in POSIX mode, where a character is a multi-byte
//     sequence and classes come from the Unicode tables in the standard
//     library.
//
// Outside POSIX mode a UTF-8 locale keeps GNU Coreutils' documented
// byte-oriented behavior, which is also what makes tr usable on a host
// whose LC_CTYPE names a codeset no installed provider carries.

package trcmd

import (
	"sort"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/qiangli/coreutils/pkg/locale"
)

// escapeBase maps a byte that begins no valid character onto a reserved
// surrogate code point, so an uninterpretable byte keeps its identity
// through set expansion, translation and output instead of collapsing
// into U+FFFD. Only 0x80..0xFF can fail to decode, so the escaped range
// is 0xDC80..0xDCFF and can never collide with a real character.
const escapeBase = 0xDC00

func escapeRune(b byte) rune { return escapeBase + rune(b) }

func isEscapedByte(r rune) bool { return r >= escapeBase+0x80 && r <= escapeBase+0xFF }

// encodedValues returns the individual encoded byte values represented by an
// expanded character set. This is the SET1 domain for POSIX -c, as distinct
// from -C's characters: a UTF-8 character contributes each byte of its
// encoding, while an uninterpretable escaped byte contributes itself.
func encodedValues(chars []rune) map[rune]bool {
	values := make(map[rune]bool)
	var encoded [utf8.UTFMax]byte
	for _, r := range chars {
		if isEscapedByte(r) {
			values[rune(byte(r-escapeBase))] = true
			continue
		}
		n := utf8.EncodeRune(encoded[:], r)
		for _, b := range encoded[:n] {
			values[rune(b)] = true
		}
	}
	return values
}

// charTables is the selected character universe.
type charTables struct {
	// multibyte selects the UTF-8 character universe.
	multibyte bool
	// bytes carries the single-byte classes and case maps; nil when
	// multibyte is set.
	bytes *ctypeTables
	// collate is an invocation-owned snapshot. It is populated after the
	// LC_CTYPE model is selected and never retains a live provider.
	collate           *collationTables
	discoverCollation bool
}

// classChars returns the members of a POSIX character class in ascending
// order together with whether the class name is recognised.
func (ct *charTables) classChars(name string) ([]rune, bool) {
	if !ct.multibyte {
		members, ok := ct.bytes.classFromTable(name)
		if !ok {
			return nil, false
		}
		out := make([]rune, len(members))
		for i, b := range members {
			out[i] = rune(b)
		}
		return out, true
	}
	build, ok := mbClasses[name]
	if !ok {
		return nil, false
	}
	return build(), true
}

// toLower and toUpper apply the locale's case mapping to one character.
func (ct *charTables) toLower(r rune) rune {
	if ct.multibyte {
		return unicode.ToLower(r)
	}
	return rune(ct.bytes.toLower[byte(r)])
}

func (ct *charTables) toUpper(r rune) rune {
	if ct.multibyte {
		return unicode.ToUpper(r)
	}
	return rune(ct.bytes.toUpper[byte(r)])
}

// mbClassPred defines each POSIX character class over Unicode. The
// definitions follow XBD 7.3.1 LC_CTYPE: digit and xdigit are fixed sets,
// alnum is alpha plus digit, graph is every printable character except
// <space> characters, and print is graph plus those <space> characters.
var mbClassPred = map[string]func(rune) bool{
	"alpha": unicode.IsLetter,
	"digit": func(r rune) bool { return r >= '0' && r <= '9' },
	"alnum": func(r rune) bool { return unicode.IsLetter(r) || (r >= '0' && r <= '9') },
	"upper": unicode.IsUpper,
	"lower": unicode.IsLower,
	"space": unicode.IsSpace,
	"blank": func(r rune) bool { return r == '\t' || unicode.Is(unicode.Zs, r) },
	"cntrl": unicode.IsControl,
	"graph": func(r rune) bool { return unicode.IsGraphic(r) && !unicode.IsSpace(r) },
	"print": func(r rune) bool { return unicode.IsGraphic(r) },
	"punct": func(r rune) bool { return unicode.IsPunct(r) || unicode.IsSymbol(r) },
	"xdigit": func(r rune) bool {
		return (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
	},
}

// mbClasses expands each class exactly once per process. Expansion is only
// needed when a class takes part in a translation pairing; membership tests
// go through mbClassPred.
var mbClasses = func() map[string]func() []rune {
	out := make(map[string]func() []rune, len(mbClassPred))
	for name, pred := range mbClassPred {
		out[name] = sync.OnceValue(func() []rune {
			var members []rune
			for r := rune(0); r <= unicode.MaxRune; r++ {
				if r >= 0xD800 && r <= 0xDFFF {
					continue // surrogates are not characters
				}
				if pred(r) {
					members = append(members, r)
				}
			}
			return members
		})
	}
	return out
}()

// isUTF8Locale reports whether a locale name selects the UTF-8 codeset.
func isUTF8Locale(name string) bool {
	name, _, _ = strings.Cut(name, "@")
	if dot := strings.LastIndexByte(name, '.'); dot >= 0 {
		name = name[dot+1:]
	}
	name = strings.ToUpper(strings.ReplaceAll(strings.ReplaceAll(name, "-", ""), "_", ""))
	return name == "UTF8"
}

func envPresent(env []string, key string) bool {
	prefix := key + "="
	for _, entry := range env {
		if entry == key || strings.HasPrefix(entry, prefix) {
			return true
		}
	}
	return false
}

// openCharTables resolves LC_CTYPE for the invocation and selects the
// character universe. The returned name is the effective locale name, for
// diagnostics.
func openCharTables(env []string, opener ctypeOpener) (*charTables, string, error) {
	name := locale.Resolve(env, locale.CType)
	if isCPOSIX(name) {
		return &charTables{bytes: buildCLocale()}, name, nil
	}
	if isUTF8Locale(name) {
		if envPresent(env, "POSIXLY_CORRECT") {
			return &charTables{multibyte: true}, name, nil
		}
		// GNU Coreutils' tr is byte-oriented and documents no multi-byte
		// support, so the extension tier keeps byte characters rather than
		// failing for want of a single-byte provider for this codeset.
		return &charTables{bytes: buildCLocale()}, name, nil
	}
	tables, err := openCTypeTables(name, opener)
	if err != nil {
		return nil, name, err
	}
	return &charTables{bytes: tables}, name, nil
}

// complementPrefix returns the first n characters of the complemented
// domain in ascending order. The complement of a set in the multi-byte
// universe is unbounded for practical purposes, so only the prefix a
// translation can actually consume is ever produced.
func complementPrefix(member func(rune) bool, n int) []rune {
	out := make([]rune, 0, n)
	for r := rune(0); r <= unicode.MaxRune && len(out) < n; r++ {
		if r >= 0xD800 && r <= 0xDFFF {
			continue
		}
		if !member(r) {
			out = append(out, r)
		}
	}
	return out
}

const unicodeScalarCount = int(unicode.MaxRune) + 1 - (0xE000 - 0xD800)

// complementFillPlan is the finite expansion of a complemented multi-byte
// SET1 paired with a symbolic [c*] in SET2. It stores only SET1 exclusions and
// the explicit SET2 edges; translate computes an input character's zero-based
// rank in the complement without materializing the Unicode scalar universe.
type complementFillPlan struct {
	member    map[rune]bool
	excluded  []rune
	prefix    []rune
	suffix    []rune
	fill      rune
	fillCount int
}

func newComplementFillPlan(member map[rune]bool, set2 *setSpec) *complementFillPlan {
	p := &complementFillPlan{member: member, fill: set2.fillChar}
	for r := range member {
		if r >= 0 && r <= unicode.MaxRune && (r < 0xD800 || r > 0xDFFF) {
			p.excluded = append(p.excluded, r)
		}
	}
	sort.Slice(p.excluded, func(i, j int) bool { return p.excluded[i] < p.excluded[j] })
	p.prefix = append([]rune(nil), set2.chars[:set2.fillPos]...)
	p.suffix = append([]rune(nil), set2.chars[set2.fillPos:]...)
	p.fillCount = unicodeScalarCount - len(p.excluded) - len(p.prefix) - len(p.suffix)
	if p.fillCount < 0 {
		p.fillCount = 0
	}
	return p
}

func (p *complementFillPlan) translate(r rune) (rune, bool) {
	if r < 0 || r > unicode.MaxRune || (r >= 0xD800 && r <= 0xDFFF) || p.member[r] {
		return 0, false
	}
	scalarIndex := int(r)
	if r > 0xDFFF {
		scalarIndex -= 0xE000 - 0xD800
	}
	excludedThrough := sort.Search(len(p.excluded), func(i int) bool { return p.excluded[i] > r })
	rank := scalarIndex - excludedThrough
	if rank < len(p.prefix) {
		return p.prefix[rank], true
	}
	rank -= len(p.prefix)
	if rank < p.fillCount {
		return p.fill, true
	}
	rank -= p.fillCount
	if rank < len(p.suffix) {
		return p.suffix[rank], true
	}
	return 0, false
}

// decodeChar reads one character. An invalid byte is one character that
// keeps its exact value, so tr never rewrites data it cannot interpret.
func decodeChar(s string, i int) (rune, int) {
	r, size := utf8.DecodeRuneInString(s[i:])
	if r == utf8.RuneError && size <= 1 {
		return escapeRune(s[i]), 1
	}
	return r, size
}
