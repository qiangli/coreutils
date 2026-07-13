// GNU-compatibility layer for the vendored Go.Sed engine.
//
// Upstream Go.Sed compiles patterns as Go/RE2 regexps and expects Go-style
// ($1) replacement templates. GNU sed instead defaults to POSIX Basic Regular
// Expressions (BRE) — `\(...\)` groups, `\{m,n\}` intervals, `\1` backrefs,
// `&` whole-match — switching to ERE only under -E/-r. This file bridges that:
// patterns are translated through coreutils/pkg/bre (the same BRE engine grep
// uses) and replacements are rewritten from GNU `\1`/`&` form into the Go
// ExpandString templates the engine consumes. The two regex-compile seams
// (substitution.go, conditions.go) call compileRE instead of regexp.Compile.
package gosed

import (
	"regexp"
	"strings"

	"github.com/qiangli/coreutils/pkg/bre"
)

// ExtendedRegex selects ERE (true; sed -E/-r) vs BRE (false; the default) for
// every pattern this engine compiles. Set it before calling New(); a sed CLI
// invocation drives a single engine per process.
var ExtendedRegex bool

// sedRegexp is the small regexp surface the engine needs.
type sedRegexp interface {
	MatchString(string) bool
	FindAllStringSubmatchIndex(string, int) [][]int
	ExpandString([]byte, string, string, []int) []byte
	FindAllSubmatchIndex([]byte, int) [][]int
	Expand([]byte, []byte, []byte, []int) []byte
}

// compileRE compiles a GNU sed regex (BRE by default, ERE under ExtendedRegex).
// BREs without back-references use RE2 through pkg/bre; BREs with \1..\9 use
// pkg/bre's bounded backtracking matcher.
//
// Two sed-specific rules are applied on top of the shared engine, both because
// sed — unlike grep — matches against a pattern space that can hold embedded
// newlines: the sed character escapes (\n, \t, …) are expanded to the
// characters they name, and '.' is compiled dot-all (see sedFlags). Matching is
// POSIX leftmost-longest, the extent GNU sed substitutes and reports.
func compileRE(pattern, flags string) (sedRegexp, error) {
	pattern = bre.SedEscapes(pattern)
	flags = sedFlags(flags)
	if ExtendedRegex {
		translated, err := bre.ToGoERE(pattern)
		if err != nil {
			return nil, err
		}
		re, err := regexp.Compile(flags + translated)
		if err != nil {
			return nil, err
		}
		re.Longest()
		return re, nil
	}
	re, err := bre.CompileWithFlags(pattern, flags)
	if err != nil {
		return nil, err
	}
	re.Longest()
	return re, nil
}

// sedFlags finalizes the RE2 flag prefix for one sed pattern. POSIX has '.'
// match any character, and sed's pattern space can contain newlines (after N or
// G), so '.' is compiled dot-all — except under the M/m modifier, where GNU
// documents the opposite: "the dot character does not match a new-line
// character in multi-line mode".
func sedFlags(flags string) string {
	if strings.Contains(flags, "m") || strings.Contains(flags, "s") {
		return flags
	}
	if flags == "" {
		return "(?s)"
	}
	return strings.TrimSuffix(flags, ")") + "s)"
}

// translateReplacement converts a GNU sed s/// replacement into the Go
// ExpandString template form: `\1`..`\9` and `\0`/`&` (whole match) become
// `${N}`; `\&`, `\\` are literals; `\n`/`\t`/`\r` are the control chars; a
// literal `$` is doubled so ExpandString leaves it alone.
func translateReplacement(r string) string {
	var b strings.Builder
	for i := 0; i < len(r); i++ {
		switch c := r[i]; c {
		case '\\':
			if i+1 >= len(r) {
				b.WriteByte('\\')
				break
			}
			n := r[i+1]
			i++
			switch {
			case n >= '0' && n <= '9':
				b.WriteString("${" + string(n) + "}")
			case n == '&':
				b.WriteByte('&')
			case n == '\\':
				b.WriteByte('\\')
			case n == 'n':
				b.WriteByte('\n')
			case n == 't':
				b.WriteByte('\t')
			case n == 'r':
				b.WriteByte('\r')
			default:
				b.WriteByte(n) // \x -> x
			}
		case '&':
			b.WriteString("${0}")
		case '$':
			b.WriteString("$$") // a literal $ for ExpandString
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}
