package morecmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
	"github.com/qiangli/coreutils/pkg/bre"
	"github.com/qiangli/coreutils/pkg/collate"
	"github.com/qiangli/coreutils/pkg/ctype"
	"github.com/qiangli/coreutils/tool"
	"golang.org/x/term"
	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

type ttyChannel struct {
	readCommand func(context.Context) (byte, error)
	editorIO    *os.File
	fd          int
	hasFd       bool
	close       func() error
}

var getTerminalSize = func(fd int) (width, height int, err error) { return term.GetSize(fd) }

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

type displayRow struct {
	data          []byte
	line, byteEnd int
}
type foldedRow struct {
	data []byte
	end  int
}

// document caches input incrementally. Backwards commands can revisit cached
// rows while the first page of a pipe is still displayed before EOF is read.
type document struct {
	name      string
	r         *bufio.Reader
	closer    io.Closer
	rows      []displayRow
	lines     []string
	eof       bool
	readErr   error
	total     int
	lastBlank bool
	seekable  bool
	width     int
	o         options
}

func newDocument(name string, r io.Reader, c io.Closer, width int, o options) *document {
	_, seekable := r.(io.Seeker)
	return &document{name: name, r: bufio.NewReader(r), closer: c, seekable: seekable, width: width, o: o}
}
func (d *document) close() error {
	if d.closer == nil {
		return nil
	}
	e := d.closer.Close()
	d.closer = nil
	return e
}
func (d *document) loadLine() {
	if d.eof {
		return
	}
	b, err := d.r.ReadBytes('\n')
	if len(b) > 0 {
		base := d.total
		d.total += len(b)
		line := len(d.lines) + 1
		d.lines = append(d.lines, string(b))
		normalized, boundaries := normalizeTerminalLineMapped(b, d.o.plain, d.o.charMode, d.o.style)
		blank := string(normalized) == "\n"
		if !(d.o.squeeze && blank && d.lastBlank) {
			for _, part := range foldLineMapped(normalized, boundaries, d.width, d.o.plain, d.o.charMode) {
				d.rows = append(d.rows, displayRow{part.data, line, base + part.end})
			}
		}
		d.lastBlank = blank
	}
	if err != nil {
		d.eof = true
		if !errors.Is(err, io.EOF) {
			d.readErr = err
		}
	}
}

func normalizeTerminalLine(line []byte, plain bool) []byte {
	normalized, _ := normalizeTerminalLineMapped(line, plain, charactersUTF8, false)
	return normalized
}

func normalizeTerminalLineMapped(line []byte, plain bool, mode characterMode, style bool) ([]byte, []int) {
	if plain {
		var out []byte
		var boundaries []int
		for i, b := range line {
			switch b {
			case '\b':
				out = append(out, '^', 'H')
				boundaries = append(boundaries, i+1, i+1)
			case '\r':
				out = append(out, '^', 'M')
				boundaries = append(boundaries, i+1, i+1)
			default:
				out = append(out, b)
				boundaries = append(boundaries, i+1)
			}
		}
		return out, boundaries
	}
	var out []byte
	var boundaries []int
	for i := 0; i < len(line); {
		if i == len(line)-2 && line[i] == '\r' && line[i+1] == '\n' {
			i++
			continue
		}
		r, size := decodeDisplayRune(line[i:], mode)
		width := displayRuneWidth(r, mode)

		// character + n backspaces + n underscores: underlined glyph.
		j := i + size
		if matchByteCount(line, j, '\b', width) && matchByteCount(line, j+width, '_', width) {
			end := j + 2*width
			appendStyled(&out, &boundaries, line[i:i+size], end, style, "\x1b[4m", "\x1b[24m")
			i = end
			continue
		}
		// n underscores + n backspaces + a width-n glyph: underlined glyph.
		if r == '_' {
			underscores := 0
			for i+underscores < len(line) && line[i+underscores] == '_' {
				underscores++
			}
			j = i + underscores
			if underscores > 0 && matchByteCount(line, j, '\b', underscores) {
				next := j + underscores
				r2, size2 := decodeDisplayRune(line[next:], mode)
				if next < len(line) && displayRuneWidth(r2, mode) == underscores {
					end := next + size2
					appendStyled(&out, &boundaries, line[next:end], end, style, "\x1b[4m", "\x1b[24m")
					i = end
					continue
				}
			}
		}
		// glyph + repeated (n backspaces + identical glyph): bold glyph.
		j = i + size
		end := j
		for matchByteCount(line, end, '\b', width) {
			next := end + width
			r2, size2 := decodeDisplayRune(line[next:], mode)
			if next >= len(line) || r2 != r {
				break
			}
			end = next + size2
		}
		if end > j {
			appendStyled(&out, &boundaries, line[i:i+size], end, style, "\x1b[1m", "\x1b[22m")
			i = end
			continue
		}
		if r == '\b' {
			if len(out) > 0 {
				outSize := 1
				if mode != charactersByte {
					_, outSize = utf8.DecodeLastRune(out)
					if outSize < 1 {
						outSize = 1
					}
				}
				out = out[:len(out)-outSize]
				boundaries = boundaries[:len(boundaries)-outSize]
			}
			i += size
			continue
		}
		appendMapped(&out, &boundaries, line[i:i+size], i+size)
		i += size
	}
	return out, boundaries
}

