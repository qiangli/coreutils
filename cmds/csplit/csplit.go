// Package csplitcmd implements a practical csplit(1) subset: split an
// input file at line numbers or regular-expression matches.
package csplitcmd

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/qiangli/coreutils/pkg/bre"
	"github.com/qiangli/coreutils/pkg/collate"
	"github.com/qiangli/coreutils/pkg/ctype"
	"github.com/qiangli/coreutils/pkg/locale"
	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "csplit",
	Synopsis: "Split a file into sections determined by context lines.",
	Usage:    "csplit [OPTION]... FILE PATTERN...",
}

func init() { cmd.Run = run; tool.Register(cmd) }

type splitPoint struct {
	line      int
	nextStart int
	skip      bool
}

type patternSpec struct {
	raw    string
	expr   string
	offset int
	skip   bool
	regex  bool
}

type ctypeProvider interface {
	bre.ByteCtype
	Close() error
}

type ctypeOpener func(string) (ctypeProvider, error)

type collateProvider interface {
	bre.ByteEquivalence
	bre.ByteEquivalenceValidity
	bre.ByteCollationWeights
	bre.ByteCollatingElements
	Close() error
}

type collateOpener func(string) (collateProvider, error)

func run(rc *tool.RunContext, args []string) int {
	return runWithLocales(rc, args, func(name string) (ctypeProvider, error) { return ctype.Open(name) }, func(name string) (collateProvider, error) { return collate.Open(name) })
}

func runWithLocales(rc *tool.RunContext, args []string, ctypeOpen ctypeOpener, collateOpen collateOpener) int {
	fs := tool.NewFlags(cmd.Name)
	prefix := fs.StringP("prefix", "f", "xx", "use PREFIX instead of 'xx'")
	digits := fs.IntP("digits", "n", 2, "use DIGITS digits in output file names")
	suffixFormat := fs.StringP("suffix-format", "b", "", "use sprintf FORMAT instead of %02d")
	silent := fs.BoolP("silent", "s", false, "do not print output file sizes")
	quiet := fs.BoolP("quiet", "q", false, "do not print output file sizes")
	keep := fs.BoolP("keep-files", "k", false, "do not remove output files on errors")
	suppressMatched := fs.Bool("suppress-matched", false, "suppress the lines matching PATTERN")
	elideEmpty := fs.BoolP("elide-empty-files", "z", false, "remove empty output files")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	if len(operands) < 2 {
		return tool.UsageError(rc, cmd, "missing operand")
	}
	if *digits < 1 {
		return tool.UsageError(rc, cmd, "invalid number of digits: '%d'", *digits)
	}
	format := *suffixFormat
	if format == "" {
		format = "%0" + strconv.Itoa(*digits) + "d"
	}
	if err := validateSuffixFormat(format); err != nil {
		return tool.UsageError(rc, cmd, "invalid suffix format '%s': %v", format, err)
	}
	tables, code := localeRegexpTables(rc, ctypeOpen, collateOpen)
	if code >= 0 {
		return code
	}

	lines, err := readLines(rc, operands[0])
	if err != nil {
		fmt.Fprintf(rc.Err, "csplit: cannot open '%s' for reading: %v\n", operands[0], tool.SysErr(err))
		return 1
	}
	points, code := resolvePatterns(rc, lines, operands[1:], *suppressMatched, tables)
	if code >= 0 {
		// csplit creates pieces as it processes operands.  Resolution is kept
		// separate from I/O here, but -k must still preserve the pieces that
		// precede a later bad operand.  Materialize the successfully resolved
		// prefix, including its current trailing piece, before returning the
		// original diagnostic status.
		if *keep && len(points) > 0 {
			if _, err := writePieces(rc, lines, points, *prefix, format, *silent || *quiet, *elideEmpty); err != nil {
				fmt.Fprintf(rc.Err, "csplit: %v\n", err)
			}
		}
		return code
	}
	created, err := writePieces(rc, lines, points, *prefix, format, *silent || *quiet, *elideEmpty)
	if err != nil {
		if !*keep {
			for _, name := range created {
				_ = os.Remove(rc.Path(name))
			}
		}
		fmt.Fprintf(rc.Err, "csplit: %v\n", err)
		return 1
	}
	return 0
}

func readLines(rc *tool.RunContext, name string) ([]string, error) {
	var r io.Reader
	if name == "-" {
		r = rc.In
		if r == nil {
			r = strings.NewReader("")
		}
	} else {
		f, err := os.Open(rc.Path(name))
		if err != nil {
			return nil, err
		}
		defer f.Close()
		r = f
	}
	br := bufio.NewReader(r)
	var lines []string
	for {
		line, err := br.ReadString('\n')
		if line != "" {
			lines = append(lines, line)
		}
		if err == io.EOF {
			return lines, nil
		}
		if err != nil {
			return nil, err
		}
	}
}

