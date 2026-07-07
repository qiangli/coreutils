// Package ptxcmd implements ptx(1): produce a permuted index of file
// contents in GNU's dumb-terminal output format — each keyword aligned
// at the center of a line of --width columns (default 72), fields
// separated by --gap-size spaces (default 3), with '/' marking truncated
// context — or roff .xx lines via -O/--format=roff.
//
// Documented deviations from GNU ptx: a context never spans input lines
// (the default context is one input line rather than GNU's end-of-sentence
// detection), and -S/-W take Go (RE2) regular expression syntax instead
// of emacs-style regexps. Field layout, widths, truncation marks, word
// selection (letter runs by default), case-sensitive word lists (folded
// to upper case for -f), and references otherwise follow GNU ptx.
package ptxcmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "ptx",
	Synopsis: "Produce a permuted index of file contents.",
	Usage:    "ptx [OPTION]... [FILE]...",
}

func init() { cmd.Run = run; tool.Register(cmd) }

const truncationMark = "/"

type ptxOptions struct {
	ignoreCase  bool
	autoRef     bool
	inputRef    bool
	traditional bool
	rightRef    bool
	roff        bool
	gap         int
	wordRE      *regexp.Regexp
	sentenceRE  *regexp.Regexp
	breakChars  string
	breakSet    bool

	// computed formatting parameters (fix_output_parameters)
	width     int
	half      int
	beforeMax int
	kaMax     int
	refMax    int
}

// lineContext is one input line (or sentence segment) holding the raw
// text and the byte spans of the words found in it.
type lineContext struct {
	text   string
	starts map[int]int // word start offset -> word end offset
}

type occurrence struct {
	ref     string
	ctx     *lineContext
	kstart  int
	kend    int
	sortKey string
}

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	autoReference := fs.BoolP("auto-reference", "A", false, "output file:line references (file part empty for stdin)")
	traditional := fs.BoolP("traditional", "G", false, "behave more like System V ptx")
	typeset := fs.BoolP("typeset-mode", "t", false, "use an output width of 100 unless --width is given")
	rightRefs := fs.BoolP("right-side-refs", "R", false, "put references at the right side")
	breakFile := fs.StringP("break-file", "b", "", "use characters in FILE as word break characters")
	sentenceRegexp := fs.StringP("sentence-regexp", "S", "", "use REGEXP to recognize context boundaries within a line")
	ignoreCase := fs.BoolP("ignore-case", "f", false, "fold lower case to upper case for sorting")
	gapSize := fs.IntP("gap-size", "g", 3, "gap size in columns between output fields")
	onlyFile := fs.StringP("only-file", "o", "", "read only word list from FILE (one word per line)")
	ignoreFile := fs.StringP("ignore-file", "i", "", "read ignore word list from FILE (one word per line)")
	references := fs.BoolP("references", "r", false, "first field of each line is a reference")
	width := fs.IntP("width", "w", 72, "output width in columns, references excluded")
	wordRegexp := fs.StringP("word-regexp", "W", "", "use REGEXP to match each keyword")
	format := fs.String("format", "", "generate output as roff directives (-O is --format=roff)")
	// GNU's -O (roff) and -T (TeX) short flags have no independent long
	// spelling; map them onto --format before parsing.
	for i, arg := range args {
		if arg == "--" {
			break
		}
		switch arg {
		case "-O":
			args[i] = "--format=roff"
		case "-T":
			args[i] = "--format=tex"
		}
	}
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	opts := ptxOptions{
		ignoreCase:  *ignoreCase,
		autoRef:     *autoReference,
		inputRef:    *references,
		traditional: *traditional,
		rightRef:    *rightRefs,
		gap:         *gapSize,
	}
	switch *format {
	case "":
	case "roff":
		opts.roff = true
	case "tex":
		return tool.NotSupported(rc, cmd, "-T/--format=tex (TeX output)")
	default:
		return tool.UsageError(rc, cmd, "invalid output format: %q", *format)
	}
	if opts.gap <= 0 {
		return tool.UsageError(rc, cmd, "invalid gap width: %d", opts.gap)
	}
	if *width <= 0 {
		return tool.UsageError(rc, cmd, "invalid line width: %d", *width)
	}
	opts.width = *width
	if *typeset && !fs.Changed("width") {
		opts.width = 100 // GNU -t: typesetter driven, wider lines
	}
	if *wordRegexp != "" {
		re, err := regexp.Compile(*wordRegexp)
		if err != nil {
			return tool.UsageError(rc, cmd, "invalid word regexp '%s'", *wordRegexp)
		}
		opts.wordRE = re
	}
	if *sentenceRegexp != "" {
		re, err := regexp.Compile(*sentenceRegexp)
		if err != nil {
			return tool.UsageError(rc, cmd, "invalid sentence regexp '%s'", *sentenceRegexp)
		}
		opts.sentenceRE = re
	}
	if *breakFile != "" {
		chars, err := readBreakChars(rc, *breakFile)
		if err != nil {
			fmt.Fprintf(rc.Err, "ptx: cannot read '%s': %v\n", *breakFile, tool.SysErr(err))
			return 1
		}
		opts.breakChars = chars
		opts.breakSet = true
	}
	only, err := readWordSet(rc, *onlyFile, opts.ignoreCase)
	if err != nil {
		fmt.Fprintf(rc.Err, "ptx: cannot read '%s': %v\n", *onlyFile, tool.SysErr(err))
		return 1
	}
	ignore, err := readWordSet(rc, *ignoreFile, opts.ignoreCase)
	if err != nil {
		fmt.Fprintf(rc.Err, "ptx: cannot read '%s': %v\n", *ignoreFile, tool.SysErr(err))
		return 1
	}
	if len(operands) == 0 {
		operands = []string{"-"}
	}

	status := 0
	var entries []occurrence
	for _, name := range operands {
		fileEntries, code := readEntries(rc, name, only, ignore, opts)
		if code != 0 {
			status = 1
		}
		entries = append(entries, fileEntries...)
	}

	// Sort by keyword bytes (folded lower->UPPER with -f); ties keep
	// input order.
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].sortKey < entries[j].sortKey
	})

	fixOutputParameters(&opts, entries)
	w := bufio.NewWriter(rc.Out)
	for _, e := range entries {
		if _, werr := w.WriteString(formatEntry(e, opts) + "\n"); werr != nil {
			fmt.Fprintf(rc.Err, "ptx: write error: %v\n", werr)
			return 1
		}
	}
	if err := w.Flush(); err != nil {
		fmt.Fprintf(rc.Err, "ptx: write error: %v\n", err)
		return 1
	}
	return status
}