func decodeDisplayRune(data []byte, mode characterMode) (rune, int) {
	if len(data) == 0 {
		return utf8.RuneError, 0
	}
	if mode == charactersByte {
		return rune(data[0]), 1
	}
	return utf8.DecodeRune(data)
}

func displayRuneWidth(r rune, mode characterMode) int {
	if mode == charactersByte {
		return 1
	}
	return max(1, runewidth.RuneWidth(r))
}

func matchByteCount(line []byte, start int, want byte, count int) bool {
	if start < 0 || count < 0 || start+count > len(line) {
		return false
	}
	for _, b := range line[start : start+count] {
		if b != want {
			return false
		}
	}
	return true
}

func appendMapped(out *[]byte, boundaries *[]int, data []byte, sourceEnd int) {
	*out = append(*out, data...)
	for range data {
		*boundaries = append(*boundaries, sourceEnd)
	}
}

func appendStyled(out *[]byte, boundaries *[]int, glyph []byte, sourceEnd int, style bool, start, reset string) {
	if !style {
		appendMapped(out, boundaries, glyph, sourceEnd)
		return
	}
	appendMapped(out, boundaries, []byte(start), sourceEnd)
	appendMapped(out, boundaries, glyph, sourceEnd)
	appendMapped(out, boundaries, []byte(reset), sourceEnd)
}
func (d *document) ensure(n int) {
	for len(d.rows) < n && !d.eof {
		d.loadLine()
	}
}
func (d *document) all() {
	for !d.eof {
		d.loadLine()
	}
}

func foldLine(line []byte, width int, plain bool) [][]byte {
	boundaries := make([]int, len(line))
	for i := range boundaries {
		boundaries[i] = i + 1
	}
	parts := foldLineMapped(line, boundaries, width, plain, charactersUTF8)
	rows := make([][]byte, len(parts))
	for i := range parts {
		rows[i] = parts[i].data
	}
	return rows
}

func foldLineMapped(line []byte, boundaries []int, width int, plain bool, mode characterMode) []foldedRow {
	if width < 1 {
		width = 80
	}
	var rows []foldedRow
	start, col, pendingStyle := 0, 0, -1
	for i := 0; i < len(line); {
		var r rune
		size := 1
		styleSequence := ansiSGRLen(line[i:])
		if styleSequence > 0 {
			size = styleSequence
			r = 0
			if string(line[i:i+size]) == "\x1b[1m" || string(line[i:i+size]) == "\x1b[4m" {
				pendingStyle = i
			}
		} else if mode == charactersByte {
			r = rune(line[i])
		} else {
			r, size = utf8.DecodeRune(line[i:])
		}
		advance := runewidth.RuneWidth(r)
		if styleSequence > 0 {
			advance = 0
		}
		if advance < 0 || plain && advance == 0 && r != '\n' {
			advance = 1
		}
		switch r {
		case '\n':
			advance = 0
		case '\t':
			advance = 8 - col%8
		case '\b':
			if !plain {
				advance = 0
				if col > 0 {
					col--
				}
			}
		case '\r':
			if !plain {
				advance = 0
				col = 0
			}
		}
		if col+advance > width && i > start {
			split := i
			if pendingStyle >= start {
				split = pendingStyle
			}
			if split > start {
				rows = append(rows, foldedRow{append([]byte(nil), line[start:split]...), boundaries[split-1]})
				start, col = split, 0
				if r == '\t' {
					advance = 8
				}
			}
		}
		col += advance
		if styleSequence == 0 && advance > 0 {
			pendingStyle = -1
		}
		if r == '\n' {
			rows = append(rows, foldedRow{append([]byte(nil), line[start:i+size]...), boundaries[i+size-1]})
			start, col = i+size, 0
		}
		i += size
	}
	if start < len(line) {
		rows = append(rows, foldedRow{append([]byte(nil), line[start:]...), boundaries[len(line)-1]})
	}
	return rows
}

func ansiSGRLen(data []byte) int {
	if len(data) < 3 || data[0] != 0x1b || data[1] != '[' {
		return 0
	}
	for i := 2; i < len(data); i++ {
		if data[i] == 'm' {
			return i + 1
		}
		if (data[i] < '0' || data[i] > '9') && data[i] != ';' {
			return 0
		}
	}
	return 0
}

func flushWriter(w io.Writer) error {
	if f, ok := w.(interface{ Flush() error }); ok {
		return f.Flush()
	}
	return nil
}

