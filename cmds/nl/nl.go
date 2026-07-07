// Package nlcmd implements a focused nl(1) subset: line numbering for
// files and standard input with common style, format, separator, width,
// and numbering-sequence options.
//
// GNU semantics implemented here: all input files form one logical
// document (numbering and section state carry across files), section
// delimiter lines are replaced by an empty line on output, unnumbered
// lines are prefixed with width+len(separator) spaces so text stays
// aligned, -d with a single character keeps ':' as the second character
// (an empty -d argument disables section matching), and p<re> styles use
// POSIX basic regular expressions.
package nlcmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/qiangli/coreutils/pkg/bre"
	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "nl",
	Synopsis: "Number lines of FILE(s), or standard input.",
	Usage:    "nl [OPTION]... [FILE]...",
}

func init() { cmd.Run = run; tool.Register(cmd) }

type options struct {
	bodyStyle    string
	headerStyle  string
	footerStyle  string
	format       string
	separator    string
	width        int
	start        int
	increment    int
	noRenumber   bool
	delimiter    string
	sectionMatch bool
	blankJoin    int
	bodyRE       *regexp.Regexp
	headerRE     *regexp.Regexp
	footerRE     *regexp.Regexp
}

// numberState carries the numbering state across all input files: GNU nl
// treats the concatenation of its inputs as one logical document.
type numberState struct {
	lineNo   int
	section  string
	blankRun int
}

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	bodyStyle := fs.StringP("body-numbering", "b", "t", "use STYLE for numbering body lines: a (all), t (nonempty), n (none), pBRE (matching lines)")
	headerStyle := fs.StringP("header-numbering", "h", "n", "use STYLE for numbering header lines: a, t, n, pBRE")
	footerStyle := fs.StringP("footer-numbering", "f", "n", "use STYLE for numbering footer lines: a, t, n, pBRE")
	format := fs.StringP("number-format", "n", "rn", "insert line numbers according to FORMAT: ln, rn, rz")
	separator := fs.StringP("number-separator", "s", "\t", "add SEP after each line number")
	width := fs.IntP("number-width", "w", 6, "use WIDTH columns for line numbers")
	start := fs.IntP("starting-line-number", "v", 1, "first line number for each section")
	increment := fs.IntP("line-increment", "i", 1, "line number increment at each line (may be negative)")
	noRenumber := fs.BoolP("no-renumber", "p", false, "do not reset line numbers for each section")
	delimiter := fs.StringP("section-delimiter", "d", "\\:", "use CC for logical page delimiters; empty CC disables section matching")
	blankJoin := fs.IntP("join-blank-lines", "l", 1, "group of NUMBER empty lines counted as one")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	o := options{
		bodyStyle: *bodyStyle, headerStyle: *headerStyle, footerStyle: *footerStyle,
		format: *format, separator: *separator, width: *width,
		start: *start, increment: *increment, noRenumber: *noRenumber,
		blankJoin: *blankJoin,
	}
	var err error
	if o.bodyRE, err = validateStyle("body", o.bodyStyle); err != nil {
		return tool.UsageError(rc, cmd, "%v", err)
	}
	if o.headerRE, err = validateStyle("header", o.headerStyle); err != nil {
		return tool.UsageError(rc, cmd, "%v", err)
	}
	if o.footerRE, err = validateStyle("footer", o.footerStyle); err != nil {
		return tool.UsageError(rc, cmd, "%v", err)
	}
	if o.format != "ln" && o.format != "rn" && o.format != "rz" {
		return tool.UsageError(rc, cmd, "invalid line numbering format: %q", o.format)
	}
	if o.width < 1 {
		return tool.UsageError(rc, cmd, "invalid line number field width: %d", o.width)
	}
	if o.blankJoin <= 0 {
		return tool.UsageError(rc, cmd, "invalid blank-line join count: %d", o.blankJoin)
	}
	// -d semantics: one character c means delimiters are c followed by
	// ':'; two or more characters are used as given; an empty argument
	// disables section matching entirely (GNU extension).
	switch len(*delimiter) {
	case 0:
		o.sectionMatch = false
	case 1:
		o.delimiter = *delimiter + ":"
		o.sectionMatch = true
	default:
		o.delimiter = *delimiter
		o.sectionMatch = true
	}

	files := operands
	if len(files) == 0 {
		files = []string{"-"}
	}
	w := bufio.NewWriter(rc.Out)
	st := &numberState{lineNo: o.start, section: "body"}
	exit := 0
	for _, name := range files {
		r, closer, err := open(rc, name)
		if err != nil {
			fmt.Fprintf(rc.Err, "nl: %s: %v\n", name, tool.SysErr(err))
			exit = 1
			continue
		}
		if err := number(r, w, o, st); err != nil {
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

func validateStyle(section, style string) (*regexp.Regexp, error) {
	switch style {
	case "a", "t", "n":
		return nil, nil
	}
	if strings.HasPrefix(style, "p") && len(style) > 1 {
		translated, err := bre.ToGo(style[1:])
		if err != nil {
			return nil, fmt.Errorf("invalid %s numbering regexp: %q", section, style[1:])
		}
		re, err := regexp.Compile(translated)
		if err != nil {
			return nil, fmt.Errorf("invalid %s numbering regexp: %q", section, style[1:])
		}
		return re, nil
	}
	return nil, fmt.Errorf("invalid %s numbering style: %q", section, style)
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

func number(r io.Reader, w *bufio.Writer, o options, st *numberState) error {
	br := bufio.NewReader(r)
	for {
		line, err := br.ReadString('\n')
		if len(line) > 0 {
			if next, delimiter := sectionDelimiter(line, o); delimiter {
				st.section = next
				if !o.noRenumber {
					st.lineNo = o.start
				}
				// GNU nl replaces the delimiter line with an empty line.
				if _, werr := w.WriteString("\n"); werr != nil {
					return werr
				}
				if err == io.EOF {
					return nil
				}
				if err != nil {
					return err
				}
				continue
			}
			style := o.styleFor(st.section)
			if st.shouldNumber(style, o.regexpFor(st.section), line, o.blankJoin) {
				if _, werr := fmt.Fprint(w, formatNumber(st.lineNo, o)); werr != nil {
					return werr
				}
				st.lineNo += o.increment
			} else {
				// Unnumbered lines get width+len(separator) spaces so the
				// text column stays aligned (GNU print_no_line_fmt).
				if _, werr := fmt.Fprintf(w, "%*s", o.width+len(o.separator), ""); werr != nil {
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

func (o options) styleFor(section string) string {
	switch section {
	case "header":
		return o.headerStyle
	case "footer":
		return o.footerStyle
	default:
		return o.bodyStyle
	}
}

func (o options) regexpFor(section string) *regexp.Regexp {
	switch section {
	case "header":
		return o.headerRE
	case "footer":
		return o.footerRE
	default:
		return o.bodyRE
	}
}

func sectionDelimiter(line string, o options) (string, bool) {
	if !o.sectionMatch {
		return "", false
	}
	text := strings.TrimRight(line, "\n")
	switch text {
	case o.delimiter + o.delimiter + o.delimiter:
		return "header", true
	case o.delimiter + o.delimiter:
		return "body", true
	case o.delimiter:
		return "footer", true
	default:
		return "", false
	}
}

func (st *numberState) shouldNumber(style string, re *regexp.Regexp, line string, blankJoin int) bool {
	text := strings.TrimRight(line, "\n")
	blank := text == ""
	switch style {
	case "a":
		if blankJoin > 1 {
			if !blank {
				st.blankRun = 0
				return true
			}
			st.blankRun++
			if st.blankRun == blankJoin {
				st.blankRun = 0
				return true
			}
			return false
		}
		return true
	case "t":
		return !blank
	case "n":
		return false
	default: // p<re>
		return re != nil && re.MatchString(text)
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
