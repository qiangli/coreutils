// Package joincmd implements join(1) per the GNU coreutils manual:
// for each pair of input lines with identical join fields, write a
// line to standard output. The default join field is the first,
// delimited by blanks.
//
// Implemented flags: -1 FIELD, -2 FIELD, -t CHAR, -a FILENUM,
// -v FILENUM, -i/--ignore-case. GNU defines no long forms for the
// first five, so they are pre-parsed manually. Both inputs must be
// sorted on the join field; like GNU, a disorder is diagnosed
// ("FILE:LINENO: is not sorted: LINE", then a fatal "input is not in
// sorted order") only when it can matter — i.e. once unpairable lines
// have been seen. Comparison is C-locale byte order, with -i folding
// ASCII case.
package joincmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "join",
	Synopsis: "Join lines of two files on a common field.",
	Usage: "join [OPTION]... FILE1 FILE2\n\n" +
		"When FILE1 or FILE2 (not both) is -, read standard input.\n\n" +
		"  -1 FIELD     join on this FIELD of file 1\n" +
		"  -2 FIELD     join on this FIELD of file 2\n" +
		"  -a FILENUM   also print unpairable lines from file FILENUM,\n" +
		"               where FILENUM is 1 or 2\n" +
		"  -v FILENUM   like -a FILENUM, but suppress joined output lines\n" +
		"  -t CHAR      use CHAR as input and output field separator\n" +
		"  -i           like --ignore-case",
}

// Run is wired in init: a literal would create an initialization
// cycle (run's flag-error paths reference cmd).
func init() { cmd.Run = run; tool.Register(cmd) }

const (
	tabDefault   = -1 // fields delimited by blanks, leading blanks ignored
	tabWholeLine = -2 // -t '': the whole line is the join field
)

// options collects everything the manual pre-parse owns, plus the
// shared seen-unpairable state that gates GNU's default order check.
type options struct {
	field          [2]int // 0-based join fields
	tab            int    // tabDefault, tabWholeLine, or a separator byte
	printU         [2]bool
	suppressed     [2]bool // -v given for that file
	ignoreCase     bool
	seenUnpairable bool
}

func run(rc *tool.RunContext, args []string) int {
	opt := options{tab: tabDefault}
	contractErr := func(format string, a ...any) int {
		fmt.Fprintf(rc.Err, "join: %s\n", fmt.Sprintf(format, a...))
		fmt.Fprintf(rc.Err, "join: not every GNU flag is implemented in pure-Go coreutils — see 'join --help' for the supported subset\n")
		return 2
	}

	// GNU join's -1 -2 -a -v -t (and -e -j -o, which we don't implement)
	// have no long forms; pre-parse the supported ones manually, with
	// getopt semantics: the value is the rest of the cluster, or else
	// the next argument.
	rest := make([]string, 0, len(args))
	for idx := 0; idx < len(args); idx++ {
		a := args[idx]
		if a == "--" {
			rest = append(rest, args[idx:]...)
			break
		}
		if len(a) < 2 || a[0] != '-' || strings.HasPrefix(a, "--") {
			rest = append(rest, a)
			continue
		}
		body := a[1:]
	cluster:
		for j := 0; j < len(body); j++ {
			c := body[j]
			switch c {
			case 'i':
				opt.ignoreCase = true
			case '1', '2', 'a', 'v', 't':
				var val string
				if j+1 < len(body) {
					val = body[j+1:]
				} else {
					idx++
					if idx >= len(args) {
						return tool.UsageError(rc, cmd, "option requires an argument -- '%c'", c)
					}
					val = args[idx]
				}
				if code := opt.apply(rc, c, val); code >= 0 {
					return code
				}
				break cluster
			default:
				return contractErr("unknown shorthand flag: %q in -%s", string(c), body)
			}
		}
	}

	fs := tool.NewFlags(cmd.Name)
	ignoreCaseLong := fs.Bool("ignore-case", false, "ignore differences in case when comparing fields")
	operands, code := tool.Parse(rc, cmd, fs, rest)
	if code >= 0 {
		return code
	}
	opt.ignoreCase = opt.ignoreCase || *ignoreCaseLong

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

	printJoined := !(opt.suppressed[0] || opt.suppressed[1])
	var files [2]*fileState
	for i, op := range operands {
		lines, err := readLines(rc, op)
		if err != nil {
			fmt.Fprintf(rc.Err, "join: %s: %v\n", op, pathErr(err))
			return 1
		}
		files[i] = &fileState{name: op, lines: lines, field: opt.field[i], idx: i, opt: &opt, rc: rc}
	}

	bw := bufio.NewWriter(rc.Out)
	osep := " "
	if opt.tab >= 0 {
		osep = string([]byte{byte(opt.tab)})
	}

	emit := func(parts []string) {
		bw.WriteString(strings.Join(parts, osep))
		bw.WriteByte('\n')
	}
	emitUnpaired := func(f *fileState, from, to int) {
		opt.seenUnpairable = true
		if !opt.printU[f.idx] {
			return
		}
		for _, l := range f.lines[from:to] {
			flds := opt.splitFields(l)
			emit(append([]string{fieldAt(flds, f.field)}, otherFields(flds, f.field)...))
		}
	}

	f1, f2 := files[0], files[1]
	g1, g2 := f1.nextGroup(), f2.nextGroup()
	for g1 != nil && g2 != nil {
		d := opt.compareKeys(g1.key, g2.key)
		switch {
		case d < 0:
			emitUnpaired(f1, g1.start, g1.end)
			g1 = f1.nextGroup()
		case d > 0:
			emitUnpaired(f2, g2.start, g2.end)
			g2 = f2.nextGroup()
		default:
			if printJoined {
				for _, l1 := range f1.lines[g1.start:g1.end] {
					flds1 := opt.splitFields(l1)
					head := append([]string{fieldAt(flds1, f1.field)}, otherFields(flds1, f1.field)...)
					for _, l2 := range f2.lines[g2.start:g2.end] {
						flds2 := opt.splitFields(l2)
						emit(append(append([]string{}, head...), otherFields(flds2, f2.field)...))
					}
				}
			}
			g1, g2 = f1.nextGroup(), f2.nextGroup()
		}
	}
	for g1 != nil {
		emitUnpaired(f1, g1.start, g1.end)
		g1 = f1.nextGroup()
	}
	for g2 != nil {
		emitUnpaired(f2, g2.start, g2.end)
		g2 = f2.nextGroup()
	}

	if err := bw.Flush(); err != nil {
		fmt.Fprintf(rc.Err, "join: write failed: %v\n", err)
		return 1
	}
	if f1.warned || f2.warned {
		fmt.Fprintln(rc.Err, "join: input is not in sorted order")
		return 1
	}
	return 0
}

