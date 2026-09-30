package bre

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/qiangli/coreutils/pkg/locale"
)

// elementCollation is immutable after snapshotting. Order is the declared
// range order; primary weights, independently, determine equivalence.
type elementCollation struct {
	elements []locale.CollatingElement
	byText   map[string]int
}

func snapshotBracketElements(input []locale.CollatingElement) (*elementCollation, error) {
	c := &elementCollation{byText: make(map[string]int)}
	if len(input) == 0 {
		return nil, fmt.Errorf("empty compiled bracket collation")
	}
	for _, e := range input {
		if e.Text == "" || len(e.Weights) == 0 {
			return nil, fmt.Errorf("invalid compiled bracket element %q", e.Text)
		}
		if _, exists := c.byText[e.Text]; exists {
			return nil, fmt.Errorf("duplicate compiled bracket element %q", e.Text)
		}
		c.byText[e.Text] = len(c.elements)
		e.Weights = [][]int{append([]int(nil), e.Weights[0]...)}
		c.elements = append(c.elements, e)
	}
	return c, nil
}

type elementBracket struct {
	texts   []string
	classes []string
	negated bool
}

// parseElementBracket retains sequences as sequences. In particular, adjacent
// ordinary c and h in [ch] remain separate members, even if ch is declared.
func parseElementBracket(pattern string, c *elementCollation, unicode bool) (elementBracket, int, error) {
	var out elementBracket
	i := 1
	if i < len(pattern) && pattern[i] == '^' {
		out.negated = true
		i++
	}
	type member struct {
		text, class string
		set         bool
		texts       []string
	}
	read := func() (member, error) {
		if i >= len(pattern) {
			return member{}, fmt.Errorf("unclosed bracket expression")
		}
		if pattern[i] == '[' && i+1 < len(pattern) && strings.ContainsRune(".:=", rune(pattern[i+1])) {
			delim := pattern[i+1]
			end := strings.Index(pattern[i+2:], string(delim)+"]")
			if end < 0 {
				return member{}, fmt.Errorf("malformed bracket member")
			}
			text := pattern[i+2 : i+2+end]
			i += end + 4
			if delim == ':' {
				return member{class: text, set: true}, nil
			}
			index, ok := c.byText[text]
			if !ok {
				return member{}, fmt.Errorf("invalid collating element %q", text)
			}
			if delim == '.' {
				return member{text: text}, nil
			}
			m := member{set: true}
			for _, e := range c.elements {
				if slices.Equal(e.Weights[0], c.elements[index].Weights[0]) {
					m.texts = append(m.texts, e.Text)
				}
			}
			return m, nil
		}
		size := 1
		if unicode {
			_, size = utf8.DecodeRuneInString(pattern[i:])
		}
		text := pattern[i : i+size]
		i += size
		return member{text: text}, nil
	}
	first := true
	for i < len(pattern) {
		if pattern[i] == ']' && !first {
			return out, i + 1, nil
		}
		first = false
		m, err := read()
		if err != nil {
			return out, 0, err
		}
		if i+1 < len(pattern) && pattern[i] == '-' && pattern[i+1] != ']' {
			i++
			n, err := read()
			if err != nil {
				return out, 0, err
			}
			if m.set || n.set {
				return out, 0, fmt.Errorf("class cannot be a range endpoint")
			}
			a, aok := c.byText[m.text]
			b, bok := c.byText[n.text]
			if !aok || !bok {
				return out, 0, fmt.Errorf("undefined collating range endpoint")
			}
			lo, hi := c.elements[a].Order, c.elements[b].Order
			if lo > hi {
				return out, 0, fmt.Errorf("invalid range end")
			}
			for _, e := range c.elements {
				if e.Order >= lo && e.Order <= hi {
					out.texts = append(out.texts, e.Text)
				}
			}
		} else if m.set {
			if m.class != "" {
				out.classes = append(out.classes, m.class)
			} else if len(m.texts) != 0 {
				out.texts = append(out.texts, m.texts...)
			} else {
				return out, 0, fmt.Errorf("empty character class")
			}
		} else {
			out.texts = append(out.texts, m.text)
		}
	}
	return out, 0, fmt.Errorf("unclosed bracket expression")
}

func localeBracketAtom(pattern []byte, codec byteTokenCodec, tables bytePatternTables, fold bool) (string, int, error) {
	if tables.elements == nil {
		class, negated, n, err := parseLocaleByteBracket(pattern, tables)
		if err != nil {
			return "", 0, err
		}
		class = expandFold(class, tables.fold, fold)
		if negated {
			class = complementByteClass(class)
		}
		return byteClassAtom(codec, class), n, nil
	}
	b, n, err := parseElementBracket(string(pattern), tables.elements, false)
	if err != nil {
		return "", 0, err
	}
	var singles [256]bool
	var alternatives []string
	for _, text := range b.texts {
		if len(text) == 1 {
			singles[text[0]] = true
			continue
		}
		if b.negated {
			continue
		} // POSIX permits negated lists to consume only single characters.
		var atom strings.Builder
		for i := range len(text) {
			atom.WriteString(literalByteAtom(codec, text[i], tables.fold, fold))
		}
		alternatives = append(alternatives, atom.String())
	}
	for _, name := range b.classes {
		class, ok := tables.classes[name]
		if !ok {
			return "", 0, fmt.Errorf("unsupported named character class %q", name)
		}
		for i, yes := range class {
			singles[i] = singles[i] || yes
		}
	}
	singles = expandFold(singles, tables.fold, fold)
	if b.negated {
		singles = complementByteClass(singles)
	}
	alternatives = append(alternatives, byteClassAtom(codec, singles))
	return "(?:" + strings.Join(alternatives, "|") + ")", n, nil
}

// Unicode consumers use the same element selection and their own character
// classes. A noncapturing atom keeps original capture numbers and quantifiers.
func unicodeElementBracket(pattern string, tables *LocaleByteTables) (string, int, error) {
	b, n, err := parseElementBracket(pattern, tables.tables.elements, true)
	if err != nil {
		return "", 0, err
	}
	var singles strings.Builder
	var alternatives []string
	for _, text := range b.texts {
		if utf8.RuneCountInString(text) == 1 {
			r, _ := utf8.DecodeRuneInString(text)
			singles.WriteString(escapeClassRune(r))
		} else if !b.negated {
			alternatives = append(alternatives, regexp.QuoteMeta(text))
		}
	}
	for _, name := range b.classes {
		content, ok := locale.CUTF8RE2ClassContent(name)
		if !ok {
			return "", 0, fmt.Errorf("unsupported named character class %q", name)
		}
		singles.WriteString(content)
	}
	if b.negated {
		alternatives = append(alternatives, "[^"+singles.String()+"]")
	} else if singles.Len() > 0 {
		alternatives = append(alternatives, "["+singles.String()+"]")
	}
	// [^] is not RE2 syntax: an empty exclusion set means any character.
	if b.negated && singles.Len() == 0 {
		alternatives = []string{"(?s:.)"}
	}
	if len(alternatives) == 0 {
		return "[^\\s\\S]", n, nil
	}
	return "(?:" + strings.Join(alternatives, "|") + ")", n, nil
}
