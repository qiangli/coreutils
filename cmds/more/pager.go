package morecmd

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/qiangli/coreutils/tool"
	"golang.org/x/term"
)

type ttyChannel struct {
	cmds  io.Reader
	fd    int
	hasFd bool
	close func() error
}

var openControllingTTY = func(rc *tool.RunContext) (*ttyChannel, bool) {
	f, err := os.Open("/dev/tty")
	if err != nil {
		return nil, false
	}
	return &ttyChannel{
		cmds:  f,
		fd:    int(f.Fd()),
		hasFd: true,
		close: f.Close,
	}, true
}

var getTerminalSize = func(fd int) (width, height int, err error) {
	return term.GetSize(fd)
}

// terminalSize resolves the screen geometry: rows from -n, else $LINES,
// else the controlling-terminal size seam, else 24; width from $COLUMNS,
// else the same tty-size seam, else 80.
func terminalSize(rc *tool.RunContext, ch *ttyChannel, nLines int) (rows, width int) {
	if nLines > 0 {
		rows = nLines
	} else if l := rc.Getenv("LINES"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			rows = v
		}
	}

	if w := rc.Getenv("COLUMNS"); w != "" {
		if v, err := strconv.Atoi(w); err == nil && v > 0 {
			width = v
		}
	}

	if (rows == 0 || width == 0) && ch != nil && ch.hasFd {
		w, h, err := getTerminalSize(ch.fd)
		if err == nil {
			if rows == 0 {
				rows = h
			}
			if width == 0 {
				width = w
			}
		}
	}

	if rows == 0 {
		rows = 24
	}
	if width == 0 {
		width = 80
	}
	return rows, width
}

// cmdReader adapts the controlling-terminal command channel to a
// cancellable read. A terminal read blocks until the operator types, so a
// plain Read would ignore a canceled context until a keystroke arrived —
// the pager must be able to give up without one.
type cmdReader struct {
	runes <-chan cmdRune
}

type cmdRune struct {
	r   rune
	err error
}

func newCmdReader(r io.Reader) *cmdReader {
	br := bufio.NewReader(r)
	ch := make(chan cmdRune)
	go func() {
		defer close(ch)
		for {
			c, _, err := br.ReadRune()
			ch <- cmdRune{r: c, err: err}
			if err != nil {
				return
			}
		}
	}()
	return &cmdReader{runes: ch}
}

func (c *cmdReader) read(ctx context.Context) (rune, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case v, ok := <-c.runes:
		if !ok {
			return 0, io.EOF
		}
		return v.r, v.err
	}
}

// recognizedCommands lists the more(1) commands POSIX defines beyond the
// <space>/quit pair this slice implements, plus the digits that prefix a
// count. A command in this set is refused as explicitly deferred; anything
// outside it is refused as unknown. Either way it is never silently
// treated as some other command.
const recognizedCommands = "0123456789 hfbjkdusgGnNmrRvqQZ=.:?/'\n\r" +
	"\x02\x04\x05\x06\x07\x0c\x0e\x10\x15\x19\x7f"

// commandName renders a command key for a diagnostic.
func commandName(r rune) string {
	switch r {
	case ' ':
		return "space"
	case '\n':
		return "newline"
	case '\r':
		return "carriage return"
	case 0x7f:
		return "^?"
	}
	if r < 0x20 {
		return fmt.Sprintf("^%c", r+'@')
	}
	return string(r)
}

type pager struct {
	rc    *tool.RunContext
	w     *bufio.Writer
	cmds  *cmdReader
	o     options
	files []string

	linesPrinted int
	col          int
	exitCode     int
}

func (p *pager) canceled() bool {
	return p.rc.Ctx != nil && p.rc.Ctx.Err() != nil
}

// refuse reports a command this slice does not implement. Recognized
// more(1) commands are named as deferred; the rest as unknown.
func (p *pager) refuse(r rune) {
	if strings.ContainsRune(recognizedCommands, r) {
		fmt.Fprintf(p.rc.Err, "more: %s: command not supported yet (deferred)\n", commandName(r))
		return
	}
	fmt.Fprintf(p.rc.Err, "more: unknown command: %s (deferred)\n", commandName(r))
}

