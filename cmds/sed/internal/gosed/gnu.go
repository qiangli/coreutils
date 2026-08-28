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
	"strings"
	"unicode/utf8"

	"github.com/qiangli/coreutils/pkg/bre"
)

// Options configures sed engine compilation and execution.
type Options struct {
	ExtendedRegex bool
	LocaleTables  *bre.LocaleByteTables
	CUTF8         bool
	CByteTables   bool
}

// sedRegexp is the small regexp surface the engine needs.
type sedRegexp interface {
	MatchString(string) (bool, error)
	FindAllStringSubmatchIndex(string, int) ([][]int, error)
	ExpandString([]byte, string, string, []int) ([]byte, error)
	FindAllSubmatchIndex([]byte, int) ([][]int, error)
	Expand([]byte, []byte, []byte, []int) ([]byte, error)
}

// legacyRegexp adapts pkg/bre to sed's error-bearing matcher seam.
type legacyRegexp struct {
	*bre.Regexp
}

// localeRegexp preserves the engine's fully error-bearing matcher seam.
type localeRegexp struct {
	*bre.LocaleByteRegexp
}

func (r localeRegexp) MatchString(s string) (bool, error) {
	return r.LocaleByteRegexp.MatchString(s)
}

func (r localeRegexp) FindAllStringSubmatchIndex(s string, n int) ([][]int, error) {
	return r.LocaleByteRegexp.FindAllStringSubmatchIndex(s, n)
}

func (r localeRegexp) FindAllSubmatchIndex(s []byte, n int) ([][]int, error) {
	return r.LocaleByteRegexp.FindAllSubmatchIndex(s, n)
}

func (r localeRegexp) ExpandString(dst []byte, template, src string, match []int) ([]byte, error) {
	return r.LocaleByteRegexp.ExpandString(dst, template, src, match)
}

func (r localeRegexp) Expand(dst, template, src []byte, match []int) ([]byte, error) {
	return r.LocaleByteRegexp.Expand(dst, template, src, match)
}

func (r legacyRegexp) MatchString(s string) (bool, error) {
	return r.Regexp.MatchStringErr(s)
}

func (r legacyRegexp) FindAllStringSubmatchIndex(s string, n int) ([][]int, error) {
	return r.Regexp.FindAllStringSubmatchIndexErr(s, n)
}

func (r legacyRegexp) FindAllSubmatchIndex(s []byte, n int) ([][]int, error) {
	return r.Regexp.FindAllSubmatchIndexErr(s, n)
}

func (r legacyRegexp) ExpandString(dst []byte, template, src string, match []int) ([]byte, error) {
	return r.Regexp.ExpandString(dst, template, src, match), nil
}

func (r legacyRegexp) Expand(dst, template, src []byte, match []int) ([]byte, error) {
	return r.Regexp.Expand(dst, template, src, match), nil
}

// compileRE compiles a GNU sed regex (BRE by default, ERE under opts.ExtendedRegex).
// BREs without back-references use RE2 through pkg/bre; BREs with \1..\9 use
// pkg/bre's bounded backtracking matcher.
//
// Two sed-specific rules are applied on top of the shared engine, both because
// sed — unlike grep — matches against a pattern space that can hold embedded
// newlines: the sed character escapes (\n, \t, …) are expanded to the
// characters they name, and '.' is compiled dot-all (see sedFlags). Matching is
// POSIX leftmost-longest, the extent GNU sed substitutes and reports.
func (opts Options) compileRE(pattern, flags string) (sedRegexp, error) {
	pattern = bre.SedEscapes(pattern)
	flags = sedFlags(flags)
	if opts.CUTF8 {
		re, err := bre.CompileCUTF8WithFlags(pattern, flags, opts.ExtendedRegex, opts.LocaleTables)
		if err != nil {
			return nil, err
		}
		re.Longest()
		return legacyRegexp{re}, nil
	}
	if opts.LocaleTables != nil && (!opts.CByteTables || cBytePatternNeedsTables(pattern, flags)) {
		syntax := bre.ByteRegexpBRE
		if opts.ExtendedRegex {
			syntax = bre.ByteRegexpERE
		}
		re, err := bre.CompileLocaleByteRegexpTables([]byte(pattern), opts.LocaleTables, bre.ByteRegexpOptions{
			Syntax: syntax, FoldCase: strings.Contains(flags, "i"),
			DotAll: strings.Contains(flags, "s"), MultiLine: strings.Contains(flags, "m"),
		})
		if err != nil {
			return nil, err
		}
		return localeRegexp{re}, nil
	}
	if opts.ExtendedRegex {
		re, err := bre.CompileEREWithFlags(pattern, flags)
		if err != nil {
			return nil, err
		}
		re.Longest()
		return legacyRegexp{re}, nil
	}
	re, err := bre.CompileWithFlags(pattern, flags)
	if err != nil {
		return nil, err
	}
	re.Longest()
	return legacyRegexp{re}, nil
}

// cBytePatternNeedsTables identifies the constructs for which RE2's rune
// model cannot implement the C locale's byte semantics. Literal-only regular
// expressions (including back-references handled by the bounded matcher) stay
// on the legacy path, which avoids reducing that
// matcher's supported grammar merely to select byte-oriented dot/classes.
func cBytePatternNeedsTables(pattern, flags string) bool {
	if strings.Contains(flags, "i") {
		return true
	}
	escaped := false
	for i := 0; i < len(pattern); i++ {
		// In the C locale every non-ASCII byte is a separate character.
		// Even an apparently literal UTF-8 rune therefore changes the
		// grammar when repetition follows it: GNU reads é* as byte 0xc3
		// followed by zero or more 0xa9 bytes, not as U+00E9 repeated.
		if pattern[i] >= utf8.RuneSelf {
			return true
		}
		if escaped {
			escaped = false
			continue
		}
		if pattern[i] == '\\' {
			escaped = true
			continue
		}
		if pattern[i] == '.' || pattern[i] == '[' {
			return true
		}
	}
	return false
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
