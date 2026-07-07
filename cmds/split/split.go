// Package splitcmd implements split(1) per the GNU coreutils manual:
// split a file into pieces.
//
// Implemented flags: -a -b -C -d -e -l -n -t --additional-suffix
// --hex-suffixes --separator --verbose.
// Suffix naming follows GNU exactly.
package splitcmd

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "split",
	Synopsis: "Output pieces of FILE to PREFIXaa, PREFIXab, ...;\ndefault size is 1000 lines, and default PREFIX is 'x'.\nWith no FILE, or when FILE is -, read standard input.",
	Usage:    "split [OPTION]... [FILE [PREFIX]]",
}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	args = rewriteObsoleteNum(args)

	fs := tool.NewFlags(cmd.Name)
	suffixLen := fs.IntP("suffix-length", "a", 0, "generate suffixes of length N (default 2)")
	bytesV := fs.StringP("bytes", "b", "", "put SIZE bytes per output file")
	hexSuffixes := fs.StringP("hex-suffixes", "", "", "use hexadecimal suffixes, optionally starting at FROM")
	fs.Lookup("hex-suffixes").NoOptDefVal = "0"
	elideEmpty := fs.BoolP("elide-empty-files", "e", false, "do not generate empty output files with -n")
	lineBytes := fs.StringP("line-bytes", "C", "", "put at most SIZE bytes of lines per output file")
	numeric := fs.StringP("numeric-suffixes", "d", "", "use numeric suffixes starting at 0, not alphabetic")
	fs.Lookup("numeric-suffixes").NoOptDefVal = "0"
	linesV := fs.StringP("lines", "l", "", "put NUMBER lines/records per output file")
	chunksV := fs.StringP("number", "n", "", "generate CHUNKS output files; supported forms: N, l/N")
	separator := fs.StringP("separator", "t", "", "use SEP as record separator instead of newline")
	additionalSuffix := fs.StringP("additional-suffix", "", "", "additional suffix to append to output file names")
	verbose := fs.BoolP("verbose", "", false, "print a diagnostic just before each output file is opened")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}

	if fs.Changed("numeric-suffixes") && *numeric != "0" {
		return tool.NotSupported(rc, cmd, "--numeric-suffixes=FROM with a nonzero start value")
	}

	if fs.Changed("hex-suffixes") {
		start, err := strconv.Atoi(*hexSuffixes)
		if err != nil || start < 0 {
			return tool.UsageError(rc, cmd, "invalid hex suffix start: %q", *hexSuffixes)
		}
	}

	nModes := 0
	for _, on := range []bool{fs.Changed("lines"), fs.Changed("bytes"), fs.Changed("number"), fs.Changed("line-bytes")} {
		if on {
			nModes++
		}
	}
	if nModes > 1 {
		return tool.UsageError(rc, cmd, "cannot split in more than one way")
	}

	if fs.Changed("suffix-length") && *suffixLen < 0 {
		return tool.UsageError(rc, cmd, "invalid suffix length: %q", strconv.Itoa(*suffixLen))
	}

	file := "-"
	prefix := "x"
	switch len(operands) {
	case 0:
	case 1:
		file = operands[0]
	case 2:
		file, prefix = operands[0], operands[1]
	default:
		return tool.UsageError(rc, cmd, "extra operand %q", operands[2])
	}

	var in io.Reader
	if file == "-" {
		in = rc.In
		if in == nil {
			in = strings.NewReader("")
		}
	} else {
		f, err := os.Open(rc.Path(file))
		if err != nil {
			fmt.Fprintf(rc.Err, "split: cannot open '%s' for reading: %v\n", file, sysErr(err))
			return 1
		}
		defer f.Close()
		in = f
	}

	radix := 26
	hexStart := 0
	useHex := false
	if fs.Changed("numeric-suffixes") {
		radix = 10
	}
	if fs.Changed("hex-suffixes") {
		radix = 16
		useHex = true
		hexStart, _ = strconv.Atoi(*hexSuffixes)
	}
	sfx := &suffixer{radix: radix, fixed: *suffixLen, useHex: useHex, hexStart: hexStart}
	out := &outFiles{
		rc:      rc,
		prefix:  prefix,
		sfx:     sfx,
		suffix:  *additionalSuffix,
		verbose: *verbose,
	}
	defer out.close()

	var err error
	switch {
	case fs.Changed("bytes"):
		size, perr := parseSize(*bytesV)
		if perr != nil || size < 1 {
			return tool.UsageError(rc, cmd, "invalid number of bytes: %q", *bytesV)
		}
		err = splitBytes(in, out, size)
	case fs.Changed("line-bytes"):
		size, perr := parseSize(*lineBytes)
		if perr != nil || size < 1 {
			return tool.UsageError(rc, cmd, "invalid number of bytes: %q", *lineBytes)
		}
		err = splitLineBytes(in, out, size, *separator)
	case fs.Changed("number"):
		var byLines bool
		var n int64
		byLines, n, code = parseChunks(rc, *chunksV)
		if code >= 0 {
			return code
		}
		err = splitChunks(in, out, n, byLines, *elideEmpty)
	default:
		lines := int64(1000)
		if fs.Changed("lines") {
			v, perr := strconv.ParseInt(*linesV, 10, 64)
			if perr != nil || v < 1 {
				return tool.UsageError(rc, cmd, "invalid number of lines: %q", *linesV)
			}
			lines = v
		}
		if fs.Changed("separator") {
			err = splitRecords(in, out, lines, []byte(*separator))
		} else {
			err = splitLines(in, out, lines)
		}
	}
	if err != nil {
		fmt.Fprintf(rc.Err, "split: %v\n", err)
		return 1
	}
	if err := out.close(); err != nil {
		fmt.Fprintf(rc.Err, "split: %v\n", err)
		return 1
	}
	return 0
}

