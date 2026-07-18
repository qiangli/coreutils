// Package headcmd implements head(1) per the GNU coreutils manual:
// output the first part of files.
//
// Fresh implementation against the GNU manual (prior art consulted:
// guonaihong/coreutils head, u-root head, aict head — none covers the
// GNU -n -K "all but last K" mode or the exact header conventions).
package headcmd

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"os"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "head",
	Synopsis: "Print the first 10 lines of each FILE to standard output.\nWith more than one FILE, precede each with a header giving the file name.\nWith no FILE, or when FILE is -, read standard input.",
	Usage:    "head [OPTION]... [FILE]...",
}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	args = rewriteObsoleteNum(args, "--lines=")

	fs := tool.NewFlags(cmd.Name)
	linesV := fs.StringP("lines", "n", "10", "print the first NUM lines instead of the first 10; with the leading '-', print all but the last NUM lines of each file")
	bytesV := fs.StringP("bytes", "c", "", "print the first NUM bytes of each file; with the leading '-', print all but the last NUM bytes of each file")
	quiet := fs.BoolP("quiet", "q", false, "never print headers giving file names")
	silent := fs.Bool("silent", false, "same as --quiet")
	verbose := fs.BoolP("verbose", "v", false, "always print headers giving file names")
	zero := fs.BoolP("zero-terminated", "z", false, "line delimiter is NUL, not newline")
	operands, code := tool.Parse(rc, cmd, fs, tool.AliasHelpVersion(args))
	if code >= 0 {
		return code
	}

	mode, hdr := scanOrder(args)
	bytesMode := fs.Changed("bytes")
	if bytesMode && fs.Changed("lines") {
		bytesMode = mode == 'c'
	}

	var count int64
	var fromEnd bool
	if bytesMode {
		n, neg, _, err := parseCount(*bytesV)
		if err != nil {
			return tool.UsageError(rc, cmd, "invalid number of bytes: %q", *bytesV)
		}
		count, fromEnd = n, neg
	} else {
		n, neg, _, err := parseCount(*linesV)
		if err != nil {
			return tool.UsageError(rc, cmd, "invalid number of lines: %q", *linesV)
		}
		count, fromEnd = n, neg
	}

	q := *quiet || *silent
	v := *verbose
	if q && v {
		// GNU getopt semantics: the option given last wins.
		q = hdr == 'q'
		v = hdr == 'v'
	}

	files := operands
	if len(files) == 0 {
		files = []string{"-"}
	}
	showHeaders := v || (len(files) > 1 && !q)

	w := bufio.NewWriter(rc.Out)
	hp := headerPrinter{}
	exit := 0
	for _, name := range files {
		r, closer, err := openOperand(rc, name)
		if err != nil {
			fmt.Fprintf(rc.Err, "head: cannot open '%s' for reading: %v\n", name, sysErr(err))
			exit = 1
			continue
		}
		if showHeaders {
			hp.print(w, displayName(name))
		}
		lineEnd := byte('\n')
		if *zero {
			lineEnd = 0
		}
		err = headStream(r, w, bytesMode, count, fromEnd, lineEnd)
		if closer != nil {
			closer.Close()
		}
		if err != nil {
			fmt.Fprintf(rc.Err, "head: error reading '%s': %v\n", name, sysErr(err))
			exit = 1
		}
	}
	if err := w.Flush(); err != nil {
		fmt.Fprintf(rc.Err, "head: write error: %v\n", err)
		return 1
	}
	return exit
}

