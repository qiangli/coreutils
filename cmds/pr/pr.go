// Package prcmd implements a pr(1) subset: GNU page
// structure (66-line pages: 5-line header, body, 5-line trailer, with
// the last page padded to full length), form-feed page breaks, page
// ranges (--pages and the +FIRST[:LAST] operand), line numbering,
// margins, and -t/-T.
//
// Across-column output (-a) and merging (-m) are not implemented and
// fail loudly.
//
// Documented deviation: like GNU pr, the header timestamp for standard
// input (or when stat fails) is the current wall-clock time, so that
// header line is nondeterministic.
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
	Synopsis: "Paginate files for printing.",
	Usage:    "pr [OPTION]... [FILE]...",
}

func init() { cmd.Run = run; tool.Register(cmd) }

const (
	linesPerHeader  = 5
	linesPerTrailer = 5
)

type options struct {
	pageLength     int
	bodyLines      int // input lines per page
	width          int
	truncate       bool // only with -W
	omitHeader     bool
	ffBreaks       bool // false with -T: input form feeds do not paginate
	header         string
	headerSet      bool
	dateFormat     string
	doubleSpace    bool
	numberLines    bool
	indent         int
	noFileWarnings bool
	expandTabs     bool
	formFeed       bool
	pageStart      int
	pageEnd        int
	firstLineNum   int
	columns        int
	separator      string
}

