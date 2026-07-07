// Package fmtcmd implements fmt(1): simple paragraph formatting.
package fmtcmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "fmt",
	Synopsis: "Reformat paragraphs from FILE(s), writing to standard output.\nWith no FILE, or when FILE is -, read standard input.",
	Usage:    "fmt [OPTION]... [FILE]...",
}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	widthValue := fs.StringP("width", "w", "75", "maximum line width")
	splitOnly := fs.BoolP("split-only", "s", false, "split long lines, but do not refill")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	width, err := strconv.Atoi(*widthValue)
	if err != nil || width <= 0 {
		return tool.UsageError(rc, cmd, "invalid width: %q", *widthValue)
	}
	if len(operands) == 0 {
		operands = []string{"-"}
	}

	out := bufio.NewWriter(rc.Out)
	status := 0
	for _, name := range operands {
		r, closer, err := openInput(rc, name)
		if err != nil {
			fmt.Fprintf(rc.Err, "fmt: %s: %v\n", name, err)
			status = 1
			continue
		}
		if err := fmtStream(r, out, width, *splitOnly); err != nil {
			fmt.Fprintf(rc.Err, "fmt: %s: %v\n", name, err)
			status = 1
		}
		if closer != nil {
			closer.Close()
		}
	}
	if err := out.Flush(); err != nil {
		fmt.Fprintf(rc.Err, "fmt: write error: %v\n", err)
		return 1
	}
	return status
}

func fmtStream(r io.Reader, w io.Writer, width int, splitOnly bool) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1024), 1024*1024)
	var para []string
	flush := func() error {
		if len(para) == 0 {
			return nil
		}
		var out string
		if splitOnly {
			out = splitLines(para, width)
		} else {
			out = wrapWords(strings.Fields(strings.Join(para, " ")), width)
		}
		_, err := io.WriteString(w, out)
		para = para[:0]
		return err
	}
	for sc.Scan() {
		line := sc.Text()
		if strings.TrimSpace(line) == "" {
			if err := flush(); err != nil {
				return err
			}
			if _, err := io.WriteString(w, "\n"); err != nil {
				return err
			}
			continue
		}
		para = append(para, line)
	}
	if err := sc.Err(); err != nil {
		return err
	}
	return flush()
}

func splitLines(lines []string, width int) string {
	var b strings.Builder
	for _, line := range lines {
		b.WriteString(wrapWords(strings.Fields(line), width))
	}
	return b.String()
}

func wrapWords(words []string, width int) string {
	var b strings.Builder
	lineLen := 0
	for _, word := range words {
		if word == "" {
			continue
		}
		rs := []rune(word)
		if lineLen == 0 {
			for len(rs) > width {
				b.WriteString(string(rs[:width]))
				b.WriteByte('\n')
				rs = rs[width:]
			}
			b.WriteString(string(rs))
			lineLen = len(rs)
			continue
		}
		if lineLen+1+len(rs) <= width {
			b.WriteByte(' ')
			b.WriteString(word)
			lineLen += 1 + len(rs)
			continue
		}
		b.WriteByte('\n')
		lineLen = 0
		for len(rs) > width {
			b.WriteString(string(rs[:width]))
			b.WriteByte('\n')
			rs = rs[width:]
		}
		b.WriteString(strings.TrimLeftFunc(string(rs), unicode.IsSpace))
		lineLen = len(rs)
	}
	if lineLen > 0 {
		b.WriteByte('\n')
	}
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
