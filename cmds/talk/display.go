package talkcmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
	"github.com/qiangli/coreutils/cmds/internal/terminfo"
	corelocale "github.com/qiangli/coreutils/pkg/locale"
	"github.com/qiangli/coreutils/tool"
	"golang.org/x/term"
)

// display is deliberately separate from the local transport. POSIX describes
// talk as a screen-oriented utility; the transport carries editing events, and
// each endpoint owns its terminal state and its two independent screen regions.
type display interface {
	Local([]byte, bool) ([]string, bool, error)
	Remote(string) error
	PeerClosed(string) error
	Close() error
}

type wireEvent struct {
	Kind string `json:"kind"`
	Text string `json:"text,omitempty"`
}

const wirePrefix = "\x00bashy-talk-1:"

func marshalEvent(e wireEvent) (string, error) {
	b, err := json.Marshal(e)
	if err != nil {
		return "", err
	}
	return wirePrefix + string(b), nil
}

func unmarshalEvent(s string) (wireEvent, error) {
	if !strings.HasPrefix(s, wirePrefix) {
		return wireEvent{}, errors.New("peer uses an incompatible talk event protocol")
	}
	var e wireEvent
	if err := json.Unmarshal([]byte(strings.TrimPrefix(s, wirePrefix)), &e); err != nil {
		return wireEvent{}, fmt.Errorf("invalid peer event: %w", err)
	}
	switch e.Kind {
	case "text":
		if e.Text == "" {
			return wireEvent{}, errors.New("empty peer text event")
		}
		for _, r := range e.Text {
			if !unicode.IsPrint(r) && !unicode.IsSpace(r) {
				return wireEvent{}, errors.New("peer text event contains a terminal control character")
			}
		}
	case "erase", "kill", "alert":
		if e.Text != "" {
			return wireEvent{}, fmt.Errorf("peer %s event contains text", e.Kind)
		}
	default:
		return wireEvent{}, fmt.Errorf("unknown peer event %q", e.Kind)
	}
	return e, nil
}

type terminalCaps struct {
	clear string
	cup   string
	el    string
	bel   string
	rows  int
	cols  int
}

var (
	checkTerminalCapabilitiesFn = checkTerminalCapabilities
	newDisplayFn                = newScreenDisplay
)

func terminalFD(stream any) (int, error) {
	f, ok := stream.(interface{ Fd() uintptr })
	if !ok {
		return 0, errors.New("terminal stream has no file descriptor")
	}
	return int(f.Fd()), nil
}

func loadTerminalCaps(rc *tool.RunContext) (terminalCaps, error) {
	outFD, err := terminalFD(rc.Out)
	if err != nil {
		return terminalCaps{}, err
	}
	name := rc.Getenv("TERM")
	if name == "" {
		// POSIX permits an unspecified default when TERM is unset. ANSI is the
		// repository's deterministic default, and is in the built-in database.
		name = "ansi"
	}
	entry, err := terminfo.Load(rc.Getenv, name)
	if err != nil {
		return terminalCaps{}, fmt.Errorf("terminal type %q: %w", name, err)
	}
	get := func(cap string) (string, error) {
		v, ok := entry.Str(cap)
		if !ok || v == "" {
			return "", fmt.Errorf("terminal type %q lacks required %s capability", name, cap)
		}
		return terminfo.StripPadding(v), nil
	}
	clear, err := get("clear")
	if err != nil {
		return terminalCaps{}, err
	}
	cup, err := get("cup")
	if err != nil {
		return terminalCaps{}, err
	}
	el, err := get("el")
	if err != nil {
		return terminalCaps{}, err
	}
	bel, ok := entry.Str("bel")
	if !ok || bel == "" {
		bel = "\a"
	}
	cols, rows, err := term.GetSize(outFD)
	if err != nil {
		return terminalCaps{}, fmt.Errorf("determine terminal size: %w", err)
	}
	if rows < 6 || cols < 20 {
		return terminalCaps{}, fmt.Errorf("terminal is too small for two screen regions (%dx%d; need at least 20x6)", cols, rows)
	}
	return terminalCaps{clear: clear, cup: cup, el: el, bel: terminfo.StripPadding(bel), rows: rows, cols: cols}, nil
}

func checkTerminalCapabilities(rc *tool.RunContext) error {
	_, err := loadTerminalCaps(rc)
	return err
}

type screenDisplay struct {
	out          io.Writer
	caps         terminalCaps
	localLabel   string
	peerLabel    string
	local        regionBuffer
	remote       regionBuffer
	controls     controlChars
	ctype        string
	pendingUTF8  []byte
	restore      func() error
	closed       bool
	peerIsClosed bool
}