// foldUpper folds ASCII lower case to upper case, the direction GNU ptx
// uses for -f (observable for bytes between 'Z' and 'a' such as '_').
func foldUpper(s string) string {
	b := []byte(s)
	changed := false
	for i, c := range b {
		if 'a' <= c && c <= 'z' {
			b[i] = c - 'a' + 'A'
			changed = true
		}
	}
	if !changed {
		return s
	}
	return string(b)
}

// readWordSet reads one word per line (GNU word-list format). Words
// compare case-sensitively unless -f is given.
func readWordSet(rc *tool.RunContext, name string, fold bool) (map[string]bool, error) {
	if name == "" {
		return nil, nil
	}
	f, err := os.Open(rc.Path(name))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	set := make(map[string]bool)
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		word := sc.Text()
		if word == "" {
			continue
		}
		if fold {
			word = foldUpper(word)
		}
		set[word] = true
	}
	return set, sc.Err()
}

func readBreakChars(rc *tool.RunContext, name string) (string, error) {
	data, err := os.ReadFile(rc.Path(name))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func readEntries(rc *tool.RunContext, name string, only, ignore map[string]bool, opts ptxOptions) ([]occurrence, int) {
	var reader io.Reader
	if name == "-" {
		if rc.In == nil {
			reader = strings.NewReader("")
		} else {
			reader = rc.In
		}
	} else {
		f, err := os.Open(rc.Path(name))
		if err != nil {
			fmt.Fprintf(rc.Err, "ptx: cannot open '%s' for reading: %v\n", name, tool.SysErr(err))
			return nil, 1
		}
		defer f.Close()
		reader = f
	}
	fileLabel := ""
	if name != "-" {
		fileLabel = name
	}
	sc := bufio.NewScanner(reader)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	var entries []occurrence
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := sc.Text()
		ref := ""
		if opts.inputRef {
			// The first whitespace-delimited field is the reference and
			// is excluded from the context.
			refEnd := 0
			for refEnd < len(line) && !isWhite(line[refEnd]) {
				refEnd++
			}
			ref = line[:refEnd]
			for refEnd < len(line) && isWhite(line[refEnd]) {
				refEnd++
			}
			line = line[refEnd:]
		}
		if opts.autoRef {
			ref = fileLabel + ":" + strconv.Itoa(lineNo)
		}
		segments := []string{line}
		if opts.sentenceRE != nil {
			segments = splitSentences(line, opts.sentenceRE)
		}
		for _, segment := range segments {
			segment = trimTrailingWhite(segment)
			spans := wordSpans(segment, opts)
			ctx := &lineContext{text: segment, starts: make(map[int]int, len(spans))}
			for _, sp := range spans {
				ctx.starts[sp[0]] = sp[1]
			}
			for _, sp := range spans {
				word := segment[sp[0]:sp[1]]
				key := word
				if opts.ignoreCase {
					key = foldUpper(word)
				}
				if ignore != nil && ignore[key] {
					continue
				}
				if only != nil && !only[key] {
					continue
				}
				entries = append(entries, occurrence{
					ref: ref, ctx: ctx, kstart: sp[0], kend: sp[1], sortKey: key,
				})
			}
		}
	}
	if err := sc.Err(); err != nil {
		fmt.Fprintf(rc.Err, "ptx: read error: %v\n", tool.SysErr(err))
		return entries, 1
	}
	return entries, 0
}

