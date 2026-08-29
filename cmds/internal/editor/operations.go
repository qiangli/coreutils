package editor

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
)

func (e *Engine) changeLines(first, last int, lines []string) error {
	e.saveUndo()
	if e.Buffer.Last() == 0 && (first == 0 || first == 1) && (last == 0 || last == 1) {
		err := e.appendLinesNoUndo(0, lines)
		if err == nil && len(lines) > 0 {
			e.changeSeq++
		}
		return err
	}
	if err := e.deleteLinesNoUndo(first, last); err != nil {
		return err
	}
	if len(lines) == 0 {
		e.changeSeq++
		return nil
	}
	err := e.appendLinesNoUndo(first-1, lines)
	if err == nil {
		e.changeSeq++
	}
	return err
}

func (e *Engine) appendLinesNoUndo(after int, lines []string) error {
	if err := e.Buffer.Append(after, lines); err != nil {
		return err
	}
	for k, n := range e.marks {
		if n > after {
			e.marks[k] = n + len(lines)
		}
	}
	e.adjustGlobalInsert(after, len(lines))
	return nil
}

func (e *Engine) deleteLinesNoUndo(first, last int) error {
	if err := e.Buffer.Delete(first, last); err != nil {
		return err
	}
	delta := last - first + 1
	e.adjustGlobalDelete(first, last)
	for k, n := range e.marks {
		if n >= first && n <= last {
			delete(e.marks, k)
		} else if n > last {
			e.marks[k] = n - delta
		}
	}
	return nil
}

func (e *Engine) join(first, last int) error {
	if first < 1 || last < first || last > e.Buffer.Last() {
		return fmt.Errorf("invalid address")
	}
	if first == last {
		return nil
	}
	e.saveUndo()
	joined := strings.Join(append([]string(nil), e.Buffer.Lines[first-1:last]...), "")
	var joinedMarks []byte
	for k, n := range e.marks {
		if n >= first && n <= last {
			joinedMarks = append(joinedMarks, k)
		}
	}
	if err := e.deleteLinesNoUndo(first, last); err != nil {
		return err
	}
	if err := e.appendLinesNoUndo(first-1, []string{joined}); err != nil {
		return err
	}
	for _, k := range joinedMarks {
		e.marks[k] = first
	}
	e.changeSeq++
	return nil
}

// addressAndSuffix parses the destination of m and t. Historical ed (and
// GNU ed) default an omitted destination to the current line.
func (e *Engine) addressAndSuffix(arg string) (int, byte, error) {
	pos := skipBlank(arg, 0)
	a, next, ok, err := e.parseAddress(arg, pos)
	if err != nil {
		return 0, 0, err
	}
	if !ok {
		a, next = e.Buffer.Current, pos
	}
	style, err := printSuffix(arg[next:])
	return a, style, err
}

func (e *Engine) copy(first, last, dest int) error {
	if dest < 0 || dest > e.Buffer.Last() {
		return fmt.Errorf("invalid address")
	}
	lines := append([]string(nil), e.Buffer.Lines[first-1:last]...)
	return e.appendLines(dest, lines)
}

func (e *Engine) move(first, last, dest int) error {
	if dest < 0 || dest > e.Buffer.Last() {
		return fmt.Errorf("invalid address")
	}
	if dest == last || dest == first-1 {
		// Moving a range after its own last line, or after the line that
		// already precedes it, leaves the buffer unchanged. Historical ed
		// accepts both and sets dot to the last line of the range.
		e.Buffer.Current = last
		return nil
	}
	if dest >= first && dest < last {
		return fmt.Errorf("invalid destination")
	}
	e.saveUndo()
	lines := append([]string(nil), e.Buffer.Lines[first-1:last]...)
	// Preserve marks attached to moved lines by remembering their offsets.
	movedMarks := map[byte]int{}
	for k, n := range e.marks {
		if n >= first && n <= last {
			movedMarks[k] = n - first
		}
	}
	if err := e.deleteLinesNoUndo(first, last); err != nil {
		return err
	}
	if dest > last {
		dest -= last - first + 1
	}
	if err := e.appendLinesNoUndo(dest, lines); err != nil {
		return err
	}
	for k, off := range movedMarks {
		e.marks[k] = dest + 1 + off
	}
	e.changeSeq++
	return nil
}

func (e *Engine) readFile(after int, arg string) error {
	name := trimBlank(arg)
	var data []byte
	var err error
	if strings.HasPrefix(name, "!") {
		if e.Shell == nil {
			return fmt.Errorf("shell escapes unavailable")
		}
		command := trimBlank(name[1:])
		if command == "" {
			return fmt.Errorf("shell command required")
		}
		data, err = e.Shell(command, nil)
		if err != nil {
			return err
		}
	} else {
		if name == "" {
			name = e.Filename
		}
		if name == "" {
			return fmt.Errorf("no current filename")
		}
		data, err = e.Files.Read(name)
		if err != nil {
			return e.fileDiagnostic(err)
		}
	}
	if err := e.appendLines(after, splitText(data)); err != nil {
		return err
	}
	if !strings.HasPrefix(trimBlank(arg), "!") && e.Filename == "" {
		e.Filename = name
	}
	if !e.Silent {
		fmt.Fprintln(e.Out, len(data))
	}
	return nil
}

