// Portions adapted from https://github.com/u-root/u-root cmds/core/uniq/uniq.go (BSD-3-Clause).
// Changes: rewired to tool framework; added -f/-s/-w key extraction, -c GNU "%7d "
// count format, first-of-group output buffering, and the [INPUT [OUTPUT]] operands.

// Package uniqcmd implements uniq(1) per the GNU coreutils manual:
// filter adjacent matching lines from INPUT (or standard input),
// writing to OUTPUT (or standard output).
//
// Implemented flags: -c -d -u -i -f N -s N -w N. Fields are runs of
// non-blank characters separated by blanks (skipped before
// characters); comparisons are byte-wise, with -i folding ASCII case
// (C-locale semantics).
package uniqcmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "uniq",
	Synopsis: "Filter adjacent matching lines from INPUT, writing to OUTPUT.",
	Usage:    "uniq [OPTION]... [INPUT [OUTPUT]]\n\nWith no INPUT, or when INPUT is -, read standard input.",
}

// Run is wired in init: a literal would create an initialization
// cycle (run's flag-error paths reference cmd).
func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	count := fs.BoolP("count", "c", false, "prefix lines by the number of occurrences")
	repeated := fs.BoolP("repeated", "d", false, "only print duplicate lines, one for each group")
	unique := fs.BoolP("unique", "u", false, "only print unique lines")
	ignoreCase := fs.BoolP("ignore-case", "i", false, "ignore differences in case when comparing")
	allRepeated := fs.StringP("all-repeated", "D", "", "print all duplicate lines; delimit-method may be none, prepend, or separate")
	fs.Lookup("all-repeated").NoOptDefVal = "none"
	group := fs.String("group", "", "show all items, separating groups with METHOD: separate, prepend, append, or both")
	fs.Lookup("group").NoOptDefVal = "separate"
	zero := fs.BoolP("zero-terminated", "z", false, "line delimiter is NUL, not newline")
	skipFields := fs.IntP("skip-fields", "f", 0, "avoid comparing the first N fields")
	skipChars := fs.IntP("skip-chars", "s", 0, "avoid comparing the first N characters")
	checkChars := fs.IntP("check-chars", "w", 0, "compare no more than N characters in lines")
	operands, code := tool.Parse(rc, cmd, fs, normalizeArgs(tool.AliasHelpVersion(args)))
	if code >= 0 {
		return code
	}

	if *skipFields < 0 {
		return tool.UsageError(rc, cmd, "invalid number of fields to skip: '%d'", *skipFields)
	}
	if *skipChars < 0 {
		return tool.UsageError(rc, cmd, "invalid number of bytes to skip: '%d'", *skipChars)
	}
	if *checkChars < 0 {
		return tool.UsageError(rc, cmd, "invalid number of bytes to compare: '%d'", *checkChars)
	}
	if len(operands) > 2 {
		return tool.UsageError(rc, cmd, "extra operand '%s'", operands[2])
	}
	delimMode := delimNone
	if fs.Changed("group") {
		if *count || *repeated || fs.Changed("all-repeated") || *unique {
			return tool.UsageError(rc, cmd, "--group is mutually exclusive with -c, -d, -D, and -u")
		}
		var ok bool
		delimMode, ok = parseDelimMode(*group, true)
		if !ok {
			return tool.UsageError(rc, cmd, "invalid group method %q", *group)
		}
	} else if fs.Changed("all-repeated") {
		var ok bool
		delimMode, ok = parseDelimMode(*allRepeated, false)
		if !ok {
			return tool.UsageError(rc, cmd, "invalid delimit method %q", *allRepeated)
		}
		if *count {
			return tool.UsageError(rc, cmd, "printing all duplicated lines and repeat counts is meaningless")
		}
		*repeated = true
	}

	input := "-"
	if len(operands) > 0 {
		input = operands[0]
	}
	lineEnd := byte('\n')
	if *zero {
		lineEnd = 0
	}
	lines, err := readLines(rc, input, lineEnd)
	if err != nil {
		fmt.Fprintf(rc.Err, "uniq: %s: %v\n", input, pathErr(err))
		return 1
	}

	var w io.Writer = rc.Out
	if len(operands) == 2 && operands[1] != "-" {
		f, err := os.Create(rc.Path(operands[1]))
		if err != nil {
			fmt.Fprintf(rc.Err, "uniq: %s: %v\n", operands[1], pathErr(err))
			return 1
		}
		defer f.Close()
		w = f
	}
	bw := bufio.NewWriter(w)

	limitChars := fs.Changed("check-chars")
	keyOf := func(line string) string {
		k := skipKey(line, *skipFields, *skipChars)
		if limitChars && len(k) > *checkChars {
			k = k[:*checkChars]
		}
		return k
	}
	equal := func(a, b string) bool {
		if *ignoreCase {
			return asciiEqualFold(a, b)
		}
		return a == b
	}

	writeTerm := func() {
		_ = bw.WriteByte(lineEnd)
	}
	writeLine := func(line string, n int) {
		if *count {
			fmt.Fprintf(bw, "%7d %s", n, line)
		} else {
			fmt.Fprint(bw, line)
		}
		writeTerm()
	}
	shouldPrint := func(group []string) bool {
		n := len(group)
		// GNU shouldPrint: -d drops singleton groups, -u drops repeated
		// groups (so -d -u prints nothing).
		if (*repeated && n == 1) || (*unique && n > 1) {
			return false
		}
		return true
	}
	flush := func(group []string) bool {
		n := len(group)
		if !shouldPrint(group) {
			return false
		}
		if fs.Changed("group") || fs.Changed("all-repeated") {
			for _, line := range group {
				writeLine(line, n)
			}
		} else {
			writeLine(group[0], n)
		}
		return true
	}

	var groupLines []string
	firstPrinted := false
	var prevKey string
	for _, line := range lines {
		k := keyOf(line)
		if len(groupLines) > 0 && equal(prevKey, k) {
			groupLines = append(groupLines, line)
			continue
		}
		if len(groupLines) > 0 {
			firstPrinted = flushWithDelimiter(bw, groupLines, shouldPrint(groupLines), flush, delimMode, firstPrinted, lineEnd)
		}
		groupLines, prevKey = []string{line}, k
	}
	if len(groupLines) > 0 {
		firstPrinted = flushWithDelimiter(bw, groupLines, shouldPrint(groupLines), flush, delimMode, firstPrinted, lineEnd)
	}
	if err := bw.Flush(); err != nil {
		fmt.Fprintf(rc.Err, "uniq: write failed: %v\n", err)
		return 1
	}
	return 0
}

