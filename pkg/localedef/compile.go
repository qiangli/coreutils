package localedef

// Compilation turns parsed POSIX locale sources into the Go-owned store.
// Unsupported collation semantics are rejected rather than discarded.

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/qiangli/coreutils/pkg/locale"
)

// ErrUndefinedSymbol reports a symbolic name the charmap does not define.
// POSIX 7.3 makes an undefined name a warning inside LC_CTYPE and LC_COLLATE
// and an error elsewhere, so compilation has to be able to tell the two apart
// rather than failing the whole definition on a class member the charmap
// simply does not carry.
var ErrUndefinedSymbol = errors.New("undefined symbol")

// CopyResolver supplies an already-compiled locale for a "copy" directive.
// The store is the only source: a copy of a definition nobody compiled cannot
// be honoured, and guessing at it would publish data the operator never wrote.
type CopyResolver func(name string) (*locale.Compiled, bool)

// ctypeClasses is the POSIX.1-2017 XBD 7.3.1 class vocabulary. charclass adds
// implementation-defined names to it, which the parser already tracked.
var ctypeClasses = strings.Fields("upper lower alpha digit alnum space cntrl punct graph print xdigit blank")

// Compile compiles src under name using cm to resolve symbolic names.
// An unresolved copy or an undefined symbolic name is an error: a compiled
// locale that silently dropped either would answer queries with data the
// definition never contained.
func Compile(name string, src *Source, cm *Charmap) (*locale.Compiled, error) {
	return CompileWithCopy(name, src, cm, nil)
}

// CompileWithCopy compiles src, resolving "copy" directives through resolve.
func CompileWithCopy(name string, src *Source, cm *Charmap, resolve CopyResolver) (*locale.Compiled, error) {
	if src == nil {
		return nil, fmt.Errorf("no locale source")
	}
	if cm == nil {
		cm = DefaultCharmap()
	}
	if !locale.ValidLocaleName(name) {
		return nil, fmt.Errorf("invalid locale name %q", name)
	}
	c := &locale.Compiled{Name: name, Charmap: cm.CodeSet, MbCurMin: cm.MinBytes, MbCurMax: cm.MaxBytes}
	for _, cat := range src.Order {
		s := src.Sections[cat]
		if s.Copy != "" {
			if err := copyCategory(c, cat, s, resolve); err != nil {
				return nil, err
			}
			continue
		}
		switch cat {
		case "LC_COLLATE":
			if err := compileCollation(c, s, cm, nil); err != nil {
				return nil, err
			}
		case "LC_CTYPE":
			if err := compileCtype(c, s, cm); err != nil {
				return nil, err
			}
		case "LC_NUMERIC", "LC_MONETARY", "LC_TIME", "LC_MESSAGES":
			if err := compileKeywords(c, cat, s, cm); err != nil {
				return nil, err
			}
		}
		// System-source extension categories are parsed and validated,
		// but carry nothing this store can serve; they are skipped
		// rather than stored empty, so Has() keeps meaning "we have data".
	}
	return c, nil
}

// copyCategory takes cat verbatim from the compiled locale named by copy.
func copyCategory(c *locale.Compiled, cat string, s *Section, resolve CopyResolver) error {
	if resolve == nil {
		return problem(s.Line, "%s: copy %q cannot be resolved: no compiled locale store is available", cat, s.Copy)
	}
	other, ok := resolve(s.Copy)
	if !ok {
		return problem(s.Line, "%s: copy %q: locale %q has not been compiled", cat, s.Copy, s.Copy)
	}
	if !other.Has(cat) {
		return problem(s.Line, "%s: copy %q: locale %q carries no %s data", cat, s.Copy, s.Copy, cat)
	}
	c.EnsureCategory(cat)
	for name, k := range other.Categories[cat] {
		c.Set(cat, name, k)
	}
	if cat == "LC_COLLATE" {
		c.Collation = other.Collation
	}
	if cat == "LC_CTYPE" {
		for name, chars := range other.Classes {
			if c.Classes == nil {
				c.Classes = map[string]string{}
			}
			c.Classes[name] = chars
		}
		c.ToUpperFrom, c.ToUpperTo = other.ToUpperFrom, other.ToUpperTo
		c.ToLowerFrom, c.ToLowerTo = other.ToLowerFrom, other.ToLowerTo
	}
	return nil
}

