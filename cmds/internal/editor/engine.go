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
	Read   func(name string) ([]byte, error)
	Write  func(name string, data []byte) error
	Append func(name string, data []byte) error
}

// Engine interprets the POSIX ed command language over a reusable Buffer.
type Engine struct {
	Buffer   Buffer
	Out      io.Writer
	Files    Files
	Silent   bool
	Prompt   string
	Filename string
	// Shell executes the command operand of ! and the !command forms of
	// e/r/w. Input is supplied for w !command; returned output is inserted for
	// e/r !command or written to ed's stdout for a bare ! command.
	Shell func(command string, input []byte) ([]byte, error)
	// PollSignal returns "interrupt" or "hangup" for process-facing ed.
	PollSignal func() string
	Signals    <-chan string
	Hangup     func(data []byte) error
	// ExitOnError selects the Issue 7 non-terminal-input rule.
	ExitOnError bool

	lastRE          string
	lastReplacement string
	lastDiagnostic  string
	lastShell       string
	promptSetting   string
	help            bool
	hadError        bool
	quitArmed       bool
	editArmed       bool
	reader          *bufio.Reader
	lineInput       <-chan lineResult
	marks           map[byte]int
	undoBuffer      Buffer
	undoMarks       map[byte]int
	undoValid       bool
	inGlobal        bool
	changeSeq       uint64
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
	e.marks = make(map[byte]int)
	e.undoValid = false
	e.Filename = name
	e.quitArmed = false
	return len(data), nil
}

