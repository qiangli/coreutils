package paxcmd

import (
	"fmt"
	"path"
	"regexp"
	"strings"
)

// substitution is one -s ed-style rewrite: /old/new/[gp].
type substitution struct {
	re     *regexp.Regexp
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
	if len(parts) < 2 {
		return s, fmt.Errorf("invalid -s expression %q: expected %csoldpattern%cnew%c[gp]", spec, sep, sep, sep)
	}
	re, err := regexp.Compile(parts[0])
	if err != nil {
		return s, fmt.Errorf("invalid -s pattern %q: %v", parts[0], err)
	}
	s.re = re
	// POSIX uses & for the whole match and \1..\9 for groups; Go uses $.
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

// translateReplacement converts an ed-style replacement to Go's syntax. A
// literal $ must be escaped, or Go would read it as a group reference that the
// author never wrote.
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

// applySubstitutions rewrites a member name. POSIX stops at the FIRST
// expression that matches; an empty result means the member is skipped
// entirely, which is how -s is used to drop members.
func applySubstitutions(subs []substitution, name string) string {
	for _, s := range subs {
		if !s.re.MatchString(name) {
			continue
		}
		if s.global {
			return s.re.ReplaceAllString(name, s.repl)
		}
		// Replace the first occurrence only.
		loc := s.re.FindStringSubmatchIndex(name)
		if loc == nil {
			continue
		}
		out := s.re.ExpandString(nil, s.repl, name, loc)
		return name[:loc[0]] + string(out) + name[loc[1]:]
	}
	return name
}

// selected reports whether a member name is chosen by the operand patterns.
// With no patterns every member is selected; -c inverts the sense.
func selected(o *options, patterns []string, name string) bool {
	if len(patterns) == 0 {
		return true
	}
	match := false
	for _, p := range patterns {
		if ok, _ := path.Match(p, name); ok {
			match = true
			break
		}
		// A directory operand selects everything beneath it, which is how
		// callers name a subtree rather than each file in it.
		if strings.HasPrefix(name, strings.TrimSuffix(p, "/")+"/") {
			match = true
			break
		}
	}
	if o.invertMatch {
		return !match
	}
	return match
}
