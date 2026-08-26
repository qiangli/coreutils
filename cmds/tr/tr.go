// Package trcmd implements tr(1) per POSIX.1-2008/2016 Issue 7 and the
// GNU coreutils manual: translate, squeeze, and/or delete characters
// from standard input, writing to standard output.
//
// The unit tr operates on is the character of the invocation's LC_CTYPE
// (resolved from rc.Env via pkg/locale), which XCU:tr:ENVIRONMENT_VARIABLES
// defines as "the interpretation of sequences of bytes of text data as
// characters ... and the behavior of character classes". Under C/POSIX a
// character is a byte and the pure-Go ASCII tables are used directly;
// under another single-byte locale an injectable ctypeOpener provides
// class membership and case maps; under a UTF-8 codeset in POSIX mode a
// character is a multi-byte sequence. See charmodel.go.
package trcmd

import (
	"bufio"
	"fmt"
	"io"
	"maps"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "tr",
	Synopsis: "Translate, squeeze, and/or delete characters from standard input, writing to standard output. Supports -C character complement.",
	Usage:    "tr [OPTION]... SET1 [SET2]",
}

// Run is wired in init: a literal would create an initialization
// cycle (run's flag-error paths reference cmd).
func init() { cmd.Run = run; tool.Register(cmd) }

// Tags mark which characters of an expanded set came from a
// case-conversion character class — translation pairs
// [:upper:]/[:lower:] positionally.
const (
	tagNone byte = iota
	tagLower
	tagUpper
)

type caseClassSpan struct {
	start int
	tag   byte
}

type setToken struct {
	chars []rune
	tag   byte
}

type setSpec struct {
	chars         []rune
	tags          []byte
	caseClasses   []caseClassSpan
	lastIsClass   bool // last construct parsed was a [:class:]
	hasCaseClass  bool // contains [:upper:] or [:lower:]
	hasOtherClass bool // contains any other [:class:]
	fillPos       int  // insertion point of a [c*] fill construct; -1 = none
	fillTokenPos  int  // logical-token insertion point for [c*]; -1 = none
	fillChar      rune
	tokens        []setToken
	usesCollation bool // contains [=c=] or a non-octal range
}

func (sp *setSpec) append(r rune, tag byte) {
	sp.chars = append(sp.chars, r)
	sp.tags = append(sp.tags, tag)
}

func (sp *setSpec) appendOrdinary(r rune) {
	sp.append(r, tagNone)
	sp.tokens = append(sp.tokens, setToken{chars: []rune{r}})
}

func (sp *setSpec) applyLogicalFill(tokenNeed, rawNeed int) {
	if sp.fillTokenPos < 0 {
		return
	}
	if tokenNeed < 0 {
		tokenNeed = 0
	}
	insert := make([]setToken, tokenNeed)
	for i := range insert {
		insert[i] = setToken{chars: []rune{sp.fillChar}}
	}
	tokens := make([]setToken, 0, len(sp.tokens)+tokenNeed)
	tokens = append(tokens, sp.tokens[:sp.fillTokenPos]...)
	tokens = append(tokens, insert...)
	tokens = append(tokens, sp.tokens[sp.fillTokenPos:]...)
	sp.tokens = tokens
	sp.fillTokenPos = -1
	sp.applyFill(rawNeed)
}

// applyFill expands a [c*] / [c*0] construct to `need` copies of the
// fill character (GNU: pad SET2 to the length of SET1).
func (sp *setSpec) applyFill(need int) {
	if sp.fillPos < 0 {
		return
	}
	if need < 0 {
		need = 0
	}
	nb := make([]rune, 0, len(sp.chars)+need)
	nt := make([]byte, 0, len(sp.tags)+need)
	nb = append(nb, sp.chars[:sp.fillPos]...)
	nt = append(nt, sp.tags[:sp.fillPos]...)
	for k := 0; k < need; k++ {
		nb = append(nb, sp.fillChar)
		nt = append(nt, tagNone)
	}
	nb = append(nb, sp.chars[sp.fillPos:]...)
	nt = append(nt, sp.tags[sp.fillPos:]...)
	sp.chars, sp.tags = nb, nt
	if need > 0 {
		for i := range sp.caseClasses {
			if sp.caseClasses[i].start >= sp.fillPos {
				sp.caseClasses[i].start += need
			}
		}
	}
	sp.fillPos = -1
}

