package editor

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/qiangli/coreutils/pkg/bre"
)

// Files isolates the editor engine from a particular filesystem or working
// directory. Names are resolved by the front end before these functions run.
// Errors returned here are written verbatim to Err (when set) before the
// POSIX "?" notification, so the adapter should format them as
// "<name>: <reason>" the way historical ed does.
type Files struct {
	Read   func(name string) ([]byte, error)
	Write  func(name string, data []byte) error
	Append func(name string, data []byte) error
}

// Engine interprets the POSIX ed command language over a reusable Buffer.
type Engine struct {
	Buffer Buffer
	Out    io.Writer
	// Err receives file diagnostics ("name: reason"). POSIX reserves standard
	// error for diagnostics; the "?" notification itself goes to Out.
	Err      io.Writer
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
	// ExitOnError selects the Issue 7 CONSEQUENCES OF ERRORS rule for a
	// regular-file standard input: the first command error terminates ed.
	// Every other input (terminal, pipe, in-process reader) writes "?" and
	// returns to command mode, exactly as GNU ed and BSD ed do.
	ExitOnError bool
	// ByteLocale selects the single-byte POSIX/C LC_CTYPE character model.
	// It controls command delimiters and l output classification.
	ByteLocale bool
	// Tables, when set, compiles every BRE through the locale byte substrate
	// so '.' and bracket expressions classify single bytes exactly as the
	// selected single-byte locale does. Nil selects the UTF-8 matcher.
	Tables *bre.LocaleByteTables

	lastRE            string
	lastReplacement   string
	hasReplacement    bool
	lastDiagnostic    string
	lastShell         string
	promptSetting     string
	help              bool
	hadError          bool
	hangupFailed      bool
	warned            bool
	reader            *bufio.Reader
	list              *bufio.Reader
	source            *lineSource
	marks             map[byte]int
	undoBuffer        Buffer
	undoMarks         map[byte]int
	undoValid         bool
	inGlobal          bool
	interactiveGlobal bool
	globalQuit        bool
	globalTargets     []int
	changeSeq         uint64
}

// errWarned is the modified-buffer warning raised by e and q. It is the one
// command error that must not clear the armed warning: a repeated e or q with
// no intervening error or buffer modification then takes effect.
var errWarned = errors.New("warning: buffer modified")

// hangupGrace bounds how long ed waits for a SIGHUP that accompanies a
// terminal disconnect. The kernel delivers the hangup signal and the EIO/EOF
// on the controlling terminal independently; when the read completes first,
// treating it as a plain end-of-file would skip the ed.hup recovery write
// that POSIX requires.
const hangupGrace = 50 * time.Millisecond

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
	e.warned = false
	return len(data), nil
}

// Run consumes commands until q/Q or EOF. It returns non-zero if any command
// diagnostic occurred. ExitOnError implements the POSIX regular-file rule
// that stops a command script after its first diagnostic.
func (e *Engine) Run(in io.Reader) int {
	e.reader = bufio.NewReader(in)
	e.list = nil
	e.source = nil
	defer e.stopSource()
	for {
		if e.PollSignal != nil {
			if sig := e.PollSignal(); sig != "" {
				if e.handleSignal(sig) {
					return e.exitStatus()
				}
				continue
			}
		}
		if e.Prompt != "" {
			fmt.Fprint(e.Out, e.Prompt)
		}
		line, err := e.readCommandLine()
		if sig, ok := err.(signalError); ok {
			if e.handleSignal(string(sig)) {
				return e.exitStatus()
			}
			continue
		}
		if err != nil && len(line) == 0 {
			if !errors.Is(err, io.EOF) {
				e.commandError(err.Error())
				return 1
			}
			if sig := e.pendingHangup(); sig != "" {
				if e.handleSignal(sig) {
					return e.exitStatus()
				}
				continue
			}
			// End-of-file in command mode acts as a q command.
			if e.Buffer.Dirty && !e.warned {
				e.warned = true
				e.commandError(errWarned.Error())
				if e.ExitOnError {
					return 1
				}
				continue
			}
			break
		}
		line = strings.TrimSuffix(line, "\n")
		quit, cmdErr := e.execute(line)
		if cmdErr != nil {
			if sig, ok := cmdErr.(signalError); ok {
				if e.handleSignal(string(sig)) {
					return e.exitStatus()
				}
				continue
			}
			e.commandError(cmdErr.Error())
			if e.ExitOnError {
				return 1
			}
		}
		if quit {
			break
		}
	}
	return e.exitStatus()
}

