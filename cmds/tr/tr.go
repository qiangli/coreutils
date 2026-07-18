// Package trcmd implements tr(1) per the GNU coreutils manual:
// translate, squeeze, and/or delete characters from standard input,
// writing to standard output.
//
// The implementation is byte-oriented (LC_ALL=C semantics, the agent
// contract): SETs expand to byte sequences; character classes are the
// C-locale ASCII definitions in ascending byte order, exactly as GNU
// tr expands them in the POSIX locale.
package trcmd

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "tr",
	Synopsis: "Translate, squeeze, and/or delete characters from standard input, writing to standard output. Supports -C as a complement alias.",
	Usage:    "tr [OPTION]... SET1 [SET2]",
}

// Run is wired in init: a literal would create an initialization
// cycle (run's flag-error paths reference cmd).
func init() { cmd.Run = run; tool.Register(cmd) }

// Tags mark which bytes of an expanded set came from a case-conversion
// character class — translation pairs [:upper:]/[:lower:] positionally.
const (
	tagNone byte = iota
	tagLower
	tagUpper
)

type setSpec struct {
	bytes         []byte
	tags          []byte
	lastIsClass   bool // last construct parsed was a [:class:]
	hasCaseClass  bool // contains [:upper:] or [:lower:]
	hasOtherClass bool // contains any other [:class:]
	fillPos       int  // insertion point of a [c*] fill construct; -1 = none
	fillByte      byte
}

func (sp *setSpec) append(b, tag byte) {
	sp.bytes = append(sp.bytes, b)
	sp.tags = append(sp.tags, tag)
}

// applyFill expands a [c*] / [c*0] construct to `need` copies of the
// fill byte (GNU: pad SET2 to the length of SET1).
func (sp *setSpec) applyFill(need int) {
	if sp.fillPos < 0 {
		return
	}
	if need < 0 {
		need = 0
	}
	nb := make([]byte, 0, len(sp.bytes)+need)
	nt := make([]byte, 0, len(sp.tags)+need)
	nb = append(nb, sp.bytes[:sp.fillPos]...)
	nt = append(nt, sp.tags[:sp.fillPos]...)
	for k := 0; k < need; k++ {
		nb = append(nb, sp.fillByte)
		nt = append(nt, tagNone)
	}
	nb = append(nb, sp.bytes[sp.fillPos:]...)
	nt = append(nt, sp.tags[sp.fillPos:]...)
	sp.bytes, sp.tags = nb, nt
	sp.fillPos = -1
}

