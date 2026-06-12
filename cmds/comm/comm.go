// Package commcmd implements comm(1) per the GNU coreutils manual:
// compare two sorted files line by line, producing three-column
// output — lines unique to FILE1, lines unique to FILE2, and lines
// common to both — with each output column indented by one TAB per
// column printed before it.
//
// Implemented flags: -1 -2 -3 (GNU defines no long forms for these, so
// they are pre-parsed manually). Comparison is C-locale byte order.
// GNU's default order checking is preserved: wrongly sorted inputs are
// diagnosed only when an input file contains unpairable lines, and the
// run then fails with "input is not in sorted order".
package commcmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "comm",
	Synopsis: "Compare two sorted files line by line.",
	Usage: "comm [OPTION]... FILE1 FILE2\n\n" +
		"When FILE1 or FILE2 (not both) is -, read standard input.\n\n" +
		"With no options, produce three-column output. Column one contains\n" +
		"lines unique to FILE1, column two lines unique to FILE2, and column\n" +
		"three lines common to both files.\n\n" +
		"  -1   suppress column 1 (lines unique to FILE1)\n" +
		"  -2   suppress column 2 (lines unique to FILE2)\n" +
		"  -3   suppress column 3 (lines that appear in both files)",
}

// Run is wired in init: a literal would create an initialization
// cycle (run's flag-error paths reference cmd).
func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	// GNU comm's -1 -2 -3 have no long forms; pre-parse them manually
	// (clusters like -12 included) and hand everything else to the
	// framework parser.
	var suppress [4]bool
	rest := make([]string, 0, len(args))
preparse:
	for idx, a := range args {
		if a == "--" {
			rest = append(rest, args[idx:]...)
			break
		}
		if len(a) > 1 && a[0] == '-' && a[1] != '-' {
			for _, c := range []byte(a[1:]) {
				if c < '1' || c > '3' {
					fmt.Fprintf(rc.Err, "comm: unknown shorthand flag: %q in %s\n", string(c), a)
					fmt.Fprintf(rc.Err, "comm: not every GNU flag is implemented in pure-Go coreutils — see 'comm --help' for the supported subset\n")
					return 2
				}
			}
			for _, c := range []byte(a[1:]) {
				suppress[c-'0'] = true
			}
			continue preparse
		}
		rest = append(rest, a)
	}

	fs := tool.NewFlags(cmd.Name)
	operands, code := tool.Parse(rc, cmd, fs, rest)
	if code >= 0 {
		return code
	}
	switch {
	case len(operands) == 0:
		return tool.UsageError(rc, cmd, "missing operand")
	case len(operands) == 1:
		return tool.UsageError(rc, cmd, "missing operand after '%s'", operands[0])
	case len(operands) > 2:
		return tool.UsageError(rc, cmd, "extra operand '%s'", operands[2])
	}

	var lines [2][]string
	for i, op := range operands {
		ls, err := readLines(rc, op)
		if err != nil {
			fmt.Fprintf(rc.Err, "comm: %s: %v\n", op, pathErr(err))
			return 1
		}
		lines[i] = ls
	}

	bw := bufio.NewWriter(rc.Out)
	emit := func(col int, line string) {
		if suppress[col] {
			return
		}
		tabs := 0
		for c := 1; c < col; c++ {
			if !suppress[c] {
				tabs++
			}
		}
		bw.WriteString(strings.Repeat("\t", tabs))
		bw.WriteString(line)
		bw.WriteByte('\n')
	}

	// GNU default order checking: each newly consumed line is compared
	// with its predecessor in the same file, but a disorder is only
	// diagnosed (once per file) after an unpairable line has been seen.
	seenUnpairable := false
	var warned [2]bool
	i, j := 0, 0
	advance := func(file int) {
		pos := &i
		if file == 2 {
			pos = &j
		}
		*pos++
		ls := lines[file-1]
		if *pos < len(ls) && seenUnpairable && !warned[file-1] && ls[*pos-1] > ls[*pos] {
			fmt.Fprintf(rc.Err, "comm: file %d is not in sorted order\n", file)
			warned[file-1] = true
		}
	}
	for i < len(lines[0]) || j < len(lines[1]) {
		var d int
		switch {
		case i >= len(lines[0]):
			d = 1
		case j >= len(lines[1]):
			d = -1
		default:
			d = strings.Compare(lines[0][i], lines[1][j])
		}
		switch {
		case d < 0:
			seenUnpairable = true
			emit(1, lines[0][i])
			advance(1)
		case d > 0:
			seenUnpairable = true
			emit(2, lines[1][j])
			advance(2)
		default:
			emit(3, lines[0][i])
			advance(1)
			advance(2)
		}
	}
	if err := bw.Flush(); err != nil {
		fmt.Fprintf(rc.Err, "comm: write failed: %v\n", err)
		return 1
	}
	if warned[0] || warned[1] {
		fmt.Fprintln(rc.Err, "comm: input is not in sorted order")
		return 1
	}
	return 0
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