func localeRegexpTables(rc *tool.RunContext, ctypeOpen ctypeOpener, collateOpen collateOpener) (*bre.LocaleByteTables, int) {
	lcCType := locale.Resolve(rc.Env, locale.CType)
	lcCollate := locale.Resolve(rc.Env, locale.Collate)
	var tables *bre.LocaleByteTables
	if lcCType != "C" && lcCType != "POSIX" {
		provider, err := ctypeOpen(lcCType)
		if err != nil {
			fmt.Fprintf(rc.Err, "csplit: LC_CTYPE %q: %v\n", lcCType, err)
			return nil, 2
		}
		var snapshotErr error
		tables, snapshotErr = bre.SnapshotLocaleByteCtypeTables(provider)
		closeErr := provider.Close()
		if snapshotErr != nil {
			fmt.Fprintf(rc.Err, "csplit: LC_CTYPE %q: %v\n", lcCType, snapshotErr)
			return nil, 2
		}
		if closeErr != nil {
			fmt.Fprintf(rc.Err, "csplit: LC_CTYPE %q: %v\n", lcCType, closeErr)
			return nil, 2
		}
	} else {
		tables, _ = bre.SnapshotLocaleByteCtypeTables(nil)
	}
	if lcCollate != "C" && lcCollate != "POSIX" {
		provider, err := collateOpen(lcCollate)
		if err != nil {
			fmt.Fprintf(rc.Err, "csplit: LC_COLLATE %q: %v\n", lcCollate, err)
			return nil, 2
		}
		var snapshotErr error
		tables, snapshotErr = tables.WithCollation(provider)
		closeErr := provider.Close()
		if snapshotErr != nil {
			fmt.Fprintf(rc.Err, "csplit: LC_COLLATE %q: %v\n", lcCollate, snapshotErr)
			return nil, 2
		}
		if closeErr != nil {
			fmt.Fprintf(rc.Err, "csplit: LC_COLLATE %q: %v\n", lcCollate, closeErr)
			return nil, 2
		}
	}
	return tables, -1
}

func resolvePatterns(rc *tool.RunContext, lines []string, patterns []string, suppressMatched bool, tables *bre.LocaleByteTables) ([]splitPoint, int) {
	points := make([]splitPoint, 0, len(patterns))
	start := 0
	last := ""
	lastNum := 0  // N of the last line-number pattern (0 = last was a regexp)
	lastLine := 0 // 1-based line of the last line-number split
	for _, pattern := range patterns {
		repeats, repeatToEOF, isRepeat, code := parseRepeat(rc, pattern)
		if code >= 0 {
			return points, code
		}
		if isRepeat {
			if last == "" {
				return points, tool.UsageError(rc, cmd, "missing pattern before repeat count")
			}
			if lastNum > 0 {
				// POSIX: a repeated line-number pattern advances by N
				// lines each round ("split every N lines").
				for i := 0; repeatToEOF || i < repeats; i++ {
					next := lastLine + lastNum
					if next > len(lines) {
						return points, tool.UsageError(rc, cmd, "'%d': line number out of range", next)
					}
					idx := next - 1
					point := splitPoint{line: idx, nextStart: idx}
					if suppressMatched {
						point.nextStart = idx + 1
					}
					points = append(points, point)
					lastLine = next
					start = idx
				}
				continue
			}
			if repeatToEOF {
				for {
					point, nextSearch, found, code := resolveOnePattern(rc, lines, last, start, suppressMatched, tables)
					if code >= 0 {
						return points, code
					}
					if !found {
						break
					}
					points = append(points, point)
					start = nextSearch
				}
				continue
			}
			for i := 0; i < repeats; i++ {
				point, nextSearch, found, code := resolveOnePattern(rc, lines, last, start, suppressMatched, tables)
				if code >= 0 {
					return points, code
				}
				if !found {
					return points, tool.UsageError(rc, cmd, "match not found: '%s'", last)
				}
				points = append(points, point)
				start = nextSearch
			}
			continue
		}
		point, nextSearch, found, code := resolveOnePattern(rc, lines, pattern, start, suppressMatched, tables)
		if code >= 0 {
			return points, code
		}
		if !found {
			return points, tool.UsageError(rc, cmd, "match not found: '%s'", pattern)
		}
		points = append(points, point)
		start = nextSearch
		last = pattern
		if n, err := strconv.Atoi(pattern); err == nil {
			lastNum, lastLine = n, n
		} else {
			lastNum = 0
		}
	}
	return points, -1
}