func rewriteObsoleteNum(args []string) []string {
	if len(args) == 0 {
		return args
	}
	a := args[0]
	if len(a) < 2 || a[0] != '-' {
		return args
	}
	for i := 1; i < len(a); i++ {
		if a[i] < '0' || a[i] > '9' {
			return args
		}
	}
	return append([]string{"--lines=" + a[1:]}, args[1:]...)
}

func parseChunks(rc *tool.RunContext, s string) (byLines bool, n int64, code int) {
	spec := s
	if i := strings.IndexByte(spec, '/'); i >= 0 {
		kind := spec[:i]
		rest := spec[i+1:]
		if kind != "l" || strings.ContainsRune(rest, '/') {
			return false, 0, tool.NotSupported(rc, cmd, fmt.Sprintf("-n %s (only the N and l/N chunk forms are supported)", s))
		}
		byLines = true
		spec = rest
	}
	v, err := strconv.ParseInt(spec, 10, 64)
	if err != nil || v < 1 {
		return false, 0, tool.UsageError(rc, cmd, "invalid number of chunks: %q", s)
	}
	return byLines, v, -1
}

type suffixer struct {
	radix    int
	fixed    int
	idx      int
	useHex   bool
	hexStart int
}

func (s *suffixer) symbol(d int) byte {
	if s.radix == 10 {
		return '0' + byte(d)
	}
	if s.radix == 16 {
		hd := d
		if hd < 10 {
			return '0' + byte(hd)
		}
		return 'a' + byte(hd-10)
	}
	return 'a' + byte(d)
}

func (s *suffixer) next() (string, error) {
	if s.useHex {
		idx := s.idx + s.hexStart
		s.idx++
		if s.fixed > 0 {
			limit := int64(1)
			for i := 0; i < s.fixed; i++ {
				limit *= 16
			}
			if int64(idx) >= limit {
				return "", errors.New("output file suffixes exhausted")
			}
			return fmt.Sprintf("%0*x", s.fixed, idx), nil
		}
		return fmt.Sprintf("%x", idx), nil
	}
	idx := s.idx
	s.idx++
	if s.fixed > 0 {
		limit := int64(1)
		for i := 0; i < s.fixed; i++ {
			limit *= int64(s.radix)
			if limit > math.MaxInt32 {
				limit = math.MaxInt32
				break
			}
		}
		if int64(idx) >= limit {
			return "", errors.New("output file suffixes exhausted")
		}
		buf := make([]byte, s.fixed)
		v := idx
		for i := s.fixed - 1; i >= 0; i-- {
			buf[i] = s.symbol(v % s.radix)
			v /= s.radix
		}
		return string(buf), nil
	}
	remaining := idx
	sub := (s.radix - 1) * s.radix
	nd := 2
	for remaining >= sub {
		remaining -= sub
		sub *= s.radix
		nd++
	}
	buf := make([]byte, nd)
	v := remaining
	for i := nd - 1; i >= 0; i-- {
		buf[i] = s.symbol(v % s.radix)
		v /= s.radix
	}
	fill := strings.Repeat(string(s.symbol(s.radix-1)), nd-2)
	return fill + string(buf), nil
}

type outFiles struct {
	rc      *tool.RunContext
	prefix  string
	sfx     *suffixer
	f       *os.File
	w       *bufio.Writer
	suffix  string
	verbose bool
}

func (o *outFiles) nextFile() error {
	if err := o.close(); err != nil {
		return err
	}
	suf, err := o.sfx.next()
	if err != nil {
		return err
	}
	name := o.prefix + suf + o.suffix
	if o.verbose {
		fmt.Fprintf(o.rc.Err, "%s\n", name)
	}
	f, err := os.Create(o.rc.Path(name))
	if err != nil {
		return fmt.Errorf("%s: %v", name, sysErr(err))
	}
	o.f = f
	o.w = bufio.NewWriter(f)
	return nil
}

func (o *outFiles) write(p []byte) error {
	_, err := o.w.Write(p)
	return err
}