// wordSpans locates keywords: -W regexp matches; with a break file, runs
// of characters not in it (GNU extension: space is a word constituent
// unless listed); by default runs of letters ([A-Za-z], GNU's \w+); in
// traditional mode runs of non-whitespace.
func wordSpans(text string, opts ptxOptions) [][2]int {
	if opts.wordRE != nil {
		var out [][2]int
		for _, m := range opts.wordRE.FindAllStringIndex(text, -1) {
			if m[1] > m[0] {
				out = append(out, [2]int{m[0], m[1]})
			}
		}
		return out
	}
	isWordByte := func(c byte) bool {
		if opts.breakSet {
			if strings.IndexByte(opts.breakChars, c) >= 0 {
				return false
			}
			if opts.traditional && (c == ' ' || c == '\t' || c == '\n') {
				return false
			}
			return true
		}
		if opts.traditional {
			return c != ' ' && c != '\t' && c != '\n'
		}
		return ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z')
	}
	var out [][2]int
	i := 0
	for i < len(text) {
		if !isWordByte(text[i]) {
			i++
			continue
		}
		start := i
		for i < len(text) && isWordByte(text[i]) {
			i++
		}
		out = append(out, [2]int{start, i})
	}
	return out
}

func splitSentences(line string, re *regexp.Regexp) []string {
	matches := re.FindAllStringIndex(line, -1)
	if len(matches) == 0 {
		return []string{line}
	}
	var out []string
	start := 0
	for _, m := range matches {
		end := m[1]
		if end > start {
			out = append(out, strings.TrimSpace(line[start:end]))
		}
		start = end
	}
	if start < len(line) {
		out = append(out, strings.TrimSpace(line[start:]))
	}
	return out
}

// fixOutputParameters mirrors GNU ptx's fix_output_parameters: reserve
// room for left-side references, then split the line into the before
// half (left context + gap) and the keyafter half, leaving room for
// truncation marks.
func fixOutputParameters(opts *ptxOptions, entries []occurrence) {
	refMax := 0
	if opts.autoRef || opts.inputRef {
		for _, e := range entries {
			if len(e.ref) > refMax {
				refMax = len(e.ref)
			}
		}
	}
	opts.refMax = refMax
	lineWidth := opts.width
	if (opts.autoRef || opts.inputRef) && !opts.rightRef {
		lineWidth -= opts.refMax + opts.gap
		if lineWidth < 0 {
			lineWidth = 0
		}
	}
	opts.half = lineWidth / 2
	tl := len(truncationMark)
	if opts.traditional {
		opts.beforeMax = opts.half - opts.gap
		opts.kaMax = opts.half - (2*tl + 1)
	} else {
		opts.beforeMax = opts.half - opts.gap - 2*tl
		if opts.beforeMax < 0 {
			opts.beforeMax = 0
		}
		opts.kaMax = opts.half - 2*tl
	}
}