func run(rc *tool.RunContext, args []string) int {
	args = scanColumnOption(args)
	fs := tool.NewFlags(cmd.Name)
	pageLength := fs.IntP("length", "l", 66, "set page length to PAGE_LENGTH lines (<= 10 implies -t)")
	width := fs.IntP("width", "w", 72, "set page width to PAGE_WIDTH columns for multi-column output")
	omitHeader := fs.BoolP("omit-header", "t", false, "omit page headers and trailers, do not pad the last page")
	omitPagination := fs.BoolP("omit-pagination", "T", false, "like -t, and eliminate input form-feed pagination")
	headerText := fs.StringP("header", "h", "", "use centered HEADER instead of file name in page header")
	dateFormat := fs.StringP("date-format", "D", "", "use FORMAT for the header date")
	doubleSpace := fs.BoolP("double-space", "d", false, "double space the output")
	numberLines := fs.BoolP("number-lines", "n", false, "precede each line with its line number")
	indent := fs.IntP("indent", "o", 0, "offset each line with MARGIN spaces")
	noFileWarnings := fs.BoolP("no-file-warnings", "r", false, "omit file open warnings")
	pages := fs.String("pages", "", "print only pages in FIRST[:LAST] range")
	expandTabs := fs.BoolP("expand-tabs", "e", false, "expand input tabs to spaces")
	across := fs.BoolP("across", "a", false, "(not supported) fill columns across rather than down")
	columns := fs.Int("columns", 1, "produce COLUMN columns, filled down")
	fs.IntP("column", "", 1, "alias for --columns")
	separator := fs.StringP("separator", "s", "", "separate columns by CHAR")
	sepString := fs.StringP("sep-string", "S", "", "separate columns by STRING")
	merge := fs.BoolP("merge", "m", false, "(not supported) print files in parallel, one per column")
	formFeed := fs.BoolP("form-feed", "F", false, "use form feed instead of blank lines to end pages")
	formFeedLower := fs.BoolP("f", "f", false, "use form feed instead of blank lines to end pages")
	pageWidth := fs.IntP("page-width", "W", 72, "set page width and truncate lines")
	firstLineNum := fs.IntP("first-line-number", "N", 1, "start counting line numbers at NUMBER")
	joinLines := fs.BoolP("join-lines", "J", false, "merge full-length lines (GNU compat, no-op in this subset)")
	indentStyle := fs.BoolP("", "i", false, "indent style alias (GNU compat, no-op in this subset)")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	if *merge {
		return tool.NotSupported(rc, cmd, "-m/--merge (parallel file merging)")
	}
	if fs.Changed("column") {
		cv, _ := fs.GetInt("column")
		*columns = cv
	}
	if *across {
		return tool.NotSupported(rc, cmd, "-a/--across (multi-column output)")
	}
	if *columns <= 0 {
		return tool.UsageError(rc, cmd, "invalid column count: %d", *columns)
	}
	if *pageLength <= 0 {
		return tool.UsageError(rc, cmd, "invalid page length: %d", *pageLength)
	}
	if *width <= 0 {
		return tool.UsageError(rc, cmd, "invalid page width: %d", *width)
	}
	if *pageWidth <= 0 {
		return tool.UsageError(rc, cmd, "invalid page width: %d", *pageWidth)
	}
	if *indent < 0 {
		return tool.UsageError(rc, cmd, "invalid indent: %d", *indent)
	}
	if *firstLineNum < 1 {
		return tool.UsageError(rc, cmd, "invalid first line number: %d", *firstLineNum)
	}
	pageStart, pageEnd, err := parsePages(*pages)
	if err != nil {
		return tool.UsageError(rc, cmd, "%v", err)
	}

	o := options{
		pageLength: *pageLength,
		width:      *width,
		header:     *headerText, headerSet: fs.Changed("header"),
		dateFormat: *dateFormat, doubleSpace: *doubleSpace, numberLines: *numberLines,
		indent: *indent, noFileWarnings: *noFileWarnings, expandTabs: *expandTabs,
		formFeed:  *formFeed || *formFeedLower,
		ffBreaks:  !*omitPagination,
		pageStart: pageStart, pageEnd: pageEnd,
		firstLineNum: *firstLineNum,
		columns:      *columns, separator: *separator,
	}
	if *sepString != "" {
		o.separator = *sepString
	}
	_ = joinLines
	_ = indentStyle
	if fs.Changed("page-width") {
		// -W sets the page width and enables line truncation; plain -w
		// never truncates single-column output (GNU semantics).
		o.width = *pageWidth
		o.truncate = true
	}
	// A page too short to hold the 5-line header and 5-line trailer
	// implies -t (GNU: page length <= 10).
	o.omitHeader = *omitHeader || *omitPagination || o.pageLength <= linesPerHeader+linesPerTrailer
	if o.omitHeader {
		o.bodyLines = o.pageLength
	} else {
		o.bodyLines = o.pageLength - linesPerHeader - linesPerTrailer
	}
	if o.doubleSpace {
		o.bodyLines /= 2
		if o.bodyLines < 1 {
			o.bodyLines = 1
		}
	}

	// The GNU/POSIX +FIRST[:LAST] operand is an alternative page range.
	var files []string
	for _, op := range operands {
		if strings.HasPrefix(op, "+") {
			start, end, err := parsePages(op[1:])
			if err != nil || op == "+" {
				return tool.UsageError(rc, cmd, "invalid page range: %q", op)
			}
			o.pageStart, o.pageEnd = start, end
			continue
		}
		files = append(files, op)
	}
	if len(files) == 0 {
		files = []string{"-"}
	}

	w := bufio.NewWriter(rc.Out)
	exit := 0
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

// scanColumnOption recognizes pr's standalone -N column shorthand. It is
// deliberately a small pre-scan: values belonging to options such as -o are
// left alone, so a negative numeric value is still reported by that option's
// normal validation.
func scanColumnOption(args []string) []string {
	const requiresValue = "-D -N -S -W -h -l -o -s -w --date-format --first-line-number --header --indent --length --page-width --separator --sep-string --width --pages --columns --column"
	needValue := map[string]bool{}
	for _, name := range strings.Fields(requiresValue) {
		needValue[name] = true
	}
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			out = append(out, args[i:]...)
			break
		}
		if i > 0 && needValue[args[i-1]] {
			out = append(out, arg)
			continue
		}
		if len(arg) > 1 && arg[0] == '-' && strings.Trim(arg[1:], "0123456789") == "" {
			out = append(out, "--columns="+arg[1:])
			continue
		}
		out = append(out, arg)
	}
	return out
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
	segments, err := readSegments(r, o)
	if err != nil {
		return err
	}
	if !o.ffBreaks {
		// -T: eliminate form-feed pagination; treat input as one segment.
		var all []string
		for _, seg := range segments {
			all = append(all, seg...)
		}
		segments = [][]string{all}
	}
	// A trailing form feed ends the last page; it does not open a new one.
	for len(segments) > 1 && len(segments[len(segments)-1]) == 0 {
		segments = segments[:len(segments)-1]
	}
	if len(segments) == 1 && len(segments[0]) == 0 {
		return nil // empty input produces no output
	}

	headerLabel := label
	if o.headerSet {
		headerLabel = o.header
	}
	physPerLine := 1
	if o.doubleSpace {
		physPerLine = 2
	}
	physBudget := o.bodyLines * physPerLine
	if o.columns > 1 {
		return printVertical(segments, w, headerLabel, stamp, o, physPerLine)
	}

	page := 1
	lineNo := o.firstLineNum
	for si, seg := range segments {
		for _, chunk := range chunkLines(seg, o.bodyLines) {
			emit := inPageRange(page, o)
			if emit && !o.omitHeader {
				if _, werr := fmt.Fprintf(w, "\n\n%s\n\n\n", headerLine(headerLabel, stamp, page, o)); werr != nil {
					return werr
				}
			}
			for _, line := range chunk {
				if emit {
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
			}
			if emit && !o.omitHeader {
				// Trailer: pad every page (including the last) to full
				// page length, or emit one form feed with -F.
				if o.formFeed {
					if _, werr := w.WriteString("\f"); werr != nil {
						return werr
					}
				} else {
					pad := physBudget - len(chunk)*physPerLine + linesPerTrailer
					if _, werr := w.WriteString(strings.Repeat("\n", pad)); werr != nil {
						return werr
					}
				}
			}
			page++
		}
		if o.omitHeader && o.ffBreaks && si < len(segments)-1 {
			if inPageRange(page-1, o) {
				if _, werr := w.WriteString("\f"); werr != nil {
					return werr
				}
			}
		}
	}
	return nil
}

