package morecmd

import (
	"bufio"
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

type pager struct {
	rc    *tool.RunContext
	w     *bufio.Writer
	cmds  *bufio.Reader
	o     options
	files []string

	linesPrinted int
	col          int
	exitCode     int
}

func (p *pager) run() int {
	for i, name := range p.files {
		if p.rc.Ctx.Err() != nil {
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

func (p *pager) printLine(line string) bool {
	for _, r := range line {
		if p.linesPrinted >= p.o.screenful {
			if !p.prompt("--More--") {
				return false
			}
		}

		if p.rc.Ctx.Err() != nil {
			return false
		}

		if r == '\b' && !p.o.plain {
			if p.col > 0 {
				p.col--
			}
			p.w.WriteRune(r)
		} else if r == '\r' && !p.o.plain {
			p.col = 0
			p.w.WriteRune(r)
		} else if r == '\t' {
			spaces := 8 - (p.col % 8)
			p.col += spaces
			if p.col > p.o.width {
				p.linesPrinted++
				p.col = spaces - (p.col - p.o.width)
				if p.col < 0 {
					p.col = 0
				}
				p.w.WriteString("\n")
			}
			p.w.WriteRune(r)
		} else if r == '\n' {
			p.linesPrinted++
			p.col = 0
			p.w.WriteRune(r)
		} else {
			p.col++
			if p.col > p.o.width {
				p.linesPrinted++
				p.col = 1
				p.w.WriteString("\n")
			}
			p.w.WriteRune(r)
		}
	}
	return true
}

func (p *pager) prompt(msg string) bool {
	p.w.WriteString("\x1b[7m" + msg + "\x1b[m")
	p.w.Flush()

	for {
		if p.rc.Ctx.Err() != nil {
			return false
		}
		r, _, err := p.cmds.ReadRune()
		if err != nil {
			return false
		}

		if r == ' ' || r == '\n' || r == '\r' {
			p.w.WriteString("\r\x1b[K")
			p.linesPrinted = 0
			return true
		} else if r == 'q' || r == 'Q' {
			p.w.WriteString("\r\x1b[K")
			return false
		} else {
			p.w.WriteString("\r\x1b[K")
			fmt.Fprintf(p.rc.Err, "more: unknown command: %c (deferred)\n", r)
			p.w.WriteString("\x1b[7m" + msg + "\x1b[m")
			p.w.Flush()
		}
	}
}

func (p *pager) processCommand(cmd string) bool {
	cmd = strings.TrimSpace(cmd)
	for _, r := range cmd {
		if r == ' ' || r == '\n' || r == '\r' {
			p.linesPrinted = 0
			return true
		} else if r == 'q' || r == 'Q' {
			return false
		} else {
			fmt.Fprintf(p.rc.Err, "more: unknown command: %c (deferred)\n", r)
		}
	}
	return true
}