type searchState struct {
	pattern         string
	forward, invert bool
}
type displayPosition struct{ row, offset int }
type pager struct {
	rc                                                            *tool.RunContext
	out                                                           *bufio.Writer
	tty                                                           *ttyChannel
	o                                                             options
	files                                                         []string
	doc                                                           *document
	fileIndex, top, next, half, outputStart, outputSpan, exitCode int
	previous                                                      displayPosition
	marks                                                         map[byte]displayPosition
	suppressCommands                                              map[int]bool
	search                                                        searchState
	previousExamined                                              string
	quit, commandFailed                                           bool
}

func (p *pager) canceled() bool              { return p.rc.Ctx != nil && p.rc.Ctx.Err() != nil }
func (p *pager) diagnose(f string, a ...any) { _, _ = fmt.Fprintf(p.rc.Err, "more: "+f+"\n", a...) }
func (p *pager) commandError(f string, a ...any) {
	p.commandFailed = true
	p.diagnose(f, a...)
}
func (p *pager) fail(f string, a ...any) bool { p.diagnose(f, a...); p.exitCode = 1; return false }
func (p *pager) writeUI(s string) bool {
	n, e := io.WriteString(p.rc.Err, s)
	if e != nil {
		return p.fail("terminal write error: %v", e)
	}
	if n != len(s) {
		return p.fail("terminal write error: %v", io.ErrShortWrite)
	}
	if e = flushWriter(p.rc.Err); e != nil {
		return p.fail("terminal flush error: %v", e)
	}
	return true
}

func (p *pager) openFile(i int) bool { return p.openFileMode(i, false) }

func (p *pager) openFileMode(i int, requireSeekable bool) bool {
	if i < 0 || i >= len(p.files) {
		return false
	}
	name := p.files[i]
	r, c, e := openInput(p.rc, name)
	if e != nil {
		p.fail("%s: %v", name, tool.SysErr(e))
		return false
	}
	if requireSeekable {
		_, seekable := r.(io.Seeker)
		if name == "-" || !seekable {
			if c != nil {
				_ = c.Close()
			}
			p.fail("%s: file is not seekable", name)
			return false
		}
	}
	if p.doc != nil && p.doc.name != name {
		p.previousExamined = p.doc.name
	}
	if p.doc != nil {
		if e := p.doc.close(); e != nil {
			p.fail("%s: close: %v", p.doc.name, tool.SysErr(e))
		}
	}
	p.doc = newDocument(name, r, c, p.o.width, p.o)
	p.fileIndex, p.top, p.next, p.outputStart, p.outputSpan = i, 0, 0, 0, 0
	p.previous = displayPosition{}
	p.marks = make(map[byte]displayPosition)
	return true
}
func (p *pager) openReachable(start, step int, affect bool) bool {
	old := p.exitCode
	for i := start; i >= 0 && i < len(p.files); i += step {
		if p.openFile(i) {
			p.runFileCommands()
			return true
		}
	}
	if !affect {
		p.exitCode = old
	}
	return false
}

func (p *pager) openFirstReachable(start int) bool {
	for i := start; i < len(p.files); i++ {
		if p.openFile(i) {
			return true
		}
	}
	return false
}

func (p *pager) runFileCommands() {
	p.commandFailed = false
	if p.o.command != "" && !p.suppressCommands[p.fileIndex] {
		// -p commands act after the ordinary first screen has logically been
		// established, even though that intermediate screen is not written.
		p.doc.ensure(p.top + p.o.screenful)
		p.next = min(len(p.doc.rows), p.top+p.o.screenful)
		p.initialCommands(p.o.command)
	}
}

