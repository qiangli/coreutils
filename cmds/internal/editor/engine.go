package editor

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/qiangli/coreutils/pkg/bre"
)

// Files isolates the editor engine from a particular filesystem or working
// directory. Names are resolved by the front end before these functions run.
type Files struct {
	Read  func(name string) ([]byte, error)
	Write func(name string, data []byte) error
}

// Engine interprets the POSIX ed command language over a reusable Buffer.
type Engine struct {
	Buffer   Buffer
	Out      io.Writer
	Files    Files
	Silent   bool
	Prompt   string
	Filename string

	lastRE          string
	lastReplacement string
	lastDiagnostic  string
	help            bool
	hadError        bool
	quitArmed       bool
	reader          *bufio.Reader
}

// Load replaces the buffer from name and remembers it as the default file.
// It returns the input byte count, which the adapter can report for an initial
// file operand using the same rule as the e command.
func (e *Engine) Load(name string) (int, error) {
	data, err := e.Files.Read(name)
	if err != nil {
		return 0, err
	}
	e.Buffer.Reset(splitText(data), false)
	e.Filename = name
	e.quitArmed = false
	return len(data), nil
}

// Run consumes commands until q/Q or EOF. It returns non-zero if any command
// diagnostic occurred; invalid commands do not abort the remaining script.
func (e *Engine) Run(in io.Reader) int {
	e.reader = bufio.NewReader(in)
	for {
		if e.Prompt != "" {
			fmt.Fprint(e.Out, e.Prompt)
		}
		line, err := e.reader.ReadString('\n')
		if err != nil && len(line) == 0 {
			if errors.Is(err, io.EOF) {
				if e.Buffer.Dirty {
					e.commandError("warning: buffer modified")
				}
				break
			}
			e.commandError(err.Error())
			break
		}
		line = strings.TrimSuffix(line, "\n")
		line = strings.TrimSuffix(line, "\r")
		quit, cmdErr := e.execute(line)
		if cmdErr != nil {
			e.commandError(cmdErr.Error())
		}
		if quit {
			break
		}
		if err != nil {
			break
		}
	}
	if e.hadError {
		return 1
	}
	return 0
}

func (e *Engine) commandError(detail string) {
	e.hadError = true
	e.lastDiagnostic = detail
	fmt.Fprintln(e.Out, "?")
	if e.help && detail != "" {
		fmt.Fprintln(e.Out, detail)
	}
}

func (e *Engine) execute(line string) (bool, error) {
	addrs, pos, explicit, err := e.parseAddresses(line)
	if err != nil {
		return false, err
	}
	for pos < len(line) && (line[pos] == ' ' || line[pos] == '\t') {
		pos++
	}
	cmd := byte('\n')
	if pos < len(line) {
		cmd = line[pos]
		pos++
	}
	arg := line[pos:]

	switch cmd {
	case '\n':
		if explicit {
			return false, e.printRange(addrs, 1, 'p')
		}
		return false, e.printRange([]int{e.Buffer.Current + 1}, 1, 'p')
	case 'a', 'i', 'c':
		if strings.TrimSpace(arg) != "" {
			return false, fmt.Errorf("unexpected command suffix")
		}
		text, err := e.readInput()
		if err != nil {
			return false, err
		}
		switch cmd {
		case 'a':
			a, err := e.oneAddress(addrs, explicit, e.Buffer.Current, true)
			if err != nil {
				return false, err
			}
			return false, e.Buffer.Append(a, text)
		case 'i':
			a, err := e.oneAddress(addrs, explicit, e.Buffer.Current, true)
			if err != nil {
				return false, err
			}
			if a == 0 {
				a = 1
			}
			return false, e.Buffer.Append(a-1, text)
		default:
			first, last, err := e.twoAddresses(addrs, explicit, e.Buffer.Current, e.Buffer.Current, true)
			if err != nil {
				return false, err
			}
			return false, e.Buffer.Change(first, last, text)
		}
	case 'd':
		first, last, err := e.twoAddresses(addrs, explicit, e.Buffer.Current, e.Buffer.Current, false)
		if err != nil {
			return false, err
		}
		if strings.TrimSpace(arg) != "" {
			return false, fmt.Errorf("unexpected command suffix")
		}
		return false, e.Buffer.Delete(first, last)
	case 'p', 'n', 'l':
		if strings.TrimSpace(arg) != "" {
			return false, fmt.Errorf("unexpected command suffix")
		}
		return false, e.printRange(addrs, 2, cmd)
	case '=':
		if strings.TrimSpace(arg) != "" {
			return false, fmt.Errorf("unexpected command suffix")
		}
		a, err := e.oneAddress(addrs, explicit, e.Buffer.Last(), true)
		if err != nil {
			return false, err
		}
		fmt.Fprintln(e.Out, a)
		return false, nil
	case 's':
		first, last, err := e.twoAddresses(addrs, explicit, e.Buffer.Current, e.Buffer.Current, false)
		if err != nil {
			return false, err
		}
		return false, e.substitute(first, last, arg)
	case 'w':
		return false, e.write(addrs, explicit, arg)
	case 'e', 'E':
		if explicit {
			return false, fmt.Errorf("unexpected address")
		}
		if cmd == 'e' && e.Buffer.Dirty {
			return false, fmt.Errorf("warning: buffer modified")
		}
		name := strings.TrimSpace(arg)
		if name == "" {
			name = e.Filename
		}
		if name == "" {
			return false, fmt.Errorf("no current filename")
		}
		n, err := e.Load(name)
		if err != nil {
			return false, err
		}
		if !e.Silent {
			fmt.Fprintln(e.Out, n)
		}
		return false, nil
	case 'q', 'Q':
		if explicit || strings.TrimSpace(arg) != "" {
			return false, fmt.Errorf("unexpected address or argument")
		}
		if cmd == 'q' && e.Buffer.Dirty && !e.quitArmed {
			e.quitArmed = true
			return false, fmt.Errorf("warning: buffer modified")
		}
		return true, nil
	case 'f':
		if explicit {
			return false, fmt.Errorf("unexpected address")
		}
		name := strings.TrimSpace(arg)
		if name != "" {
			e.Filename = name
		}
		if e.Filename == "" {
			return false, fmt.Errorf("no current filename")
		}
		fmt.Fprintln(e.Out, e.Filename)
		return false, nil
	case 'h':
		if explicit || strings.TrimSpace(arg) != "" {
			return false, fmt.Errorf("unexpected address or argument")
		}
		if e.lastDiagnostic == "" {
			return false, fmt.Errorf("no previous error")
		}
		fmt.Fprintln(e.Out, e.lastDiagnostic)
		return false, nil
	case 'H':
		if explicit || strings.TrimSpace(arg) != "" {
			return false, fmt.Errorf("unexpected address or argument")
		}
		e.help = !e.help
		if e.help && e.lastDiagnostic != "" {
			fmt.Fprintln(e.Out, e.lastDiagnostic)
		}
		return false, nil
	default:
		return false, fmt.Errorf("unknown command")
	}
}