// normalizeArgs supports GNU's -D[delimit-method] spelling and POSIX's
// obsolescent -N/+N forms for skipping fields/chars.
func normalizeArgs(args []string) []string {
	out := make([]string, 0, len(args))
	rest := false
	needsValue := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			rest = true
			out = append(out, arg)
			needsValue = false
			continue
		}
		if rest {
			out = append(out, arg)
			continue
		}
		if needsValue {
			out = append(out, arg)
			needsValue = false
			continue
		}
		if len(arg) > 2 && arg[0] == '-' && arg[1] == 'D' && arg[2] != '=' {
			out = append(out, "-D="+arg[2:])
			continue
		}
		// uutils documents the delimiter method as a separate argument to
		// -D, while GNU also accepts the optional argument attached to it.
		// Consume only recognized methods so an ordinary operand after -D
		// keeps its normal meaning.
		if arg == "-D" && i+1 < len(args) && isDelimiterMethod(args[i+1]) {
			out = append(out, "-D="+args[i+1])
			i++
			continue
		}
		if legacySkipNumber(arg) {
			switch arg[0] {
			case '-':
				out = append(out, "-f", arg[1:])
			case '+':
				out = append(out, "-s", arg[1:])
			}
			continue
		}
		out = append(out, arg)
		needsValue = optionNeedsValue(arg)
	}
	return out
}

func optionNeedsValue(arg string) bool {
	switch arg {
	case "-f", "--skip-fields", "-s", "--skip-chars", "-w", "--check-chars":
		return true
	default:
		return false
	}
}

func legacySkipNumber(arg string) bool {
	if len(arg) < 2 || (arg[0] != '-' && arg[0] != '+') {
		return false
	}
	for i := 1; i < len(arg); i++ {
		if arg[i] < '0' || arg[i] > '9' {
			return false
		}
	}
	return true
}

func isDelimiterMethod(s string) bool {
	switch s {
	case "none", "prepend", "separate":
		return true
	default:
		return false
	}
}

type delimiterMode int

const (
	delimNone delimiterMode = iota
	delimPrepend
	delimSeparate
	delimAppend
	delimBoth
)

func parseDelimMode(s string, group bool) (delimiterMode, bool) {
	switch s {
	case "", "none":
		return delimNone, !group
	case "prepend":
		return delimPrepend, true
	case "separate":
		return delimSeparate, true
	case "append":
		return delimAppend, group
	case "both":
		return delimBoth, group
	default:
		return delimNone, false
	}
}

func flushWithDelimiter(w *bufio.Writer, group []string, shouldPrint bool, flush func([]string) bool, mode delimiterMode, firstPrinted bool, lineEnd byte) bool {
	if !shouldPrint {
		return firstPrinted
	}
	printed := false
	if mode == delimPrepend || mode == delimBoth || (mode == delimSeparate && firstPrinted) {
		_ = w.WriteByte(lineEnd)
	}
	printed = flush(group)
	if printed && (mode == delimAppend || mode == delimBoth) {
		_ = w.WriteByte(lineEnd)
	}
	return firstPrinted || printed
}

// skipKey mirrors GNU uniq's find_field: skip N fields (each a run of
// blanks followed by a run of non-blanks), then N characters, clamped
// to the end of the line.
func skipKey(line string, fields, chars int) string {
	i := 0
	for n := 0; n < fields && i < len(line); n++ {
		for i < len(line) && isBlank(line[i]) {
			i++
		}
		for i < len(line) && !isBlank(line[i]) {
			i++
		}
	}
	i += chars
	if i > len(line) {
		i = len(line)
	}
	return line[i:]
}

func isBlank(c byte) bool { return c == ' ' || c == '\t' }

// asciiEqualFold is C-locale case-insensitive equality (bytewise ASCII
// upcasing — deliberately not Unicode folding).
func asciiEqualFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		if upperByte(a[i]) != upperByte(b[i]) {
			return false
		}
	}
	return true
}

func upperByte(c byte) byte {
	if c >= 'a' && c <= 'z' {
		return c - ('a' - 'A')
	}
	return c
}

func readLines(rc *tool.RunContext, operand string, lineEnd byte) ([]string, error) {
	var data []byte
	var err error
	if operand == "-" {
		data, err = io.ReadAll(rc.In)
	} else {
		data, err = os.ReadFile(rc.Path(operand))
	}
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}
	s := strings.TrimSuffix(string(data), string([]byte{lineEnd}))
	return strings.Split(s, string([]byte{lineEnd})), nil
}

func pathErr(err error) error {
	return tool.SysErr(err)
}