func (p *pager) run() int {
	p.half = max(1, p.o.screenful/2)
	p.suppressCommands = make(map[int]bool)
	if p.o.tag != "" {
		target, line, pattern, e := resolveTag(p.rc, p.o.tag)
		if e != nil {
			return p.failCode("tag %s: %v", p.o.tag, e)
		}
		p.files = append([]string{target}, p.files...)
		if !p.openFile(0) {
			return p.exitCode
		}
		if line > 0 {
			if row, found := p.rowForLineFound(line); found {
				p.top = row
			} else {
				p.top = 0
				p.diagnose("tag %s: line %d does not exist", p.o.tag, line)
				p.suppressCommands[p.fileIndex] = true
			}
		} else if pattern != "" && !p.searchFor(pattern, true, false, 1, -1) {
			p.diagnose("tag %s: pattern not found", p.o.tag)
			p.suppressCommands[p.fileIndex] = true
		}
	} else if !p.openFirstReachable(0) {
		return p.exitCode
	}
	if p.o.fromLine > 1 {
		p.top = p.rowForLine(p.o.fromLine)
	}
	if p.o.pattern != "" {
		p.literalSearch(p.o.pattern)
	}
	p.runFileCommands()
	if p.quit {
		return p.exitCode
	}
	for !p.quit && !p.canceled() {
		// POSIX permits -c to be silently ignored when the terminal cannot
		// support clearing without scrolling. TERM=dumb is such a terminal.
		if p.o.cleanPrint && p.rc.Getenv("TERM") != "dumb" && !p.writeUI("\x1b[H\x1b[2J") {
			break
		}
		if !p.render() {
			break
		}
		atEOF := p.doc.eof && p.next >= len(p.doc.rows)
		if atEOF && p.fileIndex == len(p.files)-1 && p.o.exitOnEof {
			break
		}
		if !p.prompt(atEOF) {
			break
		}
	}
	if p.doc != nil {
		if e := p.doc.close(); e != nil {
			p.fail("%s: close: %v", p.doc.name, tool.SysErr(e))
		}
	}
	return p.exitCode
}
func (p *pager) failCode(f string, a ...any) int { p.fail(f, a...); return p.exitCode }
func (p *pager) render() bool {
	start := p.top
	span := p.o.screenful
	if p.outputSpan > 0 {
		start = p.outputStart
		span = p.outputSpan
		p.outputSpan = 0
	}
	p.doc.ensure(start + span)
	if p.doc.readErr != nil {
		return p.fail("%s: %v", p.doc.name, tool.SysErr(p.doc.readErr))
	}
	end := min(len(p.doc.rows), start+span)
	for _, r := range p.doc.rows[start:end] {
		if _, e := p.out.Write(r.data); e != nil {
			return p.fail("write error: %v", e)
		}
	}
	p.next = min(len(p.doc.rows), p.top+p.o.screenful)
	if p.doc.readErr != nil {
		return p.fail("%s: %v", p.doc.name, tool.SysErr(p.doc.readErr))
	}
	return true
}
func (p *pager) promptText(eof bool) string {
	if eof {
		if p.fileIndex+1 < len(p.files) {
			return fmt.Sprintf("--More--(%s: END; Next file: %s)", p.doc.name, p.files[p.fileIndex+1])
		}
		return fmt.Sprintf("--More--(%s: END)", p.doc.name)
	}
	if p.doc.name == "-" {
		return "--More--"
	}
	return fmt.Sprintf("--More--(%s)", p.doc.name)
}
func (p *pager) prompt(eof bool) bool {
	c, ok := p.readPrompt(eof)
	if !ok {
		return false
	}
	return p.execute(c, eof)
}

func (p *pager) readPrompt(eof bool) (moreCommand, bool) {
	if e := p.out.Flush(); e != nil {
		p.fail("write error: %v", e)
		return moreCommand{}, false
	}
	dumb := p.rc.Getenv("TERM") == "dumb"
	prompt := "\x1b[7m" + p.promptText(eof) + "\x1b[m"
	if dumb {
		prompt = p.promptText(eof)
	}
	if !p.writeUI(prompt) {
		return moreCommand{}, false
	}
	in := ttyInput{p}
	c, e := readCommand(&in)
	if e != nil {
		if p.canceled() {
			return moreCommand{}, false
		}
		p.fail("terminal read error: %v", e)
		return moreCommand{}, false
	}
	clear := "\r\x1b[K"
	if dumb {
		clear = "\r"
	}
	if !p.writeUI(clear) {
		return moreCommand{}, false
	}
	return c, true
}

type moreCommand struct {
	count    int
	counted  bool
	key, sub byte
	arg      string
}
type byteInput interface{ readByte() (byte, error) }
type ttyInput struct{ p *pager }

func (i *ttyInput) readByte() (byte, error) { return i.p.tty.readCommand(i.p.rc.Ctx) }

type stringInput struct{ *strings.Reader }

func (i *stringInput) readByte() (byte, error) { return i.ReadByte() }
func readLine(in byteInput) (string, error) {
	var b strings.Builder
	for {
		c, e := in.readByte()
		if e != nil {
			if errors.Is(e, io.EOF) {
				return b.String(), nil
			}
			return "", e
		}
		if c == '\n' || c == '\r' {
			return b.String(), nil
		}
		b.WriteByte(c)
	}
}
func readCommand(in byteInput) (moreCommand, error) {
	var c moreCommand
	b, e := in.readByte()
	if e != nil {
		return c, e
	}
	for b >= '0' && b <= '9' {
		c.counted = true
		c.count = c.count*10 + int(b-'0')
		b, e = in.readByte()
		if e != nil {
			return c, e
		}
	}
	c.key = b
	switch b {
	case '/', '?':
		c.arg, e = readLine(in)
	case 'm', '\'':
		c.sub, e = in.readByte()
	case ':':
		c.sub, e = in.readByte()
		if e == nil && (c.sub == 'e' || c.sub == 't') {
			c.arg, e = readLine(in)
		}
	case 'Z':
		c.sub, e = in.readByte()
	}
	return c, e
}
func (p *pager) initialCommands(s string) {
	if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil && n > 0 {
		// The decimal -p shorthand places the requested file line at the
		// standard current position, normally the third display line.
		if row, found := p.rowForLineFound(n); found {
			p.moveLarge(max(0, row-2))
		} else {
			p.moveTo(len(p.doc.rows), p.o.screenful)
		}
		return
	}
	in := &stringInput{strings.NewReader(s)}
	for in.Len() > 0 && !p.quit {
		c, e := readCommand(in)
		if e != nil {
			p.diagnose("-p command: %v", e)
			return
		}
		if !p.execute(c, false) {
			return
		}
		if p.commandFailed {
			return
		}
	}
}
func count(c moreCommand, d int) int {
	if c.counted {
		return max(1, c.count)
	}
	return d
}
func (p *pager) moveLarge(n int) { p.top = max(0, n) }