func parseRepeat(rc *tool.RunContext, pattern string) (count int, toEOF bool, repeat bool, code int) {
	if pattern == "{*}" {
		return 0, true, true, -1
	}
	if strings.HasPrefix(pattern, "{") && strings.HasSuffix(pattern, "}") {
		n, err := strconv.Atoi(pattern[1 : len(pattern)-1])
		if err != nil || n < 0 {
			return 0, false, false, tool.UsageError(rc, cmd, "invalid repeat count: '%s'", pattern)
		}
		return n, false, true, -1
	}
	return 0, false, false, -1
}

func resolveOnePattern(rc *tool.RunContext, lines []string, pattern string, start int, suppressMatched bool, tables *bre.LocaleByteTables) (splitPoint, int, bool, int) {
	spec, code := parsePattern(rc, pattern)
	if code >= 0 {
		return splitPoint{}, start, false, code
	}
	if !spec.regex {
		n, err := strconv.Atoi(spec.raw)
		if err != nil {
			return splitPoint{}, start, false, tool.NotSupported(rc, cmd, fmt.Sprintf("pattern form '%s' (supported: line numbers, /REGEXP/[+-N], %%REGEXP%%[+-N])", pattern))
		}
		// POSIX: a line_no operand that exceeds the number of lines in the
		// file is out of range. GNU errors on N > lines (the maximum valid
		// split point is the last line itself, leaving it as the final
		// piece); it never fabricates an empty trailing piece for N=lines+1.
		if n < 1 || n > len(lines) {
			return splitPoint{}, start, false, tool.UsageError(rc, cmd, "line number out of range: '%s'", pattern)
		}
		idx := n - 1
		point := splitPoint{line: idx, nextStart: idx}
		if suppressMatched {
			point.nextStart = idx + 1
		}
		return point, idx, true, -1
	}
	matcher, err := compileMatcher(spec.expr, tables)
	if err != nil {
		return splitPoint{}, start, false, tool.UsageError(rc, cmd, "invalid regular expression '%s'", spec.expr)
	}
	found := -1
	for i := start; i < len(lines); i++ {
		if ok, err := matcher.match(strings.TrimSuffix(lines[i], "\n")); err != nil {
			return splitPoint{}, start, false, tool.UsageError(rc, cmd, "regular expression '%s': %v", spec.expr, err)
		} else if ok {
			found = i
			break
		}
	}
	if found < 0 {
		return splitPoint{}, start, false, -1
	}
	line := found + spec.offset
	if line < 0 || line > len(lines) {
		return splitPoint{}, start, false, tool.UsageError(rc, cmd, "line number out of range: '%s'", pattern)
	}
	nextStart := line
	if suppressMatched {
		nextStart = found + 1
	}
	return splitPoint{line: line, nextStart: nextStart, skip: spec.skip}, found + 1, true, -1
}

type lineMatcher struct {
	re       *bre.Regexp
	localeRe *bre.LocaleByteRegexp
}

func compileMatcher(expr string, tables *bre.LocaleByteTables) (lineMatcher, error) {
	if tables != nil {
		re, err := bre.CompileLocaleByteRegexpTables([]byte(expr), tables, bre.ByteRegexpOptions{Syntax: bre.ByteRegexpBRE})
		return lineMatcher{localeRe: re}, err
	}
	re, err := bre.Compile(expr)
	return lineMatcher{re: re}, err
}

func (m lineMatcher) match(line string) (bool, error) {
	if m.localeRe != nil {
		return m.localeRe.MatchString(line)
	}
	return m.re.MatchStringErr(line)
}

func parsePattern(rc *tool.RunContext, pattern string) (patternSpec, int) {
	spec := patternSpec{raw: pattern}
	if pattern == "" {
		return spec, -1
	}
	delim := pattern[0]
	if delim != '/' && delim != '%' {
		return spec, -1
	}
	end := findClosingDelimiter(pattern, delim)
	if end < 0 {
		return spec, tool.NotSupported(rc, cmd, fmt.Sprintf("pattern form '%s' (supported: line numbers, /REGEXP/[+-N], %%REGEXP%%[+-N])", pattern))
	}
	spec.regex = true
	spec.skip = delim == '%'
	spec.expr = pattern[1:end]
	if tail := pattern[end+1:]; tail != "" {
		offset := tail
		sign := byte('+')
		if tail[0] == '+' || tail[0] == '-' {
			sign = tail[0]
			offset = tail[1:]
		}
		n, err := strconv.Atoi(offset)
		if err != nil {
			return spec, tool.UsageError(rc, cmd, "invalid offset in pattern '%s'", pattern)
		}
		if sign == '-' {
			n = -n
		}
		spec.offset = n
	}
	return spec, -1
}

