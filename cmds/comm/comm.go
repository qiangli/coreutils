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
	if operands[0] == "-" && operands[1] == "-" {
		return tool.UsageError(rc, cmd, "both files cannot be standard input")
	}

	var inputs [2]*recordReader
	for i, op := range operands {
		input, err := openRecordReader(rc, op, *zeroTerminated)
		if err != nil {
			fmt.Fprintf(rc.Err, "comm: %s: %v\n", op, pathErr(err))
			if inputs[0] != nil {
				inputs[0].close()
			}
			return 1
		}
		inputs[i] = input
	}
	defer inputs[0].close()
	defer inputs[1].close()

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
	var current [2]string
	var have [2]bool
	advance := func(file int) bool {
		idx := file - 1
		previous, hadPrevious := current[idx], have[idx]
		next, ok, err := inputs[idx].next()
		if err != nil {
			fmt.Fprintf(rc.Err, "comm: %s: %v\n", operands[idx], pathErr(err))
			return false
		}
		current[idx], have[idx] = next, ok
		if hadPrevious && ok && previous > next {
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
	if !advance(1) || !advance(2) {
		return 1
	}
	for have[0] || have[1] {
		var d int
		switch {
		case !have[0]:
			d = 1
		case !have[1]:
			d = -1
		default:
			d = strings.Compare(current[0], current[1])
		}
		switch {
		case d < 0:
			seenUnpairable = true
			emit(1, current[0])
			if !advance(1) {
				return 1
			}
		case d > 0:
			seenUnpairable = true
			emit(2, current[1])
			if !advance(2) {
				return 1
			}
		default:
			emit(3, current[0])
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

type recordReader struct {
	reader    *bufio.Reader
	closer    io.Closer
	delimiter byte
}

func openRecordReader(rc *tool.RunContext, operand string, zeroTerminated bool) (*recordReader, error) {
	var input io.Reader
	var closer io.Closer
	if operand == "-" {
		input = rc.In
	} else {
		file, err := os.Open(rc.Path(operand))
		if err != nil {
			return nil, err
		}
		input, closer = file, file
	}
	delimiter := byte('\n')
	if zeroTerminated {
		delimiter = 0
	}
	return &recordReader{reader: bufio.NewReader(input), closer: closer, delimiter: delimiter}, nil
}

func (r *recordReader) next() (string, bool, error) {
	record, err := r.reader.ReadString(r.delimiter)
	if len(record) > 0 {
		if record[len(record)-1] == r.delimiter {
			record = record[:len(record)-1]
		}
		if err == io.EOF {
			err = nil
		}
		return record, true, err
	}
	if err == io.EOF {
		return "", false, nil
	}
	return "", false, err
}

func (r *recordReader) close() {
	if r.closer != nil {
		_ = r.closer.Close()
	}
}

func pathErr(err error) error {
	return tool.SysErr(err)
}