func (p *pager) currentPosition() displayPosition {
	p.doc.ensure(p.top + p.o.screenful)
	end := min(len(p.doc.rows), p.top+p.o.screenful)
	if end <= p.top {
		return displayPosition{row: p.top}
	}
	offset := min(2, end-p.top-1)
	if p.top == 0 {
		offset = 0
	} else if p.doc.eof && end == len(p.doc.rows) {
		offset = end - p.top - 1
	}
	return displayPosition{row: p.top + offset, offset: offset}
}

func (p *pager) restorePosition(pos displayPosition) { p.top = max(0, pos.row-pos.offset) }

func (p *pager) moveForward(n int) {
	p.moveTo(p.top+max(0, n), p.o.screenful)
}

func (p *pager) moveTo(target, span int) bool {
	p.doc.all()
	if target < 0 || target >= len(p.doc.rows) {
		p.commandFailed = true
		_ = p.writeUI("\a")
		if p.doc.name == "-" {
			p.top = max(0, len(p.doc.rows)-max(1, span))
		}
		return false
	}
	p.top = target
	return true
}

func (p *pager) scrollForwardLines(step int) bool {
	p.doc.all()
	if p.top+step >= len(p.doc.rows) || p.next+step > len(p.doc.rows) {
		p.commandFailed = true
		_ = p.writeUI("\a")
		if p.doc.name == "-" && p.next < len(p.doc.rows) {
			p.top, p.outputStart = p.next, p.next
			p.outputSpan = len(p.doc.rows) - p.next
		}
		return false
	}
	p.outputStart, p.outputSpan = p.next, step
	p.top += step
	return true
}

func (p *pager) execute(c moreCommand, eof bool) bool {
	oldDoc, oldTop := p.doc, p.top
	oldPosition := p.currentPosition()
	ok := p.executeCommand(c, eof)
	if p.doc == oldDoc && abs(p.top-oldTop) > p.o.screenful {
		p.previous = oldPosition
	}
	return ok
}

func (p *pager) executeCommand(c moreCommand, eof bool) bool {
	if eof {
		advance := advancesAtEOF(c)
		if p.fileIndex == len(p.files)-1 {
			p.quit = true
			return false
		}
		if advance {
			return p.openReachable(p.fileIndex+1, 1, true)
		}
	}
	n := count(c, 1)
	switch c.key {
	case 'q':
		p.quit = true
		return false
	case ':':
		if c.sub == 'q' {
			p.quit = true
			return false
		}
		return p.colon(c)
	case 'Z':
		if c.sub == 'Z' {
			p.quit = true
			return false
		}
	case ' ', 'j', '\n', '\r':
		step := n
		if !c.counted && c.key == ' ' {
			step = p.o.screenful
		}
		p.scrollForwardLines(step)
		return true
	case 'f', 0x06:
		step := n
		if !c.counted {
			step = p.o.screenful
		}
		p.moveForward(step)
		return true
	case 'b', 0x02:
		step := n
		if !c.counted {
			step = p.o.screenful
		}
		p.moveTo(p.top-step, p.o.screenful)
		return true
	case 'k':
		if p.moveTo(p.top-n, n) {
			p.outputStart, p.outputSpan = p.top, n
		}
		return true
	case 'd', 0x04:
		if c.counted {
			p.half = n
		}
		p.moveForward(p.half)
		return true
	case 'u', 0x15:
		if c.counted {
			p.half = n
		}
		if p.moveTo(p.top-p.half, p.half) {
			p.outputStart, p.outputSpan = p.top, p.half
		}
		return true
	case 's':
		p.doc.all()
		p.top = min(max(0, len(p.doc.rows)-p.o.screenful), p.next+n-1)
		return true
	case 'g':
		if row, found := p.rowForLineFound(n); found {
			p.moveLarge(row)
		} else {
			p.moveTo(len(p.doc.rows), p.o.screenful)
		}
		return true
	case 'G':
		if c.counted {
			if row, found := p.rowForLineFound(n); found {
				p.moveLarge(row)
			} else {
				p.moveTo(len(p.doc.rows), p.o.screenful)
			}
		} else {
			p.doc.all()
			p.moveLarge(max(0, len(p.doc.rows)-p.o.screenful))
		}
		return true
	case 'r', 0x0c:
		return true
	case 'R':
		if !p.doc.seekable {
			return true
		}
		line := 1
		if p.top < len(p.doc.rows) {
			line = p.doc.rows[p.top].line
		}
		index := p.fileIndex
		if p.openFile(index) {
			p.top = p.rowForLine(line)
			p.runFileCommands()
		}
		return true
	case 'm':
		if c.sub < 'a' || c.sub > 'z' {
			p.commandError("invalid mark")
		} else {
			p.marks[c.sub] = p.currentPosition()
		}
		return true
	case '\'':
		if c.sub == '\'' {
			p.restorePosition(p.previous)
		} else if x, ok := p.marks[c.sub]; ok {
			p.restorePosition(x)
		} else {
			p.commandError("mark %c is not set", c.sub)
		}
		return true
	case '/', '?':
		forward := c.key == '/'
		invert := strings.HasPrefix(c.arg, "!")
		pat := strings.TrimPrefix(c.arg, "!")
		if pat == "" {
			pat = p.search.pattern
			invert = p.search.invert
		}
		if pat == "" || !p.searchFor(pat, forward, invert, n, p.top) {
			p.commandError("pattern not found")
		}
		return true
	case 'n', 'N':
		if p.search.pattern == "" {
			p.commandError("no previous search")
			return true
		}
		forward := p.search.forward
		if c.key == 'N' {
			forward = !forward
		}
		if !p.searchFor(p.search.pattern, forward, p.search.invert, n, p.top) {
			p.commandError("pattern not found")
		}
		return true
	case 'h':
		return p.help()
	case 'v':
		return p.editor()
	case '=', 0x07:
		return p.position()
	}
	p.commandError("unknown command: %s", commandName(rune(c.key)))
	return true
}

