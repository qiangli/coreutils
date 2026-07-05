// Package wccmd implements wc(1) per the GNU coreutils manual:
// print newline, word, character, byte, and maximum line length counts.
//
// Fresh implementation against the GNU manual (prior art consulted:
// guonaihong/coreutils wc, u-root wc, aict wc — none implements GNU's
// column-width rule or -L). Column width follows GNU: 7 for stdin /
// non-regular inputs, digits of the summed regular-file sizes
// otherwise, and 1 when a single count is printed for a single input.
package wccmd

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "wc",
	Synopsis: "Print newline, word, and byte counts for each FILE, and a total line if\nmore than one FILE is specified. With no FILE, or when FILE is -, read\nstandard input.",
	Usage:    "wc [OPTION]... [FILE]...",
}

func init() { cmd.Run = run; tool.Register(cmd) }

type selection struct {
	lines, words, chars, bytes, maxLine bool
}

func (s selection) enabled() int {
	n := 0
	for _, b := range []bool{s.lines, s.words, s.chars, s.bytes, s.maxLine} {
		if b {
			n++
		}
	}
	return n
}

type counts struct {
	lines, words, chars, bytes, maxLine int64
}

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	cBytes := fs.BoolP("bytes", "c", false, "print the byte counts")
	cChars := fs.BoolP("chars", "m", false, "print the character counts")
	cLines := fs.BoolP("lines", "l", false, "print the newline counts")
	cMaxLine := fs.BoolP("max-line-length", "L", false, "print the maximum display width")
	cWords := fs.BoolP("words", "w", false, "print the word counts")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}

	sel := selection{lines: *cLines, words: *cWords, chars: *cChars, bytes: *cBytes, maxLine: *cMaxLine}
	if sel.enabled() == 0 {
		sel = selection{lines: true, words: true, bytes: true}
	}

	width := numberWidth(rc, operands, sel.enabled())
	w := bufio.NewWriter(rc.Out)
	exit := 0

	if len(operands) == 0 {
		var in io.Reader = rc.In
		if in == nil {
			in = strings.NewReader("")
		}
		c, err := countReader(in, sel)
		if err != nil {
			fmt.Fprintf(rc.Err, "wc: %v\n", err)
			exit = 1
		}
		printRow(w, sel, c, width, "")
		w.Flush()
		return exit
	}

	var total counts
	for _, name := range operands {
		var r io.Reader
		var closer io.Closer
		if name == "-" {
			r = rc.In
			if r == nil {
				r = strings.NewReader("")
			}
		} else {
			f, err := os.Open(rc.Path(name))
			if err != nil {
				fmt.Fprintf(rc.Err, "wc: %s: %v\n", name, sysErr(err))
				exit = 1
				continue
			}
			r = f
			closer = f
		}
		c, err := countReader(r, sel)
		if closer != nil {
			closer.Close()
		}
		if err != nil {
			fmt.Fprintf(rc.Err, "wc: %s: %v\n", name, sysErr(err))
			exit = 1
		}
		total.lines += c.lines
		total.words += c.words
		total.chars += c.chars
		total.bytes += c.bytes
		if c.maxLine > total.maxLine {
			total.maxLine = c.maxLine
		}
		printRow(w, sel, c, width, name)
	}
	if len(operands) > 1 {
		printRow(w, sel, total, width, "total")
	}
	if err := w.Flush(); err != nil {
		fmt.Fprintf(rc.Err, "wc: write error: %v\n", err)
		return 1
	}
	return exit
}

// countReader makes one pass computing the counts sel needs. Only -m
// (chars) and -L (max-line-length) require decoding UTF-8 runes; for
// every other combination, lines/words/bytes are pure byte properties
// under C-locale rules (all word separators are ASCII whitespace
// bytes, which never occur inside a multi-byte UTF-8 sequence), so a
// block-wise byte scan produces identical counts far faster.
func countReader(r io.Reader, sel selection) (counts, error) {
	if sel.chars || sel.maxLine {
		return countRunes(r)
	}
	return countBytes(r, sel.words)
}

