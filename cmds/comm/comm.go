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
	for idx, a := range args {
		if a == "--" {
			rest = append(rest, args[idx:]...)
			break
		}
		if len(a) > 1 && a[0] == '-' && a[1] != '-' {
			hasDigits := false
			for i := 1; i < len(a); i++ {
				if a[i] >= '1' && a[i] <= '3' {
					hasDigits = true
					break
				}
			}
			if hasDigits {
				var rebuilt strings.Builder
				rebuilt.WriteByte('-')
				for i := 1; i < len(a); i++ {
					if a[i] >= '1' && a[i] <= '3' {
						suppress[a[i]-'0'] = true
					} else {
						rebuilt.WriteByte(a[i])
					}
				}
				if rebuilt.Len() > 1 {
					rest = append(rest, rebuilt.String())
				}
				continue
			}
		}
		rest = append(rest, a)
	}

	fs := tool.NewFlags(cmd.Name)
	checkOrder := fs.Bool("check-order", false, "check that the input is correctly sorted")
	nocheckOrder := fs.Bool("nocheck-order", false, "do not check that the input is correctly sorted")
	outputDelimiter := fs.String("output-delimiter", "", "separate columns with STR")
	total := fs.Bool("total", false, "output a summary")
	zeroTerminated := fs.BoolP("zero-terminated", "z", false, "line delimiter is NUL, not newline")

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
		ls, err := readLines(rc, op, *zeroTerminated)
		if err != nil {
			fmt.Fprintf(rc.Err, "comm: %s: %v\n", op, pathErr(err))
			return 1
		}
		lines[i] = ls
	}

	delim := "\t"
	if fs.Changed("output-delimiter") {
		delim = *outputDelimiter
	}

	var colCounts [4]int
	bw := bufio.NewWriter(rc.Out)
	emit := func(col int, line string) {
		colCounts[col]++
		if suppress[col] {
			return
		}
		tabs := 0
		for c := 1; c < col; c++ {
			if !suppress[c] {
				tabs++
			}
		}
		bw.WriteString(strings.Repeat(delim, tabs))
		bw.WriteString(line)
		if *zeroTerminated {
			bw.WriteByte('\x00')
		} else {
			bw.WriteByte('\n')
		}
	}

	// GNU default order checking: each newly consumed line is compared
	// with its predecessor in the same file, but a disorder is only
	// diagnosed (once per file) after an unpairable line has been seen.
	seenUnpairable := false
	var warned [2]bool
	i, j := 0, 0
	advance := func(file int) bool {
		pos := &i
		if file == 2 {
			pos = &j
		}
		*pos++
		ls := lines[file-1]
		if *pos < len(ls) && ls[*pos-1] > ls[*pos] {
			if !*nocheckOrder {
				if *checkOrder {
					fmt.Fprintf(rc.Err, "comm: file %d is not in sorted order\n", file)
					return false
				}
				if seenUnpairable && !warned[file-1] {
					fmt.Fprintf(rc.Err, "comm: file %d is not in sorted order\n", file)
					warned[file-1] = true
				}
			}
		}
		return true
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
			if !advance(1) {
				return 1
			}
		case d > 0:
			seenUnpairable = true
			emit(2, lines[1][j])
			if !advance(2) {
				return 1
			}
		default:
			emit(3, lines[0][i])
			if !advance(1) || !advance(2) {
				return 1
			}
		}
	}
	if *total {
		c1 := colCounts[1]
		if suppress[1] {
			c1 = 0
		}
		c2 := colCounts[2]
		if suppress[2] {
			c2 = 0
		}
		c3 := colCounts[3]
		if suppress[3] {
			c3 = 0
		}
		sum := c1 + c2 + c3
		fmt.Fprintf(bw, "%d%s%d%s%d%s%d total", c1, delim, c2, delim, c3, delim, sum)
		if *zeroTerminated {
			bw.WriteByte('\x00')
		} else {
			bw.WriteByte('\n')
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

func readLines(rc *tool.RunContext, operand string, zeroTerminated bool) ([]string, error) {
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
	sep := "\n"
	if zeroTerminated {
		sep = "\x00"
	}
	return strings.Split(strings.TrimSuffix(string(data), sep), sep), nil
}

func pathErr(err error) error {
	return tool.SysErr(err)
}
