package gosed

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"
	"unicode/utf8"
)

var fullBuffer = errors.New("FullBuffer")

func writeString(svm *vm, str string) error {
	var err error
	end := len(svm.output)
	src := str
	srclen := len(src)
	if end < srclen {
		src = src[:end]
		srclen = end
		svm.overflow += str[end:]
		err = fullBuffer
	}
	for i := 0; i < srclen; i++ {
		svm.output[i] = src[i]
	}

	svm.output = svm.output[srclen:]
	return err
}

func cmd_quit(svm *vm) error {
	return io.EOF
}

// ---------------------------------------------------
func cmd_swap(svm *vm) error {
	svm.pat, svm.hold = svm.hold, svm.pat
	svm.ip++
	return nil
}

// ---------------------------------------------------
func cmd_get(svm *vm) error {
	svm.pat = svm.hold
	svm.ip++
	return nil
}

// ---------------------------------------------------
func cmd_hold(svm *vm) error {
	svm.hold = svm.pat
	svm.ip++
	return nil
}

// ---------------------------------------------------
func cmd_getapp(svm *vm) error {
	svm.pat = strings.Join([]string{svm.pat, svm.hold}, "\n")
	svm.ip++
	return nil
}

// ---------------------------------------------------
func cmd_holdapp(svm *vm) error {
	svm.hold = strings.Join([]string{svm.hold, svm.pat}, "\n")
	svm.ip++
	return nil
}

// ---------------------------------------------------
// newBranch generates branch instructions with specific
// targets
func cmd_newBranch(target int) instruction {
	return func(svm *vm) error {
		svm.ip = target
		return nil
	}
}

// ---------------------------------------------------
// newChangedBranch generates branch instructions with specific
// targets that only trigger on modified pattern spaces
func cmd_newChangedBranch(target int) instruction {
	return func(svm *vm) error {
		if svm.modified {
			svm.ip = target
			svm.modified = false
		} else {
			svm.ip++
		}
		return nil
	}
}

// ---------------------------------------------------
func cmd_print(svm *vm) error {
	svm.ip++

	writeString(svm, svm.pat)
	if svm.patNL {
		return writeString(svm, "\n")
	}
	return nil
}

// ---------------------------------------------------
func cmd_printFirstLine(svm *vm) error {
	svm.ip++

	idx := strings.IndexRune(svm.pat, '\n')

	if idx == -1 {
		idx = len(svm.pat)
	}

	writeString(svm, svm.pat[:idx])
	if idx < len(svm.pat) || svm.patNL {
		return writeString(svm, "\n")
	}
	return nil
}

// ---------------------------------------------------
func cmd_deleteFirstLine(svm *vm) (err error) {
	idx := strings.IndexRune(svm.pat, '\n')

	if idx == -1 {
		svm.pat = ""
		svm.ip = 0 // go back and fillNext
	} else {
		svm.pat = svm.pat[idx+1:]
		svm.ip = 1 // restart, but skip filling
	}

	return nil
}

// ---------------------------------------------------
func cmd_lineno(svm *vm) error {
	svm.ip++
	var lineno = fmt.Sprintf("%d\n", svm.lineno)
	return writeString(svm, lineno)
}

// cmd_list writes the pattern space in the unambiguous form required by the
// POSIX l command. Embedded newlines terminate one displayed line, while
// controls and backslashes receive their standard escapes.
func cmd_list(svm *vm) error {
	svm.ip++
	var b strings.Builder
	for i := 0; i < len(svm.pat); {
		r, size := utf8.DecodeRuneInString(svm.pat[i:])
		if r == utf8.RuneError && size == 1 {
			fmt.Fprintf(&b, "\\%03o", svm.pat[i])
			i++
			continue
		}
		i += size
		switch r {
		case '\\':
			b.WriteString("\\\\")
		case '\a':
			b.WriteString("\\a")
		case '\b':
			b.WriteString("\\b")
		case '\f':
			b.WriteString("\\f")
		case '\n':
			if err := writeString(svm, b.String()+"$\n"); err != nil {
				return err
			}
			b.Reset()
		case '\r':
			b.WriteString("\\r")
		case '\t':
			b.WriteString("\\t")
		case '\v':
			b.WriteString("\\v")
		default:
			if unicode.IsPrint(r) {
				b.WriteRune(r)
			} else {
				fmt.Fprintf(&b, "\\%03o", r)
			}
		}
	}
	return writeString(svm, b.String()+"$\n")
}