func isWhite(c byte) bool {
	switch c {
	case ' ', '\t', '\v', '\f', '\r', '\n':
		return true
	}
	return false
}

func trimTrailingWhite(s string) string {
	end := len(s)
	for end > 0 && isWhite(s[end-1]) {
		end--
	}
	return s[:end]
}

func skipWhiteBack(s string, pos, min int) int {
	for pos > min && isWhite(s[pos-1]) {
		pos--
	}
	return pos
}

func skipWhiteFwd(s string, pos, max int) int {
	for pos < max && isWhite(s[pos]) {
		pos++
	}
	return pos
}

// skipSomething advances over one whole word or a single separator
// character (GNU's SKIP_SOMETHING).
func (c *lineContext) skipSomething(pos int) int {
	if end, ok := c.starts[pos]; ok && end > pos {
		return end
	}
	return pos + 1
}

// fields holds the computed output fields of one index line, as offsets
// into the context text (GNU's define_all_fields). An end may be less
// than its start (degenerate empty field); widths use end-start as-is,
// matching GNU's arithmetic.
type fields struct {
	tailS, tailE     int
	beforeS, beforeE int
	kaS, kaE         int
	headS, headE     int
	tailTr           bool
	beforeTr         bool
	kaTr             bool
	headTr           bool
}

func computeFields(e occurrence, opts ptxOptions) fields {
	ctx := e.ctx.text
	L := len(ctx)
	var f fields
	f.kaS = e.kstart

	// keyafter: the keyword plus following whole words while the field
	// stays within kaMax; the keyword itself is never truncated.
	kaEnd := e.kend
	cursor := e.kend
	for cursor < L && cursor <= e.kstart+opts.kaMax {
		kaEnd = cursor
		cursor = e.ctx.skipSomething(cursor)
	}
	if cursor <= e.kstart+opts.kaMax {
		kaEnd = cursor
	}
	f.kaTr = kaEnd < L
	kaEnd = skipWhiteBack(ctx, kaEnd, e.kstart)
	f.kaE = kaEnd

	// before: left context ending at the keyword, shrunk from the left
	// by whole words until it fits beforeMax.
	bStart := 0
	bEnd := skipWhiteBack(ctx, e.kstart, bStart)
	for bStart+opts.beforeMax < bEnd {
		bStart = e.ctx.skipSomething(bStart)
	}
	f.beforeTr = skipWhiteBack(ctx, bStart, 0) > 0
	bStart = skipWhiteFwd(ctx, bStart, L)
	f.beforeS, f.beforeE = bStart, bEnd

	// tail: wraps right context that keyafter could not take into the
	// unused part of the before half.
	tailMax := opts.beforeMax - (bEnd - bStart) - opts.gap
	if tailMax > 0 {
		tStart := skipWhiteFwd(ctx, f.kaE, L)
		tEnd := tStart
		cursor = tEnd
		for cursor < L && cursor < tStart+tailMax {
			tEnd = cursor
			cursor = e.ctx.skipSomething(cursor)
		}
		if cursor < tStart+tailMax {
			tEnd = cursor
		}
		if tEnd > tStart {
			f.kaTr = false
			f.tailTr = tEnd < L
		}
		tEnd = skipWhiteBack(ctx, tEnd, tStart)
		f.tailS, f.tailE = tStart, tEnd
	}

	// head: wraps left context that before could not take into the
	// unused part of the keyafter half.
	headMax := opts.kaMax - (f.kaE - f.kaS) - opts.gap
	if headMax > 0 {
		hEnd := skipWhiteBack(ctx, f.beforeS, 0)
		hStart := 0
		for hStart+headMax < hEnd {
			hStart = e.ctx.skipSomething(hStart)
		}
		if hEnd > hStart {
			f.beforeTr = false
			f.headTr = hStart > 0
		}
		hStart = skipWhiteFwd(ctx, hStart, hEnd)
		f.headS, f.headE = hStart, hEnd
	}
	return f
}