func (e *Engine) exitStatus() int {
	if e.hadError || e.hangupFailed {
		return 1
	}
	return 0
}

// handleSignal applies the ASYNCHRONOUS EVENTS rules. It reports whether ed
// must exit.
func (e *Engine) handleSignal(sig string) bool {
	switch sig {
	case "hangup":
		e.hangupFailed = true
		if e.Buffer.Dirty && e.Buffer.Last() > 0 {
			if e.Hangup == nil || e.Hangup(joinLines(e.Buffer.Lines)) != nil {
				e.hangupFailed = true
			}
		}
		return true
	case "interrupt":
		// SIGINT interrupts the current activity and returns to command
		// mode. It is not a command error: it neither terminates a script
		// nor changes the exit status.
		e.lastDiagnostic = "interrupt"
		fmt.Fprintln(e.Out, "?")
		if e.help {
			fmt.Fprintln(e.Out, e.lastDiagnostic)
		}
	}
	return false
}

// pendingHangup gives a hangup signal that raced with end-of-file on a
// disconnected terminal a short window to arrive, but only when there is a
// modified buffer that the signal would save.
func (e *Engine) pendingHangup() string {
	if e.Signals == nil || !e.Buffer.Dirty || e.Buffer.Last() == 0 {
		return ""
	}
	timer := time.NewTimer(hangupGrace)
	defer timer.Stop()
	select {
	case sig := <-e.Signals:
		return sig
	case <-timer.C:
		return ""
	}
}

type signalError string

func (s signalError) Error() string { return string(s) }

type lineResult struct {
	line string
	err  error
}

// lineSource reads standard input on demand from a helper goroutine so a
// signal can interrupt a blocked read. Reads are request-driven: the helper
// never reads ahead of the line the engine asked for, so an in-process host
// sharing the descriptor does not lose input after ed quits, and a terminal
// that returned end-of-file can be read again after the modified-buffer
// warning.
type lineSource struct {
	req     chan struct{}
	res     chan lineResult
	stop    chan struct{}
	pending bool
}

func (e *Engine) startSource() {
	src := &lineSource{req: make(chan struct{}), res: make(chan lineResult, 1), stop: make(chan struct{})}
	reader := e.reader
	go func() {
		for {
			select {
			case <-src.req:
			case <-src.stop:
				return
			}
			line, err := reader.ReadString('\n')
			src.res <- lineResult{line: line, err: err}
		}
	}()
	e.source = src
}

func (e *Engine) stopSource() {
	if e.source != nil {
		close(e.source.stop)
		e.source = nil
	}
}

