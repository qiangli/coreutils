package localedef

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/qiangli/coreutils/pkg/locale"
)

// compileCollation implements the POSIX character order and weight levels.
func compileCollation(c *locale.Compiled, s *Section, cm *Charmap, warnings *[]Diagnostic) error {
	data := &locale.Collation{}
	names := map[string]string{}
	symbols := map[string]bool{}
	for name, b := range cm.Symbols {
		names[name] = string(b)
	}
	var rows []Entry
	started, ended := false, false
	for _, e := range s.Entries {
		if strings.HasPrefix(e.Keyword, "\x00character:") {
			text := strings.TrimPrefix(e.Keyword, "\x00character:")
			found := false
			for _, b := range cm.Symbols {
				found = found || string(b) == text
			}
			if !found {
				return problem(e.Line, "LC_COLLATE: character %q is outside the charmap", text)
			}
			name := "\x00literal:" + text
			names[name] = text
			e.Keyword = "<" + name + ">"
		}
		fail := func() error { return problem(e.Line, "LC_COLLATE: unsupported or misplaced %s", e.Keyword) }
		switch e.Keyword {
		case "collating-element", "collating-symbol":
			if started || len(e.Values) == 0 {
				return fail()
			}
			name := e.Values[0].Text
			if _, ok := names[name]; ok || symbols[name] {
				return problem(e.Line, "LC_COLLATE: duplicate name <%s>", name)
			}
			if e.Keyword == "collating-symbol" {
				symbols[name] = true
				continue
			}
			value, _, err := resolveValue(e.Values[2], cm)
			if err != nil {
				return atLine(e.Line, err)
			}
			if !utf8.ValidString(value) || utf8.RuneCountInString(value) < 2 {
				return problem(e.Line, "LC_COLLATE: element must contain at least two UTF-8 characters")
			}
			names[name] = value
		case "order_start":
			if started {
				return fail()
			}
			started = true
			groups := collationWeights(e.Values)
			if len(groups) == 0 {
				data.Backward = append(data.Backward, false)
				data.Position = append(data.Position, false)
			}
			for _, g := range groups {
				backward, position, err := collationDirectives(g)
				if err != nil {
					return atLine(e.Line, err)
				}
				data.Backward = append(data.Backward, backward)
				data.Position = append(data.Position, position)
			}
		case "order_end":
			if !started || ended {
				return fail()
			}
			ended = true
		default:
			if !started || ended || (!strings.HasPrefix(e.Keyword, "<") && e.Keyword != "UNDEFINED" && e.Keyword != "...") {
				return fail()
			}
			rows = append(rows, e)
		}
	}
	if !started || !ended {
		return fmt.Errorf("LC_COLLATE: order_start and order_end required")
	}
	var err error
	rows, err = expandCollationEllipses(rows, names, cm)
	if err != nil {
		return err
	}
	// Expand UNDEFINED from the charmap, in encoding order, at its declared
	// position. Aliases of a character do not create duplicate elements.
	explicit := map[string]bool{}
	for _, e := range rows {
		if strings.HasPrefix(e.Keyword, "<") {
			explicit[names[strings.TrimSuffix(e.Keyword[1:], ">")]] = true
		}
	}
	missingSet := map[string]bool{}
	for _, b := range cm.Symbols {
		if !explicit[string(b)] {
			missingSet[string(b)] = true
		}
	}
	var missing []string
	for text := range missingSet {
		missing = append(missing, text)
	}
	sort.Strings(missing)
	undefinedPresent := false
	for _, e := range rows {
		undefinedPresent = undefinedPresent || e.Keyword == "UNDEFINED"
	}
	if !undefinedPresent && len(missing) > 0 {
		if warnings != nil {
			*warnings = append(*warnings, Diagnostic{Line: s.Line, Warning: true, Message: fmt.Sprintf("LC_COLLATE: %d omitted coded characters appended to the order", len(missing))})
		}
		rows = append(rows, Entry{Keyword: "UNDEFINED", Line: s.Line})
	}
	var expanded []Entry
	undefinedSeen := false
	for _, e := range rows {
		if e.Keyword != "UNDEFINED" {
			expanded = append(expanded, e)
			continue
		}
		if undefinedSeen {
			return problem(e.Line, "LC_COLLATE: duplicate UNDEFINED")
		}
		undefinedSeen = true
		firstName := ""
		for i, text := range missing {
			if !utf8.ValidString(text) || utf8.RuneCountInString(text) != 1 {
				return problem(e.Line, "LC_COLLATE: UNDEFINED requires UTF-8 character encodings")
			}
			name := fmt.Sprintf("\x00undefined%d", i)
			names[name] = text
			if firstName == "" {
				firstName = name
			}
			values := append([]Value(nil), e.Values...)
			// Unspecified primary weights share a rank for multi-level
			// UNDEFINED orders. Subsequent defaults retain character order.
			groups := collationWeights(values)
			values = nil
			for level := range data.Backward {
				if level > 0 {
					values = append(values, Value{Kind: Separator, Text: ";"})
				}
				if level < len(groups) && len(groups[level]) > 0 {
					values = append(values, groups[level]...)
				} else {
					weightName := name
					if level == 0 && len(data.Backward) > 1 {
						weightName = firstName
					}
					values = append(values, Value{Kind: Symbol, Text: weightName})
				}
			}
			if len(groups) > len(data.Backward) {
				return problem(e.Line, "LC_COLLATE: too many weights")
			}
			for j, v := range values {
				if v.Kind == Ellipsis {
					values[j] = Value{Kind: Symbol, Text: name}
				}
			}
			expanded = append(expanded, Entry{Keyword: "<" + name + ">", Values: values, Line: e.Line})
		}
	}
	rows = expanded
	rank := map[string]int{}
	for i, e := range rows {
		name := strings.TrimSuffix(e.Keyword[1:], ">")
		if rank[name] != 0 {
			return problem(e.Line, "LC_COLLATE: duplicate order entry %s", e.Keyword)
		}
		if _, ok := names[name]; !ok && !symbols[name] {
			return problem(e.Line, "LC_COLLATE: undefined order entry %s", e.Keyword)
		}
		rank[name] = i + 1
	}
	// Weight references may use any charmap alias of a ranged character.
	textRank := map[string]int{}
	for name, r := range rank {
		if text, ok := names[name]; ok {
			textRank[text] = r
		}
	}
	for name, text := range names {
		if rank[name] == 0 {
			rank[name] = textRank[text]
		}
	}
	// Literal weights denote encoded text, not symbolic-name spelling. Match
	// declared collating elements longest first, as for runtime collation.
	var orderedText []string
	for text := range textRank {
		orderedText = append(orderedText, text)
	}
	sort.Slice(orderedText, func(i, j int) bool {
		if len(orderedText[i]) != len(orderedText[j]) {
			return len(orderedText[i]) > len(orderedText[j])
		}
		return orderedText[i] < orderedText[j]
	})
	literalWeights := func(text string) ([]int, error) {
		var weights []int
		for text != "" {
			matched := false
			for _, element := range orderedText {
				if strings.HasPrefix(text, element) {
					weights = append(weights, textRank[element])
					text = text[len(element):]
					matched = true
					break
				}
			}
			if !matched {
				return nil, fmt.Errorf("LC_COLLATE: weight character has no order at %q", text)
			}
		}
		return weights, nil
	}
	seen := map[string]bool{}
	for _, e := range rows {
		name := strings.TrimSuffix(e.Keyword[1:], ">")
		if symbols[name] {
			if len(e.Values) > 0 {
				return problem(e.Line, "LC_COLLATE: symbol entry cannot carry weights")
			}
			continue
		}
		text := names[name]
		if text == "" || !utf8.ValidString(text) || seen[text] {
			return problem(e.Line, "LC_COLLATE: invalid or duplicate element encoding")
		}
		seen[text] = true
		el := locale.CollatingElement{Text: text, Order: rank[name]}
		groups := collationWeights(e.Values)
		if len(groups) > len(data.Backward) {
			return problem(e.Line, "LC_COLLATE: too many weights")
		}
		for level := 0; level < len(data.Backward); level++ {
			weights := []int{rank[name]}
			if level < len(groups) && len(groups[level]) > 0 {
				g := groups[level]
				if len(g) != 1 {
					return problem(e.Line, "LC_COLLATE: unsupported weight list")
				}
				v := g[0]
				switch {
				case v.Kind == Word && v.Text == "IGNORE":
					weights = nil
				case v.Kind == Symbol:
					if rank[v.Text] == 0 {
						return problem(e.Line, "LC_COLLATE: weight <%s> has no order", v.Text)
					}
					weights = []int{rank[v.Text]}
				case v.Kind == Word || v.Kind == Number || v.Kind == Bytes:
					var err error
					weights, err = literalWeights(v.Text)
					if err != nil {
						return atLine(e.Line, err)
					}
				case v.Kind == String:
					weights = nil
					for _, p := range v.Parts {
						if p.Kind == Symbol {
							if rank[p.Text] == 0 {
								return problem(e.Line, "LC_COLLATE: weight <%s> has no order", p.Text)
							}
							weights = append(weights, rank[p.Text])
						} else {
							part, err := literalWeights(p.Text)
							if err != nil {
								return atLine(e.Line, err)
							}
							weights = append(weights, part...)
						}
					}
				default:
					return problem(e.Line, "LC_COLLATE: unsupported weight %q", v.Text)
				}
			}
			el.Weights = append(el.Weights, weights)
		}
		data.Elements = append(data.Elements, el)
	}
	if len(data.Elements) == 0 {
		return fmt.Errorf("LC_COLLATE: empty order is unsupported")
	}
	c.Collation = data
	c.EnsureCategory("LC_COLLATE")
	return nil
}