func (e *Engine) readInput() ([]string, error) {
	var lines []string
	for {
		line, err := e.reader.ReadString('\n')
		if err != nil && len(line) == 0 {
			return nil, fmt.Errorf("unexpected end of input")
		}
		line = strings.TrimSuffix(line, "\n")
		line = strings.TrimSuffix(line, "\r")
		if line == "." {
			return lines, nil
		}
		lines = append(lines, line)
		if err != nil {
			return nil, fmt.Errorf("unexpected end of input")
		}
	}
}

func (e *Engine) oneAddress(addrs []int, explicit bool, def int, allowZero bool) (int, error) {
	a := def
	if explicit {
		a = addrs[len(addrs)-1]
	}
	if a < 0 || a > e.Buffer.Last() || (!allowZero && a == 0) {
		return 0, fmt.Errorf("invalid address")
	}
	return a, nil
}

func (e *Engine) twoAddresses(addrs []int, explicit bool, defFirst, defLast int, allowZero bool) (int, int, error) {
	first, last := defFirst, defLast
	if explicit {
		last = addrs[len(addrs)-1]
		first = last
		if len(addrs) > 1 {
			first = addrs[len(addrs)-2]
		}
	}
	if first > last || first < 0 || last > e.Buffer.Last() || (!allowZero && first == 0) {
		return 0, 0, fmt.Errorf("invalid address")
	}
	return first, last, nil
}

func (e *Engine) printRange(addrs []int, max int, style byte) error {
	first, last, err := e.twoAddresses(addrs, len(addrs) > 0, e.Buffer.Current, e.Buffer.Current, false)
	if max == 1 {
		last, err = e.oneAddress(addrs, len(addrs) > 0, e.Buffer.Current, false)
		first = last
	}
	if err != nil {
		return err
	}
	for n := first; n <= last; n++ {
		line := e.Buffer.Lines[n-1]
		switch style {
		case 'n':
			fmt.Fprintf(e.Out, "%d\t%s\n", n, line)
		case 'l':
			fmt.Fprintln(e.Out, listLine(line))
		default:
			fmt.Fprintln(e.Out, line)
		}
	}
	e.Buffer.Current = last
	return nil
}

func listLine(s string) string {
	var b strings.Builder
	for len(s) > 0 {
		r, size := utf8.DecodeRuneInString(s)
		if r == utf8.RuneError && size == 1 {
			fmt.Fprintf(&b, "\\%03o", s[0])
			s = s[1:]
			continue
		}
		s = s[size:]
		switch r {
		case '\\':
			b.WriteString("\\\\")
		case '\t':
			b.WriteString("\\t")
		case '\b':
			b.WriteString("\\b")
		case '\r':
			b.WriteString("\\r")
		case '\f':
			b.WriteString("\\f")
		default:
			if unicode.IsPrint(r) {
				b.WriteRune(r)
			} else {
				fmt.Fprintf(&b, "\\%03o", r)
			}
		}
	}
	b.WriteByte('$')
	return b.String()
}

