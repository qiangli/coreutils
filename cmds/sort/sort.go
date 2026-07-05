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
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

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

	var lines []string
	for _, op := range operands {
		ls, err := readLines(rc, op)
		if err != nil {
			fmt.Fprintf(rc.Err, "sort: cannot read: %s: %v\n", op, pathErr(err))
			return 2
		}
		lines = append(lines, ls...)
	}

	sort.SliceStable(lines, func(i, j int) bool { return s.compare(lines[i], lines[j]) < 0 })

	if s.unique {
		out := lines[:0]
		for i, l := range lines {
			if i == 0 || s.compareEqual(out[len(out)-1], l) != 0 {
				out = append(out, l)
			}
		}
		lines = out
	}

	var w io.Writer = rc.Out
	if *output != "" {
		f, err := os.Create(rc.Path(*output))
		if err != nil {
			fmt.Fprintf(rc.Err, "sort: open failed: %s: %v\n", *output, pathErr(err))
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

// checkSorted implements -c: report the first out-of-order line in the
// GNU "sort: FILE:LINENO: disorder: LINE" shape and exit 1.
func (s *sorter) checkSorted(rc *tool.RunContext, op string) int {
	lines, err := readLines(rc, op)
	if err != nil {
		fmt.Fprintf(rc.Err, "sort: cannot read: %s: %v\n", op, pathErr(err))
		return 2
	}
	for i := 1; i < len(lines); i++ {
		d := s.compare(lines[i-1], lines[i])
		if d > 0 || (s.unique && d == 0) {
			fmt.Fprintf(rc.Err, "sort: %s:%d: disorder: %s\n", op, i+1, lines[i])
			return 1
		}
	}
	return 0
}

// readLines reads one operand ("-" = stdin) fully and splits it into
// newline-terminated lines; a final line without a trailing newline
// still counts (GNU appends the newline on output).
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
	return splitLines(data), nil
}

func splitLines(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	s := string(data)
	s = strings.TrimSuffix(s, "\n")
	return strings.Split(s, "\n")
}

// pathErr unwraps *fs.PathError so diagnostics read like GNU's
// "No such file or directory" instead of Go's "open /abs/path: ...".
func pathErr(err error) error {
	return tool.SysErr(err)
}