func advancesAtEOF(c moreCommand) bool {
	return c.key == 'f' || c.key == 0x06 || c.key == ' ' || c.key == 'j' ||
		c.key == '\n' || c.key == '\r' || c.key == 'd' || c.key == 0x04 || c.key == 's'
}

func (p *pager) colon(c moreCommand) bool {
	switch c.sub {
	case 'n':
		return p.navigate(p.fileIndex+count(c, 1), 1)
	case 'p':
		return p.navigate(p.fileIndex-count(c, 1), -1)
	case 'e':
		name := strings.TrimSpace(c.arg)
		if name == "" {
			name = p.doc.name
		} else if name == "#" && p.previousExamined != "" {
			name = p.previousExamined
		} else {
			x, e := expandFilename(p.rc, name)
			if e != nil {
				p.commandError("%v", e)
				return true
			}
			name = x
		}
		old := p.exitCode
		p.files = append(p.files, name)
		if !p.openFileMode(len(p.files)-1, true) {
			p.files = p.files[:len(p.files)-1]
			p.exitCode = old
			p.commandFailed = true
		} else {
			p.runFileCommands()
		}
		return true
	case 't':
		target, line, pat, e := resolveTag(p.rc, strings.TrimSpace(c.arg))
		if e != nil {
			p.commandError("tag: %v", e)
			return true
		}
		opened := true
		if p.doc.name != target && p.rc.Path(p.doc.name) != p.rc.Path(target) {
			p.files = append(p.files, target)
			opened = p.openFileMode(len(p.files)-1, true)
			if !opened {
				p.files = p.files[:len(p.files)-1]
			}
		}
		if opened {
			if line > 0 {
				if row, found := p.rowForLineFound(line); found {
					p.top = row
				} else {
					p.top = 0
					p.commandError("tag: line %d does not exist", line)
				}
			} else if pat != "" && !p.searchFor(pat, true, false, 1, -1) {
				p.commandError("tag pattern not found")
			}
			p.runFileCommands()
		}
		return true
	}
	p.commandError("unknown command: :%c", c.sub)
	return true
}

func (p *pager) navigate(start, step int) bool {
	if start < 0 || start >= len(p.files) {
		p.commandError("no file in requested direction")
		return true
	}
	if !p.openReachable(start, step, true) {
		p.commandError("no accessible file in requested direction")
	}
	return true
}
func (p *pager) rowForLine(n int) int {
	p.doc.all()
	previous := 0
	for i, r := range p.doc.rows {
		if r.line >= n {
			if r.line > n {
				return previous
			}
			return i
		}
		previous = i
	}
	return max(0, len(p.doc.rows)-p.o.screenful)
}