func validateCaseClasses(set1, set2 *setSpec) bool {
	for _, c2 := range set2.caseClasses {
		matched := false
		for _, c1 := range set1.caseClasses {
			if c1.start == c2.start {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

func planCaseTokens(set1 *setSpec, target []setToken, lastIsClass bool, tables *charTables, truncate bool, xlate map[rune]rune) string {
	if len(target) == 0 && !truncate {
		return "when not truncating set1, string2 must be non-empty"
	}

	ti := 0
	var last rune
	haveLast := false
	nextTarget := func() (rune, bool, string) {
		if ti < len(target) {
			t := target[ti]
			if t.tag != tagNone {
				return 0, false, "misaligned [:upper:] and/or [:lower:] construct"
			}
			ti++
			last, haveLast = t.chars[0], true
			return last, true, ""
		}
		if truncate {
			return 0, false, ""
		}
		if lastIsClass {
			return 0, false, "when translating with string1 longer than string2,\nthe latter string must not end with a character class"
		}
		if !haveLast {
			return 0, false, "when not truncating set1, string2 must be non-empty"
		}
		return last, true, ""
	}

	for _, source := range set1.tokens {
		if source.tag != tagNone && ti < len(target) && target[ti].tag != tagNone {
			targetToken := target[ti]
			ti++
			switch {
			case source.tag == targetToken.tag:
				for _, r := range source.chars {
					xlate[r] = r
				}
			case source.tag == tagLower && targetToken.tag == tagUpper:
				for _, r := range source.chars {
					xlate[r] = tables.toUpper(r)
				}
			case source.tag == tagUpper && targetToken.tag == tagLower:
				for _, r := range source.chars {
					xlate[r] = tables.toLower(r)
				}
			}
			continue
		}

		// An unpaired source case expands to its ordered ordinary members.
		for _, r := range source.chars {
			target, ok, errMsg := nextTarget()
			if errMsg != "" {
				return errMsg
			}
			if ok {
				xlate[r] = target
			} else {
				// Truncated away: record the identity mapping explicitly so
				// two candidate plans stay comparable key-for-key.
				xlate[r] = r
			}
		}
	}
	for ; ti < len(target); ti++ {
		if target[ti].tag != tagNone {
			return "misaligned [:upper:] and/or [:lower:] construct"
		}
	}
	return ""
}

type sourceCursor struct {
	token int
	off   int
}

func consumeSourceChar(tokens []setToken, cur *sourceCursor) bool {
	for cur.token < len(tokens) {
		if cur.off < len(tokens[cur.token].chars) {
			cur.off++
			if cur.off == len(tokens[cur.token].chars) {
				cur.token++
				cur.off = 0
			}
			return true
		}
		cur.token++
		cur.off = 0
	}
	return false
}

func sourceAfterTargetPrefix(source, prefix []setToken) (sourceCursor, bool) {
	cur := sourceCursor{}
	for _, target := range prefix {
		if target.tag == tagNone {
			consumeSourceChar(source, &cur) // Extra SET2 characters are harmless.
			continue
		}
		if cur.token >= len(source) || cur.off != 0 || source[cur.token].tag == tagNone {
			return cur, false
		}
		cur.token++
	}
	return cur, true
}

func tokensWithFill(tokens []setToken, pos, count int, r rune) []setToken {
	out := make([]setToken, 0, len(tokens)+count)
	out = append(out, tokens[:pos]...)
	for i := 0; i < count; i++ {
		out = append(out, setToken{chars: []rune{r}})
	}
	return append(out, tokens[pos:]...)
}

func planCaseTranslation(set1, set2 *setSpec, tables *charTables, truncate bool, rawFillCount int, xlate map[rune]rune) string {
	if set2.fillTokenPos < 0 {
		return planCaseTokens(set1, set2.tokens, set2.lastIsClass, tables, truncate, xlate)
	}

	fillPos := set2.fillTokenPos
	cur, ok := sourceAfterTargetPrefix(set1.tokens, set2.tokens[:fillPos])
	if !ok {
		return "misaligned [:upper:] and/or [:lower:] construct"
	}

	firstCase := -1
	for i := fillPos; i < len(set2.tokens); i++ {
		if set2.tokens[i].tag != tagNone {
			firstCase = i
			break
		}
	}
	if firstCase < 0 {
		set2.applyLogicalFill(rawFillCount, rawFillCount)
		return planCaseTokens(set1, set2.tokens, set2.lastIsClass, tables, truncate, xlate)
	}

	ordinaryDistance := firstCase - fillPos
	candidates := make([]int, 0)
	seen := make(map[int]bool)
	distance := 0
	for i, off := cur.token, cur.off; i < len(set1.tokens); i, off = i+1, 0 {
		if off == 0 && set1.tokens[i].tag != tagNone {
			if k := distance - ordinaryDistance; k >= 0 && k <= rawFillCount && !seen[k] {
				seen[k] = true
				candidates = append(candidates, k)
			}
		}
		distance += len(set1.tokens[i].chars) - off
	}

	var accepted map[rune]rune
	acceptedK := -1
	for _, k := range candidates {
		target := tokensWithFill(set2.tokens, fillPos, k, set2.fillChar)
		candidate := make(map[rune]rune)
		if planCaseTokens(set1, target, set2.lastIsClass, tables, truncate, candidate) != "" {
			continue
		}
		if accepted == nil {
			accepted = candidate
			acceptedK = k
			continue
		}
		if !maps.Equal(accepted, candidate) {
			return "misaligned [:upper:] and/or [:lower:] construct"
		}
	}
	if accepted == nil {
		return "misaligned [:upper:] and/or [:lower:] construct"
	}
	maps.Copy(xlate, accepted)
	set2.applyLogicalFill(acceptedK, rawFillCount)
	return ""
}

func run(rc *tool.RunContext, args []string) int {
	return runWithProviders(rc, args, prodOpener, prodCollateOpener)
}

func runWithCType(rc *tool.RunContext, args []string, opener ctypeOpener) int {
	return runWithLocaleProviders(rc, args, opener, nil, false)
}

func runWithProviders(rc *tool.RunContext, args []string, opener ctypeOpener, collateOpen collateOpener) int {
	return runWithLocaleProviders(rc, args, opener, collateOpen, true)
}

func runWithLocaleProviders(rc *tool.RunContext, args []string, opener ctypeOpener, collateOpen collateOpener, resolveCollate bool) int {
	// -C is the character-complement spelling and has no public long form.
	// Pre-parse it out of short-flag clusters; unlike -c's binary-value order,
	// its effective translation array follows LC_COLLATE.
	complementC := false
	pre := make([]string, 0, len(args))
	operandsStart := false
	for _, a := range args {
		if operandsStart {
			pre = append(pre, a)
			continue
		}
		if a == "--" {
			operandsStart = true
			pre = append(pre, a)
			continue
		}
		if !strings.HasPrefix(a, "-") || a == "-" {
			operandsStart = true
			pre = append(pre, "--", a)
			continue
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
	charComplement := complementC || *complementUpper
	valueComplement := *complement
	comp := valueComplement || charComplement
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

	// Resolve LC_CTYPE and select the character universe.
	tables, lcCType, ctypeErr := openCharTables(rc.Env, opener)
	if ctypeErr != nil {
		fmt.Fprintf(rc.Err, "tr: failed to open LC_CTYPE %q: %v\n", lcCType, ctypeErr)
		return 2
	}
	tables.collate = cCollationTables()
	tables.discoverCollation = resolveCollate

	set1, errMsg := parseSetTables(operands[0], false, tables)
	if errMsg != "" {
		return fail(errMsg)
	}
	var set2 *setSpec
	if nset == 2 {
		set2, errMsg = parseSetTables(operands[1], true, tables)
		if errMsg != "" {
			return fail(errMsg)
		}
	}
	// LC_COLLATE is relevant only to ranges, equivalence classes, and the
	// ordered character complement used by -C translation. Literal-only
	// deletion/squeezing must remain usable under an otherwise uncarried
	// LC_COLLATE. Parse once with C tables to discover the grammar surface,
	// then reparse with a complete non-C snapshot when the invocation needs it.
	needsCollation := set1.usesCollation || set2 != nil && set2.usesCollation || charComplement && translating
	if resolveCollate && needsCollation {
		collation, lcCollate, collateErr := openCollationTables(rc.Env, tables, collateOpen)
		if collateErr != nil {
			fmt.Fprintf(rc.Err, "tr: failed to open LC_COLLATE %q: %v\n", lcCollate, collateErr)
			return 2
		}
		tables.collate = collation
		tables.discoverCollation = false
		set1, errMsg = parseSetTables(operands[0], false, tables)
		if errMsg != "" {
			return fail(errMsg)
		}
		if nset == 2 {
			set2, errMsg = parseSetTables(operands[1], true, tables)
			if errMsg != "" {
				return fail(errMsg)
			}
		}
	}
	binaryValueMode := valueComplement && !charComplement && tables.multibyte

	// member1 is SET1 membership in the selected complement domain: encoded
	// byte values for multi-byte -c, characters otherwise.
	member1 := make(map[rune]bool, len(set1.chars))
	for _, r := range set1.chars {
		member1[r] = true
	}
	if binaryValueMode {
		member1 = encodedValues(set1.chars)
	}
	matches := func(r rune) bool {
		if comp {
			return !member1[r]
		}
		return member1[r]
	}

	// Effective SET1 is the ordered complement. Byte-value domains are small
	// enough to enumerate; the multi-byte -C character domain stays symbolic.
	eff1 := set1
	if comp && (!tables.multibyte || binaryValueMode) {
		var complementChars []rune
		if charComplement {
			complementChars = tables.characterComplement(member1)
		} else {
			for c := 0; c < 256; c++ {
				if !member1[rune(c)] {
					complementChars = append(complementChars, rune(c))
				}
			}
		}
		eff1 = &setSpec{chars: complementChars, tags: make([]byte, len(complementChars)), fillPos: -1, fillTokenPos: -1}
	}

	xlate := make(map[rune]rune)
	// defaultTarget answers every character admitted by a complemented SET1
	// that the enumerated prefix did not reach: GNU pads SET2 with its last
	// character, and every remaining complement member maps to it.
	var defaultTarget rune
	hasDefault := false
	var complementPlan *complementFillPlan
	if translating {
		rawFillCount := len(eff1.chars) - len(set2.chars)
		if rawFillCount < 0 {
			rawFillCount = 0
		}
		if set2.hasOtherClass {
			return fail("when translating, the only character classes that may appear in string2 are 'upper' and 'lower'")
		}
		if comp && set2.hasCaseClass {
			return fail("when translating with complemented character classes,\nstring2 must map all characters in the domain to one")
		}
		switch {
		case !comp && set2.hasCaseClass:
			if errMsg := planCaseTranslation(set1, set2, tables, *truncateSet1, rawFillCount, xlate); errMsg != "" {
				return fail(errMsg)
			}
		case charComplement && tables.multibyte:
			if len(set2.chars) == 0 {
				if set2.fillTokenPos < 0 && !*truncateSet1 {
					return fail("when not truncating set1, string2 must be non-empty")
				}
			}
			if set2.fillTokenPos >= 0 {
				// The Unicode scalar universe is large but finite. Keep the
				// [c*] run symbolic and map by complement rank instead of
				// allocating roughly 1.1 million entries.
				complementPlan = newComplementFillPlan(member1, set2)
			} else {
				for i, c1 := range complementPrefix(func(r rune) bool { return member1[r] }, len(set2.chars)) {
					xlate[c1] = set2.chars[i]
				}
				if !*truncateSet1 && len(set2.chars) > 0 {
					defaultTarget, hasDefault = set2.chars[len(set2.chars)-1], true
				}
			}
		default:
			// Keep complement and all-ordinary translation on the flat path.
			set2.applyFill(rawFillCount)
			if !validateCaseClasses(set1, set2) {
				return fail("misaligned [:upper:] and/or [:lower:] construct")
			}
			if *truncateSet1 {
				if len(set2.chars) == 0 {
					eff1.chars = eff1.chars[:0]
					eff1.tags = eff1.tags[:0]
				} else if len(eff1.chars) > len(set2.chars) {
					eff1.chars = eff1.chars[:len(set2.chars)]
					eff1.tags = eff1.tags[:len(set2.chars)]
				}
			} else {
				if len(set2.chars) == 0 {
					return fail("when not truncating set1, string2 must be non-empty")
				}
				if len(set2.chars) < len(eff1.chars) {
					if set2.lastIsClass {
						return fail("when translating with string1 longer than string2,\nthe latter string must not end with a character class")
					}
					last := set2.chars[len(set2.chars)-1]
					for len(set2.chars) < len(eff1.chars) {
						set2.append(last, tagNone)
					}
				}
			}
			for i, c1 := range eff1.chars {
				t1, t2 := eff1.tags[i], set2.tags[i]
				switch {
				case t1 == tagNone && t2 != tagNone:
					return fail("misaligned [:upper:] and/or [:lower:] construct")
				case t1 == tagLower && t2 == tagUpper:
					xlate[c1] = tables.toUpper(c1)
				case t1 == tagUpper && t2 == tagLower:
					xlate[c1] = tables.toLower(c1)
				default:
					xlate[c1] = set2.chars[i]
				}
			}
		}
	}

	// The squeeze set is SET2 whenever two strings were given, and the
	// effective (possibly complemented) SET1 otherwise.
	var squeezeSet map[rune]bool
	squeezeComplement := false
	if squeezing {
		if nset == 2 {
			if !translating {
				// In delete+squeeze mode SET2 is the squeeze set; a [c*]
				// fill construct still expands to the length of SET1.
				set2.applyFill(len(set1.chars) - len(set2.chars))
			}
			squeezeSet = make(map[rune]bool, len(set2.chars))
			for _, r := range set2.chars {
				squeezeSet[r] = true
			}
		} else {
			squeezeSet, squeezeComplement = member1, comp
		}
	}
	inSqueeze := func(r rune) bool {
		if squeezeSet == nil {
			return false
		}
		if squeezeComplement {
			return !squeezeSet[r]
		}
		return squeezeSet[r]
	}

	translate := func(r rune) rune {
		if to, ok := xlate[r]; ok {
			return to
		}
		if hasDefault && !member1[r] {
			return defaultTarget
		}
		if complementPlan != nil {
			if to, ok := complementPlan.translate(r); ok {
				return to
			}
		}
		return r
	}

	in := bufio.NewReader(rc.In)
	out := bufio.NewWriter(rc.Out)
	var readErr error
	writeFailed := -1
	onWriteErr := func(err error) int {
		if tool.IsClosedPipeError(err) {
			if rc.SIGPIPEIgnored {
				fmt.Fprintln(rc.Err, "tr: stdout: Broken pipe")
				return 1
			}
			return 0
		}
		return fail(fmt.Sprintf("write error: %v", err))
	}

	if binaryValueMode {
		// POSIX -c complements encoded values, not characters. Under a
		// multi-byte LC_CTYPE, consume one byte value at a time; bytes excluded
		// by SET1 are copied exactly, while complemented values may map to a
		// multi-byte SET2 character.
		var post *characterSqueezer
		if squeezing {
			post = &characterSqueezer{out: out, matches: inSqueeze}
		}
		for {
			b, err := in.ReadByte()
			if err != nil {
				if err != io.EOF {
					readErr = err
				}
				break
			}
			selected := matches(rune(b))
			if deleting && selected {
				continue
			}
			outRune := rune(b)
			raw := true
			if translating && selected {
				if mapped, ok := xlate[rune(b)]; ok {
					outRune = mapped
					raw = false
				} else if hasDefault {
					outRune = defaultTarget
					raw = false
				}
			}
			if raw && b >= utf8.RuneSelf {
				outRune = escapeRune(b)
			}
			var writeErr error
			if post != nil && raw {
				writeErr = post.writeBytes([]byte{b})
			} else if post != nil {
				writeErr = post.writeRune(outRune)
			} else if raw {
				writeErr = out.WriteByte(b)
			} else {
				writeErr = writeChar(out, outRune)
			}
			if writeErr != nil {
				writeFailed = onWriteErr(writeErr)
				break
			}
		}
		if writeFailed < 0 && post != nil {
			if err := post.finish(); err != nil {
				writeFailed = onWriteErr(err)
			}
		}
	} else if !tables.multibyte {
		// Single-byte universe: collapse the plans into 256-entry tables so
		// the transform stays one array lookup per byte.
		var deleteByte, squeezeByte [256]bool
		var xlateByte [256]byte
		for c := 0; c < 256; c++ {
			xlateByte[c] = byte(translate(rune(c)))
			deleteByte[c] = matches(rune(c))
			squeezeByte[c] = inSqueeze(rune(c))
		}
		lastOut := -1
		for {
			b, err := in.ReadByte()
			if err != nil {
				if err != io.EOF {
					readErr = err
				}
				break
			}
			if deleting && deleteByte[b] {
				continue
			}
			if translating {
				b = xlateByte[b]
			}
			if squeezing && squeezeByte[b] && int(b) == lastOut {
				continue
			}
			lastOut = int(b)
			if err := out.WriteByte(b); err != nil {
				writeFailed = onWriteErr(err)
				break
			}
		}
	} else {
		lastOut := rune(-1)
		for {
			r, err := readChar(in)
			if err != nil {
				if err != io.EOF {
					readErr = err
				}
				break
			}
			if deleting && matches(r) {
				continue
			}
			if translating {
				r = translate(r)
			}
			if squeezing && inSqueeze(r) && r == lastOut {
				continue
			}
			lastOut = r
			if err := writeChar(out, r); err != nil {
				writeFailed = onWriteErr(err)
				break
			}
		}
	}
	if writeFailed >= 0 {
		return writeFailed
	}
	if err := out.Flush(); err != nil {
		return onWriteErr(err)
	}
	if readErr != nil {
		return fail(fmt.Sprintf("read error: %v", readErr))
	}
	return 0
}

// readChar reads one multi-byte character. A byte that begins no valid
// character is one character that keeps its own value, so tr never
// rewrites data it cannot interpret.
func readChar(in *bufio.Reader) (rune, error) {
	r, size, err := in.ReadRune()
	if err != nil {
		return 0, err
	}
	if r == utf8.RuneError && size == 1 {
		if err := in.UnreadRune(); err != nil {
			return 0, err
		}
		b, err := in.ReadByte()
		if err != nil {
			return 0, err
		}
		return escapeRune(b), nil
	}
	return r, nil
}

// writeChar writes one multi-byte character, or the exact byte behind an
// uninterpretable one.
func writeChar(out *bufio.Writer, r rune) error {
	if isEscapedByte(r) {
		return out.WriteByte(byte(r - escapeBase))
	}
	_, err := out.WriteRune(r)
	return err
}

// characterSqueezer is the post-transform stage required when -c operates on
// encoded values. Deletion/translation feed it bytes, but -s compares the
// resulting LC_CTYPE characters. It buffers at most one UTF-8 character and
// preserves an invalid or incomplete byte exactly via the escaped-byte model.
type characterSqueezer struct {
	out     *bufio.Writer
	matches func(rune) bool
	pending []byte
	last    rune
	have    bool
}

func (s *characterSqueezer) writeRune(r rune) error {
	if isEscapedByte(r) {
		return s.writeBytes([]byte{byte(r - escapeBase)})
	}
	var encoded [utf8.UTFMax]byte
	n := utf8.EncodeRune(encoded[:], r)
	return s.writeBytes(encoded[:n])
}

func (s *characterSqueezer) writeBytes(p []byte) error {
	for _, b := range p {
		s.pending = append(s.pending, b)
		if err := s.drain(false); err != nil {
			return err
		}
	}
	return nil
}

func (s *characterSqueezer) finish() error { return s.drain(true) }

func (s *characterSqueezer) drain(final bool) error {
	for len(s.pending) > 0 {
		if !utf8.FullRune(s.pending) && !final {
			return nil
		}
		r, size := utf8.DecodeRune(s.pending)
		if r == utf8.RuneError && size == 1 {
			r = escapeRune(s.pending[0])
		}
		raw := s.pending[:size]
		if !(s.matches(r) && s.have && r == s.last) {
			if _, err := s.out.Write(raw); err != nil {
				return err
			}
		}
		s.last, s.have = r, true
		s.pending = s.pending[size:]
	}
	return nil
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

// parseSetTables expands a control string into its character sequence for
// the selected universe.
func parseSetTables(s string, isSet2 bool, tables *charTables) (*setSpec, string) {
	if tables.multibyte {
		return parseSetMultibyte(s, isSet2, tables)
	}
	return parseSetWithTables(s, isSet2, tables)
}

// parseSetWithTables expands a SET string into its byte sequence: literal
// and escaped characters, ranges (m-n), [:class:] constructs, [=c=]
// equivalence classes (a single member outside a collation provider), and
// — in SET2 only — [c*n] / [c*] repeat constructs. Every character is a
// byte here, which is the C/POSIX and single-byte-locale universe.
func parseSetWithTables(s string, isSet2 bool, tables *charTables) (*setSpec, string) {
	sp := &setSpec{fillPos: -1, fillTokenPos: -1}
	i := 0
	for i < len(s) {
		if s[i] == '[' {
			if cls, adv, ok := matchClass(s[i:]); ok {
				expanded, known := tables.classChars(cls)
				if !known {
					return nil, fmt.Sprintf("invalid character class '%s'", cls)
				}
				sp.addClass(cls, expanded)
				i += adv
				continue
			}
			if eqc, adv, ok, errMsg := matchEquiv(s[i:]); errMsg != "" {
				return nil, errMsg
			} else if ok {
				sp.usesCollation = true
				for _, r := range tables.collate.equivalents(eqc) {
					sp.appendOrdinary(r)
				}
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
					sp.fillPos = len(sp.chars)
					sp.fillTokenPos = len(sp.tokens)
					sp.fillChar = rune(rb)
				} else {
					for k := 0; k < count; k++ {
						sp.appendOrdinary(rune(rb))
					}
				}
				sp.lastIsClass = false
				i += adv
				continue
			}
		}
		loStart := i
		lo := parseChar(s, &i)
		loOctal := isOctalEscapeAt(s, loStart)
		if i < len(s) && s[i] == '-' && i+1 < len(s) {
			j := i + 1
			hiStart := j
			hi := parseChar(s, &j)
			hiOctal := isOctalEscapeAt(s, hiStart)
			if loOctal || hiOctal {
				if hi < lo {
					return nil, fmt.Sprintf("range-endpoints of '%c-%c' are in reverse collating sequence order", lo, hi)
				}
				for b := int(lo); b <= int(hi); b++ {
					sp.appendOrdinary(rune(b))
				}
			} else {
				sp.usesCollation = true
				if tables.discoverCollation {
					sp.appendOrdinary(rune(lo))
					sp.appendOrdinary(rune(hi))
				} else {
					expanded, ok := tables.collate.rangeChars(lo, hi)
					if !ok {
						return nil, fmt.Sprintf("range-endpoints of '%c-%c' are in reverse collating sequence order", lo, hi)
					}
					for _, r := range expanded {
						sp.appendOrdinary(r)
					}
				}
			}
			i = j
			sp.lastIsClass = false
			continue
		}
		sp.appendOrdinary(rune(lo))
		sp.lastIsClass = false
	}
	return sp, ""
}

// addClass records one [:class:] construct and its expansion, tagging
// [:upper:] and [:lower:] so a translation can pair them positionally.
func (sp *setSpec) addClass(cls string, expanded []rune) {
	tag := tagNone
	switch cls {
	case "lower":
		tag = tagLower
		sp.hasCaseClass = true
		sp.caseClasses = append(sp.caseClasses, caseClassSpan{start: len(sp.chars), tag: tagLower})
	case "upper":
		tag = tagUpper
		sp.hasCaseClass = true
		sp.caseClasses = append(sp.caseClasses, caseClassSpan{start: len(sp.chars), tag: tagUpper})
	default:
		sp.hasOtherClass = true
	}
	if tag == tagNone {
		for _, r := range expanded {
			sp.appendOrdinary(r)
		}
	} else {
		for _, r := range expanded {
			sp.append(r, tag)
		}
		sp.tokens = append(sp.tokens, setToken{chars: expanded, tag: tag})
	}
	sp.lastIsClass = true
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

// matchEquiv matches a leading "[=c=]". Without a collation provider an
// equivalence class contains exactly its own character, which is the
// complete answer in the POSIX locale.
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

func isOctalEscapeAt(s string, i int) bool {
	return i+1 < len(s) && s[i] == '\\' && s[i+1] >= '0' && s[i+1] <= '7'
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
