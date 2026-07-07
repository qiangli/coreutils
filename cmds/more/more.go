// Package morecmd implements a non-interactive more(1) fallback for
// agent use: concatenate files or stdin to stdout without terminal
// control. -P searches for a literal pattern (util-linux semantics);
// when the pattern is not found, "Pattern not found" is printed to
// standard error and the file is displayed from the start.
package morecmd

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
	Name:     "more",
	Synopsis: "Display FILE(s) or standard input. This pure-Go implementation is a non-interactive pager fallback.",
	Usage:    "more [OPTION]... [FILE]...",
}

func init() { cmd.Run = run; tool.Register(cmd) }

type options struct {
	squeeze  bool
	lines    int
	fromLine int
	pattern  string
}

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	squeeze := fs.BoolP("squeeze", "s", false, "squeeze multiple blank lines into one")
	lines := fs.IntP("lines", "n", 0, "set screen size to NUM lines in interactive mode")
	number := fs.Int("number", 0, "same as --lines")
	fromLine := fs.IntP("from-line", "F", 1, "start displaying at line NUM")
	pattern := fs.StringP("pattern", "P", "", "start displaying at the first line containing PATTERN")
	_ = fs.BoolP("silent", "d", false, "accepted for non-interactive compatibility")
	_ = fs.BoolP("logical", "l", false, "accepted for non-interactive compatibility")
	_ = fs.BoolP("exit-on-eof", "e", false, "accepted for non-interactive compatibility")
	_ = fs.BoolP("no-pause", "f", false, "accepted for non-interactive compatibility")
	_ = fs.BoolP("print-over", "p", false, "accepted for non-interactive compatibility")
	_ = fs.BoolP("plain", "u", false, "accepted for non-interactive compatibility")
	_ = fs.BoolP("clean-print", "c", false, "accepted for non-interactive compatibility")
	for i, arg := range args {
		if strings.HasPrefix(arg, "-") && len(arg) > 1 && allDigits(arg[1:]) {
			args[i] = "-n=" + arg[1:]
		}
	}
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	if *lines < 0 {
		return tool.UsageError(rc, cmd, "invalid line count: %d", *lines)
	}
	if *number < 0 {
		return tool.UsageError(rc, cmd, "invalid line count: %d", *number)
	}
	if *fromLine <= 0 {
		return tool.UsageError(rc, cmd, "invalid starting line: %d", *fromLine)
	}
	if *number > 0 {
		*lines = *number
	}
	o := options{squeeze: *squeeze, lines: *lines, fromLine: *fromLine, pattern: *pattern}
	files := operands
	if len(files) == 0 {
		files = []string{"-"}
	}
	w := bufio.NewWriter(rc.Out)
	exit := 0
	for _, name := range files {
		r, closer, err := open(rc, name)
		if err != nil {
			fmt.Fprintf(rc.Err, "more: %s: %v\n", name, tool.SysErr(err))
			exit = 1
			continue
		}
		if err := copyMore(w, rc.Err, r, o); err != nil {
			fmt.Fprintf(rc.Err, "more: %s: %v\n", name, tool.SysErr(err))
			exit = 1
		}
		if closer != nil {
			closer.Close()
		}
	}
	if err := w.Flush(); err != nil {
		fmt.Fprintf(rc.Err, "more: write error: %v\n", err)
		return 1
	}
	return exit
}

func copyMore(w *bufio.Writer, errW io.Writer, r io.Reader, o options) error {
	br := bufio.NewReader(r)
	var lines []string
	for {
		line, err := br.ReadString('\n')
		if len(line) > 0 {
			lines = append(lines, line)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	start := 0
	if o.pattern != "" {
		found := -1
		for i, line := range lines {
			if strings.Contains(strings.TrimRight(line, "\n\r"), o.pattern) {
				found = i
				break
			}
		}
		if found < 0 {
			// util-linux/uutils behavior: report and display from the start.
			fmt.Fprintln(errW, "Pattern not found")
		} else {
			start = found
		}
	}
	if o.fromLine-1 > start {
		start = o.fromLine - 1
	}
	wroteBlank := false
	for _, line := range lines[start:] {
		blank := strings.TrimRight(line, "\n\r") == ""
		if o.squeeze && blank && wroteBlank {
			continue
		}
		if _, werr := w.WriteString(line); werr != nil {
			return werr
		}
		wroteBlank = blank
	}
	return nil
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	_, err := strconv.Atoi(s)
	return err == nil
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
