// Package prcmd implements a non-interactive pr(1) subset: simple
// sequential pagination with optional headers, page length, and width.
package prcmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "pr",
	Synopsis: "Paginate or columnate files for printing. This pure-Go subset formats files sequentially without interactive features.",
	Usage:    "pr [OPTION]... [FILE]...",
}

func init() { cmd.Run = run; tool.Register(cmd) }

type options struct {
	pageLength int
	width      int
	omitHeader bool
}

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	pageLength := fs.IntP("length", "l", 66, "set page length to PAGE_LENGTH lines")
	width := fs.IntP("width", "w", 72, "set page width to WIDTH columns")
	omitHeader := fs.BoolP("omit-header", "t", false, "omit page headers and trailers")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	if *pageLength <= 0 {
		return tool.UsageError(rc, cmd, "invalid page length: %d", *pageLength)
	}
	if *width <= 0 {
		return tool.UsageError(rc, cmd, "invalid page width: %d", *width)
	}
	o := options{pageLength: *pageLength, width: *width, omitHeader: *omitHeader}

	files := operands
	if len(files) == 0 {
		files = []string{"-"}
	}
	w := bufio.NewWriter(rc.Out)
	exit := 0
	for _, name := range files {
		r, closer, label, stamp, err := open(rc, name)
		if err != nil {
			fmt.Fprintf(rc.Err, "pr: %s: %v\n", name, tool.SysErr(err))
			exit = 1
			continue
		}
		if err := printFile(r, w, label, stamp, o); err != nil {
			fmt.Fprintf(rc.Err, "pr: %s: %v\n", name, tool.SysErr(err))
			exit = 1
		}
		if closer != nil {
			closer.Close()
		}
	}
	if err := w.Flush(); err != nil {
		fmt.Fprintf(rc.Err, "pr: write error: %v\n", err)
		return 1
	}
	return exit
}

func open(rc *tool.RunContext, name string) (io.Reader, io.Closer, string, time.Time, error) {
	if name == "-" {
		if rc.In == nil {
			return strings.NewReader(""), nil, "", time.Now(), nil
		}
		return rc.In, nil, "", time.Now(), nil
	}
	path := rc.Path(name)
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, "", time.Time{}, err
	}
	stamp := time.Now()
	if st, err := f.Stat(); err == nil {
		stamp = st.ModTime()
	}
	return f, f, name, stamp, nil
}

func printFile(r io.Reader, w *bufio.Writer, label string, stamp time.Time, o options) error {
	br := bufio.NewReader(r)
	page := 1
	lineOnPage := 0
	contentPerPage := o.pageLength
	if !o.omitHeader {
		contentPerPage = o.pageLength - 2
		if contentPerPage < 1 {
			contentPerPage = 1
		}
	}

	for {
		line, err := br.ReadString('\n')
		if len(line) > 0 {
			if lineOnPage == 0 && !o.omitHeader {
				if _, werr := fmt.Fprintln(w, header(label, stamp, page, o.width)); werr != nil {
					return werr
				}
				if _, werr := w.WriteString("\n"); werr != nil {
					return werr
				}
			}
			if _, werr := w.WriteString(fitLine(line, o.width)); werr != nil {
				return werr
			}
			lineOnPage++
			if lineOnPage >= contentPerPage {
				lineOnPage = 0
				page++
				if !o.omitHeader {
					if _, werr := w.WriteString("\n"); werr != nil {
						return werr
					}
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

func header(label string, stamp time.Time, page, width int) string {
	name := label
	if name == "" {
		name = "standard input"
	}
	text := fmt.Sprintf("%s  %s  Page %d", stamp.Format("2006-01-02 15:04"), name, page)
	return fitText(text, width)
}

func fitLine(line string, width int) string {
	hasNL := strings.HasSuffix(line, "\n")
	line = strings.TrimSuffix(line, "\n")
	line = fitText(line, width)
	if hasNL {
		return line + "\n"
	}
	return line
}

func fitText(s string, width int) string {
	if len(s) <= width {
		return s
	}
	return s[:width]
}