func headStream(r io.Reader, w *bufio.Writer, bytesMode bool, n int64, fromEnd bool, lineEnd byte) error {
	br := bufio.NewReader(r)
	switch {
	case bytesMode && !fromEnd:
		if n == 0 {
			return nil
		}
		if _, err := io.CopyN(w, br, n); err != nil && err != io.EOF {
			return err
		}
		return nil
	case bytesMode && fromEnd:
		// All but the last n bytes: keep a sliding window of n bytes,
		// emit everything that overflows it.
		if n == 0 {
			_, err := io.Copy(w, br)
			return err
		}
		var keep []byte
		buf := make([]byte, 32*1024)
		for {
			m, err := br.Read(buf)
			if m > 0 {
				keep = append(keep, buf[:m]...)
				if int64(len(keep)) > n {
					cut := int64(len(keep)) - n
					if _, werr := w.Write(keep[:cut]); werr != nil {
						return werr
					}
					keep = keep[:copy(keep, keep[cut:])]
				}
			}
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return err
			}
		}
	case !fromEnd:
		remaining := n
		for remaining > 0 {
			line, err := br.ReadBytes(lineEnd)
			if len(line) > 0 {
				if _, werr := w.Write(line); werr != nil {
					return werr
				}
				remaining--
			}
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return err
			}
		}
		return nil
	default:
		// All but the last n lines.
		if n == 0 {
			_, err := io.Copy(w, br)
			return err
		}
		var q [][]byte
		for {
			line, err := br.ReadBytes(lineEnd)
			if len(line) > 0 {
				q = append(q, line)
				if int64(len(q)) > n {
					if _, werr := w.Write(q[0]); werr != nil {
						return werr
					}
					q = q[1:]
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
}

// --- shared helpers (duplicated per-package by design; cmds packages
// do not import each other) ---

// rewriteObsoleteNum implements the obsolete first-argument form
// (head -5 == head -n 5). GNU only honors it as the first argument.
func rewriteObsoleteNum(args []string, flag string) []string {
	if len(args) == 0 {
		return args
	}
	a := args[0]
	if len(a) < 2 || a[0] != '-' || a[1] < '0' || a[1] > '9' {
		return args
	}
	if _, _, _, err := parseCount(a[1:]); err != nil {
		return args
	}
	return append([]string{flag + a[1:]}, args[1:]...)
}

// scanOrder reports which of -n/-c ('n'/'c') and which of -q/-v
// ('q'/'v') appears last on the command line — GNU getopt lets the
// last occurrence win for these mutually overriding options.
func scanOrder(args []string) (mode, hdr byte) {
	skip := false
	for _, a := range args {
		if skip {
			skip = false
			continue
		}
		if a == "--" {
			break
		}
		if strings.HasPrefix(a, "--") {
			name := a[2:]
			hasVal := false
			if i := strings.IndexByte(name, '='); i >= 0 {
				name, hasVal = name[:i], true
			}
			switch {
			case longOptionPrefix(name, "lines"):
				mode = 'n'
				skip = !hasVal
			case longOptionPrefix(name, "bytes"):
				mode = 'c'
				skip = !hasVal
			case longOptionPrefix(name, "quiet"), longOptionPrefix(name, "silent"):
				hdr = 'q'
			case longOptionPrefix(name, "verbose"):
				hdr = 'v'
			}
			continue
		}
		if len(a) > 1 && a[0] == '-' {
			for i := 1; i < len(a); i++ {
				c := a[i]
				if c == 'n' || c == 'c' {
					mode = c
					skip = i == len(a)-1
					break
				}
				if c == 'q' {
					hdr = 'q'
				}
				if c == 'v' {
					hdr = 'v'
				}
			}
		}
	}
	return mode, hdr
}

// longOptionPrefix mirrors tool.Parse's accepted unambiguous long-option
// prefixes for the head-specific options that affect last-option-wins
// behavior. scanOrder runs on the original arguments, before tool.Parse has
// expanded those prefixes.
func longOptionPrefix(name, option string) bool {
	return name != "" && strings.HasPrefix(option, name)
}

// parseCount parses a GNU NUM with optional leading sign and
// multiplier suffix (b 512, kB 1000, K/KiB 1024, MB, M/MiB, …).
func parseCount(s string) (val int64, neg, plus bool, err error) {
	t := s
	switch {
	case strings.HasPrefix(t, "+"):
		plus, t = true, t[1:]
	case strings.HasPrefix(t, "-"):
		neg, t = true, t[1:]
	}
	i := 0
	for i < len(t) && t[i] >= '0' && t[i] <= '9' {
		i++
	}
	if i == 0 {
		return 0, false, false, fmt.Errorf("invalid number: %q", s)
	}
	digits, suffix := t[:i], t[i:]
	mult, ok := multiplier(suffix)
	if !ok {
		return 0, false, false, fmt.Errorf("invalid suffix: %q", s)
	}
	var n int64
	for _, c := range []byte(digits) {
		d := int64(c - '0')
		if n > (math.MaxInt64-d)/10 {
			return 0, false, false, fmt.Errorf("number too large: %q", s)
		}
		n = n*10 + d
	}
	if mult != 1 && n > math.MaxInt64/mult {
		return 0, false, false, fmt.Errorf("number too large: %q", s)
	}
	return n * mult, neg, plus, nil
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

type headerPrinter struct{ printed bool }

// print emits the GNU "==> name <==" header, with a blank line before
// every header except the first one printed.
func (h *headerPrinter) print(w io.Writer, name string) {
	if h.printed {
		fmt.Fprintf(w, "\n==> %s <==\n", name)
	} else {
		fmt.Fprintf(w, "==> %s <==\n", name)
		h.printed = true
	}
}

func displayName(name string) string {
	if name == "-" {
		return "standard input"
	}
	return name
}

func openOperand(rc *tool.RunContext, name string) (io.Reader, io.Closer, error) {
	if name == "-" {
		if rc.In == nil {
			return strings.NewReader(""), nil, nil
		}
		return rc.In, nil, nil
	}
	f, err := os.Open(rc.Path(name))
	if err != nil {
		return nil, nil, err
	}
	return f, f, nil
}

func sysErr(err error) error {
	return tool.SysErr(err)
}
