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
	classes map[string][256]bool
	fold    [256]byte
}

type localeBytePattern struct {
	codec byteTokenCodec
	re    *regexp.Regexp
}

// compileLocaleBytePattern compiles the consuming subset of POSIX BRE over
// locale-classified bytes. It is an unrouted substrate: public Regexp and all
// command consumers continue to use their existing engines.
func compileLocaleBytePattern(pattern []byte, input bytePatternTables, foldCase bool) (*localeBytePattern, error) {
	tables := bytePatternTables{classes: make(map[string][256]bool, len(input.classes)), fold: input.fold}
	for name, class := range input.classes {
		tables.classes[name] = class
	}
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
	re, err := regexp.Compile(translated)
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
				class[b] = byte(b) != '\n'
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
		if pattern[i] == '[' && i+1 < len(pattern) {
			switch pattern[i+1] {
			case ':':
				end := bytesIndex(pattern[i+2:], []byte(":]"))
				if end < 0 {
					return class, false, 0, fmt.Errorf("malformed named character class")
				}
				name := string(pattern[i+2 : i+2+end])
				named, ok := tables.classes[name]
				if !ok {
					return class, false, 0, fmt.Errorf("unsupported named character class %q", name)
				}
				for b, member := range named {
					class[b] = class[b] || member
				}
				i += end + 4
				first = false
				continue
			case '.', '=':
				return class, false, 0, fmt.Errorf("collating and equivalence elements are not supported by the locale byte substrate")
			}
		}
		if pattern[i] == '-' && !first && !(i+1 < len(pattern) && pattern[i+1] == ']') {
			return class, false, 0, fmt.Errorf("ranges are not supported by the locale byte substrate")
		}
		class[pattern[i]] = true
		i++
		first = false
	}
	return class, false, 0, fmt.Errorf("unclosed bracket expression")
}

func bytesIndex(haystack, needle []byte) int {
	return strings.Index(string(haystack), string(needle))
}
