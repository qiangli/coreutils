// Package foldcmd implements fold(1): wrap input lines.
package foldcmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode"

	"github.com/mattn/go-runewidth"
	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "fold",
	Synopsis: "Wrap input lines to fit in a specified width.\nWith no FILE, or when FILE is -, read standard input.",
	Usage:    "fold [OPTION]... [FILE]...",
}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	widthValue := fs.StringP("width", "w", "80", "use WIDTH columns instead of 80")
	bytesMode := fs.BoolP("bytes", "b", false, "count bytes rather than columns")
	characters := fs.BoolP("characters", "c", false, "count characters rather than display columns")
	spaces := fs.BoolP("spaces", "s", false, "break at spaces when possible")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	width, err := strconv.Atoi(*widthValue)
	if err != nil || width <= 0 {
		return tool.UsageError(rc, cmd, "invalid number of columns: %q", *widthValue)
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
		if err := foldStream(r, out, width, *bytesMode, *characters, *spaces); err != nil {
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

func foldStream(r io.Reader, w io.Writer, width int, bytesMode, characters, spaces bool) error {
	br := bufio.NewReader(r)
	for {
		line, err := br.ReadString('\n')
		if len(line) > 0 {
			hasNL := strings.HasSuffix(line, "\n")
			if hasNL {
				line = strings.TrimSuffix(line, "\n")
			}
			var out string
			if bytesMode {
				out = foldBytes(line, width, spaces)
			} else if characters {
				out = foldRunes(line, width, spaces)
			} else {
				out = foldDisplay(line, width, spaces)
			}
			if _, werr := io.WriteString(w, out); werr != nil {
				return werr
			}
			if hasNL {
				if _, werr := io.WriteString(w, "\n"); werr != nil {
					return werr
				}
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

func foldRunes(s string, width int, spaces bool) string {
	rs := []rune(s)
	var b strings.Builder
	for len(rs) > width {
		cut := width
		if spaces {
			limit := width
			if len(rs) > width && unicode.IsSpace(rs[width]) {
				limit = width + 1
			}
			for i := limit; i > 0; i-- {
				if unicode.IsSpace(rs[i-1]) {
					cut = i
					break
				}
			}
		}
		b.WriteString(strings.TrimRightFunc(string(rs[:cut]), unicode.IsSpace))
		b.WriteByte('\n')
		rs = rs[cut:]
		if spaces {
			for len(rs) > 0 && unicode.IsSpace(rs[0]) {
				rs = rs[1:]
			}
		}
	}
	b.WriteString(string(rs))
	return b.String()
}

type displayUnit struct {
	r rune
	w int
}

func foldDisplay(s string, width int, spaces bool) string {
	units := make([]displayUnit, 0, len(s))
	for _, r := range s {
		w := runewidth.RuneWidth(r)
		if w < 0 {
			w = 0
		}
		units = append(units, displayUnit{r: r, w: w})
	}
	var b strings.Builder
	for displayWidth(units) > width {
		cut := displayFit(units, width)
		if spaces {
			limit := cut
			if cut < len(units) && unicode.IsSpace(units[cut].r) {
				limit = cut + 1
			}
			for i := limit; i > 0; i-- {
				if unicode.IsSpace(units[i-1].r) {
					cut = i
					break
				}
			}
		}
		writeDisplayUnits(&b, trimRightSpaceUnits(units[:cut]))
		b.WriteByte('\n')
		units = units[cut:]
		if spaces {
			for len(units) > 0 && unicode.IsSpace(units[0].r) {
				units = units[1:]
			}
		}
	}
	writeDisplayUnits(&b, units)
	return b.String()
}

func displayWidth(units []displayUnit) int {
	n := 0
	for _, u := range units {
		n += u.w
	}
	return n
}

func displayFit(units []displayUnit, width int) int {
	col := 0
	for i, u := range units {
		if col+u.w > width {
			if i == 0 {
				return 1
			}
			return i
		}
		col += u.w
	}
	return len(units)
}

func trimRightSpaceUnits(units []displayUnit) []displayUnit {
	for len(units) > 0 && unicode.IsSpace(units[len(units)-1].r) {
		units = units[:len(units)-1]
	}
	return units
}

func writeDisplayUnits(b *strings.Builder, units []displayUnit) {
	for _, u := range units {
		b.WriteRune(u.r)
	}
}

func foldBytes(s string, width int, spaces bool) string {
	bs := []byte(s)
	var b strings.Builder
	for len(bs) > width {
		cut := width
		if spaces {
			limit := width
			if len(bs) > width && (bs[width] == ' ' || bs[width] == '\t') {
				limit = width + 1
			}
			for i := limit; i > 0; i-- {
				if bs[i-1] == ' ' || bs[i-1] == '\t' {
					cut = i
					break
				}
			}
		}
		b.WriteString(strings.TrimRight(string(bs[:cut]), " \t"))
		b.WriteByte('\n')
		bs = bs[cut:]
		if spaces {
			for len(bs) > 0 && (bs[0] == ' ' || bs[0] == '\t') {
				bs = bs[1:]
			}
		}
	}
	b.Write(bs)
	return b.String()
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
