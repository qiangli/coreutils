// Package cmpcmd implements cmp(1) per the GNU diffutils manual:
// compare two files byte by byte.
//
// Fresh implementation against the GNU manual (the u-root prior art
// follows Plan 9 semantics — "char N" output, exit 2 on difference —
// not GNU's). Exit status: 0 identical, 1 different, 2 trouble.
package cmpcmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "cmp",
	Synopsis: "Compare two files byte by byte.\nThe optional SKIP1 and SKIP2 specify the number of bytes to skip at the\nbeginning of each file (zero by default).",
	Usage:    "cmp [OPTION]... FILE1 [FILE2 [SKIP1 [SKIP2]]]",
}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	verbose := fs.BoolP("verbose", "l", false, "output byte numbers and differing byte values")
	silent := fs.BoolP("silent", "s", false, "suppress all normal output")
	quiet := fs.Bool("quiet", false, "same as -s/--silent")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}

	listAll := *verbose
	sil := *silent || *quiet
	if listAll && sil {
		return tool.UsageError(rc, cmd, "options -l and -s are incompatible")
	}

	if len(operands) == 0 {
		return tool.UsageError(rc, cmd, "missing operand")
	}
	if len(operands) > 4 {
		return tool.UsageError(rc, cmd, "extra operand %q", operands[4])
	}
	name1 := operands[0]
	name2 := "-"
	if len(operands) >= 2 {
		name2 = operands[1]
	}
	var skip [2]int64
	for i := 0; i < 2 && i+2 < len(operands); i++ {
		v, err := parseSkip(operands[i+2])
		if err != nil {
			return tool.UsageError(rc, cmd, "invalid --ignore-initial value %q", operands[i+2])
		}
		skip[i] = v
	}
	if sameSpecialSource(rc, name1, name2) {
		if name1 == "-" && name2 == "-" {
			fmt.Fprintln(rc.Err, "cmp: standard input may only be specified once")
		} else {
			fmt.Fprintf(rc.Err, "cmp: %s and %s are the same non-regular file\n", name1, name2)
		}
		return 2
	}

	s1, size1, code := openSrc(rc, name1, skip[0])
	if code >= 0 {
		return code
	}
	defer s1.close()
	s2, size2, code := openSrc(rc, name2, skip[1])
	if code >= 0 {
		return code
	}
	defer s2.close()

	switch {
	case sil:
		return cmpSilent(rc, s1, s2)
	case listAll:
		return cmpVerbose(rc, name1, name2, s1, s2, size1, size2)
	default:
		return cmpFirstDiff(rc, name1, name2, s1, s2)
	}
}

// sameSpecialSource reports whether both operands designate the same input
// stream. Comparing a stream with itself would consume alternating bytes
// rather than compare the stream's contents, so POSIX requires an error for
// standard input and for the same FIFO, block device, or character device.
func sameSpecialSource(rc *tool.RunContext, name1, name2 string) bool {
	if name1 == "-" && name2 == "-" {
		return true
	}
	info1, ok1 := sourceInfo(rc, name1)
	info2, ok2 := sourceInfo(rc, name2)
	if !ok1 || !ok2 || !os.SameFile(info1, info2) {
		return false
	}
	mode := info1.Mode()
	return mode&(os.ModeNamedPipe|os.ModeDevice|os.ModeCharDevice) != 0
}

func sourceInfo(rc *tool.RunContext, name string) (os.FileInfo, bool) {
	if name == "-" {
		f, ok := rc.In.(*os.File)
		if !ok {
			return nil, false
		}
		info, err := f.Stat()
		return info, err == nil
	}
	info, err := os.Stat(rc.Path(name))
	return info, err == nil
}

type src struct {
	r *bufio.Reader
	c io.Closer
}

func (s *src) close() {
	if s.c != nil {
		s.c.Close()
	}
}

// openSrc opens an operand ("-" = stdin), discards skip bytes, and
// reports the remaining size when it is knowable (regular file), else
// -1. Errors exit with status 2 (trouble), per GNU.
func openSrc(rc *tool.RunContext, name string, skip int64) (*src, int64, int) {
	var r io.Reader
	var c io.Closer
	size := int64(-1)
	if name == "-" {
		r = rc.In
		if r == nil {
			r = strings.NewReader("")
		}
	} else {
		f, err := os.Open(rc.Path(name))
		if err != nil {
			fmt.Fprintf(rc.Err, "cmp: %s: %v\n", name, sysErr(err))
			return nil, 0, 2
		}
		r = f
		c = f
		if st, err := f.Stat(); err == nil && st.Mode().IsRegular() {
			size = st.Size() - skip
			if size < 0 {
				size = 0
			}
		}
	}
	br := bufio.NewReaderSize(r, 64*1024)
	if skip > 0 {
		if _, err := io.CopyN(io.Discard, br, skip); err != nil && err != io.EOF {
			fmt.Fprintf(rc.Err, "cmp: %s: %v\n", name, sysErr(err))
			if c != nil {
				c.Close()
			}
			return nil, 0, 2
		}
	}
	return &src{r: br, c: c}, size, -1
}