// Empty operands are self weights, including leading and trailing operands.
func collationWeights(values []Value) [][]Value {
	if len(values) == 0 {
		return nil
	}
	groups := [][]Value{nil}
	for _, v := range values {
		if v.Kind == Separator && v.Text == ";" {
			groups = append(groups, nil)
		} else {
			groups[len(groups)-1] = append(groups[len(groups)-1], v)
		}
	}
	return groups
}

func collationDirectives(values []Value) (backward, position bool, err error) {
	seen := map[string]bool{}
	for i, v := range values {
		if i%2 == 1 {
			if v.Kind != Separator || v.Text != "," {
				return false, false, fmt.Errorf("LC_COLLATE: expected comma between directives")
			}
			continue
		}
		if v.Kind != Word || seen[v.Text] || (v.Text != "forward" && v.Text != "backward" && v.Text != "position") {
			return false, false, fmt.Errorf("LC_COLLATE: invalid directive %q", v.Text)
		}
		seen[v.Text] = true
	}
	if len(values)%2 == 0 || (seen["forward"] && seen["backward"]) {
		return false, false, fmt.Errorf("LC_COLLATE: invalid ordering directives")
	}
	return seen["backward"], seen["position"], nil
}

// Expand only actual characters in the supplied repertoire; symbol spelling
// does not define encoded order. Synthetic names let aliases share the range.
func expandCollationEllipses(rows []Entry, names map[string]string, cm *Charmap) ([]Entry, error) {
	chars := map[string]bool{}
	for _, b := range cm.Symbols {
		chars[string(b)] = true
	}
	var ordered []string
	for ch := range chars {
		ordered = append(ordered, ch)
	}
	sort.Strings(ordered)
	endpoint := func(e Entry) (string, error) {
		name := strings.TrimSuffix(strings.TrimPrefix(e.Keyword, "<"), ">")
		ch, ok := names[name]
		if !ok || !chars[ch] || utf8.RuneCountInString(ch) != 1 {
			return "", problem(e.Line, "LC_COLLATE: ellipsis endpoint must be a coded character")
		}
		return ch, nil
	}
	var out []Entry
	for i, e := range rows {
		if e.Keyword != "..." {
			out = append(out, e)
			continue
		}
		low, high := "\x00", ""
		if len(ordered) > 0 {
			high = ordered[len(ordered)-1]
		}
		var err error
		if i > 0 {
			low, err = endpoint(rows[i-1])
			if err != nil {
				return nil, err
			}
		}
		if i+1 < len(rows) {
			high, err = endpoint(rows[i+1])
			if err != nil {
				return nil, err
			}
		}
		if low >= high {
			return nil, problem(e.Line, "LC_COLLATE: ellipsis endpoints must increase")
		}
		for j, ch := range ordered {
			if ch <= low || ch >= high {
				continue
			}
			name := fmt.Sprintf("\x00ellipsis%d_%d", i, j)
			names[name] = ch
			values := append([]Value(nil), e.Values...)
			for k, v := range values {
				if v.Kind == Ellipsis {
					values[k] = Value{Kind: Symbol, Text: name}
				}
			}
			out = append(out, Entry{Keyword: "<" + name + ">", Values: values, Line: e.Line})
		}
	}
	return out, nil
}