// printField renders a field: every whitespace byte prints as one space;
// in roff mode double quotes are doubled.
func printField(b *strings.Builder, text string, roff bool) {
	for i := 0; i < len(text); i++ {
		c := text[i]
		switch {
		case isWhite(c):
			b.WriteByte(' ')
		case roff && c == '"':
			b.WriteString(`""`)
		default:
			b.WriteByte(c)
		}
	}
}

func spaces(b *strings.Builder, n int) {
	for ; n > 0; n-- {
		b.WriteByte(' ')
	}
}

func formatEntry(e occurrence, opts ptxOptions) string {
	f := computeFields(e, opts)
	if opts.roff {
		return formatRoff(e, f, opts)
	}
	return formatDumb(e, f, opts)
}

func field(ctx string, start, end int) string {
	if end > start {
		return ctx[start:end]
	}
	return ""
}

func truncLen(flag bool) int {
	if flag {
		return len(truncationMark)
	}
	return 0
}

// formatDumb mirrors GNU's output_one_dumb_line.
func formatDumb(e occurrence, f fields, opts ptxOptions) string {
	ctx := e.ctx.text
	hasRefs := opts.autoRef || opts.inputRef
	var b strings.Builder
	if hasRefs && !opts.rightRef {
		printField(&b, e.ref, false)
		if opts.autoRef {
			// Emacs next-error style: the trailing colon is taken from
			// the following gap.
			b.WriteByte(':')
			spaces(&b, opts.refMax+opts.gap-len(e.ref)-1)
		} else {
			spaces(&b, opts.refMax+opts.gap-len(e.ref))
		}
	}
	beforeLen := f.beforeE - f.beforeS
	if f.tailS < f.tailE {
		printField(&b, field(ctx, f.tailS, f.tailE), false)
		if f.tailTr {
			b.WriteString(truncationMark)
		}
		spaces(&b, opts.half-opts.gap-beforeLen-truncLen(f.beforeTr)-(f.tailE-f.tailS)-truncLen(f.tailTr))
	} else {
		spaces(&b, opts.half-opts.gap-beforeLen-truncLen(f.beforeTr))
	}
	if f.beforeTr {
		b.WriteString(truncationMark)
	}
	printField(&b, field(ctx, f.beforeS, f.beforeE), false)
	spaces(&b, opts.gap)
	printField(&b, field(ctx, f.kaS, f.kaE), false)
	if f.kaTr {
		b.WriteString(truncationMark)
	}
	if f.headS < f.headE {
		spaces(&b, opts.half-(f.kaE-f.kaS)-truncLen(f.kaTr)-(f.headE-f.headS)-truncLen(f.headTr))
		if f.headTr {
			b.WriteString(truncationMark)
		}
		printField(&b, field(ctx, f.headS, f.headE), false)
	} else if hasRefs && opts.rightRef {
		spaces(&b, opts.half-(f.kaE-f.kaS)-truncLen(f.kaTr))
	}
	if hasRefs && opts.rightRef {
		spaces(&b, opts.gap)
		printField(&b, e.ref, false)
	}
	return b.String()
}

// formatRoff mirrors GNU's output_one_roff_line: fields in the order
// "tail" "before" "keyword_and_after" "head" [ "reference" ].
func formatRoff(e occurrence, f fields, opts ptxOptions) string {
	ctx := e.ctx.text
	var b strings.Builder
	b.WriteString(".xx \"")
	printField(&b, field(ctx, f.tailS, f.tailE), true)
	if f.tailTr {
		b.WriteString(truncationMark)
	}
	b.WriteString(`" "`)
	if f.beforeTr {
		b.WriteString(truncationMark)
	}
	printField(&b, field(ctx, f.beforeS, f.beforeE), true)
	b.WriteString(`" "`)
	printField(&b, field(ctx, f.kaS, f.kaE), true)
	if f.kaTr {
		b.WriteString(truncationMark)
	}
	b.WriteString(`" "`)
	if f.headTr {
		b.WriteString(truncationMark)
	}
	printField(&b, field(ctx, f.headS, f.headE), true)
	b.WriteString(`"`)
	if opts.autoRef || opts.inputRef {
		b.WriteString(` "`)
		printField(&b, e.ref, true)
		b.WriteString(`"`)
	}
	return b.String()
}