func run(rc *tool.RunContext, args []string) int {
	// -C is a synonym for -c/--complement with no long form of its own
	// (GNU getopt short option): pre-parse it out of short-flag clusters.
	complementC := false
	pre := make([]string, 0, len(args))
	for k, a := range args {
		if a == "--" {
			pre = append(pre, args[k:]...)
			break
		}
		if len(a) >= 2 && a[0] == '-' && a[1] != '-' && strings.Contains(a, "C") {
			complementC = true
			a = strings.ReplaceAll(a, "C", "")
			if a == "-" {
				continue
			}
		}
		pre = append(pre, a)
	}

	fs := tool.NewFlags(cmd.Name)
	complement := fs.BoolP("complement", "c", false, "use the complement of SET1")
	complementUpper := fs.BoolP("complement-C", "C", false, "use the complement of SET1")
	del := fs.BoolP("delete", "d", false, "delete characters in SET1, do not translate")
	squeeze := fs.BoolP("squeeze-repeats", "s", false, "replace each sequence of a repeated character that is listed in the last specified SET, with a single occurrence of that character")
	truncateSet1 := fs.BoolP("truncate-set1", "t", false, "truncate SET1 to the length of SET2")
	operands, code := tool.Parse(rc, cmd, fs, pre)
	if code >= 0 {
		return code
	}
	comp := *complement || complementC || *complementUpper
	deleting, squeezing := *del, *squeeze

	if len(operands) == 0 {
		return tool.UsageError(rc, cmd, "missing operand")
	}
	if len(operands) > 2 {
		return tool.UsageError(rc, cmd, "extra operand '%s'", operands[2])
	}
	nset := len(operands)
	translating := false
	switch {
	case deleting && squeezing:
		if nset != 2 {
			fmt.Fprintf(rc.Err, "tr: missing operand after '%s'\nTwo strings must be given when both deleting and squeezing repeats.\n", operands[0])
			fmt.Fprintf(rc.Err, "Try 'tr --help' for more information.\n")
			return 2
		}
	case deleting:
		if nset == 2 {
			fmt.Fprintf(rc.Err, "tr: extra operand '%s'\nOnly one string may be given when deleting without squeezing repeats.\n", operands[1])
			fmt.Fprintf(rc.Err, "Try 'tr --help' for more information.\n")
			return 2
		}
	case squeezing && nset == 1:
		// squeeze-only mode
	default:
		if nset == 1 {
			fmt.Fprintf(rc.Err, "tr: missing operand after '%s'\nTwo strings must be given when translating.\n", operands[0])
			fmt.Fprintf(rc.Err, "Try 'tr --help' for more information.\n")
			return 2
		}
		translating = true
	}

	fail := func(msg string) int {
		fmt.Fprintf(rc.Err, "tr: %s\n", msg)
		return 1
	}

	set1, errMsg := parseSet(operands[0], false)
	if errMsg != "" {
		return fail(errMsg)
	}
	var set2 *setSpec
	if nset == 2 {
		set2, errMsg = parseSet(operands[1], true)
		if errMsg != "" {
			return fail(errMsg)
		}
	}

	// Effective SET1: complement is the ascending sequence of bytes NOT
	// in the expanded SET1 (class membership survives; case tags do not).
	var member1 [256]bool
	for _, b := range set1.bytes {
		member1[b] = true
	}
	eff1 := set1
	if comp {
		var cb []byte
		for c := 0; c < 256; c++ {
			if !member1[byte(c)] {
				cb = append(cb, byte(c))
			}
		}
		eff1 = &setSpec{bytes: cb, tags: make([]byte, len(cb)), fillPos: -1}
		member1 = [256]bool{}
		for _, b := range cb {
			member1[b] = true
		}
	}

	var xlate [256]byte
	for c := range xlate {
		xlate[c] = byte(c)
	}
	if translating {
		if set2.hasOtherClass {
			return fail("when translating, the only character classes that may appear in string2 are 'upper' and 'lower'")
		}
		if comp && set2.hasCaseClass {
			return fail("when translating with complemented character classes,\nstring2 must map all characters in the domain to one")
		}
		// Expand a [c*] fill construct to make SET2 as long as literal SET1.
		set2.applyFill(len(set1.bytes) - len(set2.bytes))
		if *truncateSet1 {
			if len(set2.bytes) == 0 {
				eff1.bytes = eff1.bytes[:0]
				eff1.tags = eff1.tags[:0]
			} else if len(eff1.bytes) > len(set2.bytes) {
				eff1.bytes = eff1.bytes[:len(set2.bytes)]
				eff1.tags = eff1.tags[:len(set2.bytes)]
			}
		} else {
			if len(set2.bytes) == 0 {
				return fail("when not truncating set1, string2 must be non-empty")
			}
			if len(set2.bytes) < len(eff1.bytes) {
				if set2.lastIsClass {
					return fail("when translating with string1 longer than string2,\nthe latter string must not end with a character class")
				}
				last := set2.bytes[len(set2.bytes)-1]
				for len(set2.bytes) < len(eff1.bytes) {
					set2.append(last, tagNone)
				}
			}
		}
		for i, c1 := range eff1.bytes {
			t1, t2 := eff1.tags[i], set2.tags[i]
			switch {
			case t1 == tagNone && t2 != tagNone:
				return fail("misaligned [:upper:] and/or [:lower:] construct")
			case t1 == tagLower && t2 == tagUpper:
				xlate[c1] = asciiUpper(c1)
			case t1 == tagUpper && t2 == tagLower:
				xlate[c1] = asciiLower(c1)
			default:
				xlate[c1] = set2.bytes[i]
			}
		}
	}

	var squeezeSet [256]bool
	if squeezing {
		src := eff1.bytes
		if nset == 2 {
			if !translating {
				// In delete+squeeze mode SET2 is the squeeze set; a [c*]
				// fill construct still expands to the length of SET1.
				set2.applyFill(len(set1.bytes) - len(set2.bytes))
			}
			src = set2.bytes
		}
		for _, b := range src {
			squeezeSet[b] = true
		}
	}

	in := bufio.NewReader(rc.In)
	out := bufio.NewWriter(rc.Out)
	lastOut := -1
	for {
		b, err := in.ReadByte()
		if err != nil {
			break
		}
		if deleting && member1[b] {
			continue
		}
		if translating {
			b = xlate[b]
		}
		if squeezing && squeezeSet[b] && int(b) == lastOut {
			continue
		}
		lastOut = int(b)
		if err := out.WriteByte(b); err != nil {
			if tool.IsClosedPipeError(err) {
				return 0
			}
			return fail(fmt.Sprintf("write error: %v", err))
		}
	}
	if err := out.Flush(); err != nil {
		if tool.IsClosedPipeError(err) {
			return 0
		}
		return fail(fmt.Sprintf("write error: %v", err))
	}
	return 0
}