func (e *Engine) readCommandLine() (string, error) {
	if e.list != nil {
		return e.list.ReadString('\n')
	}
	if e.Signals == nil {
		return e.reader.ReadString('\n')
	}
	if e.source == nil {
		e.startSource()
	}
	src := e.source
	if !src.pending {
		src.req <- struct{}{}
		src.pending = true
	}
	select {
	case r := <-src.res:
		src.pending = false
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
		e.adjustGlobalInsert(after, len(lines))
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
	e.adjustGlobalDelete(first, last)
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

func (e *Engine) adjustGlobalInsert(after, count int) {
	if count == 0 {
		return
	}
	for i, n := range e.globalTargets {
		if n > after {
			e.globalTargets[i] = n + count
		}
	}
}

func (e *Engine) adjustGlobalDelete(first, last int) {
	delta := last - first + 1
	for i, n := range e.globalTargets {
		switch {
		case n >= first && n <= last:
			e.globalTargets[i] = 0
		case n > last:
			e.globalTargets[i] = n - delta
		}
	}
}

func (e *Engine) commandError(detail string) {
	e.hadError = true
	e.lastDiagnostic = detail
	fmt.Fprintln(e.Out, "?")
	if e.help && detail != "" {
		fmt.Fprintln(e.Out, detail)
	}
}

// fileDiagnostic reports a file error the way historical ed does: the
// "name: reason" line goes to standard error and the command then fails with
// the ordinary "?" notification (explained by h/H).
func (e *Engine) fileDiagnostic(err error) error {
	if e.Err != nil {
		fmt.Fprintln(e.Err, err.Error())
	}
	return err
}

func (e *Engine) execute(line string) (quit bool, execErr error) {
	originalCurrent, originalSeq := e.Buffer.Current, e.changeSeq
	defer func() {
		if execErr != nil && e.changeSeq == originalSeq {
			e.Buffer.Current = originalCurrent
		}
		// The armed e/q warning survives only until an error or a buffer
		// modification intervenes.
		if e.changeSeq != originalSeq || (execErr != nil && execErr != errWarned) {
			e.warned = false
		}
	}()
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
			if err == nil && len(text) == 0 {
				// Unlike append, an empty insert leaves dot at the addressed
				// line, not at the insertion position immediately before it.
				// An empty buffer has no line 1 to address, so dot stays 0.
				if a > e.Buffer.Last() {
					a = e.Buffer.Last()
				}
				e.Buffer.Current = a
			}
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
		// POSIX leaves a print suffix on a print command unspecified;
		// historical ed writes the lines once, in the suffix's format.
		if style != 0 {
			cmd = style
		}
		return false, e.printRange(addrs, 2, cmd)
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
		arg, err = e.readSubstituteArgument(arg)
		if err != nil {
			return false, err
		}
		return false, e.substitute(first, last, arg)
	case 'w', 'W':
		// wq is the historical spelling for writing the addressed range and
		// then quitting. A separated q remains an ordinary filename.
		if cmd == 'w' && len(arg) > 0 && arg[0] == 'q' &&
			(len(arg) == 1 || arg[1] == ' ' || arg[1] == '\t') {
			if err := e.write(addrs, explicit, arg[1:], false); err != nil {
				return false, err
			}
			return true, nil
		}
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
		restored := e.undoBuffer.Clone()
		if !sameLines(restored.Lines, e.Buffer.Lines) {
			// u is itself a buffer-modifying command: the buffer differs
			// from what was last written even when it returns to the text
			// the previous command started from.
			restored.Dirty = true
		}
		e.Buffer, e.marks = restored, cloneMarks(e.undoMarks)
		e.undoBuffer, e.undoMarks = curB, curM
		e.changeSeq++
		if e.inGlobal {
			for i := range e.globalTargets {
				e.globalTargets[i] = 0
			}
		}
		if style != 0 && e.Buffer.Current > 0 {
			err = e.printRange([]int{e.Buffer.Current}, 1, style)
		}
		return false, err
	case 'g', 'v':
		e.globalQuit = false
		err := e.global(addrs, explicit, arg, cmd == 'g', false)
		return e.globalQuit, err
	case 'G', 'V':
		e.globalQuit = false
		err := e.global(addrs, explicit, arg, cmd == 'G', true)
		return e.globalQuit, err
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
		if cmd == 'e' && e.Buffer.Dirty && !e.warned {
			e.warned = true
			return false, errWarned
		}
		name := trimBlank(arg)
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
			command := trimBlank(name[1:])
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
			if err != nil {
				// The buffer was deleted before the read was attempted, and
				// the name is remembered so a later w can create the file.
				e.Buffer.Reset(nil, false)
				e.marks = make(map[byte]int)
				e.undoValid = false
				e.Filename = name
				e.changeSeq++
				return false, e.fileDiagnostic(err)
			}
		}
		e.changeSeq++
		if e.inGlobal {
			for i := range e.globalTargets {
				e.globalTargets[i] = 0
			}
		}
		if !e.Silent {
			fmt.Fprintln(e.Out, n)
		}
		return false, nil
	case 'q', 'Q':
		if explicit || arg != "" {
			return false, fmt.Errorf("unexpected address or argument")
		}
		if cmd == 'q' && e.Buffer.Dirty && !e.warned {
			e.warned = true
			return false, errWarned
		}
		return true, nil
	case 'f':
		if explicit {
			return false, fmt.Errorf("unexpected address")
		}
		if arg != "" && arg[0] != ' ' && arg[0] != '\t' {
			return false, fmt.Errorf("file argument requires a separating blank")
		}
		name := trimBlank(arg)
		if strings.HasPrefix(name, "!") {
			return false, fmt.Errorf("invalid filename")
		}
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
		// Without a previous "?" there is nothing to explain; h is silent.
		if e.lastDiagnostic != "" {
			fmt.Fprintln(e.Out, e.lastDiagnostic)
		}
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

func sameLines(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
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
		if e.list != nil {
			// Inside a global command list every line but the last carries
			// the list-continuation backslash, which is not input text.
			line = stripContinuation(line)
		}
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

func (e *Engine) readSubstituteArgument(arg string) (string, error) {
	for hasOddTrailingBackslashes(arg) {
		next, err := e.readCommandLine()
		if err != nil && len(next) == 0 {
			return "", fmt.Errorf("unterminated substitute replacement")
		}
		next = strings.TrimSuffix(next, "\n")
		arg = arg[:len(arg)-1] + "\n" + next
		if err != nil {
			return "", fmt.Errorf("unterminated substitute replacement")
		}
	}
	return arg, nil
}

func hasOddTrailingBackslashes(s string) bool {
	n := 0
	for i := len(s) - 1; i >= 0 && s[i] == '\\'; i-- {
		n++
	}
	return n%2 == 1
}

// stripContinuation removes the backslash that ended a line of a multi-line
// global command list.
func stripContinuation(s string) string {
	if hasOddTrailingBackslashes(s) {
		return s[:len(s)-1]
	}
	return s
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
			fmt.Fprintln(e.Out, listLine(line, e.ByteLocale))
		default:
			fmt.Fprintln(e.Out, line)
		}
	}
	e.Buffer.Current = last
	return nil
}

// listWidth is the column at which l output folds. POSIX leaves the length
// unspecified; this is the width historical implementations use for a
// non-terminal output device, and an escape sequence is never split across
// the fold so the output stays visually unambiguous.
const listWidth = 76

func listLine(s string, byteMode bool) string {
	var out strings.Builder
	col := 0
	emit := func(unit string, width int) {
		if col >= listWidth {
			out.WriteString("\\\n")
			col = 0
		}
		out.WriteString(unit)
		col += width
	}
	for len(s) > 0 {
		r, size := utf8.DecodeRuneInString(s)
		if byteMode && s[0] >= utf8.RuneSelf {
			r, size = utf8.RuneError, 1
		}
		if r == utf8.RuneError && size == 1 {
			emit(fmt.Sprintf("\\%03o", s[0]), 4)
			s = s[1:]
			continue
		}
		encoded := s[:size]
		s = s[size:]
		switch r {
		case '\\':
			emit("\\\\", 2)
		case '\a':
			emit("\\a", 2)
		case '\t':
			emit("\\t", 2)
		case '\b':
			emit("\\b", 2)
		case '\r':
			emit("\\r", 2)
		case '\f':
			emit("\\f", 2)
		case '\v':
			emit("\\v", 2)
		case '$':
			emit("\\$", 2)
		default:
			if unicode.IsPrint(r) {
				emit(encoded, 1)
			} else {
				var b strings.Builder
				for i := 0; i < len(encoded); i++ {
					fmt.Fprintf(&b, "\\%03o", encoded[i])
				}
				emit(b.String(), 4*len(encoded))
			}
		}
	}
	out.WriteByte('$')
	return out.String()
}

func (e *Engine) write(addrs []int, explicit bool, arg string, appendMode bool) error {
	if arg != "" && arg[0] != ' ' && arg[0] != '\t' {
		return fmt.Errorf("file argument requires a separating blank")
	}
	name := trimBlank(arg)
	shellCommand := strings.HasPrefix(name, "!")
	if shellCommand {
		name = trimBlank(strings.TrimPrefix(name, "!"))
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
		first, last, err = e.twoAddresses(addrs, true, first, last, false)
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
			return e.fileDiagnostic(err)
		}
	} else if err := e.Files.Write(name, data); err != nil {
		return e.fileDiagnostic(err)
	}
	if !shellCommand && e.Filename == "" {
		e.Filename = name
	}
	if !shellCommand && !appendMode && first == 1 && last == e.Buffer.Last() {
		e.Buffer.Dirty = false
	}
	e.warned = false
	if !e.Silent {
		fmt.Fprintln(e.Out, len(data))
	}
	return nil
}

