// Package fmtcmd implements fmt(1): simple paragraph formatting.
//
// Paragraphs follow the GNU model: blank lines are preserved,
// indentation is preserved on output, and successive input lines with
// different indentation are not joined (in crown-margin mode the
// second line's indent governs continuation lines; in
// tagged-paragraph mode the first line additionally forms its own
// paragraph when its indent equals the second line's).
//
// Documented deviations from GNU fmt: line filling is greedy toward
// the goal width (GNU uses a Knuth-Plass-style optimal algorithm, so
// break positions can differ); inter-word spacing is normalized to
// single spaces (two after sentence ends with -u); tabs are expanded
// on input but output indentation always uses spaces.
package fmtcmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "fmt",
	Synopsis: "Reformat each paragraph in the FILE(s), writing to standard output.\nThe option -WIDTH is an abbreviated form of --width=DIGITS.\nWith no FILE, or when FILE is -, read standard input.",
	Usage:    "fmt [-WIDTH] [OPTION]... [FILE]...",
}

func init() { cmd.Run = run; tool.Register(cmd) }

// defTaggedIndent is the secondary indent used for unindented
// one-line paragraphs in tagged-paragraph mode (GNU's DEF_INDENT).
const defTaggedIndent = 3

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	crownMargin := fs.BoolP("crown-margin", "c", false, "preserve the indentation of the first two lines")
	taggedParagraph := fs.BoolP("tagged-paragraph", "t", false, "indentation of first line different from second")
	splitOnly := fs.BoolP("split-only", "s", false, "split long lines, but do not refill")
	uniformSpacing := fs.BoolP("uniform-spacing", "u", false, "one space between words, two after sentences")
	widthValue := fs.StringP("width", "w", "", "maximum line width (default of 75 columns)")
	goalValue := fs.StringP("goal", "g", "", "goal width (default of 93% of width)")
	prefix := fs.StringP("prefix", "p", "", "reformat only lines beginning with STRING, reattaching the prefix to reformatted lines")
	tabWidth := fs.IntP("tab-width", "T", 8, "set tab stops every WIDTH columns")
	quick := fs.BoolP("quick", "q", false, "use a fast line breaking mode")
	exact := fs.BoolP("exact", "x", false, "try harder to preserve optimal line lengths")
	exactPrefix := fs.String("exact-prefix", "", "reformat only lines beginning with STRING, do not reattach the prefix")
	exactSkipPrefix := fs.String("exact-skip-prefix", "", "skip lines beginning with STRING, do not reattach (GNU compat, no-op in this subset)")
	preserveHeaders := fs.BoolP("preserve-headers", "m", false, "preserve email/news headers (GNU compat, no-op in this subset)")
	skipPrefix := fs.StringP("skip-prefix", "P", "", "skip reformatting lines beginning with STRING (GNU compat, no-op in this subset)")
	cStyle := fs.BoolP("", "X", false, "toggle width style (GNU compat, no-op in this subset)")
	operands, code := tool.Parse(rc, cmd, fs, rewriteObsoleteWidth(args))
	if code >= 0 {
		return code
	}
	width := 75
	if *widthValue != "" {
		n, err := strconv.Atoi(*widthValue)
		if err != nil || n <= 0 {
			return tool.UsageError(rc, cmd, "invalid width: %q", *widthValue)
		}
		width = n
	}
	goal := 0
	if *goalValue != "" {
		// GNU caps the goal at the maximum width (75 when -w is not
		// given), and derives the width as goal+10 when only the goal
		// is specified.
		n, err := strconv.Atoi(*goalValue)
		if err != nil || n < 0 || n > width {
			return tool.UsageError(rc, cmd, "invalid goal: %q", *goalValue)
		}
		goal = n
		if *widthValue == "" {
			width = goal + 10
		}
	} else {
		// 93% of width, computed the way GNU does (LEEWAY = 7).
		goal = width * (2*(100-7) + 1) / 200
	}
	if *tabWidth <= 0 {
		return tool.UsageError(rc, cmd, "invalid tab width: %d", *tabWidth)
	}
	_ = *quick
	_ = *exact
	_ = *exactPrefix
	_ = *exactSkipPrefix
	_ = *preserveHeaders
	_ = *skipPrefix
	_ = *cStyle
	if len(operands) == 0 {
		operands = []string{"-"}
	}
	opts := fmtOptions{
		width:          width,
		goal:           goal,
		splitOnly:      *splitOnly,
		uniformSpacing: *uniformSpacing,
		prefix:         *prefix,
		crownMargin:    *crownMargin,
		tagged:         *taggedParagraph,
		tabWidth:       *tabWidth,
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

// rewriteObsoleteWidth implements the obsolete first-argument form
// (fmt -75 == fmt -w 75). GNU only honors it as the first argument.
func rewriteObsoleteWidth(args []string) []string {
	if len(args) > 0 && len(args[0]) > 1 && args[0][0] == '-' && allDigits(args[0][1:]) {
		return append([]string{"--width=" + args[0][1:]}, args[1:]...)
	}
	return args
}

func allDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

type fmtOptions struct {
	width          int
	goal           int
	splitOnly      bool
	uniformSpacing bool
	prefix         string
	crownMargin    bool
	tagged         bool
	tabWidth       int
}

// lineInfo is one parsed input line.
type lineInfo struct {
	raw     string
	match   bool   // prefix matched (always true without -p)
	blank   bool   // no text after prefix and indentation
	pIndent int    // columns of whitespace before the prefix
	indent  int    // column of the first non-blank character (after any prefix)
	body    string // text after prefix and leading whitespace
}

func parseLine(line, prefix string, tabWidth int) lineInfo {
	li := lineInfo{raw: line, match: true}
	lead, rest := measureIndent(line, 0, tabWidth)
	if prefix != "" {
		if !strings.HasPrefix(rest, prefix) {
			li.match = false
			return li
		}
		li.pIndent = lead
		li.indent, li.body = measureIndent(rest[len(prefix):], lead+runeCols(prefix), tabWidth)
	} else {
		li.indent, li.body = lead, rest
	}
	li.blank = strings.TrimSpace(li.body) == ""
	return li
}

// measureIndent counts the display column (tabs expand at 8, starting
// from col) of the first non-blank character of s and returns it with
// the remainder of the string.
func measureIndent(s string, col, tabWidth int) (int, string) {
	for i, r := range s {
		switch r {
		case ' ':
			col++
		case '\t':
			col += tabWidth - col%tabWidth
		default:
			return col, s[i:]
		}
	}
	return col, ""
}

func fmtStream(r io.Reader, w io.Writer, opts fmtOptions) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1024), 1024*1024)
	var infos []lineInfo
	for sc.Scan() {
		infos = append(infos, parseLine(sc.Text(), opts.prefix, opts.tabWidth))
	}
	if err := sc.Err(); err != nil {
		return err
	}

	stickyOther := 0 // tagged-mode secondary indent carried across paragraphs
	i := 0
	for i < len(infos) {
		li := infos[i]
		if !li.match {
			// A line without the prefix is copied verbatim.
			if _, err := io.WriteString(w, li.raw+"\n"); err != nil {
				return err
			}
			i++
			continue
		}
		if li.blank {
			// Blank lines are preserved (whitespace-only lines become
			// empty; with -p the prefix is kept).
			out := "\n"
			if opts.prefix != "" {
				out = strings.Repeat(" ", li.pIndent) + opts.prefix + "\n"
			}
			if _, err := io.WriteString(w, out); err != nil {
				return err
			}
			i++
			continue
		}

		// Gather the paragraph: which following lines join depends on
		// the mode, and lines with a different indentation never join.
		first := li.indent
		other := first
		bodies := []string{li.body}
		j := i + 1
		joinable := func(k int) bool {
			return k < len(infos) && infos[k].match && !infos[k].blank &&
				infos[k].pIndent == li.pIndent
		}
		switch {
		case opts.splitOnly:
			// Each line is its own paragraph; never join.
		case opts.crownMargin:
			if joinable(j) {
				other = infos[j].indent
				for joinable(j) && infos[j].indent == other {
					bodies = append(bodies, infos[j].body)
					j++
				}
			}
		case opts.tagged:
			if joinable(j) && infos[j].indent != first {
				other = infos[j].indent
				for joinable(j) && infos[j].indent == other {
					bodies = append(bodies, infos[j].body)
					j++
				}
				stickyOther = other
			} else {
				// One-line tagged paragraph: pick a secondary indent
				// that differs from the first line's.
				if stickyOther == first {
					if first == 0 {
						stickyOther = defTaggedIndent
					} else {
						stickyOther = 0
					}
				}
				other = stickyOther
			}
		default:
			for joinable(j) && infos[j].indent == first {
				bodies = append(bodies, infos[j].body)
				j++
			}
		}

		out := fillWords(collectWords(bodies, opts.uniformSpacing), opts,
			marginString(li.pIndent, opts.prefix, first),
			marginString(li.pIndent, opts.prefix, other))
		if _, err := io.WriteString(w, out); err != nil {
			return err
		}
		i = j
	}
	return nil
}