// parseChar consumes one (possibly backslash-escaped) character of s
// starting at *i and returns its byte value. GNU escapes: \a \b \f \n
// \r \t \v \\ and \NNN (1-3 octal digits; an out-of-range third digit
// is left unconsumed, matching GNU's 2-byte interpretation of \400).
// A backslash before any other character yields that character; a
// trailing backslash is a literal backslash.
func parseChar(s string, i *int) byte {
	c := s[*i]
	if c != '\\' {
		*i++
		return c
	}
	if *i+1 >= len(s) {
		*i++
		return '\\'
	}
	*i++
	c = s[*i]
	switch c {
	case 'a':
		c = '\a'
	case 'b':
		c = '\b'
	case 'f':
		c = '\f'
	case 'n':
		c = '\n'
	case 'r':
		c = '\r'
	case 't':
		c = '\t'
	case 'v':
		c = '\v'
	case '\\':
		// literal backslash
	case '0', '1', '2', '3', '4', '5', '6', '7':
		val, n := 0, 0
		for n < 3 && *i < len(s) && s[*i] >= '0' && s[*i] <= '7' {
			nv := val*8 + int(s[*i]-'0')
			if nv > 255 {
				break
			}
			val = nv
			*i++
			n++
		}
		return byte(val)
	default:
		// \X with no special meaning: X itself
	}
	*i++
	return c
}

// parseSet expands a SET string into its byte sequence: literal and
// escaped characters, ranges (m-n), [:class:] constructs, [=c=]
// equivalence classes (single member in the C locale), and — in SET2
// only — [c*n] / [c*] repeat constructs.
func parseSet(s string, isSet2 bool) (*setSpec, string) {
	sp := &setSpec{fillPos: -1}
	i := 0
	for i < len(s) {
		if s[i] == '[' {
			if cls, adv, ok := matchClass(s[i:]); ok {
				expanded := classBytes(cls)
				if expanded == nil {
					return nil, fmt.Sprintf("invalid character class '%s'", cls)
				}
				tag := tagNone
				switch cls {
				case "lower":
					tag = tagLower
					sp.hasCaseClass = true
				case "upper":
					tag = tagUpper
					sp.hasCaseClass = true
				default:
					sp.hasOtherClass = true
				}
				for _, b := range expanded {
					sp.append(b, tag)
				}
				sp.lastIsClass = true
				i += adv
				continue
			}
			if eqc, adv, ok, errMsg := matchEquiv(s[i:]); errMsg != "" {
				return nil, errMsg
			} else if ok {
				sp.append(eqc, tagNone)
				sp.lastIsClass = false
				i += adv
				continue
			}
			if rb, count, fill, adv, ok, errMsg := matchRepeat(s[i:]); errMsg != "" {
				return nil, errMsg
			} else if ok {
				if !isSet2 {
					return nil, "the [c*] repeat construct may not appear in string1"
				}
				if fill {
					if sp.fillPos >= 0 {
						return nil, "only one [c*] repeat construct may appear in string2"
					}
					sp.fillPos = len(sp.bytes)
					sp.fillByte = rb
				} else {
					for k := 0; k < count; k++ {
						sp.append(rb, tagNone)
					}
				}
				sp.lastIsClass = false
				i += adv
				continue
			}
		}
		lo := parseChar(s, &i)
		if i < len(s) && s[i] == '-' && i+1 < len(s) {
			j := i + 1
			hi := parseChar(s, &j)
			if hi < lo {
				return nil, fmt.Sprintf("range-endpoints of '%c-%c' are in reverse collating sequence order", lo, hi)
			}
			for b := int(lo); b <= int(hi); b++ {
				sp.append(byte(b), tagNone)
			}
			i = j
			sp.lastIsClass = false
			continue
		}
		sp.append(lo, tagNone)
		sp.lastIsClass = false
	}
	return sp, ""
}