func (e *Engine) substitute(first, last int, arg string) error {
	if arg == "" {
		return fmt.Errorf("missing substitute expression")
	}
	delim, delimLen := e.commandDelimiter(arg)
	if delim == " " || delim == "\n" || delim == "\\" {
		return fmt.Errorf("invalid substitute delimiter")
	}
	pattern, rest, closed := scanRE(arg[delimLen:], delim)
	if !closed {
		return fmt.Errorf("unterminated regular expression")
	}
	replacement, flags, replacementClosed, err := delimitedBy(rest, delim)
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
		if !e.hasReplacement {
			return fmt.Errorf("no previous replacement")
		}
		replacement = e.lastReplacement
	}
	re, err := e.compileRE(pattern)
	if err != nil {
		return err
	}
	global, count, style, err := parseSubFlags(flags)
	if err != nil {
		return err
	}
	if !replacementClosed {
		style = 'p'
	}
	e.lastRE, e.lastReplacement, e.hasReplacement = pattern, replacement, true
	oldUndo, oldUndoMarks, oldUndoValid := e.undoBuffer.Clone(), cloneMarks(e.undoMarks), e.undoValid
	e.saveUndo()
	changed, lastChanged := false, 0
	offset := 0
	for original := first; original <= last; original++ {
		n := original + offset
		line := e.Buffer.Lines[n-1]
		matches, err := re.FindAllStringSubmatchIndex(line, -1)
		if err != nil {
			return err
		}
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
		pieces := strings.Split(b.String(), "\n")
		delta := len(pieces) - 1
		updated := make([]string, 0, len(e.Buffer.Lines)+delta)
		updated = append(updated, e.Buffer.Lines[:n-1]...)
		updated = append(updated, pieces...)
		updated = append(updated, e.Buffer.Lines[n:]...)
		e.Buffer.Lines = updated
		// The original marked line was modified and is no longer eligible
		// for another pass of the surrounding global command. Other marked
		// lines retain their identity as their addresses shift.
		for i, target := range e.globalTargets {
			switch {
			case target == n:
				e.globalTargets[i] = 0
			case target > n:
				e.globalTargets[i] = target + delta
			}
		}
		for k, mark := range e.marks {
			if mark == n {
				// When a replacement introduces newlines, ed associates a
				// mark on the original line with the last resulting line.
				e.marks[k] = mark + delta
			} else if mark > n {
				e.marks[k] = mark + delta
			}
		}
		offset += delta
		changed, lastChanged = true, n+len(pieces)-1
	}
	if !changed {
		if e.interactiveGlobal {
			// Within an interactive global command, a substitution is applied
			// to one marked line at a time; a line without a match is simply
			// left alone rather than aborting the whole interactive command.
			return nil
		}
		e.undoBuffer, e.undoMarks, e.undoValid = oldUndo, oldUndoMarks, oldUndoValid
		return fmt.Errorf("no match")
	}
	e.Buffer.Current = lastChanged
	e.Buffer.Dirty = true
	e.changeSeq++
	if style != 0 {
		return e.printRange([]int{lastChanged}, 1, style)
	}
	return nil
}

