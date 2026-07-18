// Package expandcmd implements expand(1): convert tabs to spaces.
package expandcmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "expand",
	Synopsis: "Convert tabs in each FILE to spaces, writing to standard output.\nWith no FILE, or when FILE is -, read standard input.",
	Usage:    "expand [OPTION]... [FILE]...",
}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	tabsValue := fs.StringArrayP("tabs", "t", []string{"8"}, "have tabs N characters apart, not 8; or use comma- or blank-separated LIST of explicit tab positions (repeatable; the last position may be prefixed with '/' for multiples or '+' for an increment)")
	initial := fs.BoolP("initial", "i", false, "do not convert tabs after non blanks")
	noUTF8 := fs.BoolP("no-utf8", "U", false, "interpret input bytes as columns instead of UTF-8 characters")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	tabs, err := parseTabStops(*tabsValue)
	if err != nil {
		return tool.UsageError(rc, cmd, "%v", err)
	}
	if len(operands) == 0 {
		operands = []string{"-"}
	}

	out := bufio.NewWriter(rc.Out)
	status := 0
	for _, name := range operands {
		r, closer, err := openInput(rc, name)
		if err != nil {
			fmt.Fprintf(rc.Err, "expand: %s: %v\n", name, err)
			status = 1
			continue
		}
		if err := expandStream(r, out, tabs, *initial, *noUTF8); err != nil {
			fmt.Fprintf(rc.Err, "expand: %s: %v\n", name, err)
			status = 1
		}
		if closer != nil {
			closer.Close()
		}
	}
	if err := out.Flush(); err != nil {
		fmt.Fprintf(rc.Err, "expand: write error: %v\n", err)
		return 1
	}
	return status
}

func expandStream(r io.Reader, w io.Writer, tabs *tabStops, initial bool, noUTF8 bool) error {
	if noUTF8 {
		return expandStreamBytes(r, w, tabs, initial)
	}
	br := bufio.NewReader(r)
	col := 0
	convert := true
	for {
		ch, _, err := br.ReadRune()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		switch {
		case ch == '\t' && convert:
			next, _ := tabs.next(col)
			if _, err := io.WriteString(w, strings.Repeat(" ", next-col)); err != nil {
				return err
			}
			col = next
		case ch == '\n':
			if _, err := io.WriteString(w, "\n"); err != nil {
				return err
			}
			col = 0
			convert = true
		default:
			if _, err := io.WriteString(w, string(ch)); err != nil {
				return err
			}
			if convert {
				if ch == '\b' {
					if col > 0 {
						col--
					}
				} else {
					col++
				}
				// Under -i, only tabs preceding all non-blank
				// characters are converted; a backspace also ends
				// the initial region (GNU treats any non-blank,
				// including \b, as ending it).
				if initial && ch != ' ' && ch != '\t' {
					convert = false
				}
			}
		}
	}
}

func expandStreamBytes(r io.Reader, w io.Writer, tabs *tabStops, initial bool) error {
	br := bufio.NewReader(r)
	col := 0
	convert := true
	for {
		ch, err := br.ReadByte()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		switch {
		case ch == '\t' && convert:
			next, _ := tabs.next(col)
			if _, err := io.WriteString(w, strings.Repeat(" ", next-col)); err != nil {
				return err
			}
			col = next
		case ch == '\n':
			if _, err := w.Write([]byte{'\n'}); err != nil {
				return err
			}
			col = 0
			convert = true
		default:
			if _, err := w.Write([]byte{ch}); err != nil {
				return err
			}
			if convert {
				if ch == '\b' {
					if col > 0 {
						col--
					}
				} else {
					col++
				}
				if initial && ch != ' ' && ch != '\t' {
					convert = false
				}
			}
		}
	}
}

// tabStops is a parsed --tabs specification, following the GNU manual:
// a single size repeats every N columns; an explicit ascending list
// sets individual stops, with tabs beyond the last stop replaced by
// single spaces unless the last entry carried a '/' (multiples of N
// beyond the list) or '+' (every N columns past the last explicit
// stop) prefix.
type tabStops struct {
	size      int   // single repeating interval; 0 when stops is authoritative
	stops     []int // explicit ascending tab stops
	extend    int   // '/N': stops continue at multiples of N past the list
	increment int   // '+N': stops continue every N past the last explicit stop
}

func parseTabStops(list []string) (*tabStops, error) {
	ts := &tabStops{}
	var entries []string
	for _, value := range list {
		parts := strings.Split(value, ",")
		for _, part := range parts {
			fields := strings.Fields(part)
			if len(fields) == 0 {
				return nil, fmt.Errorf("tab size contains invalid character(s): %q", value)
			}
			entries = append(entries, fields...)
		}
	}
	for i, entry := range entries {
		e := entry
		var spec byte
		if e[0] == '/' || e[0] == '+' {
			spec = e[0]
			e = e[1:]
		}
		n := 0
		if e == "" {
			return nil, fmt.Errorf("tab size contains invalid character(s): %q", entry)
		}
		for _, r := range e {
			if r < '0' || r > '9' {
				return nil, fmt.Errorf("tab size contains invalid character(s): %q", entry)
			}
			if n > (1<<30)/10 {
				return nil, fmt.Errorf("tab stop is too large %q", entry)
			}
			n = n*10 + int(r-'0')
			if n > 1<<30 {
				return nil, fmt.Errorf("tab stop is too large %q", entry)
			}
		}
		if n == 0 {
			return nil, fmt.Errorf("tab size cannot be 0")
		}
		switch spec {
		case '/':
			if i != len(entries)-1 {
				return nil, fmt.Errorf("'/' specifier only allowed with the last value")
			}
			ts.extend = n
		case '+':
			if i != len(entries)-1 {
				return nil, fmt.Errorf("'+' specifier only allowed with the last value")
			}
			ts.increment = n
		default:
			if len(ts.stops) > 0 && n <= ts.stops[len(ts.stops)-1] {
				return nil, fmt.Errorf("tab sizes must be ascending")
			}
			ts.stops = append(ts.stops, n)
		}
	}
	// Finalize per GNU: no explicit stops means a plain repeating size
	// (the '/' or '+' value if one was given, else 8); a single stop
	// with no specifier is also a plain repeating size.
	if len(ts.stops) == 0 {
		switch {
		case ts.extend > 0:
			ts.size, ts.extend = ts.extend, 0
		case ts.increment > 0:
			ts.size, ts.increment = ts.increment, 0
		default:
			ts.size = 8
		}
	} else if len(ts.stops) == 1 && ts.extend == 0 && ts.increment == 0 {
		ts.size = ts.stops[0]
		ts.stops = nil
	}
	return ts, nil
}

// next returns the first tab stop after col. last reports that col is
// past the last defined stop (the caller substitutes a single blank).
func (ts *tabStops) next(col int) (stop int, last bool) {
	if ts.size > 0 {
		return col + ts.size - col%ts.size, false
	}
	for _, s := range ts.stops {
		if s > col {
			return s, false
		}
	}
	if ts.extend > 0 {
		return col + ts.extend - col%ts.extend, false
	}
	if ts.increment > 0 {
		end := ts.stops[len(ts.stops)-1]
		return col + ts.increment - (col-end)%ts.increment, false
	}
	return col + 1, true
}

func openInput(rc *tool.RunContext, name string) (io.Reader, io.Closer, error) {
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