// apply handles one pre-parsed flag value; returns -1 to proceed.
func (o *options) apply(rc *tool.RunContext, c byte, val string) int {
	switch c {
	case '1', '2':
		n, ok := parsePositive(val)
		if !ok {
			return tool.UsageError(rc, cmd, "invalid field number: '%s'", val)
		}
		o.field[c-'1'] = n - 1
	case 'a', 'v':
		if val != "1" && val != "2" {
			return tool.UsageError(rc, cmd, "invalid file number: '%s'", val)
		}
		i := val[0] - '1'
		o.printU[i] = true
		if c == 'v' {
			o.suppressed[i] = true
		}
	case 't':
		switch {
		case val == "":
			o.tab = tabWholeLine
		case val == `\0`:
			o.tab = 0
		case len(val) == 1:
			o.tab = int(val[0])
		default:
			return tool.UsageError(rc, cmd, "multi-character tab '%s'", val)
		}
	}
	return -1
}

func parsePositive(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
		n = n*10 + int(s[i]-'0')
	}
	return n, n > 0
}

func isBlank(c byte) bool { return c == ' ' || c == '\t' }

// splitFields splits one line per the active separator mode: default
// is runs of blanks with leading blanks ignored; -t CHAR makes every
// occurrence significant (empty fields preserved); -t ” makes the
// whole line one field.
func (o *options) splitFields(line string) []string {
	switch {
	case o.tab == tabWholeLine:
		return []string{line}
	case o.tab >= 0:
		return strings.Split(line, string([]byte{byte(o.tab)}))
	default:
		var out []string
		i := 0
		for i < len(line) {
			for i < len(line) && isBlank(line[i]) {
				i++
			}
			start := i
			for i < len(line) && !isBlank(line[i]) {
				i++
			}
			if i > start {
				out = append(out, line[start:i])
			}
		}
		return out
	}
}

func fieldAt(fields []string, idx int) string {
	if idx < len(fields) {
		return fields[idx]
	}
	return ""
}

func otherFields(fields []string, idx int) []string {
	out := make([]string, 0, len(fields))
	for i, f := range fields {
		if i != idx {
			out = append(out, f)
		}
	}
	return out
}

func upperByte(c byte) byte {
	if c >= 'a' && c <= 'z' {
		return c - ('a' - 'A')
	}
	return c
}

func (o *options) compareKeys(a, b string) int {
	if !o.ignoreCase {
		return strings.Compare(a, b)
	}
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		ca, cb := upperByte(a[i]), upperByte(b[i])
		switch {
		case ca < cb:
			return -1
		case ca > cb:
			return 1
		}
	}
	switch {
	case len(a) < len(b):
		return -1
	case len(a) > len(b):
		return 1
	}
	return 0
}

// fileState walks one input as runs of equal-keyed lines, performing
// the GNU order check as each line is first touched.
type fileState struct {
	name    string
	lines   []string
	field   int
	idx     int // 0 = FILE1, 1 = FILE2
	opt     *options
	rc      *tool.RunContext
	pos     int
	checked int // highest line index order-checked so far
	warned  bool
}

type group struct {
	key        string
	start, end int
}

func (f *fileState) keyOf(i int) string {
	return fieldAt(f.opt.splitFields(f.lines[i]), f.field)
}

// touch order-checks lines up to index i (inclusive). Mirrors GNU:
// the diagnostic fires only after unpairable lines have been seen,
// and only once per file.
func (f *fileState) touch(i int) {
	for f.checked < i {
		f.checked++
		if f.opt.seenUnpairable && !f.warned &&
			f.opt.compareKeys(f.keyOf(f.checked-1), f.keyOf(f.checked)) > 0 {
			fmt.Fprintf(f.rc.Err, "join: %s:%d: is not sorted: %s\n", f.name, f.checked+1, f.lines[f.checked])
			f.warned = true
		}
	}
}

// nextGroup returns the next run of lines whose join field compares
// equal, or nil at end of input.
func (f *fileState) nextGroup() *group {
	if f.pos >= len(f.lines) {
		return nil
	}
	g := &group{start: f.pos, key: f.keyOf(f.pos)}
	f.touch(f.pos)
	i := f.pos + 1
	for i < len(f.lines) {
		f.touch(i) // GNU reads (and order-checks) the line that ends the group
		if f.opt.compareKeys(f.keyOf(i), g.key) != 0 {
			break
		}
		i++
	}
	g.end = i
	f.pos = i
	return g
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
