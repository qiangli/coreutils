package bre

import (
	"fmt"
	"regexp"
	"strings"
)

// bytePatternTables is an invocation-owned snapshot of locale byte
// classification and folding data. The compiler copies the class map; callers
// cannot change a compiled pattern by mutating their tables afterwards.
type bytePatternTables struct {
	classes    map[string][256]bool
	equivalent [256][256]bool
	equivValid [256]bool
	collseq    [256]byte
	collating  [256]bool
	fold       [256]byte
	dotAll     bool
	multi      bool
}

type localeBytePattern struct {
	codec byteTokenCodec
	re    *regexp.Regexp
}

// snapshot returns a compile-owned copy of t. The class map is the only
// reference type in bytePatternTables, so every other field copies with the
// struct; duplicating this by hand in each grammar's compiler is what let the
// equivalence table reach BRE but not ERE, so both compilers call this instead.
func (t bytePatternTables) snapshot() bytePatternTables {
	out := t
	out.classes = make(map[string][256]bool, len(t.classes))
	for name, class := range t.classes {
		out.classes[name] = class
	}
	return out
}

// compileLocaleBytePattern compiles the consuming subset of POSIX BRE over
// locale-classified bytes. It is an unrouted substrate: public Regexp and all
// command consumers continue to use their existing engines.
func compileLocaleBytePattern(pattern []byte, input bytePatternTables, foldCase bool) (*localeBytePattern, error) {
	tables := input.snapshot()
	word, ok := tables.classes["word"]
	if !ok {
		return nil, fmt.Errorf("locale byte tables lack word class")
	}
	if _, ok := tables.classes["space"]; !ok {
		return nil, fmt.Errorf("locale byte tables lack space class")
	}
	codec, err := newByteTokenCodec(word)
	if err != nil {
		return nil, err
	}
	translated, err := translateLocaleByteBRE(pattern, codec, tables, foldCase)
	if err != nil {
		return nil, err
	}
	prefix := ""
	if tables.multi {
		prefix = "(?m)"
	}
	re, err := regexp.Compile(prefix + translated)
	if err != nil {
		return nil, err
	}
	re.Longest()
	return &localeBytePattern{codec: codec, re: re}, nil
}

// findSubmatchIndex returns raw byte offsets. Any index not produced at a
// complete token boundary is rejected instead of being approximated.
func (p *localeBytePattern) findSubmatchIndex(raw []byte) ([]int, error) {
	encoded := p.codec.encodeSubject(raw)
	indices := p.re.FindStringSubmatchIndex(encoded.text)
	if indices == nil {
		return nil, nil
	}
	for i, offset := range indices {
		if offset < 0 {
			continue
		}
		rawOffset, err := encoded.rawOffset(offset)
		if err != nil {
			return nil, fmt.Errorf("regexp returned non-boundary submatch index: %w", err)
		}
		indices[i] = rawOffset
	}
	return indices, nil
}

