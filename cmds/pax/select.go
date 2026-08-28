package paxcmd

import (
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/qiangli/coreutils/pkg/bre"
)

// substitution is one -s ed-style rewrite: /old/new/[gp]. POSIX defines the
// pattern as a basic regular expression, so it is compiled with the shared
// pkg/bre provider rather than Go's regexp (whose \(...\) are literal parens).
type substitution struct {
	re     *bre.Regexp
	repl   string
	global bool
	print  bool
}

// parseSubstitution accepts -s with ANY delimiter, as POSIX requires: the
// character after 's' is the separator, so /, #, | and others are all legal.
// Assuming '/' would break the common case of rewriting paths that contain it.
func parseSubstitution(spec string) (substitution, error) {
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
	re, err := bre.Compile(parts[0])
	if err != nil {
		return s, fmt.Errorf("invalid -s pattern %q: %v", parts[0], err)
	}
	re.Longest()
	s.re = re
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
		locs, err := s.re.FindAllStringSubmatchIndexErr(name, -1)
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
			b.Write(s.re.ExpandString(nil, s.repl, name, loc))
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
}

type selectorMember struct {
	name  string
	isDir bool
}

func newSelector(o *options, patterns []string) *selector {
	return &selector{
		patterns:  patterns,
		matched:   make([]bool, len(patterns)),
		consumed:  make([]bool, len(patterns)),
		invert:    o.invertMatch,
		firstOnly: o.selectNoPattern,
		dirsOnly:  o.dirsNoDescend,
	}
}

// prime records every directory member directly selected by an operand before
// output begins. Archive formats do not require directory headers to precede
// their children, so hierarchy selection cannot depend on encounter order.
// With -n, only the first direct member match for each operand can establish a
// selected hierarchy.
func (s *selector) prime(members []selectorMember) {
	for i, p := range s.patterns {
		patternNamesDir := strings.HasSuffix(p, "/")
		matchPattern := strings.TrimSuffix(p, "/")
		for _, m := range members {
			name := strings.TrimSuffix(m.name, "/")
			if patternNamesDir && !m.isDir {
				continue
			}
			match, _ := path.Match(matchPattern, name)
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
		matchPattern := strings.TrimSuffix(p, "/")
		match := false
		if !patternNamesDir || isDir {
			match, _ = path.Match(matchPattern, name)
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
