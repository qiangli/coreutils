// Package sortcmd implements sort(1) per the GNU coreutils manual:
// sort lines of text files.
//
// Comparisons are C-locale byte comparisons (LC_ALL=C semantics).
// Implemented flags: -r -n -u -f -b -k POS1[,POS2] (with .CHAR offsets
// and per-key type letters n/b/f/r/h), -t SEP, -o FILE, -s, -c, -h.
// GNU's last-resort whole-line comparison applies unless -s/-u, and the
// global ordering options are inherited by keys that carry no options
// of their own, exactly as documented in the manual.
package sortcmd

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"unsafe"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "sort",
	Synopsis: "Write sorted concatenation of all FILE(s) to standard output.",
	Usage:    "sort [OPTION]... [FILE]...\n\nWith no FILE, or when FILE is -, read standard input.",
}

// Run is wired in init: a literal would create an initialization
// cycle (run's flag-error paths reference cmd).
func init() { cmd.Run = run; tool.Register(cmd) }

// sorter is one invocation's comparison configuration.
type sorter struct {
	keys    []keySpec
	tab     int // field separator byte; -1 = blank/non-blank transition
	reverse bool
	stable  bool
	unique  bool
}

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	blanks := fs.BoolP("ignore-leading-blanks", "b", false, "ignore leading blanks")
	check := fs.BoolP("check", "c", false, "check for sorted input; do not sort")
	fold := fs.BoolP("ignore-case", "f", false, "fold lower case to upper case characters")
	human := fs.BoolP("human-numeric-sort", "h", false, "compare human readable numbers (e.g., 2K 1G)")
	keyDefs := fs.StringArrayP("key", "k", nil, "sort via a key; KEYDEF gives location and type")
	numeric := fs.BoolP("numeric-sort", "n", false, "compare according to string numerical value")
	output := fs.StringP("output", "o", "", "write result to FILE instead of standard output")
	reverse := fs.BoolP("reverse", "r", false, "reverse the result of comparisons")
	stable := fs.BoolP("stable", "s", false, "stabilize sort by disabling last-resort comparison")
	sep := fs.StringP("field-separator", "t", "", "use SEP instead of non-blank to blank transition")
	unique := fs.BoolP("unique", "u", false, "with -c, check for strict ordering; without -c, output only the first of an equal run")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}

	if *numeric && *human {
		fmt.Fprintf(rc.Err, "sort: options '-hn' are incompatible\n")
		return 2
	}

	s := &sorter{tab: -1, reverse: *reverse, stable: *stable, unique: *unique}
	if fs.Changed("field-separator") {
		switch {
		case *sep == "":
			fmt.Fprintf(rc.Err, "sort: empty tab\n")
			return 2
		case *sep == `\0`:
			s.tab = 0
		case len(*sep) == 1:
			s.tab = int((*sep)[0])
		default:
			fmt.Fprintf(rc.Err, "sort: multi-character tab '%s'\n", *sep)
			return 2
		}
	}

	for _, def := range *keyDefs {
		k, errMsg, badType := parseKeySpec(def)
		if badType != 0 {
			return tool.NotSupported(rc, cmd, fmt.Sprintf("key type letter '%c' (in '-k %s')", badType, def))
		}
		if errMsg != "" {
			fmt.Fprintf(rc.Err, "sort: %s: invalid field specification '%s'\n", errMsg, def)
			return 2
		}
		if k.opts.numeric && k.opts.human {
			fmt.Fprintf(rc.Err, "sort: options '-hn' are incompatible\n")
			return 2
		}
		s.keys = append(s.keys, k)
	}

	// GNU inheritance: a key with no ordering options of its own (and no
	// per-key r) takes all the global ordering options. When no key is
	// given at all and any global ordering option is set, the whole line
	// becomes one key carrying the global options.
	gOpts := keyOpts{numeric: *numeric, human: *human, fold: *fold, skipSB: *blanks, skipEB: *blanks, reverse: *reverse}
	for i := range s.keys {
		k := &s.keys[i]
		if !k.opts.hasMods() && !k.opts.reverse {
			k.opts = gOpts
		}
	}
	if len(s.keys) == 0 && gOpts.hasMods() {
		s.keys = append(s.keys, keySpec{sword: 0, schar: 0, eword: -1, echar: 0, opts: gOpts})
	}

	if len(operands) == 0 {
		operands = []string{"-"}
	}

	if *check {
		if *output != "" {
			fmt.Fprintf(rc.Err, "sort: options '-co' are incompatible\n")
			return 2
		}
		if len(operands) > 1 {
			fmt.Fprintf(rc.Err, "sort: extra operand '%s' not allowed with -c\n", operands[1])
			return 2
		}
		return s.checkSorted(rc, operands[0])
	}

	if s.canUsePreparedNumeric() {
		var nlines numericLineSet
		for _, op := range operands {
			if err := nlines.read(rc, op); err != nil {
				fmt.Fprintf(rc.Err, "sort: cannot read: %s: %v\n", op, pathErr(err))
				return 2
			}
		}
		slices.SortStableFunc(nlines.lines, s.compareNumericLines)
		if s.unique {
			out := nlines.lines[:0]
			for i, l := range nlines.lines {
				if i == 0 {
					out = append(out, l)
					continue
				}
				prev := out[len(out)-1]
				if compareNumericKey(prev.text, prev.num, l.text, l.num) != 0 {
					out = append(out, l)
				}
			}
			nlines.lines = out
		}
		return writeNumericLines(rc, *output, nlines.lines)
	}

	var lines lineSet
	for _, op := range operands {
		if err := lines.read(rc, op); err != nil {
			fmt.Fprintf(rc.Err, "sort: cannot read: %s: %v\n", op, pathErr(err))
			return 2
		}
	}

	slices.SortStableFunc(lines.lines, s.compare)

	if s.unique {
		out := lines.lines[:0]
		for i, l := range lines.lines {
			if i == 0 || s.compareEqual(out[len(out)-1], l) != 0 {
				out = append(out, l)
			}
		}
		lines.lines = out
	}

	return writeStringLines(rc, *output, lines.lines)
}

