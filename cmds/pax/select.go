package paxcmd

import (
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/qiangli/coreutils/pkg/bre"
)

// substitution is one -s ed-style rewrite: /old/new/[gp]. POSIX defines the
// pattern as a basic regular expression, so it is compiled with the shared
// pkg/bre provider rather than Go's regexp (whose \(...\) are literal parens).
type substitution struct {
	re     substitutionMatcher
	repl   string
	global bool
	print  bool
}

type substitutionMatcher interface {
	FindAllStringSubmatchIndex(string, int) ([][]int, error)
	ExpandString([]byte, string, string, []int) ([]byte, error)
}

type cSubstitutionMatcher struct{ re *bre.Regexp }

func (m cSubstitutionMatcher) FindAllStringSubmatchIndex(s string, n int) ([][]int, error) {
	return m.re.FindAllStringSubmatchIndexErr(s, n)
}
func (m cSubstitutionMatcher) ExpandString(dst []byte, template, src string, match []int) ([]byte, error) {
	return m.re.ExpandString(dst, template, src, match), nil
}

// parseSubstitution accepts -s with ANY delimiter, as POSIX requires: the
// character after 's' is the separator, so /, #, | and others are all legal.
// Assuming '/' would break the common case of rewriting paths that contain it.
func parseSubstitution(spec string) (substitution, error) {
	return parseSubstitutionLocale(spec, nil)
}

func parseSubstitutionLocale(spec string, tables *bre.LocaleByteTables) (substitution, error) {
	var s substitution
	if len(spec) < 2 {
		return s, fmt.Errorf("invalid -s expression %q", spec)
	}
	sep := spec[0]
	rest := spec[1:]
	// Split on unescaped separators only; a member name may legitimately
	// contain an escaped delimiter.
	var parts []string
	var cur strings.Builder
	for i := 0; i < len(rest); i++ {
		if rest[i] == '\\' && i+1 < len(rest) && rest[i+1] == sep {
			cur.WriteByte(sep)
			i++
			continue
		}
		if rest[i] == sep {
			parts = append(parts, cur.String())
			cur.Reset()
			continue
		}
		cur.WriteByte(rest[i])
	}
	parts = append(parts, cur.String())
	if len(parts) != 3 {
		return s, fmt.Errorf("invalid -s expression %q: expected %csoldpattern%cnew%c[gp]", spec, sep, sep, sep)
	}
	// The old part is a POSIX BRE. Longest() gives the leftmost-longest match
	// POSIX specifies (the same setting sed's s/// uses).
	if tables == nil {
		re, err := bre.Compile(parts[0])
		if err != nil {
			return s, fmt.Errorf("invalid -s pattern %q: %v", parts[0], err)
		}
		re.Longest()
		s.re = cSubstitutionMatcher{re: re}
	} else {
		re, err := bre.CompileLocaleByteRegexpTables([]byte(parts[0]), tables, bre.ByteRegexpOptions{Syntax: bre.ByteRegexpBRE})
		if err != nil {
			return s, fmt.Errorf("invalid -s pattern %q: %v", parts[0], err)
		}
		s.re = re
	}
	// POSIX uses & for the whole match and \1..\9 for groups; the bre matcher's
	// ExpandString reads ${0}..${9} the way Go's regexp does.
	s.repl = translateReplacement(parts[1])
	if len(parts) > 2 {
		for _, f := range parts[2] {
			switch f {
			case 'g':
				s.global = true
			case 'p':
				s.print = true
			default:
				return s, fmt.Errorf("invalid -s flag %q", string(f))
			}
		}
	}
	return s, nil
}

// translateReplacement converts an ed-style replacement to the ${n} template
// syntax pkg/bre.ExpandString understands. A literal $ must be escaped, or it
// would be read as a group reference that the author never wrote.
func translateReplacement(r string) string {
	var b strings.Builder
	for i := 0; i < len(r); i++ {
		switch {
		case r[i] == '\\' && i+1 < len(r) && r[i+1] >= '1' && r[i+1] <= '9':
			b.WriteString("${")
			b.WriteByte(r[i+1])
			b.WriteString("}")
			i++
		case r[i] == '\\' && i+1 < len(r):
			b.WriteByte(r[i+1])
			i++
		case r[i] == '&':
			b.WriteString("${0}")
		case r[i] == '$':
			b.WriteString("$$")
		default:
			b.WriteByte(r[i])
		}
	}
	return b.String()
}

