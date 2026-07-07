// Package expandcmd implements expand(1): convert tabs to spaces.
package expandcmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"

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
	tabsValue := fs.StringP("tabs", "t", "8", "use comma-separated tab stops instead of 8")
	initial := fs.BoolP("initial", "i", false, "do not convert tabs after non blanks")
	noUTF8 := fs.BoolP("no-utf8", "U", false, "interpret input as bytes rather than UTF-8")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	tabs, err := parseTabStops(*tabsValue)
	if err != nil {
		return tool.UsageError(rc, cmd, "invalid tab size: %q", *tabsValue)
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

func expandStream(r io.Reader, w io.Writer, tabs []int, initial, noUTF8 bool) error {
	if noUTF8 {
		return expandBytes(r, w, tabs, initial)
	}
	br := bufio.NewReader(r)
	col := 0
	convertTabs := true
	for {
		ch, size, err := br.ReadRune()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		switch ch {
		case '\t':
			if initial && !convertTabs {
				if _, err := io.WriteString(w, "\t"); err != nil {
					return err
				}
				col = nextStop(col, tabs)
				continue
			}
			n := nextStop(col, tabs) - col
			if n <= 0 {
				n = 1
			}
			if _, err := io.WriteString(w, strings.Repeat(" ", n)); err != nil {
				return err
			}
			col += n
		case '\n':
			if _, err := io.WriteString(w, "\n"); err != nil {
				return err
			}
			col = 0
			convertTabs = true
		case '\b':
			if _, err := io.WriteString(w, "\b"); err != nil {
				return err
			}
			if col > 0 {
				col--
			}
		default:
			if _, err := io.WriteString(w, string(ch)); err != nil {
				return err
			}
			if ch == utf8.RuneError && size == 1 {
				col++
			} else {
				col++
			}
			if ch != ' ' {
				convertTabs = false
			}
		}
	}
}

func expandBytes(r io.Reader, w io.Writer, tabs []int, initial bool) error {
	br := bufio.NewReader(r)
	col := 0
	convertTabs := true
	for {
		c, err := br.ReadByte()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		switch c {
		case '\t':
			if initial && !convertTabs {
				if _, err := w.Write([]byte{c}); err != nil {
					return err
				}
				col = nextStop(col, tabs)
				continue
			}
			n := nextStop(col, tabs) - col
			if n <= 0 {
				n = 1
			}
			if _, err := io.WriteString(w, strings.Repeat(" ", n)); err != nil {
				return err
			}
			col += n
		case '\n':
			if _, err := w.Write([]byte{c}); err != nil {
				return err
			}
			col = 0
			convertTabs = true
		case '\b':
			if _, err := w.Write([]byte{c}); err != nil {
				return err
			}
			if col > 0 {
				col--
			}
		default:
			if _, err := w.Write([]byte{c}); err != nil {
				return err
			}
			col++
			if c != ' ' {
				convertTabs = false
			}
		}
	}
}

func parseTabStops(s string) ([]int, error) {
	if s == "" {
		return nil, fmt.Errorf("empty")
	}
	parts := strings.Split(s, ",")
	out := make([]int, 0, len(parts))
	prev := 0
	for _, p := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil || n <= 0 || n <= prev {
			return nil, fmt.Errorf("invalid")
		}
		out = append(out, n)
		prev = n
	}
	return out, nil
}

func nextStop(col int, stops []int) int {
	if len(stops) == 1 {
		n := stops[0]
		return ((col / n) + 1) * n
	}
	for _, stop := range stops {
		if stop > col {
			return stop
		}
	}
	return col + 1
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