func writeStringLines(rc *tool.RunContext, output string, lines []string) int {
	var w io.Writer = rc.Out
	if output != "" {
		f, err := os.Create(rc.Path(output))
		if err != nil {
			fmt.Fprintf(rc.Err, "sort: open failed: %s: %v\n", output, pathErr(err))
			return 2
		}
		defer f.Close()
		w = f
	}
	bw := bufio.NewWriter(w)
	for _, l := range lines {
		bw.WriteString(l)
		bw.WriteByte('\n')
	}
	if err := bw.Flush(); err != nil {
		fmt.Fprintf(rc.Err, "sort: write failed: %v\n", err)
		return 2
	}
	return 0
}

func writeNumericLines(rc *tool.RunContext, output string, lines []numericLine) int {
	var w io.Writer = rc.Out
	if output != "" {
		f, err := os.Create(rc.Path(output))
		if err != nil {
			fmt.Fprintf(rc.Err, "sort: open failed: %s: %v\n", output, pathErr(err))
			return 2
		}
		defer f.Close()
		w = f
	}
	bw := bufio.NewWriter(w)
	for _, l := range lines {
		bw.WriteString(l.text)
		bw.WriteByte('\n')
	}
	if err := bw.Flush(); err != nil {
		fmt.Fprintf(rc.Err, "sort: write failed: %v\n", err)
		return 2
	}
	return 0
}

func (s *sorter) canUsePreparedNumeric() bool {
	if len(s.keys) != 1 {
		return false
	}
	k := &s.keys[0]
	return k.sword == 0 && k.schar == 0 && k.eword == -1 && k.echar == 0 &&
		k.opts.numeric && !k.opts.human && !k.opts.fold && !k.opts.skipSB && !k.opts.skipEB
}

