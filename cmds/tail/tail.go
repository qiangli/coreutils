// Package tailcmd implements tail(1) per the GNU coreutils manual:
// output the last part of files.
package tailcmd

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"time"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "tail",
	Synopsis: "Print the last 10 lines of each FILE to standard output.\nWith more than one FILE, precede each with a header giving the file name.\nWith no FILE, or when FILE is -, read standard input.",
	Usage:    "tail [OPTION]... [FILE]...",
}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	args = rewriteObsoleteNum(args, "--lines=")

	fs := tool.NewFlags(cmd.Name)
	linesV := fs.StringP("lines", "n", "10", "output the last NUM lines, instead of the last 10; or use -n +NUM to output starting with line NUM")
	bytesV := fs.StringP("bytes", "c", "", "output the last NUM bytes; or use -c +NUM to output starting with byte NUM of each file")
	follow := fs.StringP("follow", "f", "", "output appended data as the file grows")
	fs.Lookup("follow").NoOptDefVal = "descriptor"
	fFlag := fs.BoolP("F", "F", false, "same as --follow=name --retry")
	quiet := fs.BoolP("quiet", "q", false, "never print headers giving file names")
	silent := fs.Bool("silent", false, "same as --quiet")
	verbose := fs.BoolP("verbose", "v", false, "always print headers giving file names")
	zero := fs.BoolP("zero-terminated", "z", false, "line delimiter is NUL, not newline")
	retryV := fs.Bool("retry", false, "keep trying to open a file even when it is temporarily inaccessible")
	sleepV := fs.Float64P("sleep-interval", "s", 1.0, "with -f, sleep for approximately N seconds (default 1.0) between iterations")
	pidV := fs.Int("pid", 0, "with -f, terminate after process ID PID dies")
	maxUnchanged := fs.Int("max-unchanged-stats", 5, "with --follow=name, reopen a FILE which has not changed size after N iterations (default 5)")
	usePolling := fs.Bool("use-polling", false, "use polling instead of inotify (polling is always used)")
	debugV := fs.Bool("debug", false, "print diagnostic information")
	operands, code := tool.Parse(rc, cmd, fs, tool.AliasHelpVersion(args))
	if code >= 0 {
		return code
	}

	followMode := *follow
	if *fFlag {
		followMode = "name"
		*retryV = true
	}
	if followMode != "" {
		if followMode != "descriptor" && followMode != "name" {
			return tool.UsageError(rc, cmd, "valid arguments for -f are 'descriptor' and 'name'")
		}
	}

	mode, hdr := scanOrder(args)
	bytesMode := fs.Changed("bytes")
	if bytesMode && fs.Changed("lines") {
		bytesMode = mode == 'c'
	}

	var count int64
	var fromStart bool
	if bytesMode {
		n, _, plus, err := parseCount(*bytesV)
		if err != nil {
			return tool.UsageError(rc, cmd, "invalid number of bytes: %q", *bytesV)
		}
		count, fromStart = n, plus
	} else {
		n, _, plus, err := parseCount(*linesV)
		if err != nil {
			return tool.UsageError(rc, cmd, "invalid number of lines: %q", *linesV)
		}
		count, fromStart = n, plus
	}

	if *sleepV <= 0 {
		return tool.UsageError(rc, cmd, "invalid sleep interval: %v", *sleepV)
	}

	q := *quiet || *silent
	v := *verbose
	if q && v {
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
		if showHeaders {
			hp.print(w, displayName(name))
		}
		lineEnd := byte('\n')
		if *zero {
			lineEnd = 0
		}

		if followMode == "" {
			r, closer, err := openOperand(rc, name)
			if err != nil {
				fmt.Fprintf(rc.Err, "tail: cannot open '%s' for reading: %v\n", name, sysErr(err))
				exit = 1
				continue
			}
			err = tailStream(r, w, bytesMode, count, fromStart, lineEnd)
			if closer != nil {
				closer.Close()
			}
			if err != nil {
				fmt.Fprintf(rc.Err, "tail: error reading '%s': %v\n", name, sysErr(err))
				exit = 1
			}
		} else {
			if name == "-" {
				fmt.Fprintf(rc.Err, "tail: cannot follow standard input by %s\n", followMode)
				exit = 1
				continue
			}
			path := rc.Path(name)
			fo := followOpts{
				path:          path,
				name:          name,
				followByName:  followMode == "name",
				retry:         *retryV,
				sleepInterval: time.Duration(*sleepV * float64(time.Second)),
				pid:           *pidV,
				maxUnchanged:  *maxUnchanged,
				usePolling:    *usePolling,
				debug:         *debugV,
				bytesMode:     bytesMode,
				count:         count,
				fromStart:     fromStart,
				lineEnd:       lineEnd,
			}
			if err := tailFollow(rc, w, fo); err != nil {
				fmt.Fprintf(rc.Err, "tail: %v\n", tool.SysErr(err))
				exit = 1
			}
		}
	}
	if err := w.Flush(); err != nil {
		fmt.Fprintf(rc.Err, "tail: write error: %v\n", err)
		return 1
	}
	return exit
}