func (e *Engine) write(addrs []int, explicit bool, arg string) error {
	name := strings.TrimSpace(arg)
	if name == "" {
		name = e.Filename
	}
	if name == "" {
		return fmt.Errorf("no current filename")
	}
	first, last := 1, e.Buffer.Last()
	if explicit {
		var err error
		first, last, err = e.twoAddresses(addrs, true, first, last, true)
		if err != nil {
			return err
		}
	}
	var lines []string
	if last >= first && last > 0 {
		lines = e.Buffer.Lines[first-1 : last]
	}
	data := joinLines(lines)
	if err := e.Files.Write(name, data); err != nil {
		return err
	}
	e.Filename = name
	if first == 1 && last == e.Buffer.Last() {
		e.Buffer.Dirty = false
	}
	e.quitArmed = false
	if !e.Silent {
		fmt.Fprintln(e.Out, len(data))
	}
	return nil
}

func (e *Engine) substitute(first, last int, arg string) error {
	if arg == "" {
		return fmt.Errorf("missing substitute expression")
	}
	delim := arg[0]
	if delim == ' ' || delim == '\t' {
		return fmt.Errorf("invalid substitute delimiter")
	}
	pattern, rest, closed, err := delimited(arg[1:], delim)
	if err != nil || !closed {
		return fmt.Errorf("unterminated regular expression")
	}
	replacement, flags, replacementClosed, err := delimited(rest, delim)
	if err != nil {
		return err
	}
	if pattern == "" {
		pattern = e.lastRE
	}
	if pattern == "" {
		return fmt.Errorf("no previous regular expression")
	}
	if replacement == "%" {
		if e.lastReplacement == "" {
			return fmt.Errorf("no previous replacement")
		}
		replacement = e.lastReplacement
	}
	re, err := bre.Compile(pattern)
	if err != nil {
		return err
	}
	re.Longest()
	global, count, style, err := parseSubFlags(flags)
	if err != nil {
		return err
	}
	if !replacementClosed {
		style = 'p'
	}
	e.lastRE, e.lastReplacement = pattern, replacement
	changed, lastChanged := false, 0
	for n := first; n <= last; n++ {
		line := e.Buffer.Lines[n-1]
		matches := re.FindAllStringSubmatchIndex(line, -1)
		if len(matches) == 0 {
			continue
		}
		selected := make([]bool, len(matches))
		if count > 0 {
			if count > len(matches) {
				continue
			}
			selected[count-1] = true
		} else if global {
			for i := range selected {
				selected[i] = true
			}
		} else {
			selected[0] = true
		}
		var b strings.Builder
		at := 0
		for i, m := range matches {
			if !selected[i] {
				continue
			}
			b.WriteString(line[at:m[0]])
			b.WriteString(expandReplacement(replacement, line, m))
			at = m[1]
		}
		b.WriteString(line[at:])
		e.Buffer.Lines[n-1] = b.String()
		changed, lastChanged = true, n
	}
	if !changed {
		return fmt.Errorf("no match")
	}
	e.Buffer.Current = lastChanged
	e.Buffer.Dirty = true
	e.quitArmed = false
	if style != 0 {
		return e.printRange([]int{lastChanged}, 1, style)
	}
	return nil
}

func parseSubFlags(s string) (global bool, count int, style byte, err error) {
	for i := 0; i < len(s); {
		switch s[i] {
		case 'g':
			global = true
			i++
		case 'p', 'n', 'l':
			style = s[i]
			i++
		case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
			j := i + 1
			for j < len(s) && s[j] >= '0' && s[j] <= '9' {
				j++
			}
			count, err = strconv.Atoi(s[i:j])
			if err != nil || count == 0 {
				return false, 0, 0, fmt.Errorf("invalid substitution count")
			}
			i = j
		case ' ', '\t':
			i++
		default:
			return false, 0, 0, fmt.Errorf("invalid substitution flag")
		}
	}
	return
}

func expandReplacement(repl, src string, match []int) string {
	var b strings.Builder
	for i := 0; i < len(repl); i++ {
		if repl[i] == '&' {
			b.WriteString(src[match[0]:match[1]])
			continue
		}
		if repl[i] == '\\' && i+1 < len(repl) {
			i++
			if repl[i] >= '1' && repl[i] <= '9' {
				n := int(repl[i]-'0') * 2
				if n+1 < len(match) && match[n] >= 0 {
					b.WriteString(src[match[n]:match[n+1]])
				}
			} else {
				b.WriteByte(repl[i])
			}
			continue
		}
		b.WriteByte(repl[i])
	}
	return b.String()
}

// delimited returns text through the next unescaped delimiter. Escaped
// delimiters are unescaped; other backslashes are preserved for BRE/replacement
// interpretation. A missing replacement delimiter is accepted at end-of-line.
func delimited(s string, delim byte) (text, rest string, closed bool, err error) {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == delim {
			return b.String(), s[i+1:], true, nil
		}
		if s[i] == '\\' && i+1 < len(s) {
			if s[i+1] == delim {
				b.WriteByte(delim)
				i++
				continue
			}
			b.WriteByte('\\')
			b.WriteByte(s[i+1])
			i++
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String(), "", false, nil
}