// Run consumes commands until q/Q or EOF. It returns non-zero if any command
// diagnostic occurred. ExitOnError implements the POSIX non-terminal-input
// rule that stops a command script after its first diagnostic.
func (e *Engine) Run(in io.Reader) int {
	e.reader = bufio.NewReader(in)
	e.lineInput = nil
	for {
		if e.PollSignal != nil {
			switch e.PollSignal() {
			case "hangup":
				if e.Buffer.Dirty && e.Hangup != nil {
					_ = e.Hangup(joinLines(e.Buffer.Lines))
				}
				return 1
			case "interrupt":
				e.commandError("interrupt")
				if e.ExitOnError {
					return 1
				}
				continue
			}
		}
		if e.Prompt != "" {
			fmt.Fprint(e.Out, e.Prompt)
		}
		line, err := e.readCommandLine()
		if sig, ok := err.(signalError); ok {
			if string(sig) == "hangup" {
				if e.Buffer.Dirty && e.Hangup != nil {
					_ = e.Hangup(joinLines(e.Buffer.Lines))
				}
				return 1
			}
			e.commandError("interrupt")
			if e.ExitOnError {
				return 1
			}
			continue
		}
		if err != nil && len(line) == 0 {
			if errors.Is(err, io.EOF) {
				if e.Buffer.Dirty && !e.quitArmed {
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
			if sig, ok := cmdErr.(signalError); ok {
				if string(sig) == "hangup" {
					if e.Buffer.Dirty && e.Hangup != nil {
						_ = e.Hangup(joinLines(e.Buffer.Lines))
					}
					return 1
				}
				e.commandError("interrupt")
				if e.ExitOnError {
					return 1
				}
				continue
			}
			e.commandError(cmdErr.Error())
			if e.ExitOnError {
				break
			}
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

type signalError string

func (s signalError) Error() string { return string(s) }

type lineResult struct {
	line string
	err  error
}

func (e *Engine) readCommandLine() (string, error) {
	if e.Signals == nil {
		return e.reader.ReadString('\n')
	}
	if e.lineInput == nil {
		input := make(chan lineResult, 1)
		reader := e.reader
		e.lineInput = input
		go func() {
			defer close(input)
			for {
				line, err := reader.ReadString('\n')
				input <- lineResult{line: line, err: err}
				if err != nil {
					return
				}
			}
		}()
	}
	select {
	case r, ok := <-e.lineInput:
		if !ok {
			return "", io.EOF
		}
		return r.line, r.err
	case sig := <-e.Signals:
		return "", signalError(sig)
	}
}

func cloneMarks(src map[byte]int) map[byte]int {
	dst := make(map[byte]int, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func (e *Engine) saveUndo() {
	if e.inGlobal {
		return
	}
	e.undoBuffer, e.undoMarks, e.undoValid = e.Buffer.Clone(), cloneMarks(e.marks), true
}

func (e *Engine) appendLines(after int, lines []string) error {
	if after < 0 || after > e.Buffer.Last() {
		return fmt.Errorf("invalid address")
	}
	if len(lines) > 0 {
		e.saveUndo()
	}
	if err := e.Buffer.Append(after, lines); err != nil {
		return err
	}
	if len(lines) > 0 {
		e.changeSeq++
	}
	for k, n := range e.marks {
		if n > after {
			e.marks[k] = n + len(lines)
		}
	}
	return nil
}

func (e *Engine) deleteLines(first, last int) error {
	e.saveUndo()
	if err := e.Buffer.Delete(first, last); err != nil {
		return err
	}
	e.changeSeq++
	delta := last - first + 1
	for k, n := range e.marks {
		if n >= first && n <= last {
			delete(e.marks, k)
		} else if n > last {
			e.marks[k] = n - delta
		}
	}
	return nil
}

func (e *Engine) commandError(detail string) {
	e.hadError = true
	e.lastDiagnostic = detail
	fmt.Fprintln(e.Out, "?")
	if e.help && detail != "" {
		fmt.Fprintln(e.Out, detail)
	}
}

func (e *Engine) execute(line string) (quit bool, execErr error) {
	originalCurrent, originalSeq := e.Buffer.Current, e.changeSeq
	defer func() {
		if execErr != nil && e.changeSeq == originalSeq {
			e.Buffer.Current = originalCurrent
		}
	}()
	wasQuitArmed, wasEditArmed := e.quitArmed, e.editArmed
	e.quitArmed, e.editArmed = false, false
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
		style, err := printSuffix(arg)
		if err != nil {
			return false, err
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
			err = e.appendLines(a, text)
		case 'i':
			a, err := e.oneAddress(addrs, explicit, e.Buffer.Current, true)
			if err != nil {
				return false, err
			}
			if a == 0 {
				a = 1
			}
			err = e.appendLines(a-1, text)
		default:
			first, last, err := e.twoAddresses(addrs, explicit, e.Buffer.Current, e.Buffer.Current, true)
			if err != nil {
				return false, err
			}
			if first == 0 {
				first = 1
			}
			if last == 0 {
				last = 1
			}
			err = e.changeLines(first, last, text)
		}
		if err == nil && style != 0 {
			err = e.printRange([]int{e.Buffer.Current}, 1, style)
		}
		return false, err
	case 'd':
		first, last, err := e.twoAddresses(addrs, explicit, e.Buffer.Current, e.Buffer.Current, false)
		if err != nil {
			return false, err
		}
		style, err := printSuffix(arg)
		if err != nil {
			return false, err
		}
		if err = e.deleteLines(first, last); err == nil && style != 0 && e.Buffer.Last() > 0 {
			err = e.printRange([]int{e.Buffer.Current}, 1, style)
		}
		return false, err
	case 'j':
		first, last, err := e.twoAddresses(addrs, explicit, e.Buffer.Current, e.Buffer.Current+1, false)
		if err != nil {
			return false, err
		}
		style, err := printSuffix(arg)
		if err != nil {
			return false, err
		}
		err = e.join(first, last)
		if err == nil && style != 0 {
			err = e.printRange([]int{e.Buffer.Current}, 1, style)
		}
		return false, err
	case 'k':
		a, err := e.oneAddress(addrs, explicit, e.Buffer.Current, false)
		if err != nil {
			return false, err
		}
		arg = strings.TrimRight(arg, " \t")
		if len(arg) < 1 || len(arg) > 2 || arg[0] < 'a' || arg[0] > 'z' {
			return false, fmt.Errorf("invalid mark")
		}
		style, err := printSuffix(arg[1:])
		if err != nil {
			return false, err
		}
		if e.marks == nil {
			e.marks = make(map[byte]int)
		}
		e.marks[arg[0]] = a
		if style != 0 {
			err = e.printRange([]int{e.Buffer.Current}, 1, style)
		}
		return false, err
	case 'm', 't':
		first, last, err := e.twoAddresses(addrs, explicit, e.Buffer.Current, e.Buffer.Current, false)
		if err != nil {
			return false, err
		}
		dest, style, err := e.addressAndSuffix(arg)
		if err != nil {
			return false, err
		}
		if cmd == 'm' {
			err = e.move(first, last, dest)
		} else {
			err = e.copy(first, last, dest)
		}
		if err == nil && style != 0 {
			err = e.printRange([]int{e.Buffer.Current}, 1, style)
		}
		return false, err
	case 'p', 'n', 'l':
		style, err := printSuffix(arg)
		if err != nil {
			return false, err
		}
		if err = e.printRange(addrs, 2, cmd); err == nil && style != 0 {
			err = e.printRange([]int{e.Buffer.Current}, 1, style)
		}
		return false, err
	case '=':
		style, err := printSuffix(arg)
		if err != nil {
			return false, err
		}
		a, err := e.oneAddress(addrs, explicit, e.Buffer.Last(), true)
		if err != nil {
			return false, err
		}
		fmt.Fprintln(e.Out, a)
		if style != 0 && e.Buffer.Current > 0 {
			err = e.printRange([]int{e.Buffer.Current}, 1, style)
		}
		return false, err
	case 's':
		first, last, err := e.twoAddresses(addrs, explicit, e.Buffer.Current, e.Buffer.Current, false)
		if err != nil {
			return false, err
		}
		return false, e.substitute(first, last, arg)
	case 'w', 'W':
		return false, e.write(addrs, explicit, arg, cmd == 'W')
	case 'r':
		a, err := e.oneAddress(addrs, explicit, e.Buffer.Last(), true)
		if err != nil {
			return false, err
		}
		if arg != "" && arg[0] != ' ' && arg[0] != '\t' {
			return false, fmt.Errorf("file argument requires a separating blank")
		}
		return false, e.readFile(a, arg)
	case 'u':
		if explicit {
			return false, fmt.Errorf("unexpected address")
		}
		style, err := printSuffix(arg)
		if err != nil {
			return false, err
		}
		if !e.undoValid {
			return false, fmt.Errorf("nothing to undo")
		}
		curB, curM := e.Buffer.Clone(), cloneMarks(e.marks)
		e.Buffer, e.marks = e.undoBuffer.Clone(), cloneMarks(e.undoMarks)
		e.undoBuffer, e.undoMarks = curB, curM
		e.changeSeq++
		if style != 0 && e.Buffer.Current > 0 {
			err = e.printRange([]int{e.Buffer.Current}, 1, style)
		}
		return false, err
	case 'g', 'v':
		return false, e.global(addrs, explicit, arg, cmd == 'g', false)
	case 'G', 'V':
		return false, e.global(addrs, explicit, arg, cmd == 'G', true)
	case 'P':
		if explicit {
			return false, fmt.Errorf("unexpected address or argument")
		}
		style, err := printSuffix(arg)
		if err != nil {
			return false, err
		}
		if e.promptSetting == "" {
			if e.Prompt != "" {
				e.promptSetting = e.Prompt
			} else {
				e.promptSetting = "*"
			}
		}
		if e.Prompt == "" {
			e.Prompt = e.promptSetting
		} else {
			e.Prompt = ""
		}
		if style != 0 && e.Buffer.Current > 0 {
			err = e.printRange([]int{e.Buffer.Current}, 1, style)
		}
		return false, err
	case '!':
		if explicit {
			return false, fmt.Errorf("unexpected address")
		}
		return false, e.shellEscape(arg)
	case 'e', 'E':
		if explicit {
			return false, fmt.Errorf("unexpected address")
		}
		if arg != "" && arg[0] != ' ' && arg[0] != '\t' {
			return false, fmt.Errorf("file argument requires a separating blank")
		}
		if cmd == 'e' && e.Buffer.Dirty && !wasEditArmed {
			e.editArmed = true
			return false, fmt.Errorf("warning: buffer modified")
		}
		name := strings.TrimSpace(arg)
		if name == "" {
			name = e.Filename
		}
		if name == "" {
			return false, fmt.Errorf("no current filename")
		}
		var n int
		if strings.HasPrefix(name, "!") {
			if e.Shell == nil {
				return false, fmt.Errorf("shell escapes unavailable")
			}
			command := strings.TrimSpace(name[1:])
			if command == "" {
				return false, fmt.Errorf("shell command required")
			}
			data, shellErr := e.Shell(command, nil)
			if shellErr != nil {
				return false, shellErr
			}
			e.Buffer.Reset(splitText(data), false)
			e.marks = make(map[byte]int)
			e.undoValid = false
			n = len(data)
		} else {
			n, err = e.Load(name)
		}
		if err != nil {
			return false, err
		}
		e.changeSeq++
		if !e.Silent {
			fmt.Fprintln(e.Out, n)
		}
		return false, nil
	case 'q', 'Q':
		if explicit || strings.TrimSpace(arg) != "" {
			return false, fmt.Errorf("unexpected address or argument")
		}
		if cmd == 'q' && e.Buffer.Dirty && !wasQuitArmed {
			e.quitArmed = true
			return false, fmt.Errorf("warning: buffer modified")
		}
		return true, nil
	case 'f':
		if explicit {
			return false, fmt.Errorf("unexpected address")
		}
		if arg != "" && arg[0] != ' ' && arg[0] != '\t' {
			return false, fmt.Errorf("file argument requires a separating blank")
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
		if explicit {
			return false, fmt.Errorf("unexpected address or argument")
		}
		style, err := printSuffix(arg)
		if err != nil {
			return false, err
		}
		if e.lastDiagnostic == "" {
			return false, fmt.Errorf("no previous error")
		}
		fmt.Fprintln(e.Out, e.lastDiagnostic)
		if style != 0 && e.Buffer.Current > 0 {
			err = e.printRange([]int{e.Buffer.Current}, 1, style)
		}
		return false, err
	case 'H':
		if explicit {
			return false, fmt.Errorf("unexpected address or argument")
		}
		style, err := printSuffix(arg)
		if err != nil {
			return false, err
		}
		e.help = !e.help
		if e.help && e.lastDiagnostic != "" {
			fmt.Fprintln(e.Out, e.lastDiagnostic)
		}
		if style != 0 && e.Buffer.Current > 0 {
			err = e.printRange([]int{e.Buffer.Current}, 1, style)
		}
		return false, err
	default:
		return false, fmt.Errorf("unknown command")
	}
}

func printSuffix(arg string) (byte, error) {
	if arg == "" {
		return 0, nil
	}
	if len(arg) == 1 && strings.ContainsRune("pln", rune(arg[0])) {
		return arg[0], nil
	}
	return 0, fmt.Errorf("unexpected command suffix")
}

func (e *Engine) readInput() ([]string, error) {
	var lines []string
	for {
		line, err := e.readCommandLine()
		if err != nil && len(line) == 0 {
			if errors.Is(err, io.EOF) {
				return lines, nil
			}
			return nil, err
		}
		line = strings.TrimSuffix(line, "\n")
		line = strings.TrimSuffix(line, "\r")
		if line == "." {
			return lines, nil
		}
		lines = append(lines, line)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return lines, nil
			}
			return nil, err
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
	var escaped strings.Builder
	for len(s) > 0 {
		r, size := utf8.DecodeRuneInString(s)
		if r == utf8.RuneError && size == 1 {
			fmt.Fprintf(&escaped, "\\%03o", s[0])
			s = s[1:]
			continue
		}
		encoded := s[:size]
		s = s[size:]
		switch r {
		case '\\':
			escaped.WriteString("\\\\")
		case '\a':
			escaped.WriteString("\\a")
		case '\t':
			escaped.WriteString("\\t")
		case '\b':
			escaped.WriteString("\\b")
		case '\r':
			escaped.WriteString("\\r")
		case '\f':
			escaped.WriteString("\\f")
		case '\v':
			escaped.WriteString("\\v")
		case '$':
			escaped.WriteString("\\$")
		default:
			if unicode.IsPrint(r) {
				escaped.WriteRune(r)
			} else {
				for i := 0; i < len(encoded); i++ {
					fmt.Fprintf(&escaped, "\\%03o", encoded[i])
				}
			}
		}
	}
	escaped.WriteByte('$')
	text := escaped.String()
	if len(text) <= 70 {
		return text
	}
	var out strings.Builder
	for len(text) > 70 {
		out.WriteString(text[:69])
		out.WriteString("\\\n")
		text = text[69:]
	}
	out.WriteString(text)
	return out.String()
}

func (e *Engine) write(addrs []int, explicit bool, arg string, appendMode bool) error {
	if arg != "" && arg[0] != ' ' && arg[0] != '\t' {
		return fmt.Errorf("file argument requires a separating blank")
	}
	name := strings.TrimSpace(arg)
	shellCommand := strings.HasPrefix(name, "!")
	if shellCommand {
		name = strings.TrimSpace(strings.TrimPrefix(name, "!"))
		if name == "" {
			return fmt.Errorf("shell command required")
		}
	}
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
	if shellCommand {
		if e.Shell == nil {
			return fmt.Errorf("shell escapes unavailable")
		}
		out, err := e.Shell(name, data)
		if err != nil {
			return err
		}
		if len(out) > 0 {
			if _, err := e.Out.Write(out); err != nil {
				return err
			}
		}
	} else if appendMode {
		if e.Files.Append == nil {
			return fmt.Errorf("append unavailable")
		}
		if err := e.Files.Append(name, data); err != nil {
			return err
		}
	} else if err := e.Files.Write(name, data); err != nil {
		return err
	}
	if !shellCommand && e.Filename == "" {
		e.Filename = name
	}
	if !shellCommand && !appendMode && first == 1 && last == e.Buffer.Last() {
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
	oldUndo, oldUndoMarks, oldUndoValid := e.undoBuffer.Clone(), cloneMarks(e.undoMarks), e.undoValid
	e.saveUndo()
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
		e.undoBuffer, e.undoMarks, e.undoValid = oldUndo, oldUndoMarks, oldUndoValid
		return fmt.Errorf("no match")
	}
	e.Buffer.Current = lastChanged
	e.Buffer.Dirty = true
	e.changeSeq++
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