// matcher is the regular-expression surface the engine needs from either
// the UTF-8 matcher or the locale byte substrate.
type matcher interface {
	MatchString(string) (bool, error)
	FindAllStringSubmatchIndex(string, int) ([][]int, error)
}

type goMatcher struct{ re *bre.Regexp }

func (m goMatcher) MatchString(s string) (bool, error) { return m.re.MatchStringErr(s) }
func (m goMatcher) FindAllStringSubmatchIndex(s string, n int) ([][]int, error) {
	return m.re.FindAllStringSubmatchIndexErr(s, n)
}

// compileRE compiles a POSIX BRE for the selected LC_CTYPE model. The match
// extent follows the leftmost-longest rule in both models.
func (e *Engine) compileRE(pattern string) (matcher, error) {
	if e.Tables != nil {
		re, err := bre.CompileLocaleByteRegexpTables([]byte(pattern), e.Tables, bre.ByteRegexpOptions{})
		if err != nil {
			return nil, err
		}
		return re, nil
	}
	re, err := bre.Compile(pattern)
	if err != nil {
		return nil, err
	}
	re.Longest()
	return goMatcher{re}, nil
}

func (e *Engine) commandDelimiter(s string) (string, int) {
	if s == "" {
		return "", 0
	}
	if e.ByteLocale {
		return s[:1], 1
	}
	_, size := utf8.DecodeRuneInString(s)
	return s[:size], size
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

// scanRE returns the regular expression that runs through the next
// unescaped delimiter. A backslash before the delimiter yields the literal
// delimiter; every other backslash pair is preserved for BRE interpretation.
// A delimiter inside a bracket expression is an ordinary member of the
// expression, as in every historical ed and in sed. A missing closing
// delimiter is accepted at end-of-line and reported through closed.
func scanRE(s, delim string) (pattern, rest string, closed bool) {
	var b strings.Builder
	for i := 0; i < len(s); {
		if strings.HasPrefix(s[i:], delim) {
			return b.String(), s[i+len(delim):], true
		}
		if s[i] == '\\' && strings.HasPrefix(s[i+1:], delim) {
			b.WriteString(delim)
			i += 1 + len(delim)
			continue
		}
		if s[i] == '\\' && i+1 < len(s) {
			b.WriteByte(s[i])
			b.WriteByte(s[i+1])
			i += 2
			continue
		}
		if s[i] == '[' {
			if end := bracketEnd(s, i); end > 0 {
				b.WriteString(s[i:end])
				i = end
				continue
			}
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String(), "", false
}

// bracketEnd returns the index just past the bracket expression opening at
// s[start], or 0 when the expression is not terminated. It honours the POSIX
// rules that a ']' first in the list (after an optional '^') is literal and
// that [:class:], [=equiv=], and [.coll.] members contain their own brackets.
func bracketEnd(s string, start int) int {
	i := start + 1
	if i < len(s) && s[i] == '^' {
		i++
	}
	if i < len(s) && s[i] == ']' {
		i++
	}
	for i < len(s) {
		switch {
		case s[i] == ']':
			return i + 1
		case s[i] == '[' && i+1 < len(s) && (s[i+1] == ':' || s[i+1] == '=' || s[i+1] == '.'):
			term := string(s[i+1]) + "]"
			end := strings.Index(s[i+2:], term)
			if end < 0 {
				return 0
			}
			i += 2 + end + len(term)
		default:
			i++
		}
	}
	return 0
}

func delimitedBy(s, delim string) (text, rest string, closed bool, err error) {
	var b strings.Builder
	for i := 0; i < len(s); {
		if strings.HasPrefix(s[i:], delim) {
			return b.String(), s[i+len(delim):], true, nil
		}
		if s[i] == '\\' && strings.HasPrefix(s[i+1:], delim) {
			b.WriteString(delim)
			i += 1 + len(delim)
			continue
		}
		if s[i] == '\\' && i+1 < len(s) {
			b.WriteByte(s[i])
			b.WriteByte(s[i+1])
			i += 2
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String(), "", false, nil
}