func translateLocaleByteBRE(pattern []byte, codec byteTokenCodec, tables bytePatternTables, foldCase bool) (string, error) {
	var out strings.Builder
	state := posStart
	groups := 0
	for i := 0; i < len(pattern); {
		value := pattern[i]
		switch value {
		case '\\':
			if i+1 >= len(pattern) {
				return "", fmt.Errorf("trailing backslash (\\)")
			}
			next := pattern[i+1]
			switch next {
			case '(':
				out.WriteByte('(')
				groups++
				state = posStart
			case ')':
				if groups == 0 {
					return "", fmt.Errorf("unmatched \\)")
				}
				out.WriteByte(')')
				groups--
				state = posAtom
			case '|':
				out.WriteByte('|')
				state = posStart
			case '+', '?':
				if state != posAtom {
					return "", fmt.Errorf("\\%c with nothing to repeat", next)
				}
				out.WriteByte(next)
				state = posAnchor
			case '{':
				if state != posAtom {
					return "", fmt.Errorf("\\{ with nothing to repeat")
				}
				end := bytesIndex(pattern[i+2:], []byte(`\}`))
				if end < 0 {
					return "", fmt.Errorf("unmatched \\{")
				}
				inner := string(pattern[i+2 : i+2+end])
				normalized, valid := normalizeInterval(inner)
				if !valid {
					return "", fmt.Errorf("invalid interval \\{%s\\}", inner)
				}
				out.WriteByte('{')
				out.WriteString(normalized)
				out.WriteByte('}')
				state = posAnchor
				i += end + 2
			case '}':
				return "", fmt.Errorf("unmatched \\}")
			case 'w', 'W', 's', 'S':
				name := "word"
				if next == 's' || next == 'S' {
					name = "space"
				}
				class := expandFold(tables.classes[name], tables.fold, foldCase)
				if next == 'W' || next == 'S' {
					class = complementByteClass(class)
				}
				out.WriteString(byteClassAtom(codec, class))
				state = posAtom
			case 'b', 'B', '<', '>':
				return "", fmt.Errorf("word-boundary escape \\%c is not supported by the locale byte substrate", next)
			default:
				if next >= '1' && next <= '9' {
					return "", fmt.Errorf("back-reference \\%c is not supported by the locale byte substrate", next)
				}
				if isAlnumByte(next) {
					return "", fmt.Errorf("unsupported escape \\%c", next)
				}
				out.WriteString(literalByteAtom(codec, next, tables.fold, foldCase))
				state = posAtom
			}
			i += 2
		case '[':
			class, negated, consumed, err := parseLocaleByteBracket(pattern[i:], tables)
			if err != nil {
				return "", err
			}
			class = expandFold(class, tables.fold, foldCase)
			if negated {
				class = complementByteClass(class)
			}
			out.WriteString(byteClassAtom(codec, class))
			state = posAtom
			i += consumed
		case '.':
			var class [256]bool
			for b := 0; b < 256; b++ {
				class[b] = tables.dotAll || byte(b) != '\n'
			}
			out.WriteString(byteClassAtom(codec, class))
			state = posAtom
			i++
		case '*':
			if state != posAtom {
				return "", fmt.Errorf("* with nothing to repeat")
			}
			out.WriteByte('*')
			state = posAnchor
			i++
		case '^':
			if state != posStart {
				out.WriteString(literalByteAtom(codec, value, tables.fold, foldCase))
				state = posAtom
			} else {
				out.WriteByte('^')
				state = posAnchor
			}
			i++
		case '$':
			anchor := i == len(pattern)-1 || (i+2 < len(pattern) && pattern[i+1] == '\\' && (pattern[i+2] == ')' || pattern[i+2] == '|'))
			if anchor {
				out.WriteByte('$')
				state = posAnchor
			} else {
				out.WriteString(literalByteAtom(codec, value, tables.fold, foldCase))
				state = posAtom
			}
			i++
		default:
			out.WriteString(literalByteAtom(codec, value, tables.fold, foldCase))
			state = posAtom
			i++
		}
	}
	if groups != 0 {
		return "", fmt.Errorf("unmatched \\(")
	}
	return out.String(), nil
}

func literalByteAtom(codec byteTokenCodec, value byte, fold [256]byte, foldCase bool) string {
	var class [256]bool
	class[value] = true
	return byteClassAtom(codec, expandFold(class, fold, foldCase))
}

func byteClassAtom(codec byteTokenCodec, class [256]bool) string {
	parts := make([]string, 0, 256)
	for i, member := range class {
		if member {
			parts = append(parts, regexp.QuoteMeta(codec.tokens[i]))
		}
	}
	if len(parts) == 0 {
		return `(?:[^\s\S])`
	}
	return "(?:" + strings.Join(parts, "|") + ")"
}

func expandFold(class [256]bool, fold [256]byte, enabled bool) [256]bool {
	if !enabled {
		return class
	}
	original := class
	for candidate := 0; candidate < 256; candidate++ {
		for member, present := range original {
			if present && fold[byte(candidate)] == fold[byte(member)] {
				class[candidate] = true
				break
			}
		}
	}
	return class
}

func complementByteClass(class [256]bool) [256]bool {
	for i := range class {
		class[i] = !class[i]
	}
	return class
}