type followOpts struct {
	path          string
	name          string
	followByName  bool
	retry         bool
	sleepInterval time.Duration
	pid           int
	maxUnchanged  int
	usePolling    bool
	debug         bool
	bytesMode     bool
	count         int64
	fromStart     bool
	lineEnd       byte
}

func tailFollow(rc *tool.RunContext, w *bufio.Writer, fo followOpts) error {
	ctx := rc.Ctx
	if ctx == nil {
		ctx = context.Background()
	}

	if fo.debug {
		fmode := "descriptor"
		if fo.followByName {
			fmode = "name"
		}
		fmt.Fprintf(rc.Err, "==> tail: following %q (by=%s retry=%v sleep=%v pid=%d max-unchanged=%d)\n",
			fo.name, fmode, fo.retry, fo.sleepInterval, fo.pid, fo.maxUnchanged)
	}

	f, err := openOrRetry(rc, fo.path, fo.name, fo.retry, fo.sleepInterval)
	if err != nil {
		return err
	}
	if f != nil {
		defer f.Close()
	}

	if f != nil {
		if err := tailStream(f, w, fo.bytesMode, fo.count, fo.fromStart, fo.lineEnd); err != nil {
			return err
		}
		if err := w.Flush(); err != nil {
			return err
		}
		if !fo.followByName {
			if _, err := f.Seek(0, io.SeekEnd); err != nil {
				return err
			}
		}
	}

	unchanged := 0
	lastSize := int64(-1)
	lastIno := uint64(0)

	if f != nil {
		fi, err := f.Stat()
		if err == nil {
			lastSize = fi.Size()
			lastIno = inodeKey(fi)
		}
	}

	for {
		if fo.pid > 0 {
			if !processExists(fo.pid) {
				if fo.debug {
					fmt.Fprintf(rc.Err, "==> tail: pid %d is dead; stopping\n", fo.pid)
				}
				return nil
			}
		}

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(fo.sleepInterval):
		}

		if fo.followByName {
			fi, err := os.Stat(fo.path)
			if err != nil {
				if fo.retry {
					if f != nil {
						f.Close()
						f = nil
					}
					if fo.debug {
						fmt.Fprintf(rc.Err, "==> tail: %q has become inaccessible; retrying\n", fo.name)
					}
					f, err = openOrRetry(rc, fo.path, fo.name, fo.retry, fo.sleepInterval)
					if err != nil {
						return err
					}
					if f != nil {
						fi, _ = f.Stat()
					}
					unchanged = 0
					lastSize = -1
					continue
				}
				return fmt.Errorf("cannot stat %q: %v", fo.name, err)
			}

			newIno := inodeKey(fi)
			if newIno != lastIno && lastIno != 0 {
				if f != nil {
					f.Close()
				}
				f, err = openOrRetry(rc, fo.path, fo.name, fo.retry, fo.sleepInterval)
				if err != nil {
					return err
				}
				if f != nil {
					fi, _ = f.Stat()
					lastIno = inodeKey(fi)
				}
				lastSize = -1
				unchanged = 0
				continue
			}
			lastIno = newIno

			if fi.Size() == lastSize {
				unchanged++
			} else if fi.Size() < lastSize {
				unchanged = 0
				if f != nil {
					f.Close()
				}
				f, err = openOrRetry(rc, fo.path, fo.name, fo.retry, fo.sleepInterval)
				if err != nil {
					return err
				}
				lastSize = fi.Size()
				if fo.debug {
					fmt.Fprintf(rc.Err, "==> tail: %q was truncated; reading from start\n", fo.name)
				}
			} else {
				unchanged = 0
			}

			if unchanged >= fo.maxUnchanged && fo.maxUnchanged > 0 {
				if f != nil {
					f.Close()
				}
				f, err = openOrRetry(rc, fo.path, fo.name, fo.retry, fo.sleepInterval)
				if err != nil {
					return err
				}
				if f != nil {
					fi, _ = f.Stat()
				}
				unchanged = 0
				lastSize = -1
				continue
			}
		}

		if f == nil {
			continue
		}

		buf := make([]byte, 32*1024)
		n, err := f.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return werr
			}
			if err := w.Flush(); err != nil {
				return err
			}
			if fo.followByName {
				lastSize += int64(n)
			}
		}
		if err != nil && err != io.EOF {
			return err
		}
	}
}