// matchClass matches a leading "[:name:]" and returns (name, length, ok).
// A malformed construct (no closing ":]") is not an error — the bytes
// are then taken literally, as GNU does.
func matchClass(s string) (string, int, bool) {
	if len(s) < 4 || s[1] != ':' {
		return "", 0, false
	}
	end := strings.Index(s[2:], ":]")
	if end < 0 {
		return "", 0, false
	}
	return s[2 : 2+end], 2 + end + 2, true
}

// matchEquiv matches a leading "[=c=]". In the C locale an equivalence
// class contains exactly its own character.
func matchEquiv(s string) (byte, int, bool, string) {
	if len(s) < 4 || s[1] != '=' {
		return 0, 0, false, ""
	}
	end := strings.Index(s[2:], "=]")
	if end < 0 {
		return 0, 0, false, ""
	}
	inner := s[2 : 2+end]
	j := 0
	var c byte
	if inner != "" {
		c = parseChar(inner, &j)
	}
	if j != len(inner) || inner == "" {
		return 0, 0, false, fmt.Sprintf("%s: equivalence class operand must be a single character", inner)
	}
	return c, 2 + end + 2, true, ""
}

// matchRepeat matches a leading "[c*n]" / "[c*]". n is decimal, or
// octal with a leading 0; n omitted means "pad SET2 to the length of
// SET1" (fill). n=0 is a valid explicit repeat count of zero.
func matchRepeat(s string) (b byte, count int, fill bool, adv int, ok bool, errMsg string) {
	if len(s) < 2 {
		return
	}
	j := 1
	c := parseChar(s, &j)
	if j >= len(s) || s[j] != '*' {
		return
	}
	j++
	digStart := j
	for j < len(s) && s[j] >= '0' && s[j] <= '9' {
		j++
	}
	if j >= len(s) || s[j] != ']' {
		return
	}
	digits := s[digStart:j]
	n := 0
	if digits != "" {
		base := 10
		if digits[0] == '0' {
			base = 8
		}
		v, err := strconv.ParseInt(digits, base, 32)
		if err != nil {
			errMsg = fmt.Sprintf("invalid repeat count '%s' in [c*n] construct", digits)
			return
		}
		n = int(v)
	}
	return c, n, digits == "", j + 1, true, ""
}

// classBytes returns the C-locale members of a POSIX character class in
// ascending byte order, or nil for an unknown class name.
func classBytes(name string) []byte {
	pred, known := classPred[name]
	if !known {
		return nil
	}
	var out []byte
	for c := 0; c < 256; c++ {
		if pred(byte(c)) {
			out = append(out, byte(c))
		}
	}
	return out
}

var classPred = map[string]func(byte) bool{
	"alnum":  func(b byte) bool { return isAlpha(b) || isDigit(b) },
	"alpha":  isAlpha,
	"blank":  func(b byte) bool { return b == ' ' || b == '\t' },
	"cntrl":  func(b byte) bool { return b < 32 || b == 127 },
	"digit":  isDigit,
	"graph":  isGraph,
	"lower":  func(b byte) bool { return b >= 'a' && b <= 'z' },
	"print":  func(b byte) bool { return b >= 32 && b <= 126 },
	"punct":  func(b byte) bool { return isGraph(b) && !isAlpha(b) && !isDigit(b) },
	"space":  func(b byte) bool { return b == ' ' || (b >= '\t' && b <= '\r') },
	"upper":  func(b byte) bool { return b >= 'A' && b <= 'Z' },
	"xdigit": func(b byte) bool { return isDigit(b) || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F') },
}

func isAlpha(b byte) bool { return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') }
func isDigit(b byte) bool { return b >= '0' && b <= '9' }
func isGraph(b byte) bool { return b >= 33 && b <= 126 }

func asciiUpper(b byte) byte {
	if b >= 'a' && b <= 'z' {
		return b - 32
	}
	return b
}

func asciiLower(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b + 32
	}
	return b
}