func (p *pager) rowForLineFound(n int) (int, bool) {
	p.doc.all()
	if n < 1 || n > len(p.doc.lines) {
		return 0, false
	}
	return p.rowForLine(n), true
}
func (p *pager) literalSearch(s string) bool {
	p.doc.all()
	for i, r := range p.doc.rows {
		if strings.Contains(string(r.data), s) {
			p.top = i
			return true
		}
	}
	p.diagnose("Pattern not found")
	return false
}
func (p *pager) searchFor(pattern string, forward, invert bool, n, from int) bool {
	p.doc.all()
	match, e := compileMoreMatcher(p.o, pattern)
	if e != nil {
		p.diagnose("invalid pattern: %v", e)
		return false
	}
	start := 0
	if from >= 0 && from < len(p.doc.rows) {
		start = p.doc.rows[from].line
	}
	seen := 0
	if forward {
		for i, line := range p.doc.lines {
			matched, err := match(strings.TrimRight(line, "\r\n"))
			if err != nil {
				p.diagnose("pattern match: %v", err)
				return false
			}
			if i+1 <= start || matched == invert {
				continue
			}
			seen++
			if seen == n {
				p.top = p.rowForLine(i + 1)
				p.search = searchState{pattern, true, invert}
				return true
			}
		}
	} else {
		if start == 0 {
			start = len(p.doc.lines) + 1
		}
		for i := min(len(p.doc.lines), start-1) - 1; i >= 0; i-- {
			matched, err := match(strings.TrimRight(p.doc.lines[i], "\r\n"))
			if err != nil {
				p.diagnose("pattern match: %v", err)
				return false
			}
			if matched == invert {
				continue
			}
			seen++
			if seen == n {
				p.top = p.rowForLine(i + 1)
				p.search = searchState{pattern, false, invert}
				return true
			}
		}
	}
	return false
}

func compileMoreMatcher(o options, pattern string) (func(string) (bool, error), error) {
	if o.charMode == charactersUTF8 {
		if strings.Contains(pattern, "[") && o.ctypeName != "C" && o.ctypeName != "POSIX" {
			return nil, fmt.Errorf("locale-sensitive UTF-8 bracket expressions are unavailable for %s", o.ctypeName)
		}
		flags := ""
		if o.ignoreCase {
			flags = "(?i)"
		}
		re, err := bre.CompileWithFlags(pattern, flags)
		if err != nil {
			return nil, err
		}
		return func(s string) (bool, error) { return re.MatchString(s), nil }, nil
	}

	var ctypeProvider *ctype.Provider
	var byteProvider bre.ByteCtype
	var err error
	if o.ctypeName != "C" && o.ctypeName != "POSIX" {
		ctypeProvider, err = ctype.Open(o.ctypeName)
		if err != nil {
			return nil, err
		}
		defer ctypeProvider.Close()
		byteProvider = ctypeProvider
	}
	tables, err := bre.SnapshotLocaleByteCtypeTables(byteProvider)
	if err != nil {
		return nil, err
	}
	if o.collateName != "C" && o.collateName != "POSIX" {
		provider, openErr := collate.Open(o.collateName)
		if openErr != nil {
			return nil, openErr
		}
		tables, err = tables.WithCollation(provider)
		closeErr := provider.Close()
		if err == nil {
			err = closeErr
		}
		if err != nil {
			return nil, err
		}
	}
	re, err := bre.CompileLocaleByteRegexpTables([]byte(pattern), tables, bre.ByteRegexpOptions{Syntax: bre.ByteRegexpBRE, FoldCase: o.ignoreCase})
	if err != nil {
		return nil, err
	}
	return re.MatchString, nil
}
func (p *pager) help() bool {
	const summary = "more commands\n" +
		"h                     display this help\n" +
		"f ^F b ^B space j k   move by screens or lines\n" +
		"d ^D u ^U s           half-screen movement and skip\n" +
		"g G r ^L R            go to line/end and refresh\n" +
		"mletter 'letter ''    mark and restore positions\n" +
		"/BRE ?BRE n N         search and repeat\n" +
		":e :n :p :t           examine files and tags\n" +
		"v = ^G                editor and position\n" +
		"q :q ZZ               quit\n"
	hp := *p
	hp.files = []string{"help"}
	hp.fileIndex, hp.top, hp.next = 0, 0, 0
	hp.doc = newDocument("help", strings.NewReader(summary), nil, p.o.width, p.o)
	hp.marks = make(map[byte]displayPosition)
	hp.suppressCommands = make(map[int]bool)
	hp.o.command = ""
	for !hp.quit {
		if !hp.render() {
			p.exitCode = max(p.exitCode, hp.exitCode)
			return false
		}
		eof := hp.doc.eof && hp.next >= len(hp.doc.rows)
		c, ok := hp.readPrompt(eof)
		if !ok {
			p.exitCode = max(p.exitCode, hp.exitCode)
			return false
		}
		if eof && advancesAtEOF(c) {
			return true
		}
		if !hp.execute(c, false) {
			p.quit = hp.quit
			return !p.quit
		}
	}
	return false
}
func (p *pager) position() bool {
	p.doc.all()
	line, b := 0, 0
	if p.next > 0 && p.next <= len(p.doc.rows) {
		line, b = p.doc.rows[p.next-1].line, p.doc.rows[p.next-1].byteEnd
	}
	pct := 100
	if p.doc.total > 0 {
		pct = b * 100 / p.doc.total
	}
	return p.writeUI(fmt.Sprintf("%s %d/%d line %d byte %d/%d %d%%\n", p.doc.name, p.fileIndex+1, len(p.files), line, b, p.doc.total, pct))
}