func openOrRetry(rc *tool.RunContext, path, name string, retry bool, sleep time.Duration) (*os.File, error) {
	for {
		f, err := os.Open(path)
		if err == nil {
			return f, nil
		}
		if !retry {
			return nil, fmt.Errorf("cannot open %q for reading: %v", name, err)
		}
		fmt.Fprintf(rc.Err, "tail: %q has become inaccessible, retrying...\n", name)
		ctx := rc.Ctx
		if ctx == nil {
			ctx = context.Background()
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(sleep):
		}
	}
}

func tailStream(r io.Reader, w *bufio.Writer, bytesMode bool, n int64, fromStart bool, lineEnd byte) error {
	br := bufio.NewReader(r)
	switch {
	case bytesMode && fromStart:
		skip := n - 1
		if skip > 0 {
			if _, err := io.CopyN(io.Discard, br, skip); err != nil {
				if err == io.EOF {
					return nil
				}
				return err
			}
		}
		_, err := io.Copy(w, br)
		return err
	case bytesMode:
		var keep []byte
		buf := make([]byte, 32*1024)
		for {
			m, err := br.Read(buf)
			if m > 0 && n > 0 {
				keep = append(keep, buf[:m]...)
				if int64(len(keep)) > n {
					keep = keep[:copy(keep, keep[int64(len(keep))-n:])]
				}
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}
		}
		_, err := w.Write(keep)
		return err
	case fromStart:
		skip := n - 1
		for skip > 0 {
			line, err := br.ReadBytes(lineEnd)
			if len(line) > 0 {
				skip--
			}
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return err
			}
		}
		_, err := io.Copy(w, br)
		return err
	default:
		var q [][]byte
		for {
			line, err := br.ReadBytes(lineEnd)
			if len(line) > 0 && n > 0 {
				q = append(q, line)
				if int64(len(q)) > n {
					q = q[1:]
				}
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}
		}
		for _, line := range q {
			if _, err := w.Write(line); err != nil {
				return err
			}
		}
		return nil
	}
}

// --- shared helpers (duplicated per-package by design; cmds packages
// do not import each other) ---

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
			switch name {
			case "lines":
				mode = 'n'
				skip = !hasVal
			case "bytes":
				mode = 'c'
				skip = !hasVal
			case "quiet", "silent":
				hdr = 'q'
			case "verbose":
				hdr = 'v'
			}
			continue
		}
		if len(a) > 1 && a[0] == '-' {
			for i := 1; i < len(a); i++ {
				c := a[i]
				if c == 'n' || c == 'c' || c == 'f' {
					if c != 'f' {
						mode = c
						skip = i == len(a)-1
					}
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
