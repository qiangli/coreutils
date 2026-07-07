// Package prcmd implements a non-interactive pr(1) subset: simple
// sequential pagination with optional headers, page length, and width.
package prcmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
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
	pageLength     int
	width          int
	omitHeader     bool
	header         string
	dateFormat     string
	doubleSpace    bool
	numberLines    bool
	indent         int
	noFileWarnings bool
	expandTabs     bool
	across         bool
	columns        int
	separator      string
	merge          bool
	formFeed       bool
	pageStart      int
	pageEnd        int
}

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	pageLength := fs.IntP("length", "l", 66, "set page length to PAGE_LENGTH lines")
	width := fs.IntP("width", "w", 72, "set page width to WIDTH columns")
	omitHeader := fs.BoolP("omit-header", "t", false, "omit page headers and trailers")
	omitPagination := fs.BoolP("omit-pagination", "T", false, "omit page headers and trailers and do not paginate")
	headerText := fs.StringP("header", "h", "", "use centered STRING instead of file name in page header")
	dateFormat := fs.StringP("date-format", "D", "", "use FORMAT for the header date")
	doubleSpace := fs.BoolP("double-space", "d", false, "double space the output")
	numberLines := fs.BoolP("number-lines", "n", false, "precede each line with its line number")
	indent := fs.IntP("indent", "o", 0, "offset each line with MARGIN spaces")
	noFileWarnings := fs.BoolP("no-file-warnings", "r", false, "omit file open warnings")
	pages := fs.String("pages", "", "print only pages in FIRST[:LAST] range")
	expandTabs := fs.BoolP("expand-tabs", "e", false, "expand input tabs to spaces")
	across := fs.BoolP("across", "a", false, "fill columns across rather than down")
	columns := fs.Int("columns", 1, "produce COLUMN columns")
	separator := fs.StringP("separator", "s", "\t", "separate columns by CHAR")
	sepString := fs.StringP("sep-string", "S", "", "separate columns by STRING")
	merge := fs.BoolP("merge", "m", false, "print files in parallel, one per column")
	formFeed := fs.BoolP("form-feed", "F", false, "use form feed between pages")
	pageWidth := fs.IntP("page-width", "W", 0, "set page width, always")
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
	if *pageWidth > 0 {
		*width = *pageWidth
	}
	if *indent < 0 {
		return tool.UsageError(rc, cmd, "invalid indent: %d", *indent)
	}
	if *columns <= 0 {
		return tool.UsageError(rc, cmd, "invalid column count: %d", *columns)
	}
	pageStart, pageEnd, err := parsePages(*pages)
	if err != nil {
		return tool.UsageError(rc, cmd, "%v", err)
	}
	sep := *separator
	if *sepString != "" {
		sep = *sepString
	}
	o := options{
		pageLength: *pageLength, width: *width, omitHeader: *omitHeader || *omitPagination,
		header: *headerText, dateFormat: *dateFormat, doubleSpace: *doubleSpace, numberLines: *numberLines,
		indent: *indent, noFileWarnings: *noFileWarnings, expandTabs: *expandTabs,
		across: *across, columns: *columns, separator: sep, merge: *merge, formFeed: *formFeed,
		pageStart: pageStart, pageEnd: pageEnd,
	}

	files := operands
	if len(files) == 0 {
		files = []string{"-"}
	}
	w := bufio.NewWriter(rc.Out)
	exit := 0
	if o.merge {
		exit = printMerged(rc, w, files, o)
		if err := w.Flush(); err != nil {
			fmt.Fprintf(rc.Err, "pr: write error: %v\n", err)
			return 1
		}
		return exit
	}
	for _, name := range files {
		r, closer, label, stamp, err := open(rc, name)
		if err != nil {
			if !o.noFileWarnings {
				fmt.Fprintf(rc.Err, "pr: %s: %v\n", name, tool.SysErr(err))
			}
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
	lines, err := readLines(r, o)
	if err != nil {
		return err
	}
	page := 1
	lineOnPage := 0
	lineNo := 1
	contentPerPage := o.pageLength
	if !o.omitHeader {
		contentPerPage = o.pageLength - 2
		if contentPerPage < 1 {
			contentPerPage = 1
		}
	}

	for _, line := range columnize(lines, o) {
		if lineOnPage == 0 && !o.omitHeader {
			headerLabel := label
			if o.header != "" {
				headerLabel = o.header
			}
			if inPageRange(page, o) {
				if _, werr := fmt.Fprintln(w, header(headerLabel, stamp, page, o)); werr != nil {
					return werr
				}
				if _, werr := w.WriteString("\n"); werr != nil {
					return werr
				}
			}
		}
		if inPageRange(page, o) {
			if _, werr := w.WriteString(formatLine(line, lineNo, o)); werr != nil {
				return werr
			}
			if o.doubleSpace {
				if _, werr := w.WriteString("\n"); werr != nil {
					return werr
				}
			}
		}
		lineNo++
		lineOnPage++
		if lineOnPage >= contentPerPage {
			lineOnPage = 0
			page++
			if !o.omitHeader {
				if inPageRange(page-1, o) {
					sep := "\n"
					if o.formFeed {
						sep = "\f"
					}
					if _, werr := w.WriteString(sep); werr != nil {
						return werr
					}
				}
			}
		}
	}
	return nil
}

func header(label string, stamp time.Time, page int, o options) string {
	name := label
	if name == "" {
		name = "standard input"
	}
	format := "2006-01-02 15:04"
	if o.dateFormat != "" {
		format = strftimeLayout(o.dateFormat)
	}
	text := fmt.Sprintf("%s  %s  Page %d", stamp.Format(format), name, page)
	return fitText(text, o.width)
}

func formatLine(line string, lineNo int, o options) string {
	hasNL := strings.HasSuffix(line, "\n")
	line = strings.TrimSuffix(line, "\n")
	if o.numberLines {
		line = fmt.Sprintf("%5d\t%s", lineNo, line)
	}
	if o.indent > 0 {
		line = strings.Repeat(" ", o.indent) + line
	}
	line = fitText(line, o.width)
	if hasNL {
		return line + "\n"
	}
	return line
}

func readLines(r io.Reader, o options) ([]string, error) {
	br := bufio.NewReader(r)
	var lines []string
	for {
		line, err := br.ReadString('\n')
		if len(line) > 0 {
			if o.expandTabs {
				line = expandTabs(line, 8)
			}
			lines = append(lines, line)
		}
		if err == io.EOF {
			return lines, nil
		}
		if err != nil {
			return nil, err
		}
	}
}

func columnize(lines []string, o options) []string {
	if o.columns <= 1 {
		return lines
	}
	rows := (len(lines) + o.columns - 1) / o.columns
	out := make([]string, 0, rows)
	colWidth := o.width / o.columns
	if colWidth < 1 {
		colWidth = 1
	}
	for r := 0; r < rows; r++ {
		var parts []string
		for c := 0; c < o.columns; c++ {
			idx := r*o.columns + c
			if !o.across {
				idx = c*rows + r
			}
			part := ""
			if idx < len(lines) {
				part = strings.TrimRight(lines[idx], "\n")
			}
			parts = append(parts, fitText(part, colWidth))
		}
		out = append(out, strings.Join(parts, o.separator)+"\n")
	}
	return out
}

func printMerged(rc *tool.RunContext, w *bufio.Writer, files []string, o options) int {
	var all [][]string
	exit := 0
	for _, name := range files {
		r, closer, _, _, err := open(rc, name)
		if err != nil {
			if !o.noFileWarnings {
				fmt.Fprintf(rc.Err, "pr: %s: %v\n", name, tool.SysErr(err))
			}
			exit = 1
			continue
		}
		lines, err := readLines(r, o)
		if closer != nil {
			closer.Close()
		}
		if err != nil {
			fmt.Fprintf(rc.Err, "pr: %s: %v\n", name, tool.SysErr(err))
			exit = 1
			continue
		}
		all = append(all, lines)
	}
	maxLines := 0
	for _, lines := range all {
		if len(lines) > maxLines {
			maxLines = len(lines)
		}
	}
	for i := 0; i < maxLines; i++ {
		var parts []string
		for _, lines := range all {
			part := ""
			if i < len(lines) {
				part = strings.TrimRight(lines[i], "\n")
			}
			parts = append(parts, part)
		}
		fmt.Fprintln(w, strings.Join(parts, o.separator))
	}
	return exit
}

func parsePages(s string) (int, int, error) {
	if s == "" {
		return 1, 0, nil
	}
	parts := strings.SplitN(s, ":", 2)
	start, err := strconv.Atoi(parts[0])
	if err != nil || start <= 0 {
		return 0, 0, fmt.Errorf("invalid page range: %q", s)
	}
	end := start
	if len(parts) == 2 {
		if parts[1] == "" {
			end = 0
		} else if end, err = strconv.Atoi(parts[1]); err != nil || end < start {
			return 0, 0, fmt.Errorf("invalid page range: %q", s)
		}
	}
	return start, end, nil
}

func inPageRange(page int, o options) bool {
	return page >= o.pageStart && (o.pageEnd == 0 || page <= o.pageEnd)
}

func expandTabs(s string, tabWidth int) string {
	var b strings.Builder
	col := 0
	for _, r := range s {
		if r == '\t' {
			spaces := tabWidth - col%tabWidth
			b.WriteString(strings.Repeat(" ", spaces))
			col += spaces
			continue
		}
		b.WriteRune(r)
		if r == '\n' {
			col = 0
		} else {
			col++
		}
	}
	return b.String()
}

func strftimeLayout(format string) string {
	replacements := []struct{ old, new string }{
		{"%Y", "2006"}, {"%y", "06"}, {"%m", "01"}, {"%d", "02"},
		{"%H", "15"}, {"%M", "04"}, {"%S", "05"}, {"%b", "Jan"},
		{"%B", "January"}, {"%a", "Mon"}, {"%A", "Monday"},
	}
	for _, r := range replacements {
		format = strings.ReplaceAll(format, r.old, r.new)
	}
	return format
}

func fitText(s string, width int) string {
	if len(s) <= width {
		return s
	}
	return s[:width]
}