// isSpaceByte marks the C-locale whitespace bytes (word separators).
var isSpaceByte [256]bool

func init() {
	for _, b := range []byte{' ', '\t', '\n', '\v', '\f', '\r'} {
		isSpaceByte[b] = true
	}
}

// countBytes computes lines, words, and bytes with a block byte scan.
// Newlines are counted with bytes.Count (vectorized); word boundaries
// with a byte-indexed table. inWord carries across block boundaries.
func countBytes(r io.Reader, words bool) (counts, error) {
	var c counts
	buf := make([]byte, 64*1024)
	inWord := false
	for {
		n, err := r.Read(buf)
		if n > 0 {
			b := buf[:n]
			c.bytes += int64(n)
			c.lines += int64(bytes.Count(b, []byte{'\n'}))
			if words {
				for _, ch := range b {
					if isSpaceByte[ch] {
						inWord = false
					} else if !inWord {
						c.words++
						inWord = true
					}
				}
			}
		}
		if err == io.EOF {
			return c, nil
		}
		if err != nil {
			return c, err
		}
	}
}

// countRunes is the full rune-decoding pass, needed when -m or -L is
// selected. Word boundaries and line-length widths use C-locale rules
// (ASCII whitespace; only printable ASCII has width 1, tab advances
// to the next multiple of 8, \r and \f reset the position — matching
// GNU in the C locale). Characters are counted as UTF-8 runes; each
// invalid byte counts as one character.
func countRunes(r io.Reader) (counts, error) {
	br := bufio.NewReaderSize(r, 64*1024)
	var c counts
	inWord := false
	var linepos int64
	finishLine := func() {
		if linepos > c.maxLine {
			c.maxLine = linepos
		}
		linepos = 0
	}
	for {
		r0, size, err := br.ReadRune()
		if err == io.EOF {
			finishLine()
			return c, nil
		}
		if err != nil {
			finishLine()
			return c, err
		}
		c.bytes += int64(size)
		c.chars++

		switch r0 {
		case '\n':
			c.lines++
			finishLine()
		case '\r', '\f':
			finishLine()
		case '\t':
			linepos += 8 - linepos%8
		default:
			if r0 >= 32 && r0 <= 126 {
				linepos++
			}
		}

		switch r0 {
		case ' ', '\t', '\n', '\v', '\f', '\r':
			inWord = false
		default:
			if !inWord {
				c.words++
			}
			inWord = true
		}
	}
}

// numberWidth implements GNU wc's column-width rule.
func numberWidth(rc *tool.RunContext, operands []string, nsel int) int {
	if len(operands) == 0 {
		if nsel == 1 {
			return 1
		}
		return 7
	}
	if len(operands) == 1 && nsel == 1 {
		return 1
	}
	minW := 1
	var total int64
	for _, op := range operands {
		if op == "-" {
			minW = 7
			continue
		}
		st, err := os.Stat(rc.Path(op))
		if err != nil {
			continue
		}
		if st.Mode().IsRegular() {
			total += st.Size()
		} else {
			minW = 7
		}
	}
	if total == 0 {
		return minW
	}
	w := 0
	for v := total; v > 0; v /= 10 {
		w++
	}
	if w < minW {
		w = minW
	}
	return w
}

func printRow(w io.Writer, sel selection, c counts, width int, name string) {
	type field struct {
		on bool
		v  int64
	}
	first := true
	for _, f := range []field{
		{sel.lines, c.lines},
		{sel.words, c.words},
		{sel.chars, c.chars},
		{sel.bytes, c.bytes},
		{sel.maxLine, c.maxLine},
	} {
		if !f.on {
			continue
		}
		if first {
			fmt.Fprintf(w, "%*d", width, f.v)
			first = false
		} else {
			fmt.Fprintf(w, " %*d", width, f.v)
		}
	}
	if name != "" {
		fmt.Fprintf(w, " %s", name)
	}
	fmt.Fprintln(w)
}

func sysErr(err error) error {
	return tool.SysErr(err)
}