type regionBuffer struct{ text []rune }

func (b *regionBuffer) textEvent(s string) { b.text = append(b.text, []rune(s)...) }

func (b *regionBuffer) erase() {
	if n := len(b.text); n > 0 && b.text[n-1] != '\n' {
		b.text = b.text[:n-1]
	}
}

func (b *regionBuffer) kill() {
	for len(b.text) > 0 && b.text[len(b.text)-1] != '\n' {
		b.text = b.text[:len(b.text)-1]
	}
}

func newScreenDisplay(rc *tool.RunContext, self, peer string) (display, error) {
	caps, err := loadTerminalCaps(rc)
	if err != nil {
		return nil, err
	}
	inFD, err := terminalFD(rc.In)
	if err != nil {
		return nil, err
	}
	controls, restore, err := enterTerminalMode(inFD)
	if err != nil {
		return nil, fmt.Errorf("enter character-at-a-time terminal mode: %w", err)
	}
	d := &screenDisplay{
		out: rc.Out, caps: caps, localLabel: safeLabel(self), peerLabel: safeLabel(peer),
		controls: controls, ctype: corelocale.Resolve(rc.Env, corelocale.CType),
		restore: restore,
	}
	if err := d.redraw(); err != nil {
		_ = d.restore()
		return nil, err
	}
	return d, nil
}

func safeLabel(s string) string {
	label, _ := printableInput([]byte(s), "C")
	return label
}

func (d *screenDisplay) cursor(row, col int) (string, error) {
	return terminfo.Instantiate(d.caps.cup, []string{strconv.Itoa(row), strconv.Itoa(col)})
}

func (d *screenDisplay) redraw() error {
	var out strings.Builder
	out.WriteString(d.caps.clear)
	topRows := d.caps.rows/2 - 1
	bottomStart := d.caps.rows / 2
	bottomRows := d.caps.rows - bottomStart - 1
	if err := d.renderRegion(&out, 0, topRows, "talk: you ("+d.localLabel+")", &d.local); err != nil {
		return err
	}
	if err := d.renderRegion(&out, bottomStart, bottomRows, "talk: "+d.peerLabel, &d.remote); err != nil {
		return err
	}
	row, col := d.regionCursor(1, topRows, &d.local)
	pos, err := d.cursor(row, col)
	if err != nil {
		return err
	}
	out.WriteString(pos)
	_, err = io.WriteString(d.out, out.String())
	return err
}

func (d *screenDisplay) renderRegion(out *strings.Builder, start, height int, label string, b *regionBuffer) error {
	pos, err := d.cursor(start, 0)
	if err != nil {
		return err
	}
	out.WriteString(pos)
	out.WriteString(d.caps.el)
	out.WriteString(clipRunes("--- "+label+" ---", d.caps.cols))
	lines := wrappedLines(b.text, d.caps.cols)
	if len(lines) > height {
		lines = lines[len(lines)-height:]
	}
	for i := 0; i < height; i++ {
		pos, err = d.cursor(start+1+i, 0)
		if err != nil {
			return err
		}
		out.WriteString(pos)
		out.WriteString(d.caps.el)
		if i < len(lines) {
			out.WriteString(lines[i])
		}
	}
	return nil
}

func (d *screenDisplay) regionCursor(start, height int, b *regionBuffer) (int, int) {
	lines := wrappedLines(b.text, d.caps.cols)
	row := len(lines) - 1
	if row >= height {
		row = height - 1
	}
	col := runewidth.StringWidth(lines[len(lines)-1])
	if col >= d.caps.cols {
		row++
		col = 0
		if row >= height {
			row = height - 1
		}
	}
	return start + row, col
}

func wrappedLines(text []rune, width int) []string {
	lines := []string{""}
	for _, r := range text {
		if r == '\n' || r == '\r' || r == '\v' {
			lines = append(lines, "")
			continue
		}
		if r == '\t' {
			spaces := 8 - runewidth.StringWidth(lines[len(lines)-1])%8
			for range spaces {
				lines = appendWrapped(lines, ' ', width)
			}
			continue
		}
		lines = appendWrapped(lines, r, width)
	}
	return lines
}

func appendWrapped(lines []string, r rune, width int) []string {
	last := len(lines) - 1
	runeWidth := max(0, runewidth.RuneWidth(r))
	if runewidth.StringWidth(lines[last])+runeWidth > width {
		lines = append(lines, "")
		last++
	}
	lines[last] += string(r)
	return lines
}

func clipRunes(s string, width int) string {
	var out strings.Builder
	columns := 0
	for _, r := range s {
		w := max(0, runewidth.RuneWidth(r))
		if columns+w > width {
			break
		}
		out.WriteRune(r)
		columns += w
	}
	return out.String()
}

