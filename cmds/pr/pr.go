// Package prcmd implements a pr(1) subset: GNU page
// structure (66-line pages: 5-line header, body, 5-line trailer, with
// the last page padded to full length), form-feed page breaks, page
// ranges (--pages and the +FIRST[:LAST] operand), line numbering,
// margins, and -t/-T.
//
// Across-column output (-a) and parallel merging (-m) are supported.
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
	numberSep      string
	numberWidth    int
	indent         int
	noFileWarnings bool
	expandTabs     bool
	expandChar     rune
	expandWidth    int
	formFeed       bool
	pageStart      int
	pageEnd        int
	firstLineNum   int
	columns        int
	across         bool
	separator      string
	merge          bool
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
	numberLines := fs.StringP("number-lines", "n", "", "precede each line with its line number")
	// POSIX defines both operands as optional and attached.  A bare option
	// selects the historical TAB/5 defaults and must not consume a file name.
	fs.Lookup("number-lines").NoOptDefVal = "\t5"
	indent := fs.IntP("indent", "o", 0, "offset each line with MARGIN spaces")
	noFileWarnings := fs.BoolP("no-file-warnings", "r", false, "omit file open warnings")
	pages := fs.String("pages", "", "print only pages in FIRST[:LAST] range")
	expandTabs := fs.StringP("expand-tabs", "e", "", "expand input tabs to spaces")
	fs.Lookup("expand-tabs").NoOptDefVal = "\t8"
	across := fs.BoolP("across", "a", false, "fill columns across rather than down")
	columns := fs.Int("columns", 1, "produce COLUMN columns, filled down")
	fs.IntP("column", "", 1, "alias for --columns")
	separator := fs.StringP("separator", "s", "", "separate columns by CHAR")
	// GNU/POSIX pr accepts -s with an optional attached character.  A bare -s
	// selects TAB and must not consume the following file operand.
	fs.Lookup("separator").NoOptDefVal = "\t"
	sepString := fs.StringP("sep-string", "S", "", "separate columns by STRING")
	merge := fs.BoolP("merge", "m", false, "print files in parallel, one per column")
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
	if fs.Changed("column") {
		cv, _ := fs.GetInt("column")
		*columns = cv
	}
	if *merge && *across {
		fmt.Fprintln(rc.Err, "pr: cannot specify both printing across and printing in parallel")
		return 1
	}
	if *merge && (fs.Changed("columns") || fs.Changed("column")) {
		fmt.Fprintln(rc.Err, "pr: cannot specify number of columns when printing in parallel")
		return 1
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
	numberSep, numberWidth, err := parseOptionalCharNumber(*numberLines, '\t', 5)
	if err != nil {
		return tool.UsageError(rc, cmd, "invalid number-lines value: %q", *numberLines)
	}
	expandChar, expandWidth, err := parseOptionalCharNumber(*expandTabs, '\t', 8)
	if err != nil {
		return tool.UsageError(rc, cmd, "invalid expand-tabs value: %q", *expandTabs)
	}

	o := options{
		pageLength: *pageLength,
		width:      *width,
		header:     *headerText, headerSet: fs.Changed("header"),
		dateFormat: *dateFormat, doubleSpace: *doubleSpace,
		numberLines: fs.Changed("number-lines"), numberSep: string(numberSep), numberWidth: numberWidth,
		indent: *indent, noFileWarnings: *noFileWarnings,
		expandTabs: fs.Changed("expand-tabs"), expandChar: expandChar, expandWidth: expandWidth,
		formFeed:  *formFeed || *formFeedLower,
		ffBreaks:  !*omitPagination,
		pageStart: pageStart, pageEnd: pageEnd,
		firstLineNum: *firstLineNum,
		columns:      *columns, across: *across, separator: *separator, merge: *merge,
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
	if o.merge {
		return runMerge(rc, files, w, o)
	}
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

// runMerge opens every input before emitting rows, since -m consumes one
// corresponding line from each file at a time.
func runMerge(rc *tool.RunContext, files []string, w *bufio.Writer, o options) int {
	readers := make([]io.Reader, 0, len(files))
	closers := make([]io.Closer, 0, len(files))
	for _, name := range files {
		r, closer, _, _, err := open(rc, name)
		if err != nil {
			if !o.noFileWarnings {
				fmt.Fprintf(rc.Err, "pr: %s: %v\\n", name, tool.SysErr(err))
			}
			for _, c := range closers {
				c.Close()
			}
			return 1
		}
		readers = append(readers, r)
		if closer != nil {
			closers = append(closers, closer)
		}
	}
	defer func() {
		for _, c := range closers {
			c.Close()
		}
	}()
	if err := printMerge(readers, w, o); err != nil {
		fmt.Fprintf(rc.Err, "pr: merge: %v\\n", tool.SysErr(err))
		return 1
	}
	if err := w.Flush(); err != nil {
		fmt.Fprintf(rc.Err, "pr: write error: %v\\n", err)
		return 1
	}
	return 0
}

// scanColumnOption recognizes pr's standalone -N column shorthand. It is
// deliberately a small pre-scan: values belonging to options such as -o are
// left alone, so a negative numeric value is still reported by that option's
// normal validation.
func scanColumnOption(args []string) []string {
	const requiresValue = "-D -N -S -W -h -l -o -w --date-format --first-line-number --header --indent --length --page-width --sep-string --width --pages --columns --column"
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
		// pflag treats a string option with NoOptDefVal as a completed short
		// flag, so preserve pr's attached optional-argument spellings.
		if len(arg) > 2 && (strings.HasPrefix(arg, "-s") || strings.HasPrefix(arg, "-e") || strings.HasPrefix(arg, "-n")) {
			longName := map[byte]string{'s': "separator", 'e': "expand-tabs", 'n': "number-lines"}[arg[1]]
			out = append(out, "--"+longName+"="+arg[2:])
			continue
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
	// Single-column pr is a page stream: it must not wait for EOF before
	// producing the first complete page. Besides bounding memory, this is
	// required for pipes and FIFOs whose producer remains open while consuming
	// pr's output. Multi-column layouts need page-wide lookahead and retain the
	// buffered path below.
	if o.columns == 1 {
		return printSingleColumn(r, w, label, stamp, o)
	}
	// A multi-column layout needs one page of lookahead, not the entire input.
	// Stream complete pages so a FIFO producer can remain open while consuming
	// pr's paginated output.
	return printColumnsStream(r, w, label, stamp, o)
}

func printColumnsStream(r io.Reader, w *bufio.Writer, label string, stamp time.Time, o options) error {
	headerLabel := label
	if o.headerSet {
		headerLabel = o.header
	}
	physPerLine := 1
	if o.doubleSpace {
		physPerLine = 2
	}
	page, lineNo := 1, o.firstLineNum
	pageSize := o.bodyLines * o.columns
	lines := make([]string, 0, pageSize)
	segmentHadData := false
	var pendingBreaks []bool
	pendingEmptySegments := 0

	emit := func(chunk []string) error {
		if err := printColumnChunk(w, chunk, page, &lineNo, headerLabel, stamp, o, physPerLine); err != nil {
			return err
		}
		page++
		return w.Flush()
	}
	resolveBreaks := func() error {
		if len(pendingBreaks) == 0 {
			return nil
		}
		if o.omitHeader {
			for _, visible := range pendingBreaks {
				if visible {
					if _, err := w.WriteString("\f"); err != nil {
						return err
					}
				}
			}
		} else {
			for i := 0; i < pendingEmptySegments; i++ {
				if err := emit(nil); err != nil {
					return err
				}
			}
		}
		pendingBreaks = pendingBreaks[:0]
		pendingEmptySegments = 0
		return w.Flush()
	}
	addLine := func(line string) error {
		if err := resolveBreaks(); err != nil {
			return err
		}
		segmentHadData = true
		lines = append(lines, line)
		if len(lines) == pageSize {
			if err := emit(lines); err != nil {
				return err
			}
			lines = lines[:0]
		}
		return nil
	}
	endSegment := func() error {
		if len(lines) > 0 {
			if err := emit(lines); err != nil {
				return err
			}
			lines = lines[:0]
		} else if !segmentHadData {
			if o.omitHeader {
				page++ // an interior empty segment is one empty page
			} else {
				pendingEmptySegments++
			}
		}
		segmentHadData = false
		pendingBreaks = append(pendingBreaks, inPageRange(page-1, o))
		return nil
	}

	br := bufio.NewReader(r)
	for {
		line, err := br.ReadString('\n')
		if len(line) > 0 {
			if o.expandTabs {
				line = expandChars(line, o.expandChar, o.expandWidth)
			}
			frags := strings.Split(line, "\f")
			for i, frag := range frags {
				if i < len(frags)-1 {
					if frag != "" {
						if err := addLine(frag + "\n"); err != nil {
							return err
						}
					}
					if o.ffBreaks {
						if err := endSegment(); err != nil {
							return err
						}
					}
				} else if frag != "" {
					if err := addLine(frag); err != nil {
						return err
					}
				}
			}
		}
		if err == io.EOF {
			if len(lines) > 0 {
				return emit(lines)
			}
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func printSingleColumn(r io.Reader, w *bufio.Writer, label string, stamp time.Time, o options) error {
	headerLabel := label
	if o.headerSet {
		headerLabel = o.header
	}
	physPerLine := 1
	if o.doubleSpace {
		physPerLine = 2
	}
	physBudget := o.bodyLines * physPerLine
	page, lineNo := 1, o.firstLineNum
	lines := make([]string, 0, o.bodyLines)
	segmentHadData := false
	pendingBreaks, pendingEmptySegments := 0, 0

	emitPage := func(chunk []string) error {
		emit := inPageRange(page, o)
		if emit && !o.omitHeader {
			if _, err := fmt.Fprintf(w, "\n\n%s\n\n\n", headerLine(headerLabel, stamp, page, o)); err != nil {
				return err
			}
		}
		for _, line := range chunk {
			if emit {
				if _, err := w.WriteString(formatLine(line, lineNo, o)); err != nil {
					return err
				}
				if o.doubleSpace {
					if _, err := w.WriteString("\n"); err != nil {
						return err
					}
				}
			}
			lineNo++
		}
		if emit && !o.omitHeader {
			if o.formFeed {
				if _, err := w.WriteString("\f"); err != nil {
					return err
				}
			} else {
				pad := physBudget - len(chunk)*physPerLine + linesPerTrailer
				if _, err := w.WriteString(strings.Repeat("\n", pad)); err != nil {
					return err
				}
			}
		}
		page++
		// Expose each completed page to a downstream pipe immediately.
		return w.Flush()
	}
	resolveBreaks := func() error {
		if pendingBreaks == 0 {
			return nil
		}
		if o.omitHeader {
			if inPageRange(page-1+pendingEmptySegments, o) {
				if _, err := w.WriteString(strings.Repeat("\f", pendingBreaks)); err != nil {
					return err
				}
			}
		} else {
			for i := 0; i < pendingEmptySegments; i++ {
				if err := emitPage(nil); err != nil {
					return err
				}
			}
		}
		pendingBreaks, pendingEmptySegments = 0, 0
		return w.Flush()
	}
	addLine := func(line string) error {
		if err := resolveBreaks(); err != nil {
			return err
		}
		segmentHadData = true
		lines = append(lines, line)
		if len(lines) == o.bodyLines {
			if err := emitPage(lines); err != nil {
				return err
			}
			lines = lines[:0]
		}
		return nil
	}
	endSegment := func() error {
		if len(lines) > 0 {
			if err := emitPage(lines); err != nil {
				return err
			}
			lines = lines[:0]
		} else if !segmentHadData {
			pendingEmptySegments++
		}
		segmentHadData = false
		pendingBreaks++
		return nil
	}

	br := bufio.NewReader(r)
	for {
		line, err := br.ReadString('\n')
		if len(line) > 0 {
			if o.expandTabs {
				line = expandChars(line, o.expandChar, o.expandWidth)
			}
			frags := strings.Split(line, "\f")
			for i, frag := range frags {
				if i < len(frags)-1 {
					if frag != "" {
						if err := addLine(frag + "\n"); err != nil {
							return err
						}
					}
					if o.ffBreaks {
						if err := endSegment(); err != nil {
							return err
						}
					}
				} else if frag != "" {
					if err := addLine(frag); err != nil {
						return err
					}
				}
			}
		}
		if err == io.EOF {
			if len(lines) > 0 {
				return emitPage(lines)
			}
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func printMerge(readers []io.Reader, w *bufio.Writer, o options) error {
	pages := make([][][]string, len(readers))
	pageCount := 0
	for i, r := range readers {
		segments, err := readSegments(r, o)
		if err != nil {
			return err
		}
		pages[i] = mergePages(segments, o)
		if len(pages[i]) > pageCount {
			pageCount = len(pages[i])
		}
	}
	if pageCount == 0 {
		return nil
	}
	stamp, label := time.Now(), ""
	if o.headerSet {
		label = o.header
	}
	physPerLine := 1
	if o.doubleSpace {
		physPerLine = 2
	}
	physBudget := o.bodyLines * physPerLine
	for page := 1; page <= pageCount; page++ {
		emit := inPageRange(page, o)
		if emit && !o.omitHeader {
			if _, err := fmt.Fprintf(w, "\n\n%s\n\n\n", headerLine(label, stamp, page, o)); err != nil {
				return err
			}
		}
		rows := 0
		for _, inputPages := range pages {
			if page <= len(inputPages) && len(inputPages[page-1]) > rows {
				rows = len(inputPages[page-1])
			}
		}
		for row := 0; row < rows; row++ {
			if emit {
				if _, err := w.WriteString(mergeLine(pages, page-1, row, o)); err != nil {
					return err
				}
				if o.doubleSpace {
					if _, err := w.WriteString("\n"); err != nil {
						return err
					}
				}
			}
		}
		if emit && !o.omitHeader {
			if o.formFeed {
				if _, err := w.WriteString("\f"); err != nil {
					return err
				}
			} else if _, err := w.WriteString(strings.Repeat("\n", physBudget-rows*physPerLine+linesPerTrailer)); err != nil {
				return err
			}
		}
	}
	return nil
}

func mergePages(segments [][]string, o options) [][]string {
	if !o.ffBreaks {
		var all []string
		for _, segment := range segments {
			all = append(all, segment...)
		}
		segments = [][]string{all}
	}
	for len(segments) > 1 && len(segments[len(segments)-1]) == 0 {
		segments = segments[:len(segments)-1]
	}
	if len(segments) == 1 && len(segments[0]) == 0 {
		return nil
	}
	var pages [][]string
	for _, segment := range segments {
		pages = append(pages, chunkLines(segment, o.bodyLines)...)
	}
	return pages
}

func mergeLine(pages [][][]string, page, row int, o options) string {
	columns, columnWidth := len(pages), o.width/len(pages)
	if columnWidth < 1 {
		columnWidth = 1
	}
	var b strings.Builder
	for col, inputPages := range pages {
		line := ""
		if page < len(inputPages) && row < len(inputPages[page]) {
			line = strings.TrimSuffix(inputPages[page][row], "\n")
		}
		limit := columnWidth
		if col < columns-1 {
			limit--
		}
		if limit < 0 {
			limit = 0
		}
		if len(line) > limit {
			line = line[:limit]
		}
		b.WriteString(line)
		if col < columns-1 {
			// GNU emits tabs for padding where a tab stop fits before the next
			// column boundary; retaining that detail matters for byte-for-byte
			// merge output, including when -s is present.
			padTo := (col + 1) * columnWidth
			if o.separator != "" {
				padTo--
			}
			writeMergePadding(&b, col*columnWidth+len(line), padTo)
			if o.separator != "" {
				b.WriteString(o.separator)
			}
		}
	}
	return b.String() + "\n"
}

func writeMergePadding(b *strings.Builder, pos, target int) {
	for pos < target {
		nextTab := pos + 8 - pos%8
		if nextTab <= target {
			b.WriteByte('\t')
			pos = nextTab
		} else {
			b.WriteByte(' ')
			pos++
		}
	}
}

func printColumns(segments [][]string, w *bufio.Writer, headerLabel string, stamp time.Time, o options, physPerLine int) error {
	page := 1
	lineNo := o.firstLineNum
	pageSize := o.bodyLines * o.columns
	for si, seg := range segments {
		for _, chunk := range chunkLines(seg, pageSize) {
			if err := printColumnChunk(w, chunk, page, &lineNo, headerLabel, stamp, o, physPerLine); err != nil {
				return err
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

func printColumnChunk(w *bufio.Writer, chunk []string, page int, lineNo *int, headerLabel string, stamp time.Time, o options, physPerLine int) error {
	emit := inPageRange(page, o)
	if emit && !o.omitHeader {
		if _, err := fmt.Fprintf(w, "\n\n%s\n\n\n", headerLine(headerLabel, stamp, page, o)); err != nil {
			return err
		}
	}
	rows := (len(chunk) + o.columns - 1) / o.columns
	columnWidth := o.width / o.columns
	if columnWidth < 1 {
		columnWidth = 1
	}
	formatted := make([]string, len(chunk))
	cellOptions := o
	cellOptions.indent = 0
	for i, inputLine := range chunk {
		line := formatLine(inputLine, *lineNo+i, cellOptions)
		line = strings.TrimSuffix(line, "\n")
		if len(line) > columnWidth {
			line = line[:columnWidth]
		}
		formatted[i] = line
	}
	*lineNo += len(chunk)
	for row := 0; row < rows; row++ {
		if !emit {
			continue
		}
		if _, err := w.WriteString(strings.Repeat(" ", o.indent)); err != nil {
			return err
		}
		for col := 0; col < o.columns; col++ {
			index := row + col*rows
			if o.across {
				index = row*o.columns + col
			}
			if index >= len(formatted) {
				continue
			}
			line := formatted[index]
			nextIndex := index + rows
			if o.across {
				nextIndex = index + 1
			}
			if col < o.columns-1 && nextIndex < len(formatted) && len(line) >= columnWidth {
				line = line[:columnWidth-1]
			}
			if col > 0 {
				if _, err := w.WriteString(o.separator); err != nil {
					return err
				}
			}
			if o.separator == "" && col < o.columns-1 && nextIndex < len(formatted) {
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
			_, err := w.WriteString("\f")
			return err
		}
		_, err := w.WriteString(strings.Repeat("\n", (o.bodyLines-rows)*physPerLine+linesPerTrailer))
		return err
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
		line = fmt.Sprintf("%*d%s%s", o.numberWidth, lineNo, o.numberSep, line)
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
				line = expandChars(line, o.expandChar, o.expandWidth)
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

func parseOptionalCharNumber(spec string, defaultChar rune, defaultNumber int) (rune, int, error) {
	if spec == "" {
		return defaultChar, defaultNumber, nil
	}
	runes := []rune(spec)
	char := defaultChar
	digits := spec
	if runes[0] < '0' || runes[0] > '9' {
		char = runes[0]
		digits = string(runes[1:])
	}
	if digits == "" {
		return char, defaultNumber, nil
	}
	for _, r := range digits {
		if r < '0' || r > '9' {
			return 0, 0, fmt.Errorf("not decimal")
		}
	}
	n, err := strconv.Atoi(digits)
	if err != nil || n <= 0 {
		return 0, 0, fmt.Errorf("not positive")
	}
	return char, n, nil
}

func expandChars(s string, expandChar rune, width int) string {
	var b strings.Builder
	col := 0
	for _, r := range s {
		if r == expandChar {
			spaces := width - col%width
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