func (e *Engine) shellEscape(arg string) error {
	if e.Shell == nil {
		return fmt.Errorf("shell escapes unavailable")
	}
	command, expanded, err := e.prepareShell(trimBlank(arg))
	if err != nil {
		return err
	}
	if expanded {
		fmt.Fprintln(e.Out, command)
	}
	out, err := e.Shell(command, nil)
	if len(out) > 0 {
		if _, werr := e.Out.Write(out); werr != nil && err == nil {
			err = werr
		}
	}
	if err == nil && !e.Silent {
		_, err = fmt.Fprintln(e.Out, "!")
	}
	return err
}

// prepareShell applies ed's command-line substitutions before invoking sh.
// A leading unescaped '!' recalls the previous command, while every unescaped
// '%' denotes the current filename. A backslash quotes either special byte and
// is removed before the command reaches the shell.
func (e *Engine) prepareShell(command string) (string, bool, error) {
	if command == "" {
		return "", false, fmt.Errorf("shell command required")
	}
	replaced := false
	if command[0] == '!' {
		if e.lastShell == "" {
			return "", false, fmt.Errorf("no previous shell command")
		}
		command = e.lastShell + command[1:]
		replaced = true
	}
	var expanded strings.Builder
	for i := 0; i < len(command); i++ {
		if command[i] == '\\' && i+1 < len(command) &&
			(command[i+1] == '%' || command[i+1] == '!') {
			expanded.WriteByte(command[i+1])
			i++
			continue
		}
		if command[i] == '%' {
			if e.Filename == "" {
				return "", false, fmt.Errorf("no current filename")
			}
			expanded.WriteString(e.Filename)
			replaced = true
			continue
		}
		expanded.WriteByte(command[i])
	}
	e.lastShell = expanded.String()
	return e.lastShell, replaced, nil
}

func (e *Engine) global(addrs []int, explicit bool, arg string, match, interactive bool) (retErr error) {
	first, last, err := e.twoAddresses(addrs, explicit, 1, e.Buffer.Last(), false)
	if err != nil {
		return err
	}
	if arg == "" {
		return fmt.Errorf("missing regular expression")
	}
	delim, delimLen := e.commandDelimiter(arg)
	if delim == " " || delim == "\n" || delim == "\\" {
		return fmt.Errorf("invalid regular expression delimiter")
	}
	pattern, rest, closed := scanRE(arg[delimLen:], delim)
	if !closed {
		rest = ""
	}
	if pattern == "" {
		pattern = e.lastRE
	}
	if pattern == "" {
		return fmt.Errorf("no previous regular expression")
	}
	re, err := e.compileRE(pattern)
	if err != nil {
		return err
	}
	e.lastRE = pattern
	command := trimBlank(rest)
	var interactiveSuffix byte
	if interactive {
		// G and V take no command list on their command line, but like every
		// ed command except the documented exclusions they may carry one of
		// the l, n, or p print suffixes.  The lines following the command are
		// still consumed once per selected line as the interactive commands.
		interactiveSuffix, err = printSuffix(rest)
		if err != nil {
			return err
		}
		command = ""
	}
	if command == "" && !interactive {
		command = "p"
	}
	// A multi-line command list ends every line but the last with a
	// backslash. The backslashes stay in the list: executeGlobalList strips
	// the ones that only continue the list, while an s command keeps its
	// escaped newline so the replacement can split the line.
	for !interactive && hasOddTrailingBackslashes(lastLine(command)) {
		next, readErr := e.readCommandLine()
		if readErr != nil && len(next) == 0 {
			return fmt.Errorf("unterminated command list")
		}
		command += "\n" + strings.TrimSuffix(next, "\n")
	}
	targets := make([]int, 0)
	for n := first; n <= last; n++ {
		ok, err := re.MatchString(e.Buffer.Lines[n-1])
		if err != nil {
			return err
		}
		if ok == match {
			targets = append(targets, n)
		}
	}
	oldUndo, oldUndoMarks, oldUndoValid := e.undoBuffer.Clone(), cloneMarks(e.undoMarks), e.undoValid
	beforeSeq := e.changeSeq
	e.saveUndo()
	previousInteractiveGlobal := e.interactiveGlobal
	e.inGlobal = true
	e.interactiveGlobal = interactive
	e.globalTargets = targets
	e.globalSubCommand = -1
	e.globalSubAttempt = nil
	e.globalSubChanged = nil
	defer func() {
		e.inGlobal = false
		e.interactiveGlobal = previousInteractiveGlobal
		e.globalTargets = nil
		e.globalSubCommand = -1
		e.globalSubAttempt = nil
		e.globalSubChanged = nil
		if beforeSeq == e.changeSeq {
			if retErr != nil {
				// A failed no-op global command must not hide the preceding
				// successful editing operation from undo.
				e.undoBuffer, e.undoMarks, e.undoValid = oldUndo, oldUndoMarks, oldUndoValid
			} else {
				// A completed global command owns the undo boundary, even when
				// its command list made no change.
				e.undoBuffer, e.undoMarks, e.undoValid = e.Buffer.Clone(), cloneMarks(e.marks), true
				e.changeSeq++
			}
		}
	}()
	previousInteractive := ""
	for targetIndex := range e.globalTargets {
		n := e.globalTargets[targetIndex]
		if n == 0 {
			continue
		}
		// A command that changes this line must not cause it to be selected
		// again. Address-shifting helpers keep the remaining entries aligned.
		e.globalTargets[targetIndex] = 0
		e.Buffer.Current = n
		lineCommand := command
		if interactive {
			fmt.Fprintln(e.Out, e.Buffer.Lines[n-1])
			var readErr error
			lineCommand, readErr = e.readCommandLine()
			if readErr != nil && len(lineCommand) == 0 {
				return readErr
			}
			lineCommand = strings.TrimSuffix(lineCommand, "\n")
			if lineCommand == "" {
				continue
			}
			if lineCommand == "&" {
				if previousInteractive == "" {
					return fmt.Errorf("no previous interactive command")
				}
				lineCommand = previousInteractive
			} else {
				previousInteractive = lineCommand
			}
			_, p, _, parseErr := e.parseAddresses(lineCommand)
			if parseErr != nil {
				return parseErr
			}
			p = skipBlank(lineCommand, p)
			if p < len(lineCommand) && strings.ContainsRune("acigGvV", rune(lineCommand[p])) {
				return fmt.Errorf("command not permitted in interactive global")
			}
		}
		quit, execErr := e.executeGlobalList(lineCommand)
		if execErr != nil {
			return execErr
		}
		if quit {
			e.globalQuit = true
			return nil
		}
	}
	for i := range e.globalSubAttempt {
		if e.globalSubAttempt[i] && !e.globalSubChanged[i] {
			return fmt.Errorf("no match")
		}
	}
	if interactiveSuffix != 0 && e.Buffer.Current > 0 {
		return e.printRange([]int{e.Buffer.Current}, 1, interactiveSuffix)
	}
	return nil
}

