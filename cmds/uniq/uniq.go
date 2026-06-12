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
	skipFields := fs.IntP("skip-fields", "f", 0, "avoid comparing the first N fields")
	skipChars := fs.IntP("skip-chars", "s", 0, "avoid comparing the first N characters")
	checkChars := fs.IntP("check-chars", "w", 0, "compare no more than N characters in lines")
	operands, code := tool.Parse(rc, cmd, fs, args)
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

	input := "-"
	if len(operands) > 0 {
		input = operands[0]
	}
	lines, err := readLines(rc, input)
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

	flush := func(first string, n int) {
		// GNU shouldPrint: -d drops singleton groups, -u drops repeated
		// groups (so -d -u prints nothing).
		if (*repeated && n == 1) || (*unique && n > 1) {
			return
		}
		if *count {
			fmt.Fprintf(bw, "%7d %s\n", n, first)
		} else {
			fmt.Fprintf(bw, "%s\n", first)
		}
	}

	groupN := 0
	var first, prevKey string
	for _, line := range lines {
		k := keyOf(line)
		if groupN > 0 && equal(prevKey, k) {
			groupN++
			continue
		}
		if groupN > 0 {
			flush(first, groupN)
		}
		first, prevKey, groupN = line, k, 1
	}
	if groupN > 0 {
		flush(first, groupN)
	}
	if err := bw.Flush(); err != nil {
		fmt.Fprintf(rc.Err, "uniq: write failed: %v\n", err)
		return 1
	}
	return 0
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

func readLines(rc *tool.RunContext, operand string) ([]string, error) {
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
	return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n"), nil
}

func pathErr(err error) error {
	if pe, ok := err.(*os.PathError); ok {
		return pe.Err
	}
	return err
}
