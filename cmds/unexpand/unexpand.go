// Package unexpandcmd implements unexpand(1): convert spaces to tabs.
package unexpandcmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "unexpand",
	Synopsis: "Convert blanks in each FILE to tabs, writing to standard output.\nBy default only leading blanks are converted. With -a, also convert all\nsequences of two or more blanks before a tab stop.\nWith no FILE, or when FILE is -, read standard input.",
	Usage:    "unexpand [OPTION]... [FILE]...",
}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	tabsValue := fs.StringArrayP("tabs", "t", []string{"8"}, "have tabs N characters apart instead of 8 (enables -a); or use comma- or blank-separated LIST of explicit tab positions (repeatable; the last position may be prefixed with '/' for multiples or '+' for an increment)")
	all := fs.BoolP("all", "a", false, "convert all blanks, instead of just initial blanks")
	firstOnly := fs.Bool("first-only", false, "convert only leading sequences of blanks (overrides -a)")
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
	convertAll := *all || fs.Changed("tabs")
	if *firstOnly {
		convertAll = false
	}

	out := bufio.NewWriter(rc.Out)
	status := 0
	for _, name := range operands {
		r, closer, err := openInput(rc, name)
		if err != nil {
			fmt.Fprintf(rc.Err, "unexpand: %s: %v\n", name, err)
			status = 1
			continue
		}
		if err := unexpandStream(r, out, tabs, convertAll, *noUTF8); err != nil {
			fmt.Fprintf(rc.Err, "unexpand: %s: %v\n", name, err)
			status = 1
		}
		if closer != nil {
			closer.Close()
		}
	}
	if err := out.Flush(); err != nil {
		fmt.Fprintf(rc.Err, "unexpand: write error: %v\n", err)
		return 1
	}
	return status
}

func unexpandStream(r io.Reader, w io.Writer, tabs *tabStops, all bool, noUTF8 bool) error {
	br := bufio.NewReader(r)
	for {
		line, err := br.ReadString('\n')
		if len(line) > 0 {
			out := unexpandLine(line, tabs, all)
			if noUTF8 {
				out = unexpandLineBytes(line, tabs, all)
			}
			if _, werr := io.WriteString(w, out); werr != nil {
				return werr
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

func unexpandLineBytes(line string, tabs *tabStops, all bool) string {
	var b strings.Builder
	var pending []byte
	col := 0
	convert := true
	oneBlankBeforeStop := false
	prevBlank := true
	flush := func() {
		if len(pending) == 0 {
			return
		}
		if len(pending) > 1 && oneBlankBeforeStop {
			pending[0] = '\t'
		}
		for _, p := range pending {
			b.WriteByte(p)
		}
		pending = pending[:0]
		oneBlankBeforeStop = false
	}
	for i := 0; i < len(line); i++ {
		ch := line[i]
		if !convert {
			b.WriteByte(ch)
			continue
		}
		blank := ch == ' ' || ch == '\t'
		writeCh := true
		if blank {
			next, last := tabs.next(col)
			switch {
			case last:
				convert = false
			case ch == '\t':
				col = next
				if len(pending) > 0 {
					pending[0] = '\t'
				}
				if oneBlankBeforeStop {
					pending = pending[:1]
				} else {
					pending = pending[:0]
				}
			default:
				col++
				if !(prevBlank && col >= next) {
					if col == next {
						oneBlankBeforeStop = true
					}
					pending = append(pending, ch)
					prevBlank = true
					continue
				}
				b.WriteByte('\t')
				if oneBlankBeforeStop {
					pending = pending[:1]
					pending[0] = '\t'
				} else {
					pending = pending[:0]
				}
				writeCh = false
			}
		} else if ch == '\b' {
			if col > 0 {
				col--
			}
		} else {
			col++
		}
		flush()
		prevBlank = blank
		if !all && !blank {
			convert = false
		}
		if writeCh {
			b.WriteByte(ch)
		}
	}
	flush()
	return b.String()
}

// unexpandLine converts blanks in one line (with or without a trailing
// newline) following the GNU rules:
//
//   - Only a maximal run of blanks (spaces and tabs together) that
//     reaches a tab stop is replaced, and — except at the start of a
//     line — only when the run spans two or more columns: a single
//     interior space is never turned into a tab.
//   - Blanks beyond the last explicit tab stop are left unchanged.
//   - A backspace decrements the column count.
//   - In default (first-only) mode conversion stops at the first
//     non-blank character.
func unexpandLine(line string, tabs *tabStops, all bool) string {
	var b strings.Builder
	var pending []rune // buffered blanks not yet decided
	col := 0
	convert := true
	oneBlankBeforeStop := false // a single pending blank ended exactly on a stop
	prevBlank := true           // line start acts as if preceded by a blank
	flush := func() {
		if len(pending) == 0 {
			return
		}
		if len(pending) > 1 && oneBlankBeforeStop {
			// The run started with a blank that ended exactly on a tab
			// stop: that first blank becomes the tab.
			pending[0] = '\t'
		}
		for _, p := range pending {
			b.WriteRune(p)
		}
		pending = pending[:0]
		oneBlankBeforeStop = false
	}
	for _, ch := range line {
		if !convert {
			b.WriteRune(ch)
			continue
		}
		blank := ch == ' ' || ch == '\t'
		writeCh := true
		if blank {
			next, last := tabs.next(col)
			switch {
			case last:
				// Past the last tab stop: leave the rest of the line
				// (including this blank) unchanged.
				convert = false
			case ch == '\t':
				col = next
				// A tab absorbs any pending blanks into itself…
				if len(pending) > 0 {
					pending[0] = '\t'
				}
				// …keeping one converted blank only if a single blank
				// ended exactly on the previous tab stop.
				if oneBlankBeforeStop {
					pending = pending[:1]
				} else {
					pending = pending[:0]
				}
			default: // space
				col++
				if !(prevBlank && col >= next) {
					if col == next {
						oneBlankBeforeStop = true
					}
					pending = append(pending, ch)
					prevBlank = true
					continue
				}
				// A run of two or more blanks reached the stop:
				// replace it (and this space) with a tab.
				b.WriteByte('\t')
				if oneBlankBeforeStop {
					pending = pending[:1]
					pending[0] = '\t'
				} else {
					pending = pending[:0]
				}
				writeCh = false
			}
		} else if ch == '\b' {
			if col > 0 {
				col--
			}
		} else {
			col++
		}
		flush()
		prevBlank = blank
		if !all && !blank {
			convert = false
		}
		if writeCh {
			b.WriteRune(ch)
		}
	}
	flush() // a final line without '\n' still flushes its pending blanks
	return b.String()
}

// tabStops is a parsed --tabs specification, following the GNU manual:
// a single size repeats every N columns; an explicit ascending list
// sets individual stops, with blanks beyond the last stop left
// unchanged unless the last entry carried a '/' (multiples of N beyond
// the list) or '+' (every N columns past the last explicit stop)
// prefix.
type tabStops struct {
	size      int   // single repeating interval; 0 when stops is authoritative
	stops     []int // explicit ascending tab stops
	extend    int   // '/N': stops continue at multiples of N past the list
	increment int   // '+N': stops continue every N past the last explicit stop
}

func parseTabStops(list []string) (*tabStops, error) {
	ts := &tabStops{}
	entries := strings.FieldsFunc(strings.Join(list, ","), func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t'
	})
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
// past the last defined stop (blanks there are left unchanged).
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
