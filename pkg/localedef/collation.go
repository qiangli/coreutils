package localedef

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/qiangli/coreutils/pkg/locale"
)

// compileCollation implements explicit POSIX orders. Ellipsis and position
// are refused until their semantics can be represented without approximation.
func compileCollation(c *locale.Compiled, s *Section, cm *Charmap) error {
	data := &locale.Collation{}
	names := map[string]string{}
	symbols := map[string]bool{}
	for name, b := range cm.Symbols {
		names[name] = string(b)
	}
	var rows []Entry
	started, ended := false, false
	for _, e := range s.Entries {
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
			groups, err := splitGroups(e.Values)
			if err != nil {
				return err
			}
			if len(groups) == 0 {
				data.Backward = append(data.Backward, false)
			}
			for _, g := range groups {
				if len(g) != 1 || (g[0].Text != "forward" && g[0].Text != "backward") {
					return fail()
				}
				data.Backward = append(data.Backward, g[0].Text == "backward")
			}
		case "order_end":
			if !started || ended {
				return fail()
			}
			ended = true
		default:
			if !started || ended || (!strings.HasPrefix(e.Keyword, "<") && e.Keyword != "UNDEFINED") {
				return fail()
			}
			rows = append(rows, e)
		}
	}
	if !started || !ended {
		return fmt.Errorf("LC_COLLATE: order_start and order_end required")
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
			if len(values) == 0 {
				values = []Value{{Kind: Symbol, Text: firstName}}
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
		groups, err := splitGroups(e.Values)
		if err != nil {
			return err
		}
		if len(groups) > len(data.Backward) {
			return problem(e.Line, "LC_COLLATE: too many weights")
		}
		for level := 0; level < len(data.Backward); level++ {
			weights := []int{rank[name]}
			if level < len(groups) {
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
				case v.Kind == String:
					weights = nil
					for _, p := range v.Parts {
						if p.Kind != Symbol || rank[p.Text] == 0 {
							return problem(e.Line, "LC_COLLATE: weight strings require ordered symbolic names")
						}
						weights = append(weights, rank[p.Text])
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
