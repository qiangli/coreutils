package morecmd

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/qiangli/coreutils/tool"
	"golang.org/x/term"
)

// ttyChannel is deliberately synchronous. Platform implementations make
// readCommand cancellable before constructing one; the pager never starts a
// helper goroutine which could outlive an in-process invocation.
type ttyChannel struct {
	readCommand func(context.Context) (byte, error)
	fd          int
	hasFd       bool
	close       func() error
}

var getTerminalSize = func(fd int) (width, height int, err error) {
	return term.GetSize(fd)
}

func terminalSize(rc *tool.RunContext, ch *ttyChannel, nLines int) (rows, width int) {
	if nLines > 0 {
		rows = nLines
	} else if v, err := strconv.Atoi(rc.Getenv("LINES")); err == nil && v > 0 {
		rows = v
	}
	if v, err := strconv.Atoi(rc.Getenv("COLUMNS")); err == nil && v > 0 {
		width = v
	}
	if (rows == 0 || width == 0) && ch != nil && ch.hasFd {
		if w, h, err := getTerminalSize(ch.fd); err == nil {
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

const recognizedCommands = "0123456789 hfbjkdusgGnNmrRvqQZ=.:?/'\n\r" +
	"\x02\x04\x05\x06\x07\x0c\x0e\x10\x15\x19\x7f"

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
	out   *bufio.Writer
	tty   *ttyChannel
	o     options
	files []string

	linesPrinted int
	col          int
	exitCode     int
	quit         bool
}

func (p *pager) canceled() bool {
	return p.rc.Ctx != nil && p.rc.Ctx.Err() != nil
}

func (p *pager) diagnose(format string, args ...any) {
	fmt.Fprintf(p.rc.Err, "more: "+format+"\n", args...)
}

func (p *pager) fail(format string, args ...any) bool {
	p.diagnose(format, args...)
	p.exitCode = 1
	return false
}

func (p *pager) run() int {
	for i, name := range p.files {
		if p.canceled() || p.quit {
			break
		}
		r, closer, err := openInput(p.rc, name)
		if err != nil {
			p.fail("%s: %v", name, tool.SysErr(err))
			continue
		}

		p.linesPrinted, p.col = 0, 0
		if p.o.cleanPrint && !p.writeUI("\x1b[H\x1b[2J") {
			p.closeSource(name, closer)
			break
		}
		if p.o.command != "" && !p.processCommand(p.o.command) {
			p.closeSource(name, closer)
			break
		}
		if !p.stream(name, r) {
			p.closeSource(name, closer)
			break
		}
		if !p.closeSource(name, closer) {
			break
		}
		if p.quit || p.canceled() {
			break
		}

		if i < len(p.files)-1 {
			if !p.prompt(fmt.Sprintf("--More--(Next file: %s)", p.files[i+1])) {
				break
			}
		} else if !p.o.exitOnEof && !p.prompt("--More--(END)") {
			break
		}
	}
	return p.exitCode
}

func (p *pager) closeSource(name string, closer io.Closer) bool {
	if closer == nil {
		return true
	}
	if err := closer.Close(); err != nil {
		return p.fail("%s: close: %v", name, tool.SysErr(err))
	}
	return true
}

// stream consumes the source only as display space becomes available. In the
// normal case it reads one byte at a time, so a full first screen is flushed
// before an open pipe (or an infinite stdin) is asked for another byte.
func (p *pager) stream(name string, r io.Reader) bool {
	br := bufio.NewReader(r)
	line := 1
	active := p.o.pattern == "" && line >= p.o.fromLine
	var pending []byte // only used by the optional literal-pattern extension
	searchStart := 0
	wroteBlank := false
	atLineStart := true

	for {
		b, err := br.ReadByte()
		if err != nil {
			if err == io.EOF {
				if p.o.pattern != "" && !active {
					last := strings.TrimRight(string(pending[searchStart:]), "\r\n")
					if strings.Contains(last, p.o.pattern) && line >= p.o.fromLine {
						pending = pending[searchStart:]
					} else {
						fmt.Fprintln(p.rc.Err, "Pattern not found")
					}
				}
				if len(pending) != 0 && !p.emitBytes(pending, &wroteBlank, &atLineStart) {
					return false
				}
				return true
			}
			return p.fail("%s: %v", name, tool.SysErr(err))
		}

		if p.o.pattern != "" && !active {
			pending = append(pending, b)
			if b != '\n' {
				continue
			}
			text := strings.TrimRight(string(pending[searchStart:]), "\r\n")
			if strings.Contains(text, p.o.pattern) && line >= p.o.fromLine {
				active = true
				if !p.emitBytes(pending[searchStart:], &wroteBlank, &atLineStart) {
					return false
				}
				pending = pending[:0]
				searchStart = 0
			} else {
				searchStart = len(pending)
			}
			line++
			continue
		}

		if !active {
			if b == '\n' {
				line++
				active = line >= p.o.fromLine
			}
			continue
		}
		if !p.emitBytes([]byte{b}, &wroteBlank, &atLineStart) {
			return false
		}
	}
}

func (p *pager) emitBytes(bs []byte, wroteBlank, atLineStart *bool) bool {
	for _, b := range bs {
		blank := b == '\n' && *atLineStart
		if !(p.o.squeeze && blank && *wroteBlank) {
			if !p.emitByte(b) {
				return false
			}
		}
		if b == '\n' {
			*wroteBlank = blank
			*atLineStart = true
		} else {
			*atLineStart = false
		}
	}
	return true
}

func (p *pager) emitByte(b byte) bool {
	if p.canceled() {
		return false
	}
	if err := p.out.WriteByte(b); err != nil {
		return p.fail("write error: %v", err)
	}

	switch {
	case b == '\b' && !p.o.plain:
		if p.col > 0 {
			p.col--
		}
	case b == '\r' && !p.o.plain:
		p.col = 0
	case b == '\t':
		p.col += 8 - p.col%8
		p.accountWrap()
	case b == '\n':
		p.linesPrinted++
		p.col = 0
	default:
		p.col++
		p.accountWrap()
	}
	if p.linesPrinted >= p.o.screenful && !p.prompt("--More--") {
		return false
	}
	return true
}

func (p *pager) accountWrap() {
	for p.col > p.o.width {
		p.linesPrinted++
		p.col -= p.o.width
	}
}

func flushWriter(w io.Writer) error {
	if f, ok := w.(interface{ Flush() error }); ok {
		return f.Flush()
	}
	return nil
}

func (p *pager) writeUI(s string) bool {
	if _, err := io.WriteString(p.rc.Err, s); err != nil {
		return p.fail("terminal write error: %v", err)
	}
	if err := flushWriter(p.rc.Err); err != nil {
		return p.fail("terminal flush error: %v", err)
	}
	return true
}

func (p *pager) prompt(msg string) bool {
	// Content must be visible before the prompt, and the prompt must be visible
	// before the first controlling-terminal read.
	if err := p.out.Flush(); err != nil {
		return p.fail("write error: %v", err)
	}
	if !p.writeUI("\x1b[7m" + msg + "\x1b[m") {
		return false
	}

	r, err := p.tty.readCommand(p.rc.Ctx)
	if err != nil {
		if p.canceled() {
			return false
		}
		return p.fail("terminal read error: %v", err)
	}
	switch r {
	case ' ':
		if !p.writeUI("\r\x1b[K") {
			return false
		}
		p.linesPrinted = 0
		return true
	case 'q':
		if !p.writeUI("\r\x1b[K") {
			return false
		}
		p.quit = true
		return false
	default:
		if strings.ContainsRune(recognizedCommands, rune(r)) {
			p.diagnose("%s: command not supported", commandName(rune(r)))
		} else {
			p.diagnose("unknown command: %s", commandName(rune(r)))
		}
		p.exitCode = 1
		return false
	}
}

func (p *pager) processCommand(command string) bool {
	for _, r := range command {
		switch r {
		case ' ':
			p.linesPrinted = 0
		case 'q':
			p.quit = true
			return false
		default:
			p.diagnose("%s: command not supported", commandName(r))
			p.exitCode = 1
			return false
		}
	}
	return true
}