var runEditor = func(ctx context.Context, rc *tool.RunContext, tty *ttyChannel, editor string, line int, name string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	args := []string{name}
	base := filepath.Base(editor)
	if base == "vi" || base == "ex" {
		args = []string{"-c", strconv.Itoa(line), name}
	}
	c := exec.CommandContext(ctx, editor, args...)
	c.Dir, c.Env = rc.Dir, rc.Env
	c.Stdin, c.Stdout, c.Stderr = rc.In, rc.Out, rc.Err
	if tty != nil && tty.editorIO != nil {
		c.Stdin, c.Stdout, c.Stderr = tty.editorIO, tty.editorIO, tty.editorIO
	}
	return c.Run()
}

func (p *pager) editor() bool {
	if p.doc.name == "-" {
		p.commandError("cannot edit standard input")
		return true
	}
	ed := p.rc.Getenv("EDITOR")
	if ed == "" {
		ed = "vi"
	}
	resolved := p.rc.ResolveCommand(ed)
	if resolved == "" {
		p.commandError("editor: %s: not found", ed)
		return true
	}
	line := 1
	if p.top < len(p.doc.rows) {
		line = p.doc.rows[p.top].line
	}
	if e := runEditor(p.rc.Ctx, p.rc, p.tty, resolved, line, p.rc.Path(p.doc.name)); e != nil {
		p.commandError("editor: %v", e)
	}
	return true
}

func expandFilename(rc *tool.RunContext, raw string) (string, error) {
	var words []*syntax.Word
	parser := syntax.NewParser(syntax.Variant(syntax.LangPOSIX))
	if err := parser.Words(strings.NewReader(raw), func(word *syntax.Word) bool {
		words = append(words, word)
		return true
	}); err != nil {
		return "", fmt.Errorf(":e expansion: %w", err)
	}
	env := append(append([]string(nil), rc.Env...), "PWD="+rc.Dir)
	fields, err := expand.Fields(&expand.Config{
		Env: expand.ListEnviron(env...),
		ReadDir2: func(path string) ([]os.DirEntry, error) {
			return os.ReadDir(rc.Path(path))
		},
		Lang: syntax.LangPOSIX,
		CmdSubst: func(w io.Writer, cs *syntax.CmdSubst) error {
			ctx := rc.Ctx
			if ctx == nil {
				ctx = context.Background()
			}
			runner, err := interp.New(
				interp.Dir(rc.Dir),
				interp.Env(expand.ListEnviron(env...)),
				interp.StdIO(strings.NewReader(""), w, rc.Err),
				interp.ExecHandler(func(context.Context, []string) error {
					return fmt.Errorf("external utilities are not permitted in :e expansion")
				}),
			)
			if err != nil {
				return err
			}
			return runner.Run(ctx, &syntax.File{Stmts: cs.Stmts})
		},
	}, words...)
	if err != nil {
		return "", fmt.Errorf(":e expansion: %w", err)
	}
	if len(fields) != 1 {
		return "", fmt.Errorf(":e expansion produced multiple pathnames")
	}
	return fields[0], nil
}
func resolveTag(rc *tool.RunContext, tag string) (string, int, string, error) {
	data, e := os.ReadFile(rc.Path("tags"))
	if e != nil {
		return "", 0, "", e
	}
	for _, raw := range strings.Split(string(data), "\n") {
		if raw == "" || strings.HasPrefix(raw, "!_TAG_") {
			continue
		}
		f := strings.SplitN(raw, "\t", 3)
		if len(f) < 3 || f[0] != tag {
			continue
		}
		ex := strings.TrimSuffix(f[2], `;"`)
		if n, e := strconv.Atoi(ex); e == nil && n > 0 {
			return f[1], n, "", nil
		}
		if len(ex) >= 2 && (ex[0] == '/' || ex[0] == '?') && ex[len(ex)-1] == ex[0] {
			pat := ex[1 : len(ex)-1]
			pat = strings.ReplaceAll(pat, `\`+string(ex[0]), string(ex[0]))
			return f[1], 0, pat, nil
		}
		return "", 0, "", fmt.Errorf("unsupported tag address %q", ex)
	}
	return "", 0, "", fmt.Errorf("not found")
}
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
	if unicode.IsPrint(r) {
		return string(r)
	}
	return fmt.Sprintf("U+%04X", r)
}
func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