func parseLocaleByteBracket(pattern []byte, tables bytePatternTables) ([256]bool, bool, int, error) {
	var class [256]bool
	negated := false
	i := 1
	if i < len(pattern) && pattern[i] == '^' {
		negated = true
		i++
	}
	first := true
	for i < len(pattern) {
		if pattern[i] == ']' && !first {
			return class, negated, i + 1, nil
		}
		first = false
		if isBracketSetOpener(pattern, i) {
			set, kind, consumed, err := parseLocaleBracketSet(pattern[i:], tables)
			if err != nil {
				return class, false, 0, err
			}
			i += consumed
			if i+1 < len(pattern) && pattern[i] == '-' && pattern[i+1] != ']' {
				return class, false, 0, fmt.Errorf("%s cannot be a range endpoint", kind)
			}
			for b, member := range set {
				class[b] = class[b] || member
			}
			continue
		}
		start, consumed, err := parseLocaleBracketEndpoint(pattern, i, tables)
		if err != nil {
			return class, false, 0, err
		}
		i += consumed
		if i+1 < len(pattern) && pattern[i] == '-' && pattern[i+1] != ']' {
			i++
			if isBracketSetOpener(pattern, i) {
				_, kind, _, err := parseLocaleBracketSet(pattern[i:], tables)
				if err != nil {
					return class, false, 0, err
				}
				return class, false, 0, fmt.Errorf("%s cannot be a range endpoint", kind)
			}
			end, endConsumed, err := parseLocaleBracketEndpoint(pattern, i, tables)
			if err != nil {
				return class, false, 0, err
			}
			i += endConsumed
			startOrder, endOrder := tables.collseq[start], tables.collseq[end]
			if endOrder < startOrder {
				return class, false, 0, fmt.Errorf("invalid range end")
			}
			for b, order := range tables.collseq {
				if tables.collating[b] && order >= startOrder && order <= endOrder {
					class[b] = true
				}
			}
			continue
		}
		class[start] = true
	}
	return class, false, 0, fmt.Errorf("unclosed bracket expression")
}

func isBracketSetOpener(pattern []byte, i int) bool {
	return pattern[i] == '[' && i+1 < len(pattern) && (pattern[i+1] == ':' || pattern[i+1] == '=')
}

func parseLocaleBracketSet(pattern []byte, tables bytePatternTables) ([256]bool, string, int, error) {
	if pattern[1] == ':' {
		end := bytesIndex(pattern[2:], []byte(":]"))
		if end < 0 {
			return [256]bool{}, "", 0, fmt.Errorf("malformed named character class")
		}
		name := string(pattern[2 : 2+end])
		named, ok := tables.classes[name]
		if !ok {
			return [256]bool{}, "", 0, fmt.Errorf("unsupported named character class %q", name)
		}
		return named, "character class", end + 4, nil
	}
	end := bytesIndex(pattern[2:], []byte("=]"))
	if end < 0 {
		return [256]bool{}, "", 0, fmt.Errorf("malformed collating element")
	}
	content := pattern[2 : 2+end]
	if len(content) != 1 {
		return [256]bool{}, "", 0, fmt.Errorf("multi-byte collating elements are not supported by the locale byte substrate")
	}
	if !tables.collating[content[0]] {
		return [256]bool{}, "", 0, fmt.Errorf("invalid collating element %#02x", content[0])
	}
	if !tables.equivValid[content[0]] {
		return [256]bool{}, "", 0, fmt.Errorf("invalid equivalence class element %#02x", content[0])
	}
	return tables.equivalent[content[0]], "equivalence class", end + 4, nil
}

func parseLocaleBracketEndpoint(pattern []byte, i int, tables bytePatternTables) (byte, int, error) {
	if pattern[i] == '[' && i+1 < len(pattern) && pattern[i+1] == '.' {
		end := bytesIndex(pattern[i+2:], []byte(".]"))
		if end < 0 {
			return 0, 0, fmt.Errorf("malformed collating element")
		}
		content := pattern[i+2 : i+2+end]
		if len(content) != 1 {
			return 0, 0, fmt.Errorf("multi-byte collating elements are not supported by the locale byte substrate")
		}
		if !tables.collating[content[0]] {
			return 0, 0, fmt.Errorf("invalid collating element %#02x", content[0])
		}
		return content[0], end + 4, nil
	}
	if !tables.collating[pattern[i]] {
		return 0, 0, fmt.Errorf("invalid collating element %#02x", pattern[i])
	}
	return pattern[i], 1, nil
}

func bytesIndex(haystack, needle []byte) int {
	return strings.Index(string(haystack), string(needle))
}
