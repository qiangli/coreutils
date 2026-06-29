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

// compileRE compiles a GNU sed regex (BRE by default, ERE under ExtendedRegex)
// into a Go regexp. flags is an optional RE2 flag prefix like "(?i)" / "(?im)".
func compileRE(pattern, flags string) (*regexp.Regexp, error) {
	translated := pattern
	if ExtendedRegex {
		if err := bre.ValidateERE(pattern); err != nil {
			return nil, err
		}
	} else {
		t, err := bre.ToGo(pattern)
		if err != nil {
			return nil, err
		}
		translated = t
	}
	return regexp.Compile(flags + translated)
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
