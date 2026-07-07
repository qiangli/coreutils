// Package nlcmd implements a focused nl(1) subset: line numbering for
// files and standard input with the common body style, number format,
// separator, and width options.
package nlcmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "nl",
	Synopsis: "Number lines of FILE(s), or standard input.",
	Usage:    "nl [OPTION]... [FILE]...",
}

func init() { cmd.Run = run; tool.Register(cmd) }

type options struct {
	bodyStyle string
	format    string
	separator string
	width     int
}

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	bodyStyle := fs.StringP("body-numbering", "b", "t", "use STYLE for numbering body lines: a (all), t (nonempty), n (none)")
	format := fs.StringP("number-format", "n", "rn", "insert line numbers according to FORMAT: ln, rn, rz")
	separator := fs.StringP("number-separator", "s", "\t", "add SEP after each line number")
	width := fs.IntP("number-width", "w", 6, "use WIDTH columns for line numbers")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	o := options{bodyStyle: *bodyStyle, format: *format, separator: *separator, width: *width}
	if o.bodyStyle != "a" && o.bodyStyle != "t" && o.bodyStyle != "n" {
		return tool.UsageError(rc, cmd, "invalid body numbering style: %q", o.bodyStyle)
	}
	if o.format != "ln" && o.format != "rn" && o.format != "rz" {
		return tool.UsageError(rc, cmd, "invalid number format: %q", o.format)
	}
	if o.width < 0 {
		return tool.UsageError(rc, cmd, "invalid line number width: %d", o.width)
	}

	files := operands
	if len(files) == 0 {
		files = []string{"-"}
	}
	w := bufio.NewWriter(rc.Out)
	lineNo := 1
	exit := 0
	for _, name := range files {
		r, closer, err := open(rc, name)
		if err != nil {
			fmt.Fprintf(rc.Err, "nl: %s: %v\n", name, tool.SysErr(err))
			exit = 1
			continue
		}
		if err := number(r, w, o, &lineNo); err != nil {
			fmt.Fprintf(rc.Err, "nl: %s: %v\n", name, tool.SysErr(err))
			exit = 1
		}
		if closer != nil {
			closer.Close()
		}
	}
	if err := w.Flush(); err != nil {
		fmt.Fprintf(rc.Err, "nl: write error: %v\n", err)
		return 1
	}
	return exit
}

func open(rc *tool.RunContext, name string) (io.Reader, io.Closer, error) {
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

func number(r io.Reader, w *bufio.Writer, o options, lineNo *int) error {
	br := bufio.NewReader(r)
	for {
		line, err := br.ReadString('\n')
		if len(line) > 0 {
			numbered := shouldNumber(o.bodyStyle, line)
			if numbered {
				if _, werr := fmt.Fprint(w, formatNumber(*lineNo, o)); werr != nil {
					return werr
				}
				*lineNo++
			} else if o.bodyStyle != "n" {
				if _, werr := fmt.Fprintf(w, "%*s%s", o.width, "", o.separator); werr != nil {
					return werr
				}
			}
			if _, werr := w.WriteString(line); werr != nil {
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

func shouldNumber(style, line string) bool {
	switch style {
	case "a":
		return true
	case "t":
		return strings.TrimRight(line, "\n") != ""
	default:
		return false
	}
}

func formatNumber(n int, o options) string {
	switch o.format {
	case "ln":
		return fmt.Sprintf("%-*d%s", o.width, n, o.separator)
	case "rz":
		return fmt.Sprintf("%0*d%s", o.width, n, o.separator)
	default:
		return fmt.Sprintf("%*d%s", o.width, n, o.separator)
	}
}