func (o *outFiles) close() error {
	if o.f == nil {
		return nil
	}
	var err error
	if ferr := o.w.Flush(); ferr != nil {
		err = ferr
	}
	if cerr := o.f.Close(); cerr != nil && err == nil {
		err = cerr
	}
	o.f, o.w = nil, nil
	return err
}

func splitLines(in io.Reader, out *outFiles, perFile int64) error {
	br := bufio.NewReader(in)
	var inFile int64
	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			if inFile == 0 {
				if nerr := out.nextFile(); nerr != nil {
					return nerr
				}
			}
			if werr := out.write(line); werr != nil {
				return werr
			}
			inFile++
			if inFile == perFile {
				inFile = 0
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func splitRecords(in io.Reader, out *outFiles, perFile int64, sep []byte) error {
	data, err := io.ReadAll(in)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return nil
	}
	var lastEnd int
	var count int64
	for i := 0; i < len(data); {
		j := bytes.Index(data[i:], sep)
		if j < 0 {
			if lastEnd == 0 && count == 0 {
				if nerr := out.nextFile(); nerr != nil {
					return nerr
				}
			}
			return out.write(data[lastEnd:])
		}
		end := i + j + len(sep)
		if count == 0 {
			if nerr := out.nextFile(); nerr != nil {
				return nerr
			}
		}
		if werr := out.write(data[lastEnd:end]); werr != nil {
			return werr
		}
		lastEnd = end
		i = end
		count++
		if count == perFile {
			count = 0
		}
	}
	return nil
}

func splitBytes(in io.Reader, out *outFiles, perFile int64) error {
	buf := make([]byte, 64*1024)
	var remaining int64
	for {
		n, err := in.Read(buf)
		chunk := buf[:n]
		for len(chunk) > 0 {
			if remaining == 0 {
				if nerr := out.nextFile(); nerr != nil {
					return nerr
				}
				remaining = perFile
			}
			take := int64(len(chunk))
			if take > remaining {
				take = remaining
			}
			if werr := out.write(chunk[:take]); werr != nil {
				return werr
			}
			remaining -= take
			chunk = chunk[take:]
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func splitLineBytes(in io.Reader, out *outFiles, perFile int64, sepStr string) error {
	sep := byte('\n')
	if sepStr != "" && len(sepStr) == 1 {
		sep = sepStr[0]
	}
	br := bufio.NewReader(in)
	var inFile int64
	for {
		line, err := br.ReadBytes(sep)
		n := int64(len(line))
		for len(line) > 0 {
			if inFile == 0 {
				if nerr := out.nextFile(); nerr != nil {
					return nerr
				}
			}
			take := n
			remaining := perFile - inFile
			if take > remaining {
				take = remaining
			}
			if werr := out.write(line[:take]); werr != nil {
				return werr
			}
			inFile += take
			line = line[take:]
			n -= take
			if inFile >= perFile {
				inFile = 0
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func splitChunks(in io.Reader, out *outFiles, n int64, byLines, elideEmpty bool) error {
	data, err := io.ReadAll(in)
	if err != nil {
		return err
	}
	size := int64(len(data))
	if !byLines {
		chunk := size / n
		for i := int64(0); i < n; i++ {
			start := i * chunk
			end := start + chunk
			if i == n-1 {
				end = size
			}
			b := data[start:end]
			if elideEmpty && len(b) == 0 {
				continue
			}
			if err := out.nextFile(); err != nil {
				return err
			}
			if err := out.write(b); err != nil {
				return err
			}
		}
		return nil
	}
	chunk := size / n
	if chunk == 0 {
		chunk = 1
	}
	if err := out.nextFile(); err != nil {
		return err
	}
	cur := int64(0)
	var pos int64
	var fileStarted bool
	for pos < size {
		nl := int64(-1)
		if i := bytes.IndexByte(data[pos:], '\n'); i >= 0 {
			nl = pos + int64(i)
		}
		end := size
		if nl >= 0 {
			end = nl + 1
		}
		if !fileStarted {
			out.write(data[pos:end])
			fileStarted = true
		} else {
			if err := out.write(data[pos:end]); err != nil {
				return err
			}
		}
		pos = end
		for cur < n-1 && pos >= (cur+1)*chunk {
			cur++
			if err := out.nextFile(); err != nil {
				return err
			}
		}
	}
	for cur < n-1 {
		cur++
		if err := out.nextFile(); err != nil {
			return err
		}
	}
	return nil
}

func parseSize(s string) (int64, error) {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == 0 {
		return 0, fmt.Errorf("invalid number: %q", s)
	}
	digits, suffix := s[:i], s[i:]
	mult, ok := multiplier(suffix)
	if !ok {
		return 0, fmt.Errorf("invalid suffix: %q", s)
	}
	var n int64
	for _, c := range []byte(digits) {
		d := int64(c - '0')
		if n > (math.MaxInt64-d)/10 {
			return 0, fmt.Errorf("number too large: %q", s)
		}
		n = n*10 + d
	}
	if mult != 1 && n > math.MaxInt64/mult {
		return 0, fmt.Errorf("number too large: %q", s)
	}
	return n * mult, nil
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