// applySubstitutions rewrites a member name. POSIX stops at the FIRST expression
// that matches; an empty result means the member is skipped entirely, which is
// how -s is used to drop members. When the matching substitution carries the
// 'p' flag and report is non-nil, the "old >> new" rename is written to it.
func applySubstitutions(subs []substitution, name string, report io.Writer) string {
	out, _ := applySubstitutionsErr(subs, name, report)
	return out
}

func applySubstitutionsErr(subs []substitution, name string, report io.Writer) (string, error) {
	for _, s := range subs {
		locs, err := s.re.FindAllStringSubmatchIndex(name, -1)
		if err != nil {
			return "", err
		}
		if len(locs) == 0 {
			continue
		}
		var b strings.Builder
		last := 0
		for i, loc := range locs {
			// Without the g flag only the first occurrence is rewritten.
			if !s.global && i > 0 {
				break
			}
			b.WriteString(name[last:loc[0]])
			expanded, err := s.re.ExpandString(nil, s.repl, name, loc)
			if err != nil {
				return "", err
			}
			b.Write(expanded)
			last = loc[1]
		}
		b.WriteString(name[last:])
		out := b.String()
		if s.print && report != nil {
			fmt.Fprintf(report, "%s >> %s\n", name, out)
		}
		return out, nil
	}
	return name, nil
}

// selector applies the operand patterns to member names and records which
// patterns matched, so an unmatched pattern can be diagnosed as POSIX requires.
// It also carries the -c (complement), -n (first match only) and -d (do not
// descend) modifiers that shape hierarchy selection.
type selector struct {
	patterns  []string
	matched   []bool // per-pattern: did this pattern ever match a member
	consumed  []bool // per-pattern: has -n already taken its first match
	invert    bool   // -c
	firstOnly bool   // -n
	dirsOnly  bool   // -d

	activeDirs []string // paths of explicitly matched directories (with trailing slash)
	compiled   []*bre.LocaleByteRegexp
}

type selectorMember struct {
	name  string
	isDir bool
}

func newSelector(o *options, patterns []string) *selector {
	s, err := newSelectorLocale(o, patterns, nil)
	if err != nil {
		panic(err) // package-local tests use only statically valid patterns
	}
	return s
}

func newSelectorLocale(o *options, patterns []string, tables *bre.LocaleByteTables) (*selector, error) {
	if tables == nil {
		tables, _ = bre.SnapshotLocaleByteCtypeTables(nil)
	}
	s := &selector{
		patterns:  patterns,
		matched:   make([]bool, len(patterns)),
		consumed:  make([]bool, len(patterns)),
		invert:    o.invertMatch,
		firstOnly: o.selectNoPattern,
		dirsOnly:  o.dirsNoDescend,
	}
	for _, pattern := range patterns {
		ere, err := shellPatternERE(strings.TrimSuffix(pattern, "/"))
		if err != nil {
			return nil, err
		}
		re, err := bre.CompileLocaleByteRegexpTables([]byte(ere), tables, bre.ByteRegexpOptions{Syntax: bre.ByteRegexpERE})
		if err != nil {
			return nil, fmt.Errorf("invalid pattern %q: %v", pattern, err)
		}
		s.compiled = append(s.compiled, re)
	}
	return s, nil
}