func findClosingDelimiter(pattern string, delim byte) int {
	escaped := false
	for i := 1; i < len(pattern); i++ {
		c := pattern[i]
		if escaped {
			escaped = false
			continue
		}
		if c == '\\' {
			escaped = true
			continue
		}
		if c == delim {
			return i
		}
	}
	return -1
}

func writePieces(rc *tool.RunContext, lines []string, points []splitPoint, prefix, suffixFormat string, silent, elideEmpty bool) ([]string, error) {
	var created []string
	start := 0
	seq := 0
	for _, point := range points {
		if point.line < start {
			return created, fmt.Errorf("split point moved backwards")
		}
		if !point.skip {
			name, wrote, err := writePiece(rc, lines[start:point.line], prefix, suffixFormat, seq, silent, elideEmpty)
			if err != nil {
				return created, err
			}
			if wrote {
				created = append(created, name)
				seq++
			}
		}
		start = point.nextStart
	}
	name, wrote, err := writePiece(rc, lines[start:], prefix, suffixFormat, seq, silent, elideEmpty)
	if err != nil {
		return created, err
	}
	if wrote {
		created = append(created, name)
	}
	return created, nil
}

func writePiece(rc *tool.RunContext, lines []string, prefix, suffixFormat string, seq int, silent, elideEmpty bool) (string, bool, error) {
	suffix, err := formatSuffix(suffixFormat, seq)
	if err != nil {
		return prefix, false, err
	}
	name := prefix + suffix
	var buf bytes.Buffer
	for _, line := range lines {
		buf.WriteString(line)
	}
	if elideEmpty && buf.Len() == 0 {
		return name, false, nil
	}
	if err := os.WriteFile(rc.Path(name), buf.Bytes(), 0o666); err != nil {
		return name, false, err
	}
	if !silent {
		fmt.Fprintf(rc.Out, "%d\n", buf.Len())
	}
	return name, true, nil
}

func formatSuffix(format string, seq int) (suffix string, err error) {
	if err := validateSuffixFormat(format); err != nil {
		return "", err
	}
	defer func() {
		if r := recover(); r != nil {
			suffix = ""
			err = fmt.Errorf("requires one integer conversion")
		}
	}()
	suffix = fmt.Sprintf(goSuffixFormat(format), seq)
	if strings.Contains(suffix, "%!") {
		return "", fmt.Errorf("requires one integer conversion")
	}
	return suffix, nil
}

func goSuffixFormat(format string) string {
	var b strings.Builder
	for i := 0; i < len(format); i++ {
		if format[i] != '%' {
			b.WriteByte(format[i])
			continue
		}
		if i+1 < len(format) && format[i+1] == '%' {
			b.WriteString("%%")
			i++
			continue
		}
		b.WriteByte(format[i])
		for i++; i < len(format); i++ {
			c := format[i]
			if c == 'i' || c == 'u' {
				b.WriteByte('d')
				break
			}
			b.WriteByte(c)
			if strings.ContainsRune("doxX", rune(c)) {
				break
			}
		}
	}
	return b.String()
}

// validateSuffixFormat accepts the printf subset specified by csplit: one
// integer conversion, with optional flags, width, and precision. A literal
// percent (%%) does not count as a conversion.
func validateSuffixFormat(format string) error {
	conversions := 0
	for i := 0; i < len(format); {
		if format[i] != '%' {
			i++
			continue
		}
		i++
		if i == len(format) {
			return fmt.Errorf("requires one integer conversion")
		}
		if format[i] == '%' {
			i++
			continue
		}
		if conversions != 0 {
			return fmt.Errorf("requires one integer conversion")
		}
		conversions++
		for i < len(format) && strings.ContainsRune("#0- +", rune(format[i])) {
			i++
		}
		for i < len(format) && format[i] >= '0' && format[i] <= '9' {
			i++
		}
		if i < len(format) && format[i] == '.' {
			i++
			for i < len(format) && format[i] >= '0' && format[i] <= '9' {
				i++
			}
		}
		if i == len(format) || !strings.ContainsRune("diuoxX", rune(format[i])) {
			return fmt.Errorf("requires one integer conversion")
		}
		i++
	}
	if conversions != 1 {
		return fmt.Errorf("requires one integer conversion")
	}
	return nil
}