func printVertical(segments [][]string, w *bufio.Writer, headerLabel string, stamp time.Time, o options, physPerLine int) error {
	page := 1
	lineNo := o.firstLineNum
	pageSize := o.bodyLines * o.columns
	columnWidth := o.width / o.columns
	if columnWidth < 1 {
		columnWidth = 1
	}
	for si, seg := range segments {
		for _, chunk := range chunkLines(seg, pageSize) {
			emit := inPageRange(page, o)
			if emit && !o.omitHeader {
				if _, err := fmt.Fprintf(w, "\n\n%s\n\n\n", headerLine(headerLabel, stamp, page, o)); err != nil {
					return err
				}
			}
			rows := (len(chunk) + o.columns - 1) / o.columns
			formatted := make([]string, len(chunk))
			cellOptions := o
			cellOptions.indent = 0
			for i, inputLine := range chunk {
				line := formatLine(inputLine, lineNo+i, cellOptions)
				line = strings.TrimSuffix(line, "\n")
				if len(line) > columnWidth {
					line = line[:columnWidth]
				}
				formatted[i] = line
			}
			lineNo += len(chunk)
			for row := 0; row < rows; row++ {
				if !emit {
					continue
				}
				if _, err := w.WriteString(strings.Repeat(" ", o.indent)); err != nil {
					return err
				}
				for col := 0; col < o.columns; col++ {
					index := row + col*rows
					if index >= len(formatted) {
						continue
					}
					line := formatted[index]
					if col > 0 {
						if _, err := w.WriteString(o.separator); err != nil {
							return err
						}
					}
					if o.separator == "" && col < o.columns-1 && index+rows < len(formatted) {
						line += strings.Repeat(" ", columnWidth-len(line))
					}
					if _, err := w.WriteString(line); err != nil {
						return err
					}
				}
				if _, err := w.WriteString("\n"); err != nil {
					return err
				}
				if o.doubleSpace {
					if _, err := w.WriteString("\n"); err != nil {
						return err
					}
				}
			}
			if emit && !o.omitHeader {
				if o.formFeed {
					if _, err := w.WriteString("\f"); err != nil {
						return err
					}
				} else if _, err := w.WriteString(strings.Repeat("\n", (o.bodyLines-rows)*physPerLine+linesPerTrailer)); err != nil {
					return err
				}
			}
			page++
		}
		if o.omitHeader && o.ffBreaks && si < len(segments)-1 && inPageRange(page-1, o) {
			if _, err := w.WriteString("\f"); err != nil {
				return err
			}
		}
	}
	return nil
}

