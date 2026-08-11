package bre

import (
	"fmt"
	"regexp"
	"strings"
)

// compileLocaleByteERE compiles the consuming subset of POSIX ERE over
// locale-classified bytes. Like the BRE variant, it is unexported and unrouted.
func compileLocaleByteERE(pattern []byte, input bytePatternTables, foldCase bool) (*localeBytePattern, error) {
	tables := bytePatternTables{classes: make(map[string][256]bool, len(input.classes)), fold: input.fold, dotAll: input.dotAll, multi: input.multi}
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
	translated, err := translateLocaleByteERE(pattern, codec, tables, foldCase)
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

func translateLocaleByteERE(pattern []byte, codec byteTokenCodec, tables bytePatternTables, foldCase bool) (string, error) {
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
				if !strings.ContainsRune(`.[]\\*^$()+?{}|`, rune(next)) {
					return "", fmt.Errorf("unsupported ERE escape \\%c", next)
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
		case '(':
			out.WriteByte('(')
			groups++
			state = posStart
			i++
		case ')':
			if groups == 0 {
				return "", fmt.Errorf("unmatched )")
			}
			if state == posStart {
				return "", fmt.Errorf("empty group or alternative")
			}
			out.WriteByte(')')
			groups--
			state = posAtom
			i++
		case '|':
			if state == posStart {
				return "", fmt.Errorf("empty alternative")
			}
			out.WriteByte('|')
			state = posStart
			i++
		case '*', '+', '?':
			if state != posAtom {
				return "", fmt.Errorf("%c with nothing to repeat", value)
			}
			out.WriteByte(value)
			state = posAnchor
			i++
		case '{':
			if state != posAtom {
				return "", fmt.Errorf("{ with nothing to repeat")
			}
			end := bytesIndex(pattern[i+1:], []byte("}"))
			if end < 0 {
				return "", fmt.Errorf("unmatched {")
			}
			inner := string(pattern[i+1 : i+1+end])
			normalized, valid := normalizeInterval(inner)
			if !valid {
				return "", fmt.Errorf("invalid interval {%s}", inner)
			}
			out.WriteByte('{')
			out.WriteString(normalized)
			out.WriteByte('}')
			state = posAnchor
			i += end + 2
		case '}':
			return "", fmt.Errorf("unmatched }")
		case '^':
			if state == posStart {
				out.WriteByte('^')
				state = posAnchor
			} else {
				out.WriteString(literalByteAtom(codec, value, tables.fold, foldCase))
				state = posAtom
			}
			i++
		case '$':
			anchor := i == len(pattern)-1 || (i+1 < len(pattern) && (pattern[i+1] == ')' || pattern[i+1] == '|'))
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
		return "", fmt.Errorf("unmatched (")
	}
	if state == posStart && len(pattern) != 0 {
		return "", fmt.Errorf("empty trailing alternative")
	}
	return out.String(), nil
}