func (p *pager) run() int {
	for i, name := range p.files {
		if p.canceled() {
			return p.exitCode
		}

		r, closer, err := open(p.rc, name)
		if err != nil {
			fmt.Fprintf(p.rc.Err, "more: %s: %v\n", name, tool.SysErr(err))
			p.exitCode = 1
			continue
		}

		lines, err := readLines(r)
		if closer != nil {
			closer.Close()
		}
		if err != nil && err != io.EOF {
			fmt.Fprintf(p.rc.Err, "more: %s: %v\n", name, tool.SysErr(err))
			p.exitCode = 1
		}

		start := computeStart(lines, p.o, p.rc.Err)

		if p.o.cleanPrint {
			p.w.WriteString("\x1b[H\x1b[2J")
		}

		p.linesPrinted = 0
		p.col = 0

		if p.o.command != "" {
			if !p.processCommand(p.o.command) {
				return p.exitCode
			}
		}

		wroteBlank := false

		for _, line := range lines[start:] {
			blank := line == "\n"
			if p.o.squeeze && blank && wroteBlank {
				continue
			}

			if !p.printLine(line) {
				return p.exitCode
			}
			wroteBlank = blank
		}

		if i < len(p.files)-1 {
			if !p.prompt(fmt.Sprintf("--More--(Next file: %s)", p.files[i+1])) {
				return p.exitCode
			}
		} else {
			if p.o.exitOnEof {
				return p.exitCode
			}
			if !p.prompt("--More--(END)") {
				return p.exitCode
			}
		}
	}
	return p.exitCode
}

// wrap accounts for a column overflow: the terminal would move to the next
// line, so fold the output there and charge the screenful a line.
func (p *pager) wrap() {
	for p.col > p.o.width {
		p.linesPrinted++
		p.col -= p.o.width
		p.w.WriteString("\n")
	}
}

func (p *pager) printLine(line string) bool {
	for _, r := range line {
		if p.linesPrinted >= p.o.screenful {
			if !p.prompt("--More--") {
				return false
			}
		}

		if p.canceled() {
			return false
		}

		switch {
		case r == '\b' && !p.o.plain:
			if p.col > 0 {
				p.col--
			}
			p.w.WriteRune(r)
		case r == '\r' && !p.o.plain:
			p.col = 0
			p.w.WriteRune(r)
		case r == '\t':
			p.col += 8 - (p.col % 8)
			p.wrap()
			p.w.WriteRune(r)
		case r == '\n':
			p.linesPrinted++
			p.col = 0
			p.w.WriteRune(r)
		default:
			p.col++
			p.wrap()
			p.w.WriteRune(r)
		}
	}
	return true
}

func (p *pager) prompt(msg string) bool {
	p.w.WriteString("\x1b[7m" + msg + "\x1b[m")
	if err := p.w.Flush(); err != nil {
		// The prompt never reached the screen; reading a reply to a
		// prompt nobody can see would be a hang in all but name. The
		// caller's final Flush reports the error and sets the exit code.
		return false
	}

	for {
		if p.canceled() {
			return false
		}
		r, err := p.cmds.read(p.rc.Ctx)
		if err != nil {
			return false
		}

		switch r {
		case ' ':
			p.w.WriteString("\r\x1b[K")
			p.linesPrinted = 0
			return true
		case 'q', 'Q':
			p.w.WriteString("\r\x1b[K")
			return false
		default:
			p.w.WriteString("\r\x1b[K")
			p.refuse(r)
			p.w.WriteString("\x1b[7m" + msg + "\x1b[m")
			if err := p.w.Flush(); err != nil {
				return false
			}
		}
	}
}

// processCommand runs the -p command at a new file's first screen. It sees
// the same command set as the interactive prompt: <space> (a no-op before
// any output) and quit, with everything else refused rather than guessed at.
func (p *pager) processCommand(cmd string) bool {
	for _, r := range strings.TrimSpace(cmd) {
		switch r {
		case ' ':
			p.linesPrinted = 0
		case 'q', 'Q':
			return false
		default:
			p.refuse(r)
		}
	}
	return true
}
