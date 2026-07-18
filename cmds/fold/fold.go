// Package foldcmd implements fold(1): wrap input lines.
package foldcmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/mattn/go-runewidth"
	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "fold",
	Synopsis: "Wrap input lines in each FILE, writing to standard output.\nWith no FILE, or when FILE is -, read standard input.",
	Usage:    "fold [OPTION]... [FILE]...",
}

func init() { cmd.Run = run; tool.Register(cmd) }

type countMode int

const (
	countColumns countMode = iota
	countBytes
	countCharacters
)

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	widthValue := fs.StringP("width", "w", "80", "use WIDTH columns instead of 80")
	bytesMode := fs.BoolP("bytes", "b", false, "count bytes rather than columns")
	characters := fs.BoolP("characters", "c", false, "count characters rather than columns")
	spaces := fs.BoolP("spaces", "s", false, "break at spaces")
	operands, code := tool.Parse(rc, cmd, fs, rewriteObsoleteWidth(args))
	if code >= 0 {
		return code
	}
	width, err := strconv.Atoi(*widthValue)
	if err != nil || width <= 0 {
		return tool.UsageError(rc, cmd, "invalid number of columns: %q", *widthValue)
	}
	mode := countColumns
	if *bytesMode {
		mode = countBytes
	} else if *characters {
		mode = countCharacters
	}
	if len(operands) == 0 {
		operands = []string{"-"}
	}

	out := bufio.NewWriter(rc.Out)
	status := 0
	for _, name := range operands {
		r, closer, err := openInput(rc, name)
		if err != nil {
			fmt.Fprintf(rc.Err, "fold: %s: %v\n", name, err)
			status = 1
			continue
		}
		if err := foldStream(r, out, width, mode, *spaces); err != nil {
			fmt.Fprintf(rc.Err, "fold: %s: %v\n", name, err)
			status = 1
		}
		if closer != nil {
			closer.Close()
		}
	}
	if err := out.Flush(); err != nil {
		fmt.Fprintf(rc.Err, "fold: write error: %v\n", err)
		return 1
	}
	return status
}

// rewriteObsoleteWidth implements the obsolete option syntax
// (fold -72 == fold -w 72), which GNU fold accepts anywhere on the
// command line; the last width given wins.
func rewriteObsoleteWidth(args []string) []string {
	out := make([]string, 0, len(args))
	for i, a := range args {
		if a == "--" {
			out = append(out, args[i:]...)
			break
		}
		if len(a) >= 2 && a[0] == '-' && allDigits(a[1:]) {
			out = append(out, "-w"+a[1:])
			continue
		}
		out = append(out, a)
	}
	return out
}

func allDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

func foldStream(r io.Reader, w io.Writer, width int, mode countMode, spaces bool) error {
	if mode == countBytes {
		return foldBytesStream(r, w, width, spaces)
	}
	return foldColumnsStream(r, w, width, mode, spaces)
}

// foldColumnsStream folds counting display columns (default) or
// characters (-c). Per POSIX/GNU, in both modes a tab advances to the
// next multiple of 8, a backspace decreases the column count, and a
// carriage return resets it to zero. Nothing is ever deleted: with -s
// the break goes after the last blank (which is kept), and the
// remainder keeps its leading blanks.
func foldColumnsStream(r io.Reader, w io.Writer, width int, mode countMode, spaces bool) error {
	br := bufio.NewReader(r)
	var line []rune
	col := 0
	adjust := func(c int, ch rune) int {
		switch ch {
		case '\b':
			if c > 0 {
				c--
			}
		case '\r':
			c = 0
		case '\t':
			c += 8 - c%8
		default:
			cw := 1
			if mode == countColumns {
				cw = runewidth.RuneWidth(ch)
				if cw < 0 {
					cw = 1
				}
			}
			c += cw
		}
		return c
	}
	writeLine := func(rs []rune, nl bool) error {
		if _, err := io.WriteString(w, string(rs)); err != nil {
			return err
		}
		if nl {
			if _, err := io.WriteString(w, "\n"); err != nil {
				return err
			}
		}
		return nil
	}
	for {
		ch, _, err := br.ReadRune()
		if err == io.EOF {
			if len(line) > 0 {
				return writeLine(line, false)
			}
			return nil
		}
		if err != nil {
			return err
		}
		if ch == '\n' {
			if err := writeLine(line, true); err != nil {
				return err
			}
			line = line[:0]
			col = 0
			continue
		}
	rescan:
		newCol := adjust(col, ch)
		if newCol > width {
			if spaces {
				if i := lastBlankRune(line); i >= 0 {
					// Break after the blank, keeping it; the remainder
					// (leading blanks and all) starts the next line.
					if err := writeLine(line[:i+1], true); err != nil {
						return err
					}
					line = append(line[:0], line[i+1:]...)
					col = 0
					for _, r := range line {
						col = adjust(col, r)
					}
					goto rescan
				}
			}
			if len(line) == 0 {
				// A single unit wider than the width still goes out.
				line = append(line, ch)
				col = newCol
				continue
			}
			if err := writeLine(line, true); err != nil {
				return err
			}
			line = line[:0]
			col = 0
			goto rescan
		}
		line = append(line, ch)
		col = newCol
	}
}

// foldBytesStream folds counting bytes (-b): tabs, backspaces, and
// carriage returns each count one, like any other byte.
func foldBytesStream(r io.Reader, w io.Writer, width int, spaces bool) error {
	br := bufio.NewReader(r)
	var line []byte
	writeLine := func(bs []byte, nl bool) error {
		if _, err := w.Write(bs); err != nil {
			return err
		}
		if nl {
			if _, err := io.WriteString(w, "\n"); err != nil {
				return err
			}
		}
		return nil
	}
	for {
		c, err := br.ReadByte()
		if err == io.EOF {
			if len(line) > 0 {
				return writeLine(line, false)
			}
			return nil
		}
		if err != nil {
			return err
		}
		if c == '\n' {
			if err := writeLine(line, true); err != nil {
				return err
			}
			line = line[:0]
			continue
		}
	rescan:
		if len(line)+1 > width {
			if spaces {
				if i := lastBlankByte(line); i >= 0 {
					if err := writeLine(line[:i+1], true); err != nil {
						return err
					}
					line = append(line[:0], line[i+1:]...)
					goto rescan
				}
			}
			if len(line) == 0 {
				line = append(line, c)
				continue
			}
			if err := writeLine(line, true); err != nil {
				return err
			}
			line = line[:0]
			goto rescan
		}
		line = append(line, c)
	}
}

func lastBlankRune(rs []rune) int {
	for i := len(rs) - 1; i >= 0; i-- {
		if rs[i] == ' ' || rs[i] == '\t' {
			return i
		}
	}
	return -1
}

func lastBlankByte(bs []byte) int {
	for i := len(bs) - 1; i >= 0; i-- {
		if bs[i] == ' ' || bs[i] == '\t' {
			return i
		}
	}
	return -1
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
