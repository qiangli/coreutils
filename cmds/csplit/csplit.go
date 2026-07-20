// Package csplitcmd implements a practical csplit(1) subset: split an
// input file at line numbers or regular-expression matches.
package csplitcmd

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/qiangli/coreutils/pkg/bre"
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

func run(rc *tool.RunContext, args []string) int {
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

	lines, err := readLines(rc, operands[0])
	if err != nil {
		fmt.Fprintf(rc.Err, "csplit: cannot open '%s' for reading: %v\n", operands[0], tool.SysErr(err))
		return 1
	}
	points, code := resolvePatterns(rc, lines, operands[1:], *suppressMatched)
	if code >= 0 {
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

func resolvePatterns(rc *tool.RunContext, lines []string, patterns []string, suppressMatched bool) ([]splitPoint, int) {
	points := make([]splitPoint, 0, len(patterns))
	start := 0
	last := ""
	lastNum := 0  // N of the last line-number pattern (0 = last was a regexp)
	lastLine := 0 // 1-based line of the last line-number split
	for _, pattern := range patterns {
		repeats, repeatToEOF, isRepeat, code := parseRepeat(rc, pattern)
		if code >= 0 {
			return nil, code
		}
		if isRepeat {
			if last == "" {
				return nil, tool.UsageError(rc, cmd, "missing pattern before repeat count")
			}
			if lastNum > 0 {
				// POSIX: a repeated line-number pattern advances by N
				// lines each round ("split every N lines").
				for i := 0; repeatToEOF || i < repeats; i++ {
					next := lastLine + lastNum
					if next > len(lines) {
						if repeatToEOF {
							break
						}
						return nil, tool.UsageError(rc, cmd, "'%d': line number out of range", next)
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
					point, nextSearch, found, code := resolveOnePattern(rc, lines, last, start, suppressMatched)
					if code >= 0 {
						return nil, code
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
				point, nextSearch, found, code := resolveOnePattern(rc, lines, last, start, suppressMatched)
				if code >= 0 {
					return nil, code
				}
				if !found {
					return nil, tool.UsageError(rc, cmd, "match not found: '%s'", last)
				}
				points = append(points, point)
				start = nextSearch
			}
			continue
		}
		point, nextSearch, found, code := resolveOnePattern(rc, lines, pattern, start, suppressMatched)
		if code >= 0 {
			return nil, code
		}
		if !found {
			return nil, tool.UsageError(rc, cmd, "match not found: '%s'", pattern)
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

func resolveOnePattern(rc *tool.RunContext, lines []string, pattern string, start int, suppressMatched bool) (splitPoint, int, bool, int) {
	spec, code := parsePattern(rc, pattern)
	if code >= 0 {
		return splitPoint{}, start, false, code
	}
	if !spec.regex {
		n, err := strconv.Atoi(spec.raw)
		if err != nil {
			return splitPoint{}, start, false, tool.NotSupported(rc, cmd, fmt.Sprintf("pattern form '%s' (supported: line numbers, /REGEXP/[+-N], %%REGEXP%%[+-N])", pattern))
		}
		if n < 1 || n > len(lines)+1 {
			return splitPoint{}, start, false, tool.UsageError(rc, cmd, "line number out of range: '%s'", pattern)
		}
		idx := n - 1
		point := splitPoint{line: idx, nextStart: idx}
		if suppressMatched {
			point.nextStart = idx + 1
		}
		return point, idx, true, -1
	}
	// csplit patterns are POSIX basic regular expressions, like grep's
	// default mode — translate through the shared BRE engine.
	translated, err := bre.ToGo(spec.expr)
	if err != nil {
		return splitPoint{}, start, false, tool.UsageError(rc, cmd, "invalid regular expression '%s'", spec.expr)
	}
	re, err := regexp.Compile(translated)
	if err != nil {
		return splitPoint{}, start, false, tool.UsageError(rc, cmd, "invalid regular expression '%s'", spec.expr)
	}
	found := -1
	for i := start; i < len(lines); i++ {
		if re.MatchString(strings.TrimSuffix(lines[i], "\n")) {
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
		sign := tail[0]
		if sign != '+' && sign != '-' {
			return spec, tool.UsageError(rc, cmd, "invalid offset in pattern '%s'", pattern)
		}
		n, err := strconv.Atoi(tail[1:])
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