func compileKeywords(c *locale.Compiled, cat string, s *Section, cm *Charmap) error {
	c.EnsureCategory(cat)
	for _, e := range s.Entries {
		groups, err := splitGroups(e.Values)
		if err != nil {
			return atLine(e.Line, err)
		}
		values := make([]string, 0, len(groups))
		numeric := len(groups) > 0
		for _, g := range groups {
			if len(g) != 1 {
				return problem(e.Line, "%s: %s takes plain values, not a pair", cat, e.Keyword)
			}
			v, isNum, err := resolveValue(g[0], cm)
			if err != nil {
				return atLine(e.Line, err)
			}
			if !utf8.ValidString(v) {
				return problem(e.Line, "%s: %s resolves to bytes that are not valid UTF-8; the Go locale store carries UTF-8 and single-byte ASCII-compatible codesets only", cat, e.Keyword)
			}
			values = append(values, v)
			numeric = numeric && isNum
		}
		c.Set(cat, e.Keyword, locale.Keyword{Values: values, Numeric: numeric})
	}
	return nil
}

func compileCtype(c *locale.Compiled, s *Section, cm *Charmap) error {
	c.EnsureCategory("LC_CTYPE")
	declared := map[string]bool{}
	for _, name := range ctypeClasses {
		declared[name] = true
	}
	for _, e := range s.Entries {
		if e.Keyword != "charclass" {
			continue
		}
		for _, v := range e.Values {
			if v.Kind == String && v.Text != "" {
				declared[v.Text] = true
			}
		}
	}
	for _, e := range s.Entries {
		switch {
		case e.Keyword == "charclass":
			continue
		case e.Keyword == "toupper" || e.Keyword == "tolower":
			from, to, err := compileCaseMap(e, cm)
			if err != nil {
				return err
			}
			if e.Keyword == "toupper" {
				c.ToUpperFrom, c.ToUpperTo = from, to
			} else {
				c.ToLowerFrom, c.ToLowerTo = from, to
			}
		case declared[e.Keyword]:
			chars, err := compileClass(e, cm)
			if err != nil {
				return err
			}
			if c.Classes == nil {
				c.Classes = map[string]string{}
			}
			c.Classes[e.Keyword] += chars
		}
	}
	return nil
}

// compileClass resolves one character-class list. An ellipsis between two
// members expands over the intervening single-byte encodings, which is the
// only range POSIX defines for a single-byte charmap.
func compileClass(e Entry, cm *Charmap) (string, error) {
	groups, err := splitGroups(e.Values)
	if err != nil {
		return "", atLine(e.Line, err)
	}
	var b strings.Builder
	pending := false
	for _, g := range groups {
		if len(g) == 1 && g[0].Kind == Ellipsis {
			if b.Len() == 0 {
				return "", problem(e.Line, "%s: range has no start", e.Keyword)
			}
			pending = true
			continue
		}
		if len(g) != 1 {
			return "", problem(e.Line, "%s: expected a character, not a pair", e.Keyword)
		}
		v, _, err := resolveValue(g[0], cm)
		if errors.Is(err, ErrUndefinedSymbol) {
			// Validate already reported this as a warning for LC_CTYPE; drop
			// the member rather than discard the whole class.
			continue
		}
		if err != nil {
			return "", atLine(e.Line, err)
		}
		if pending {
			prev := b.String()
			if len(v) != 1 || len(prev) == 0 || prev[len(prev)-1] >= 0x80 || v[0] >= 0x80 {
				return "", problem(e.Line, "%s: symbolic range is defined for single-byte ASCII encodings only", e.Keyword)
			}
			for ch := prev[len(prev)-1] + 1; ch < v[0]; ch++ {
				b.WriteByte(ch)
			}
			pending = false
		}
		if !utf8.ValidString(v) {
			return "", problem(e.Line, "%s: class member is not valid UTF-8", e.Keyword)
		}
		b.WriteString(v)
	}
	if pending {
		return "", problem(e.Line, "%s: range has no end", e.Keyword)
	}
	return b.String(), nil
}