// compare is GNU sort's compare(): keys first; when all keys tie, the
// last-resort whole-line byte comparison applies unless -s or -u, with
// the global -r reversing its result.
func (s *sorter) compare(a, b string) int {
	if len(s.keys) > 0 {
		if d := s.compareKeys(a, b); d != 0 || s.stable || s.unique {
			return d
		}
	}
	d := strings.Compare(a, b)
	if s.reverse {
		return -d
	}
	return d
}

// compareEqual is the equality used by -u: keys when present, else the
// whole line byte-for-byte.
func (s *sorter) compareEqual(a, b string) int {
	if len(s.keys) > 0 {
		return s.compareKeys(a, b)
	}
	return strings.Compare(a, b)
}

func (s *sorter) compareKeys(a, b string) int {
	for i := range s.keys {
		k := &s.keys[i]
		ka := extractKey(a, k, s.tab)
		kb := extractKey(b, k, s.tab)
		var d int
		switch {
		case k.opts.numeric:
			d = numCompare(ka, kb)
		case k.opts.human:
			d = humanCompare(ka, kb)
		case k.opts.fold:
			d = foldCompare(ka, kb)
		default:
			d = strings.Compare(ka, kb)
		}
		if d != 0 {
			if k.opts.reverse {
				return -d
			}
			return d
		}
	}
	return 0
}

type numericKey struct {
	sign                           int8
	ipStart, ipLen, fpStart, fpLen int32
}

type numericLine struct {
	text string
	num  numericKey
}

type numericLineSet struct {
	buffers [][]byte
	lines   []numericLine
}

func (ls *numericLineSet) read(rc *tool.RunContext, operand string) error {
	data, err := readOperand(rc, operand)
	if err != nil {
		return err
	}
	ls.buffers = append(ls.buffers, data)
	splitNumericLines(data, &ls.lines)
	return nil
}

func parseNumericKey(s string) numericKey {
	i := 0
	for i < len(s) && isBlank(s[i]) {
		i++
	}
	neg := false
	if i < len(s) && s[i] == '-' {
		neg = true
		i++
	}
	j := i
	for j < len(s) && isDigit(s[j]) {
		j++
	}
	ipStart, ipEnd := i, j
	for ipStart < ipEnd && s[ipStart] == '0' {
		ipStart++
	}
	fpStart, fpEnd := 0, 0
	if j < len(s) && s[j] == '.' {
		k := j + 1
		for k < len(s) && isDigit(s[k]) {
			k++
		}
		fpStart, fpEnd = j+1, k
		for fpEnd > fpStart && s[fpEnd-1] == '0' {
			fpEnd--
		}
	}
	if ipStart == ipEnd && fpStart == fpEnd {
		return numericKey{}
	}
	sign := int8(1)
	if neg {
		sign = -1
	}
	return numericKey{
		sign:    sign,
		ipStart: int32(ipStart),
		ipLen:   int32(ipEnd - ipStart),
		fpStart: int32(fpStart),
		fpLen:   int32(fpEnd - fpStart),
	}
}

func compareNumericKey(as string, a numericKey, bs string, b numericKey) int {
	if a.sign != b.sign {
		return cmpInt(int(a.sign), int(b.sign))
	}
	m := cmpInt(int(a.ipLen), int(b.ipLen))
	if m == 0 {
		ai, bi := int(a.ipStart), int(b.ipStart)
		m = strings.Compare(as[ai:ai+int(a.ipLen)], bs[bi:bi+int(b.ipLen)])
	}
	if m == 0 {
		af, bf := int(a.fpStart), int(b.fpStart)
		m = strings.Compare(as[af:af+int(a.fpLen)], bs[bf:bf+int(b.fpLen)])
	}
	if a.sign < 0 {
		return -m
	}
	return m
}