// cmpFirstDiff is the default mode: report the first difference as
// "FILE1 FILE2 differ: byte N, line L" (stdout, exit 1), or the GNU
// EOF diagnostic (stderr, exit 1) when one file is a prefix of the
// other.
func cmpFirstDiff(rc *tool.RunContext, name1, name2 string, s1, s2 *src) int {
	var matched, newlines int64
	lastWasNL := false
	for {
		b1, err1 := s1.r.ReadByte()
		b2, err2 := s2.r.ReadByte()
		if err1 == io.EOF && err2 == io.EOF {
			return 0
		}
		if err1 == io.EOF || err2 == io.EOF {
			name := name1
			if err2 == io.EOF {
				name = name2
			}
			eofDiag(rc, name, matched, newlines, lastWasNL, true)
			return 1
		}
		if err1 != nil {
			fmt.Fprintf(rc.Err, "cmp: %s: %v\n", name1, err1)
			return 2
		}
		if err2 != nil {
			fmt.Fprintf(rc.Err, "cmp: %s: %v\n", name2, err2)
			return 2
		}
		if b1 != b2 {
			fmt.Fprintf(rc.Out, "%s %s differ: byte %d, line %d\n", name1, name2, matched+1, newlines+1)
			return 1
		}
		matched++
		lastWasNL = b1 == '\n'
		if lastWasNL {
			newlines++
		}
	}
}

// cmpVerbose is -l: print every difference as "OFFSET OCT1 OCT2".
// The offset column is right-aligned to the width of the largest
// possible byte number — min(file sizes) when both are regular files,
// else the width of the largest off_t, matching GNU.
func cmpVerbose(rc *tool.RunContext, name1, name2 string, s1, s2 *src, size1, size2 int64) int {
	width := 19 // digits in max int64, GNU's fallback for unseekable inputs
	if size1 >= 0 && size2 >= 0 {
		m := size1
		if size2 < m {
			m = size2
		}
		width = 1
		for v := m; v >= 10; v /= 10 {
			width++
		}
	}
	var pos int64
	differed := false
	for {
		b1, err1 := s1.r.ReadByte()
		b2, err2 := s2.r.ReadByte()
		if err1 == io.EOF && err2 == io.EOF {
			if differed {
				return 1
			}
			return 0
		}
		if err1 == io.EOF || err2 == io.EOF {
			name := name1
			if err2 == io.EOF {
				name = name2
			}
			eofDiag(rc, name, pos, 0, false, false)
			return 1
		}
		if err1 != nil {
			fmt.Fprintf(rc.Err, "cmp: %s: %v\n", name1, err1)
			return 2
		}
		if err2 != nil {
			fmt.Fprintf(rc.Err, "cmp: %s: %v\n", name2, err2)
			return 2
		}
		pos++
		if b1 != b2 {
			differed = true
			fmt.Fprintf(rc.Out, "%*d %3o %3o\n", width, pos, b1, b2)
		}
	}
}

func cmpSilent(rc *tool.RunContext, s1, s2 *src) int {
	for {
		b1, err1 := s1.r.ReadByte()
		b2, err2 := s2.r.ReadByte()
		if err1 == io.EOF && err2 == io.EOF {
			return 0
		}
		if err1 == io.EOF || err2 == io.EOF {
			return 1
		}
		if err1 != nil || err2 != nil {
			return 2
		}
		if b1 != b2 {
			return 1
		}
	}
}

// eofDiag prints GNU's prefix diagnostic: "cmp: EOF on NAME which is
// empty" when nothing matched, otherwise "cmp: EOF on NAME after byte
// N[, in line L]" (the line part only in the line-tracking default
// mode).
func eofDiag(rc *tool.RunContext, name string, matched, newlines int64, lastWasNL, withLine bool) {
	if matched == 0 {
		fmt.Fprintf(rc.Err, "cmp: EOF on %s which is empty\n", name)
		return
	}
	if withLine {
		line := newlines
		if !lastWasNL {
			line++
		}
		fmt.Fprintf(rc.Err, "cmp: EOF on %s after byte %d, in line %d\n", name, matched, line)
		return
	}
	fmt.Fprintf(rc.Err, "cmp: EOF on %s after byte %d\n", name, matched)
}

// parseSkip parses a SKIP operand: decimal, octal (leading 0) or hex
// (leading 0x), with optional GNU multiplier suffix.
func parseSkip(s string) (int64, error) {
	digits := func(c byte) bool { return c >= '0' && c <= '9' }
	i := 0
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		i = 2
		digits = func(c byte) bool {
			return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
		}
	}
	j := i
	for j < len(s) && digits(s[j]) {
		j++
	}
	if j == i {
		return 0, fmt.Errorf("invalid value: %q", s)
	}
	v, err := strconv.ParseInt(s[:j], 0, 64)
	if err != nil || v < 0 {
		return 0, fmt.Errorf("invalid value: %q", s)
	}
	mult, ok := multiplier(s[j:])
	if !ok {
		return 0, fmt.Errorf("invalid suffix: %q", s)
	}
	if mult != 1 && v > (1<<62)/mult {
		return 0, fmt.Errorf("value too large: %q", s)
	}
	return v * mult, nil
}

func multiplier(suf string) (int64, bool) {
	if suf == "" {
		return 1, true
	}
	if suf == "b" {
		return 512, true
	}
	powers := map[byte]int{'K': 1, 'M': 2, 'G': 3, 'T': 4, 'P': 5, 'E': 6}
	c := suf[0]
	if c >= 'a' && c <= 'z' {
		c -= 'a' - 'A'
	}
	p, ok := powers[c]
	if !ok {
		return 0, false
	}
	var base int64
	switch {
	case len(suf) == 1:
		base = 1024
	case len(suf) == 2 && suf[1] == 'B':
		base = 1000
	case len(suf) == 3 && suf[1] == 'i' && suf[2] == 'B':
		base = 1024
	default:
		return 0, false
	}
	m := int64(1)
	for i := 0; i < p; i++ {
		m *= base
	}
	return m, true
}

func sysErr(err error) error {
	return tool.SysErr(err)
}