// shellPatternERE translates POSIX pattern-matching notation into an anchored
// byte ERE. Slash remains special, as it is for pax pathname operands, while
// POSIX bracket elements are retained for the locale-aware regexp compiler.
func shellPatternERE(pattern string) (string, error) {
	var b strings.Builder
	b.WriteByte('^')
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '*':
			b.WriteString("[^/]*")
		case '?':
			b.WriteString("[^/]")
		case '\\':
			if i+1 == len(pattern) {
				return "", fmt.Errorf("invalid pattern %q: trailing backslash", pattern)
			}
			i++
			b.WriteString(regexp.QuoteMeta(pattern[i : i+1]))
		case '[':
			end, err := shellBracketEnd(pattern, i)
			if err != nil {
				return "", err
			}
			bracket := pattern[i : end+1]
			if len(bracket) > 2 && bracket[1] == '!' {
				bracket = "[^" + bracket[2:]
			}
			b.WriteString(bracket)
			i = end
		default:
			b.WriteString(regexp.QuoteMeta(pattern[i : i+1]))
		}
	}
	b.WriteByte('$')
	return b.String(), nil
}

func shellBracketEnd(pattern string, start int) (int, error) {
	i := start + 1
	if i < len(pattern) && (pattern[i] == '!' || pattern[i] == '^') {
		i++
	}
	if i < len(pattern) && pattern[i] == ']' { // literal ] in first position
		i++
	}
	for i < len(pattern) {
		if pattern[i] == '[' && i+1 < len(pattern) && strings.ContainsRune(".:=", rune(pattern[i+1])) {
			delim := pattern[i+1]
			close := strings.Index(pattern[i+2:], string([]byte{delim, ']'}))
			if close < 0 {
				return 0, fmt.Errorf("invalid pattern %q: unterminated bracket element", pattern)
			}
			i += close + 4
			continue
		}
		if pattern[i] == ']' {
			return i, nil
		}
		i++
	}
	return 0, fmt.Errorf("invalid pattern %q: unterminated bracket expression", pattern)
}

// prime records every directory member directly selected by an operand before
// output begins. Archive formats do not require directory headers to precede
// their children, so hierarchy selection cannot depend on encounter order.
// With -n, only the first direct member match for each operand can establish a
// selected hierarchy.
func (s *selector) prime(members []selectorMember) {
	for i, p := range s.patterns {
		patternNamesDir := strings.HasSuffix(p, "/")
		for _, m := range members {
			name := strings.TrimSuffix(m.name, "/")
			if patternNamesDir && !m.isDir {
				continue
			}
			match, _ := s.compiled[i].MatchString(name)
			if !match {
				continue
			}
			s.matched[i] = true
			if m.isDir && !s.dirsOnly {
				s.activeDirs = append(s.activeDirs, name+"/")
			}
			if s.firstOnly {
				break
			}
		}
	}
}

// keep reports whether a member name is selected, updating match bookkeeping.
// With no patterns every member is selected. Members are expected in archive
// order so that -n reliably takes the first match of each pattern.
func (s *selector) keep(name string, isDir bool) bool {
	name = strings.TrimSuffix(name, "/")
	if len(s.patterns) == 0 {
		return true
	}

	underDir := false
	if !s.dirsOnly {
		for _, d := range s.activeDirs {
			if name != strings.TrimSuffix(d, "/") && strings.HasPrefix(name+"/", d) {
				underDir = true
				break
			}
		}
	}

	patternMatched := false
	for i, p := range s.patterns {
		if s.firstOnly && s.consumed[i] {
			continue
		}

		patternNamesDir := strings.HasSuffix(p, "/")
		match := false
		if !patternNamesDir || isDir {
			match, _ = s.compiled[i].MatchString(name)
		}
		if match {
			s.matched[i] = true
			if s.firstOnly {
				s.consumed[i] = true
			}
			patternMatched = true
		}
	}

	matchedHierarchy := patternMatched || underDir
	if patternMatched && isDir && !s.dirsOnly {
		s.activeDirs = append(s.activeDirs, name+"/")
	}

	keep := matchedHierarchy

	if s.invert {
		keep = !keep
	}

	return keep
}

// unmatched returns the operand patterns that matched no member, in operand
// order, for the required diagnostic.
func (s *selector) unmatched() []string {
	var out []string
	for i, ok := range s.matched {
		if !ok {
			out = append(out, s.patterns[i])
		}
	}
	return out
}