func (s *sorter) compareNumericLines(a, b numericLine) int {
	if d := compareNumericKey(a.text, a.num, b.text, b.num); d != 0 || s.stable || s.unique {
		if d != 0 && s.keys[0].opts.reverse {
			return -d
		}
		return d
	}
	d := strings.Compare(a.text, b.text)
	if s.reverse {
		return -d
	}
	return d
}

// checkSorted implements -c: report the first out-of-order line in the
// GNU "sort: FILE:LINENO: disorder: LINE" shape and exit 1.
func (s *sorter) checkSorted(rc *tool.RunContext, op string) int {
	var lines lineSet
	if err := lines.read(rc, op); err != nil {
		fmt.Fprintf(rc.Err, "sort: cannot read: %s: %v\n", op, pathErr(err))
		return 2
	}
	for i := 1; i < len(lines.lines); i++ {
		d := s.compare(lines.lines[i-1], lines.lines[i])
		if d > 0 || (s.unique && d == 0) {
			fmt.Fprintf(rc.Err, "sort: %s:%d: disorder: %s\n", op, i+1, lines.lines[i])
			return 1
		}
	}
	return 0
}

// lineSet retains input buffers while lines point directly into them.
type lineSet struct {
	buffers [][]byte
	lines   []string
}

// read reads one operand ("-" = stdin) fully and splits it into
// newline-terminated lines; a final line without a trailing newline
// still counts (GNU appends the newline on output).
func (ls *lineSet) read(rc *tool.RunContext, operand string) error {
	data, err := readOperand(rc, operand)
	if err != nil {
		return err
	}
	ls.buffers = append(ls.buffers, data)
	splitLines(data, &ls.lines)
	return nil
}

func readOperand(rc *tool.RunContext, operand string) ([]byte, error) {
	if operand == "-" {
		return io.ReadAll(rc.In)
	}
	return os.ReadFile(rc.Path(operand))
}

func splitLines(data []byte, lines *[]string) {
	if len(data) == 0 {
		return
	}
	if data[len(data)-1] == '\n' {
		data = data[:len(data)-1]
	}
	if len(data) == 0 {
		*lines = append(*lines, "")
		return
	}
	n := bytes.Count(data, []byte{'\n'}) + 1
	old := len(*lines)
	if cap(*lines)-old < n {
		next := make([]string, old, old+n)
		copy(next, *lines)
		*lines = next
	}
	for {
		i := bytes.IndexByte(data, '\n')
		if i < 0 {
			*lines = append(*lines, bytesString(data))
			return
		}
		*lines = append(*lines, bytesString(data[:i]))
		data = data[i+1:]
	}
}

func splitNumericLines(data []byte, lines *[]numericLine) {
	if len(data) == 0 {
		return
	}
	if data[len(data)-1] == '\n' {
		data = data[:len(data)-1]
	}
	if len(data) == 0 {
		*lines = append(*lines, numericLine{})
		return
	}
	n := bytes.Count(data, []byte{'\n'}) + 1
	old := len(*lines)
	if cap(*lines)-old < n {
		next := make([]numericLine, old, old+n)
		copy(next, *lines)
		*lines = next
	}
	for {
		i := bytes.IndexByte(data, '\n')
		if i < 0 {
			l := bytesString(data)
			*lines = append(*lines, numericLine{text: l, num: parseNumericKey(l)})
			return
		}
		l := bytesString(data[:i])
		*lines = append(*lines, numericLine{text: l, num: parseNumericKey(l)})
		data = data[i+1:]
	}
}

func bytesString(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return unsafe.String(unsafe.SliceData(b), len(b))
}

// pathErr unwraps *fs.PathError so diagnostics read like GNU's
// "No such file or directory" instead of Go's "open /abs/path: ...".
func pathErr(err error) error {
	return tool.SysErr(err)
}