// ---------------------------------------------------
func cmd_fillNext(svm *vm) error {
	var err error

	// first, put out any stored-up 'a\'ppended text:
	if svm.appl != nil {
		err = writeString(svm, *svm.appl)
		svm.appl = nil
		if err != nil {
			return err // ok, since IP unchanged
		}
	}

	// just return if we're at EOF
	if svm.lastl {
		return io.EOF
	}

	// otherwise, copy nxtl to the pattern space and
	// refill.
	svm.ip++

	svm.pat = svm.nxtl
	svm.patNL = svm.nxtlNL
	svm.lineno++
	svm.modified = false

	var line string
	line, err = svm.input.ReadString('\n')
	if len(line) > 0 {
		svm.nxtlNL = strings.HasSuffix(line, "\n")
		if svm.nxtlNL {
			line = strings.TrimSuffix(line, "\n")
		}
		svm.nxtl = line
	} else {
		svm.nxtl = ""
		svm.nxtlNL = false
	}

	if err == io.EOF {
		if len(svm.nxtl) == 0 {
			svm.lastl = true
		}
		err = nil
	}

	return err
}

func cmd_fillNextAppend(svm *vm) error {
	var lines = make([]string, 2)
	lines[0] = svm.pat
	err := cmd_fillNext(svm) // usually increments ip for us...
	if err == nil {
		lines[1] = svm.pat
		svm.pat = strings.Join(lines, "\n")
	} else if err == io.EOF {
		// we have to increment ip when we are ignoring EOF
		svm.ip++
	}
	return nil
}

// --------------------------------------------------

type cmd_simplecond struct {
	cond     condition // the condition to check
	metloc   int       // where to jump if the condition is met
	unmetloc int       // where to jump if the condition is not met
}

func (c *cmd_simplecond) run(svm *vm) error {
	if c.cond.isMet(svm) {
		svm.ip = c.metloc
	} else {
		svm.ip = c.unmetloc
	}
	return nil
}

// --------------------------------------------------
type cmd_twocond struct {
	start    condition // the condition that begines the block
	end      condition // the condition that ends the block
	metloc   int       // where to jump if the condition is met
	unmetloc int       // where to jump if the condition is not met
	isOn     bool      // are we active already?
	offFrom  int       // if we saw the end condition, what line was it on?
}

func newTwoCond(c1 condition, c2 condition, metloc int, unmetloc int) *cmd_twocond {
	return &cmd_twocond{c1, c2, metloc, unmetloc, false, 0}
}

// isLastLine is here to support multi-line "c\" commands.
// The command needs to know when it's the end of the
// section so it can do the replacement.
func (c *cmd_twocond) isLastLine(svm *vm) bool {
	return c.isOn && (c.offFrom == svm.lineno)
}

func (c *cmd_twocond) run(svm *vm) error {
	if c.isOn && (c.offFrom > 0) && (c.offFrom < svm.lineno) {
		c.isOn = false
		c.offFrom = 0
	}

	if !c.isOn {
		if c.start.isMet(svm) {
			svm.ip = c.metloc
			c.isOn = true
		} else {
			svm.ip = c.unmetloc
		}
	} else {
		if c.end.isMet(svm) {
			c.offFrom = svm.lineno
		}
		svm.ip = c.metloc
	}
	return nil
}

// --------------------------------------------------
func cmd_newChanger(text string, guard *cmd_twocond) instruction {
	return func(svm *vm) error {
		svm.ip = 0 // go to the the next cycle

		var err error
		if (guard == nil) || guard.isLastLine(svm) {
			err = writeString(svm, text)
		}
		return err
	}
}

// --------------------------------------------------
func cmd_newAppender(text string) instruction {
	return func(svm *vm) error {
		svm.ip++
		if svm.appl == nil {
			svm.appl = &text
		} else {
			var newstr = *svm.appl + text
			svm.appl = &newstr
		}
		return nil
	}
}

// --------------------------------------------------
func cmd_newInserter(text string) instruction {
	return func(svm *vm) error {
		svm.ip++
		return writeString(svm, text)
	}
}

// --------------------------------------------------
// The 'r' command queues a file's contents for output at the end of the
// current cycle. GNU sed treats an unreadable file as empty.
func cmd_newReader(filename string, readFile ReadFileFunc) instruction {
	return func(svm *vm) error {
		svm.ip++
		bytes, err := readFile(filename)
		if err != nil {
			return nil
		}
		text := string(bytes)
		if svm.appl == nil {
			svm.appl = &text
		} else {
			newstr := *svm.appl + text
			svm.appl = &newstr
		}
		return nil
	}
}

// --------------------------------------------------
// The 'w' command appends the current pattern space
// to the named filsvm.  In this implementation, it opens
// the file for appending, writes the file, and then
// closes the filsvm.  This appears to be consistent with
// what OS X sed does.
func defaultWriteFile(filename, pattern string) error {
	f, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err = f.WriteString(pattern); err != nil {
		return err
	}
	_, err = f.WriteString("\n")
	return err
}

func cmd_newWriter(filename string, writeFile WriteFileFunc) instruction {
	return func(svm *vm) error {
		svm.ip++
		return writeFile(filename, svm.pat)
	}
}
