// Package morecmd implements more(1). When standard output is a terminal
// and a controlling-terminal command channel is available, it pages the
// input a screenful at a time with an interactive prompt (this slice
// supports advancing with <space> and quitting with `q`; every other
// recognized command fails loudly as deferred). Otherwise — no terminal,
// or the terminal channel cannot be opened — it degrades to a
// non-interactive pass-through: files/stdin are copied to stdout
// unmodified except for `-s` (squeeze), matching POSIX's requirement that
// no option other than `-s` take effect when stdout is not a terminal.
//
// -P searches for a literal pattern (util-linux semantics); when the
// pattern is not found, "Pattern not found" is printed to standard error
// and the file is displayed from the start.
package morecmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"golang.org/x/term"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "more",
	Synopsis: "Display FILE(s) or standard input a screenful at a time (pure-Go pager).",
	Usage:    "more [OPTION]... [FILE]...",
}

func init() { cmd.Run = run; tool.Register(cmd) }

type options struct {
	squeeze    bool
	lines      int
	fromLine   int
	pattern    string
	plain      bool   // -u: treat backspace as printable, keep trailing CR
	exitOnEof  bool   // -e: exit after the last line of the last file
	cleanPrint bool   // -c: redraw not scroll (may be ignored per POSIX)
	command    string // -p: more-command run at each new file's first screen

	// Terminal-mode geometry (unused in the non-interactive path).
	rows      int
	width     int
	screenful int
}

var isTerminal = func(w io.Writer) bool {
	if f, ok := w.(interface{ Fd() uintptr }); ok {
		return term.IsTerminal(int(f.Fd()))
	}
	return false
}

// openTTY is the injectable controlling-terminal command channel seam.
// It returns a channel and true when interactive paging is possible; it
// returns false (fail closed — degrade to the copy path, never hang) when
// no controlling terminal can be opened. Tests replace it with a fake.
var openTTY = func(rc *tool.RunContext) (*ttyChannel, bool) {
	return openControllingTTY(rc)
}

func parseMORE(env []string) []string {
	var moreEnv string
	for _, e := range env {
		if strings.HasPrefix(e, "MORE=") {
			moreEnv = e[5:]
		}
	}
	if moreEnv == "" {
		return nil
	}
	var args []string
	var buf []byte
	for i := 0; i < len(moreEnv); i++ {
		c := moreEnv[i]
		if c == ' ' || c == '\t' {
			if len(buf) > 0 {
				args = append(args, string(buf))
				buf = buf[:0]
			}
		} else {
			buf = append(buf, c)
		}
	}
	if len(buf) > 0 {
		args = append(args, string(buf))
	}
	return args
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
	_ = fs.BoolP("ignore-case", "i", false, "ignore case in interactive searches (deferred)")
	exitOnEof := fs.BoolP("exit-on-eof", "e", false, "exit after the last line of the last file")
	_ = fs.BoolP("no-pause", "f", false, "accepted for non-interactive compatibility")
	command := fs.StringP("command", "p", "", "run COMMAND at each new file's first screen")
	_ = fs.StringP("tag", "t", "", "start at TAG (deferred)")
	plain := fs.BoolP("plain", "u", false, "treat backspace as printable, keep trailing carriage return")
	cleanPrint := fs.BoolP("clean-print", "c", false, "redraw the screen rather than scrolling")

	args = append(parseMORE(rc.Env), args...)
	for i, arg := range args {
		if strings.HasPrefix(arg, "-") && len(arg) > 1 && allDigits(arg[1:]) {
			args[i] = "-n=" + arg[1:]
		}
	}
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}

	terminal := isTerminal(rc.Out)
	var ch *ttyChannel
	interactive := false
	if terminal {
		// Search (-i) and tag navigation (-t) are not part of this slice;
		// refuse them loudly rather than accept them as silent no-ops.
		for _, r := range []struct{ name, flag string }{
			{"ignore-case", "-i"}, {"tag", "-t"},
		} {
			if fs.Changed(r.name) {
				return tool.NotSupported(rc, cmd, r.flag)
			}
		}
		if c, ok := openTTY(rc); ok {
			ch = c
			defer ch.close()
			interactive = true
		}
		// Fail closed: if the terminal channel could not be opened we fall
		// through to the non-interactive copy path below — never hang.
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
	if !terminal {
		// POSIX: when stdout is not a terminal only -s takes effect.
		*fromLine = 1
		*pattern = ""
	}

	o := options{
		squeeze:    *squeeze,
		lines:      *lines,
		fromLine:   *fromLine,
		pattern:    *pattern,
		plain:      *plain,
		exitOnEof:  *exitOnEof,
		cleanPrint: *cleanPrint,
		command:    *command,
	}
	files := operands
	if len(files) == 0 {
		files = []string{"-"}
	}

	w := bufio.NewWriter(rc.Out)
	if interactive {
		o.rows, o.width = terminalSize(rc, ch, o.lines)
		o.screenful = o.rows - 1
		if o.screenful < 1 {
			o.screenful = 1
		}
		p := &pager{rc: rc, w: w, cmds: bufio.NewReader(ch.cmds), o: o, files: files}
		exit := p.run()
		if err := w.Flush(); err != nil {
			fmt.Fprintf(rc.Err, "more: write error: %v\n", err)
			return 1
		}
		return exit
	}

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
	lines, err := readLines(r)
	if err != nil {
		return err
	}
	start := computeStart(lines, o, errW)
	wroteBlank := false
	for _, line := range lines[start:] {
		blank := line == "\n"
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

// readLines reads r fully into a slice of lines, each retaining its
// trailing newline (a final line without one is kept as-is).
func readLines(r io.Reader) ([]string, error) {
	br := bufio.NewReader(r)
	var lines []string
	for {
		line, err := br.ReadString('\n')
		if len(line) > 0 {
			lines = append(lines, line)
		}
		if err == io.EOF {
			return lines, nil
		}
		if err != nil {
			return lines, err
		}
	}
}

// computeStart resolves the first line index to display from the -P
// literal-pattern search and the -F starting line. Reporting "Pattern not
// found" to errW matches util-linux behavior (display from the start).
func computeStart(lines []string, o options, errW io.Writer) int {
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
			fmt.Fprintln(errW, "Pattern not found")
		} else {
			start = found
		}
	}
	if o.fromLine-1 > start {
		start = o.fromLine - 1
	}
	if start > len(lines) {
		start = len(lines)
	}
	return start
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
