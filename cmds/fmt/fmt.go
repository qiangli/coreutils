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
	goalValue := fs.StringP("goal", "g", "", "goal line width")
	tabWidthValue := fs.StringP("tab-width", "T", "8", "tab width for line length calculation")
	splitOnly := fs.BoolP("split-only", "s", false, "split long lines, but do not refill")
	uniformSpacing := fs.BoolP("uniform-spacing", "u", false, "use one space between words and two after sentences")
	prefix := fs.StringP("prefix", "p", "", "reformat only lines beginning with PREFIX")
	crownMargin := fs.BoolP("crown-margin", "c", false, "preserve first and second line indentation")
	taggedParagraph := fs.BoolP("tagged-paragraph", "t", false, "format tagged paragraphs")
	preserveHeaders := fs.BoolP("preserve-headers", "m", false, "preserve mail headers")
	skipPrefix := fs.StringP("skip-prefix", "P", "", "do not reformat lines beginning with PSKIP")
	exactPrefix := fs.BoolP("exact-prefix", "x", false, "match PREFIX only at the beginning of a line")
	exactSkipPrefix := fs.BoolP("exact-skip-prefix", "X", false, "match PSKIP only at the beginning of a line")
	quick := fs.BoolP("quick", "q", false, "format more quickly")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	width, err := strconv.Atoi(*widthValue)
	if err != nil || width <= 0 {
		return tool.UsageError(rc, cmd, "invalid width: %q", *widthValue)
	}
	goal := 0
	if *goalValue != "" {
		goal, err = strconv.Atoi(*goalValue)
		if err != nil || goal <= 0 || goal > width {
			return tool.UsageError(rc, cmd, "invalid goal: %q", *goalValue)
		}
	}
	tabWidth, err := strconv.Atoi(*tabWidthValue)
	if err != nil || tabWidth <= 0 {
		return tool.UsageError(rc, cmd, "invalid tab width: %q", *tabWidthValue)
	}
	if len(operands) == 0 {
		operands = []string{"-"}
	}
	opts := fmtOptions{
		width:           width,
		goal:            goal,
		tabWidth:        tabWidth,
		splitOnly:       *splitOnly,
		uniformSpacing:  *uniformSpacing,
		prefix:          *prefix,
		crownMargin:     *crownMargin,
		tagged:          *taggedParagraph,
		preserveHeader:  *preserveHeaders,
		skipPrefix:      *skipPrefix,
		exactPrefix:     *exactPrefix,
		exactSkipPrefix: *exactSkipPrefix,
		quick:           *quick,
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
		if err := fmtStream(r, out, opts); err != nil {
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

type fmtOptions struct {
	width           int
	goal            int
	tabWidth        int
	splitOnly       bool
	uniformSpacing  bool
	prefix          string
	crownMargin     bool
	tagged          bool
	preserveHeader  bool
	skipPrefix      string
	exactPrefix     bool
	exactSkipPrefix bool
	quick           bool
}

func fmtStream(r io.Reader, w io.Writer, opts fmtOptions) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1024), 1024*1024)
	var para []string
	flush := func() error {
		if len(para) == 0 {
			return nil
		}
		out := formatParagraph(para, opts)
		_, err := io.WriteString(w, out)
		para = para[:0]
		return err
	}
	for sc.Scan() {
		line := sc.Text()
		if opts.preserveHeader && isMailHeader(line) {
			if err := flush(); err != nil {
				return err
			}
			if _, err := io.WriteString(w, line+"\n"); err != nil {
				return err
			}
			continue
		}
		if opts.skipPrefix != "" && matchesPrefix(line, opts.skipPrefix, opts.exactSkipPrefix) {
			if err := flush(); err != nil {
				return err
			}
			if _, err := io.WriteString(w, line+"\n"); err != nil {
				return err
			}
			continue
		}
		if opts.prefix != "" {
			body, ok := stripPrefix(line, opts.prefix, opts.exactPrefix)
			if !ok {
				if err := flush(); err != nil {
					return err
				}
				if _, err := io.WriteString(w, line+"\n"); err != nil {
					return err
				}
				continue
			}
			line = body
		}
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

func formatParagraph(lines []string, opts fmtOptions) string {
	firstPrefix, restPrefix, bodyLines := paragraphMargins(lines, opts)
	bodyOpts := opts
	bodyOpts.width = max(1, opts.width-displayLenRunes([]rune(restPrefix), opts.tabWidth))
	if opts.splitOnly {
		return addMargins(splitLines(bodyLines, bodyOpts), firstPrefix, restPrefix)
	}
	return wrapWordsWithMargins(collectWords(bodyLines, opts.uniformSpacing), opts, firstPrefix, restPrefix)
}

func paragraphMargins(lines []string, opts fmtOptions) (string, string, []string) {
	body := append([]string(nil), lines...)
	firstPrefix, restPrefix := "", ""
	if opts.prefix != "" {
		firstPrefix = opts.prefix
		restPrefix = opts.prefix
	}
	if opts.crownMargin && len(lines) > 0 {
		firstPrefix = leadingBlankPrefix(lines[0])
		restPrefix = firstPrefix
		if len(lines) > 1 {
			restPrefix = leadingBlankPrefix(lines[1])
		}
		body = trimLinePrefixes(body, []string{firstPrefix, restPrefix})
	}
	if opts.tagged && len(lines) > 0 {
		firstPrefix = leadingBlankPrefix(lines[0])
		restPrefix = firstPrefix
		if len(lines) > 1 {
			second := leadingBlankPrefix(lines[1])
			if second != firstPrefix {
				restPrefix = second
			}
		}
		body = trimLinePrefixes(body, []string{firstPrefix, restPrefix})
	}
	return firstPrefix, restPrefix, body
}

func trimLinePrefixes(lines []string, prefixes []string) []string {
	out := make([]string, len(lines))
	for i, line := range lines {
		out[i] = line
		for _, prefix := range prefixes {
			if prefix != "" && strings.HasPrefix(out[i], prefix) {
				out[i] = strings.TrimPrefix(out[i], prefix)
				break
			}
		}
		out[i] = strings.TrimLeftFunc(out[i], unicode.IsSpace)
	}
	return out
}

func leadingBlankPrefix(s string) string {
	for i, r := range s {
		if r != ' ' && r != '\t' {
			return s[:i]
		}
	}
	return s
}

func splitLines(lines []string, opts fmtOptions) string {
	var b strings.Builder
	for _, line := range lines {
		b.WriteString(wrapWords(collectWords([]string{line}, opts.uniformSpacing), opts))
	}
	return b.String()
}

func wrapWords(words []string, opts fmtOptions) string {
	return wrapWordsWithMargins(words, opts, "", "")
}

func wrapWordsWithMargins(words []string, opts fmtOptions, firstPrefix, restPrefix string) string {
	var b strings.Builder
	lineLen := 0
	nextSep := 1
	prefix := firstPrefix
	available := max(1, opts.width-displayLenRunes([]rune(prefix), opts.tabWidth))
	writePrefix := func() {
		if lineLen == 0 && prefix != "" {
			b.WriteString(prefix)
		}
	}
	newLine := func() {
		b.WriteByte('\n')
		lineLen = 0
		prefix = restPrefix
		available = max(1, opts.width-displayLenRunes([]rune(prefix), opts.tabWidth))
	}
	for _, word := range words {
		if word == "" {
			continue
		}
		if word == "\000\000" {
			nextSep = 2
			continue
		}
		rs := []rune(word)
		if lineLen == 0 {
			writePrefix()
			for displayLenRunes(rs, opts.tabWidth) > available {
				cut := fitRunes(rs, available, opts.tabWidth)
				b.WriteString(string(rs[:cut]))
				newLine()
				rs = rs[cut:]
				writePrefix()
			}
			b.WriteString(string(rs))
			lineLen = displayLenRunes(rs, opts.tabWidth)
			nextSep = 1
			continue
		}
		sep := nextSep
		nextSep = 1
		if lineLen+sep+displayLenRunes(rs, opts.tabWidth) <= available {
			if sep == 2 {
				b.WriteString("  ")
			} else {
				b.WriteByte(' ')
			}
			b.WriteString(word)
			lineLen += sep + displayLenRunes(rs, opts.tabWidth)
			continue
		}
		newLine()
		writePrefix()
		for displayLenRunes(rs, opts.tabWidth) > available {
			cut := fitRunes(rs, available, opts.tabWidth)
			b.WriteString(string(rs[:cut]))
			newLine()
			rs = rs[cut:]
			writePrefix()
		}
		b.WriteString(strings.TrimLeftFunc(string(rs), unicode.IsSpace))
		lineLen = displayLenRunes(rs, opts.tabWidth)
	}
	if lineLen > 0 {
		b.WriteByte('\n')
	}
	return b.String()
}

func collectWords(lines []string, uniform bool) []string {
	if !uniform {
		return strings.Fields(strings.Join(lines, " "))
	}
	var words []string
	for _, line := range lines {
		fields := strings.Fields(line)
		searchFrom := 0
		for i, field := range fields {
			if i > 0 && endsSentence(fields[i-1]) {
				start := strings.Index(line[searchFrom:], field)
				if start >= 0 {
					gap := line[searchFrom : searchFrom+start]
					if strings.Count(gap, " ")+strings.Count(gap, "\t") >= 2 {
						words = append(words, "\000\000")
					}
					searchFrom += start
				}
			}
			words = append(words, field)
			if idx := strings.Index(line[searchFrom:], field); idx >= 0 {
				searchFrom += idx + len(field)
			}
		}
		if len(fields) > 0 && endsSentence(fields[len(fields)-1]) {
			words = append(words, "\000\000")
		}
	}
	if len(words) > 0 && words[len(words)-1] == "\000\000" {
		words = words[:len(words)-1]
	}
	return words
}

func endsSentence(s string) bool {
	if s == "" {
		return false
	}
	switch s[len(s)-1] {
	case '.', '?', '!':
		return true
	default:
		return false
	}
}

func displayLenRunes(rs []rune, tabWidth int) int {
	col := 0
	for _, r := range rs {
		if r == '\t' {
			col += tabWidth - col%tabWidth
		} else {
			col++
		}
	}
	return col
}

func fitRunes(rs []rune, width, tabWidth int) int {
	col := 0
	for i, r := range rs {
		next := col + 1
		if r == '\t' {
			next = col + tabWidth - col%tabWidth
		}
		if next > width {
			if i == 0 {
				return 1
			}
			return i
		}
		col = next
	}
	return len(rs)
}

func stripPrefix(line, prefix string, exact bool) (string, bool) {
	if exact {
		if !strings.HasPrefix(line, prefix) {
			return "", false
		}
		return strings.TrimPrefix(line, prefix), true
	}
	trimmed := strings.TrimLeftFunc(line, unicode.IsSpace)
	if !strings.HasPrefix(trimmed, prefix) {
		return "", false
	}
	return strings.TrimPrefix(trimmed, prefix), true
}

func matchesPrefix(line, prefix string, exact bool) bool {
	if exact {
		return strings.HasPrefix(line, prefix)
	}
	return strings.HasPrefix(strings.TrimLeftFunc(line, unicode.IsSpace), prefix)
}

func isMailHeader(line string) bool {
	if line == "" || unicode.IsSpace([]rune(line)[0]) {
		return false
	}
	i := strings.IndexByte(line, ':')
	return i > 0 && !strings.ContainsAny(line[:i], " \t")
}

func addMargins(s, firstPrefix, restPrefix string) string {
	if s == "" {
		return s
	}
	lines := strings.SplitAfter(s, "\n")
	var b strings.Builder
	prefix := firstPrefix
	for _, line := range lines {
		if line == "" {
			continue
		}
		if line == "\n" {
			b.WriteByte('\n')
			continue
		}
		b.WriteString(prefix)
		b.WriteString(line)
		prefix = restPrefix
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