func lastLine(s string) string {
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		return s[i+1:]
	}
	return s
}

func (e *Engine) executeGlobalList(commands string) (bool, error) {
	savedList := e.list
	e.list = bufio.NewReader(strings.NewReader(commands + "\n"))
	defer func() {
		e.list = savedList
	}()
	subCommand := 0
	for {
		line, err := e.readCommandLine()
		if err != nil && len(line) == 0 {
			if errors.Is(err, io.EOF) {
				return false, nil
			}
			return false, err
		}
		line = strings.TrimSuffix(line, "\n")
		_, p, _, parseErr := e.parseAddresses(line)
		if parseErr != nil {
			return false, parseErr
		}
		p = skipBlank(line, p)
		letter := commandLetter(line)
		if letter != 's' || e.closedSubstitutionAt(line, p) {
			line = stripContinuation(line)
		}
		if p < len(line) && strings.ContainsRune("gGvV", rune(line[p])) {
			return false, fmt.Errorf("cannot nest global commands")
		}
		if letter == 's' && !e.interactiveGlobal {
			e.globalSubCommand = subCommand
			subCommand++
			for len(e.globalSubAttempt) <= e.globalSubCommand {
				e.globalSubAttempt = append(e.globalSubAttempt, false)
				e.globalSubChanged = append(e.globalSubChanged, false)
			}
		} else {
			e.globalSubCommand = -1
		}
		quit, execErr := e.execute(line)
		if execErr != nil || quit {
			return quit, execErr
		}
		if err != nil {
			return false, nil
		}
	}
}

// closedSubstitutionAt reports whether the replacement delimiter of the s
// command at p is present.  In a global command list, a backslash following a
// complete substitute expression continues the command list.  A backslash
// before that delimiter instead escapes a newline into the replacement and
// must reach readSubstituteArgument unchanged.
func (e *Engine) closedSubstitutionAt(line string, p int) bool {
	if p >= len(line) || line[p] != 's' || p+1 >= len(line) {
		return false
	}
	arg := line[p+1:]
	delim, delimLen := e.commandDelimiter(arg)
	if delim == "" || delim == " " || delim == "\n" || delim == "\\" {
		return false
	}
	_, rest, patternClosed := scanRE(arg[delimLen:], delim)
	if !patternClosed {
		return false
	}
	_, _, replacementClosed, err := delimitedBy(rest, delim)
	return err == nil && replacementClosed
}