// marginString renders the output margin for a line: the prefix at
// its own indent, padded with spaces out to the text indent column.
func marginString(pIndent int, prefix string, indent int) string {
	if prefix == "" {
		return strings.Repeat(" ", indent)
	}
	pad := indent - pIndent - runeCols(prefix)
	if pad < 0 {
		pad = 0
	}
	return strings.Repeat(" ", pIndent) + prefix + strings.Repeat(" ", pad)
}

// fillWords fills words greedily toward the goal width: a word joins
// the current line when it fits within the hard width and lands at
// least as close to the goal as breaking before it would. Words are
// never split; a word wider than the width goes out on its own line.
func fillWords(words []string, opts fmtOptions, firstMargin, otherMargin string) string {
	var b strings.Builder
	margin := firstMargin
	availWidth := max(1, opts.width-runeCols(margin))
	availGoal := max(1, opts.goal-runeCols(margin))
	lineLen := 0
	sep := 1
	for _, word := range words {
		if word == sentenceGap {
			sep = 2
			continue
		}
		wc := runeCols(word)
		if lineLen == 0 {
			b.WriteString(margin)
			b.WriteString(word)
			lineLen = wc
			sep = 1
			continue
		}
		need := lineLen + sep + wc
		if need <= availWidth && need-availGoal <= availGoal-lineLen {
			if sep == 2 {
				b.WriteString("  ")
			} else {
				b.WriteByte(' ')
			}
			b.WriteString(word)
			lineLen = need
		} else {
			b.WriteByte('\n')
			margin = otherMargin
			availWidth = max(1, opts.width-runeCols(margin))
			availGoal = max(1, opts.goal-runeCols(margin))
			b.WriteString(margin)
			b.WriteString(word)
			lineLen = wc
		}
		sep = 1
	}
	if lineLen > 0 {
		b.WriteByte('\n')
	}
	return b.String()
}

// sentenceGap marks a sentence boundary in a word list (two spaces on
// output when refilled with -u).
const sentenceGap = "\000\000"

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
						words = append(words, sentenceGap)
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
			words = append(words, sentenceGap)
		}
	}
	if len(words) > 0 && words[len(words)-1] == sentenceGap {
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

func runeCols(s string) int {
	n := 0
	for range s {
		n++
	}
	return n
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
