// Package nlcmd implements a focused nl(1) subset: line numbering for
// files and standard input with common style, format, separator, width,
// and numbering-sequence options.
package nlcmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
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
	bodyStyle   string
	headerStyle string
	footerStyle string
	format      string
	separator   string
	width       int
	start       int
	increment   int
	noRenumber  bool
	delimiter   string
	blankJoin   int
	bodyRE      *regexp.Regexp
	headerRE    *regexp.Regexp
	footerRE    *regexp.Regexp
}

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	bodyStyle := fs.StringP("body-numbering", "b", "t", "use STYLE for numbering body lines: a (all), t (nonempty), n (none)")
	headerStyle := fs.StringP("header-numbering", "h", "n", "use STYLE for numbering header lines: a, t, n")
	footerStyle := fs.StringP("footer-numbering", "f", "n", "use STYLE for numbering footer lines: a, t, n")
	format := fs.StringP("number-format", "n", "rn", "insert line numbers according to FORMAT: ln, rn, rz")
	separator := fs.StringP("number-separator", "s", "\t", "add SEP after each line number")
	width := fs.IntP("number-width", "w", 6, "use WIDTH columns for line numbers")
	start := fs.IntP("starting-line-number", "v", 1, "first line number on each logical page")
	increment := fs.IntP("line-increment", "i", 1, "line number increment")
	noRenumber := fs.BoolP("no-renumber", "p", false, "do not reset line numbers at logical page delimiters")
	delimiter := fs.StringP("section-delimiter", "d", "\\:", "use CC as logical page delimiter characters")
	blankJoin := fs.IntP("join-blank-lines", "l", 1, "number one of every NUMBER adjacent blank lines")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	o := options{
		bodyStyle: *bodyStyle, headerStyle: *headerStyle, footerStyle: *footerStyle,
		format: *format, separator: *separator, width: *width,
		start: *start, increment: *increment, noRenumber: *noRenumber,
		delimiter: *delimiter, blankJoin: *blankJoin,
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
		return tool.UsageError(rc, cmd, "invalid number format: %q", o.format)
	}
	if o.width < 0 {
		return tool.UsageError(rc, cmd, "invalid line number width: %d", o.width)
	}
	if o.increment <= 0 {
		return tool.UsageError(rc, cmd, "invalid line increment: %d", o.increment)
	}
	if o.blankJoin <= 0 {
		return tool.UsageError(rc, cmd, "invalid blank-line join count: %d", o.blankJoin)
	}
	if o.delimiter == "" {
		return tool.UsageError(rc, cmd, "section delimiter must not be empty")
	}

	files := operands
	if len(files) == 0 {
		files = []string{"-"}
	}
	w := bufio.NewWriter(rc.Out)
	lineNo := o.start
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

func validateStyle(section, style string) (*regexp.Regexp, error) {
	if style != "a" && style != "t" && style != "n" {
		if strings.HasPrefix(style, "p") && len(style) > 1 {
			re, err := regexp.Compile(style[1:])
			if err != nil {
				return nil, fmt.Errorf("invalid %s numbering regexp: %q", section, style[1:])
			}
			return re, nil
		}
		return nil, fmt.Errorf("invalid %s numbering style: %q", section, style)
	}
	return nil, nil
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
	section := "body"
	blankRun := 0
	for {
		line, err := br.ReadString('\n')
		if len(line) > 0 {
			if next, delimiter := sectionDelimiter(line, o.delimiter); delimiter {
				section = next
				blankRun = 0
				if !o.noRenumber {
					*lineNo = o.start
				}
				if _, werr := w.WriteString(line); werr != nil {
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
			style := o.styleFor(section)
			numbered := shouldNumber(style, o.regexpFor(section), line, o.blankJoin, &blankRun)
			if numbered {
				if _, werr := fmt.Fprint(w, formatNumber(*lineNo, o)); werr != nil {
					return werr
				}
				*lineNo += o.increment
			} else if style == "t" {
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

func sectionDelimiter(line, delimiter string) (string, bool) {
	text := strings.TrimRight(line, "\n")
	switch text {
	case delimiter + delimiter + delimiter:
		return "header", true
	case delimiter + delimiter:
		return "body", true
	case delimiter:
		return "footer", true
	default:
		return "", false
	}
}

func shouldNumber(style string, re *regexp.Regexp, line string, blankJoin int, blankRun *int) bool {
	text := strings.TrimRight(line, "\n")
	blank := text == ""
	if blank {
		(*blankRun)++
	} else {
		*blankRun = 0
	}
	switch style {
	case "a":
		if blank && blankJoin > 1 {
			return *blankRun%blankJoin == 0
		}
		return true
	case "t":
		return !blank
	default:
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