// compileCaseMap resolves a toupper/tolower list of (from,to) pairs into two
// parallel strings, which is how the store carries a mapping without needing
// a second JSON shape.
func compileCaseMap(e Entry, cm *Charmap) (string, string, error) {
	groups, err := splitGroups(e.Values)
	if err != nil {
		return "", "", atLine(e.Line, err)
	}
	var from, to strings.Builder
	for _, g := range groups {
		if len(g) != 5 || g[0].Kind != Separator || g[0].Text != "(" || g[2].Text != "," || g[4].Text != ")" {
			return "", "", problem(e.Line, "%s: expected (from,to) pairs", e.Keyword)
		}
		a, _, errA := resolveValue(g[1], cm)
		b, _, errB := resolveValue(g[3], cm)
		if errors.Is(errA, ErrUndefinedSymbol) || errors.Is(errB, ErrUndefinedSymbol) {
			continue
		}
		if errA != nil {
			return "", "", atLine(e.Line, errA)
		}
		if errB != nil {
			return "", "", atLine(e.Line, errB)
		}
		if !utf8.ValidString(a) || !utf8.ValidString(b) {
			return "", "", problem(e.Line, "%s: mapping is not valid UTF-8", e.Keyword)
		}
		from.WriteString(a)
		to.WriteString(b)
	}
	return from.String(), to.String(), nil
}

// splitGroups splits a value list on top-level semicolons, keeping any
// parenthesised pair intact as one group.
func splitGroups(vs []Value) ([][]Value, error) {
	var out [][]Value
	var cur []Value
	depth := 0
	for _, v := range vs {
		if v.Kind == Separator {
			switch v.Text {
			case "(":
				depth++
			case ")":
				depth--
			case ";":
				if depth == 0 {
					if len(cur) == 0 {
						return nil, problem(0, "empty list element")
					}
					out = append(out, cur)
					cur = nil
					continue
				}
			}
		}
		cur = append(cur, v)
	}
	if len(cur) > 0 {
		out = append(out, cur)
	}
	return out, nil
}

// resolveValue renders one parsed value as the bytes the locale carries. The
// bool reports a numeric value, which locale(1) writes unquoted.
func resolveValue(v Value, cm *Charmap) (string, bool, error) {
	switch v.Kind {
	case Number:
		return v.Text, true, nil
	case Word, Bytes:
		return v.Text, false, nil
	case Symbol:
		b, ok := cm.Symbols[v.Text]
		if !ok {
			return "", false, fmt.Errorf("%w <%s>", ErrUndefinedSymbol, v.Text)
		}
		return string(b), false, nil
	case String:
		var b strings.Builder
		for _, p := range v.Parts {
			s, _, err := resolveValue(p, cm)
			if err != nil {
				return "", false, err
			}
			b.WriteString(s)
		}
		return b.String(), false, nil
	}
	return "", false, problem(0, "unexpected value %q", v.Text)
}

// CheckTarget rejects literal or copied data outside an explicit target's
// repertoire. Symbol mapping alone cannot check these paths through a source.
func CheckTarget(c *locale.Compiled, target string) error {
	if target == "" {
		return nil
	}
	check := func(s string) error {
		if !utf8.ValidString(s) {
			return fmt.Errorf("%w: invalid UTF-8 data", ErrCodeset)
		}
		if target == "ASCII" {
			for _, ch := range s {
				if ch > 127 {
					return fmt.Errorf("%w: U%04X is outside ASCII", ErrCodeset, ch)
				}
			}
		}
		return nil
	}
	for _, keywords := range c.Categories {
		for _, keyword := range keywords {
			for _, value := range keyword.Values {
				if err := check(value); err != nil {
					return err
				}
			}
		}
	}
	for _, chars := range c.Classes {
		if err := check(chars); err != nil {
			return err
		}
	}
	for _, chars := range []string{c.ToUpperFrom, c.ToUpperTo, c.ToLowerFrom, c.ToLowerTo} {
		if err := check(chars); err != nil {
			return err
		}
	}
	if c.Collation != nil {
		for _, e := range c.Collation.Elements {
			if err := check(e.Text); err != nil {
				return err
			}
		}
	}
	return nil
}