// headerLine builds the GNU header text line: margin, date at the left,
// the file name (or -h string) centered, and "Page N" at the right,
// filling the page width.
func headerLine(label string, stamp time.Time, page int, o options) string {
	format := "2006-01-02 15:04"
	if o.dateFormat != "" {
		format = strftimeLayout(o.dateFormat)
	}
	date := stamp.Format(format)
	pageText := fmt.Sprintf("Page %d", page)
	avail := o.width - len(date) - len(label) - len(pageText)
	if avail < 0 {
		avail = 0
	}
	lhs := avail / 2
	rhs := avail - lhs
	if lhs < 1 {
		lhs = 1
	}
	if rhs < 1 {
		rhs = 1
	}
	return strings.Repeat(" ", o.indent) + date + strings.Repeat(" ", lhs) + label + strings.Repeat(" ", rhs) + pageText
}

func formatLine(line string, lineNo int, o options) string {
	hasNL := strings.HasSuffix(line, "\n")
	line = strings.TrimSuffix(line, "\n")
	if o.numberLines {
		line = fmt.Sprintf("%5d\t%s", lineNo, line)
	}
	if o.truncate && len(line) > o.width {
		line = line[:o.width]
	}
	if o.indent > 0 {
		line = strings.Repeat(" ", o.indent) + line
	}
	if hasNL {
		return line + "\n"
	}
	return line
}

// readSegments reads all input lines, splitting into segments at input
// form feeds: each '\f' ends the current segment (and its page); text
// after a mid-line form feed starts the next segment.
func readSegments(r io.Reader, o options) ([][]string, error) {
	br := bufio.NewReader(r)
	segments := [][]string{nil}
	for {
		line, err := br.ReadString('\n')
		if len(line) > 0 {
			if o.expandTabs {
				line = expandTabs(line, 8)
			}
			if strings.ContainsRune(line, '\f') {
				frags := strings.Split(line, "\f")
				for i, frag := range frags {
					if i < len(frags)-1 {
						if frag != "" {
							segments[len(segments)-1] = append(segments[len(segments)-1], frag+"\n")
						}
						segments = append(segments, nil)
					} else if frag != "" {
						segments[len(segments)-1] = append(segments[len(segments)-1], frag)
					}
				}
			} else {
				segments[len(segments)-1] = append(segments[len(segments)-1], line)
			}
		}
		if err == io.EOF {
			return segments, nil
		}
		if err != nil {
			return nil, err
		}
	}
}

// chunkLines splits a segment into page-sized chunks; an empty segment
// (from consecutive form feeds) is one empty page.
func chunkLines(lines []string, size int) [][]string {
	if len(lines) == 0 {
		return [][]string{nil}
	}
	var out [][]string
	for i := 0; i < len(lines); i += size {
		end := i + size
		if end > len(lines) {
			end = len(lines)
		}
		out = append(out, lines[i:end])
	}
	return out
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