func (d *screenDisplay) Local(data []byte, accept bool) ([]string, bool, error) {
	events, terminate := d.localEvents(data)
	if !accept {
		return nil, terminate, nil
	}
	wires := make([]string, 0, len(events))
	redraw := false
	for _, e := range events {
		switch e.Kind {
		case "refresh":
			redraw = true
			continue // control-L refreshes the sender only.
		case "text":
			d.local.textEvent(e.Text)
		case "erase":
			d.local.erase()
		case "kill":
			d.local.kill()
		}
		wire, err := marshalEvent(e)
		if err != nil {
			return nil, false, err
		}
		wires = append(wires, wire)
		if e.Kind != "alert" {
			redraw = true
		}
	}
	if redraw {
		if err := d.redraw(); err != nil {
			return nil, false, err
		}
	}
	return wires, terminate, nil
}

func (d *screenDisplay) localEvents(data []byte) ([]wireEvent, bool) {
	var events []wireEvent
	var text strings.Builder
	terminate := false
	flush := func() {
		if text.Len() > 0 {
			events = append(events, wireEvent{Kind: "text", Text: text.String()})
			text.Reset()
		}
	}
input:
	for _, c := range data {
		switch c {
		case d.controls.intr, d.controls.eof:
			flush()
			terminate = true
			break input
		case d.controls.erase:
			flush()
			events = append(events, wireEvent{Kind: "erase"})
		case d.controls.kill:
			flush()
			events = append(events, wireEvent{Kind: "kill"})
		case '\a':
			flush()
			events = append(events, wireEvent{Kind: "alert"})
		case '\f':
			flush()
			events = append(events, wireEvent{Kind: "refresh"})
		default:
			text.WriteByte(c)
		}
	}
	flush()
	for i := range events {
		if events[i].Kind == "text" {
			events[i].Text = d.printable(events[i].Text)
		}
	}
	if terminate && len(d.pendingUTF8) > 0 {
		var escaped strings.Builder
		for _, c := range d.pendingUTF8 {
			fmt.Fprintf(&escaped, "\\x%02X", c)
		}
		d.pendingUTF8 = nil
		events = append(events, wireEvent{Kind: "text", Text: escaped.String()})
	}
	kept := events[:0]
	for _, event := range events {
		if event.Kind != "text" || event.Text != "" {
			kept = append(kept, event)
		}
	}
	return kept, terminate
}

func (d *screenDisplay) printable(s string) string {
	data := append(d.pendingUTF8, []byte(s)...)
	d.pendingUTF8 = nil
	if !isUTF8Locale(d.ctype) {
		body, _ := printableInput(data, d.ctype)
		return body
	}
	var out strings.Builder
	for len(data) > 0 {
		if !utf8.FullRune(data) {
			d.pendingUTF8 = append(d.pendingUTF8, data...)
			break
		}
		r, n := utf8.DecodeRune(data)
		if r == utf8.RuneError && n == 1 {
			fmt.Fprintf(&out, "\\x%02X", data[0])
			data = data[1:]
			continue
		}
		data = data[n:]
		switch {
		case unicode.IsPrint(r) || unicode.IsSpace(r):
			out.WriteRune(r)
		case r < 0x20:
			out.WriteByte('^')
			out.WriteByte(byte(r) + '@')
		case r == 0x7f:
			out.WriteString("^?")
		default:
			fmt.Fprintf(&out, "\\u{%X}", r)
		}
	}
	return out.String()
}

func (d *screenDisplay) Remote(wire string) error {
	e, err := unmarshalEvent(wire)
	if err != nil {
		return err
	}
	switch e.Kind {
	case "alert":
		_, err = io.WriteString(d.out, d.caps.bel)
		return err
	case "text":
		d.remote.textEvent(e.Text)
	case "erase":
		d.remote.erase()
	case "kill":
		d.remote.kill()
	}
	return d.redraw()
}

func (d *screenDisplay) PeerClosed(peer string) error {
	if d.peerIsClosed {
		return nil
	}
	d.peerIsClosed = true
	d.remote.textEvent("\ntalk: " + safeLabel(peer) + " has terminated the session; only local exit remains\n")
	return d.redraw()
}

func (d *screenDisplay) Close() error {
	if d.closed {
		return nil
	}
	d.closed = true
	var first error
	if d.restore != nil {
		first = d.restore()
	}
	pos, err := d.cursor(d.caps.rows-1, 0)
	if err == nil {
		_, err = io.WriteString(d.out, pos+d.caps.el)
	}
	if first != nil {
		return first
	}
	return err
}
