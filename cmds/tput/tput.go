// Package tputcmd implements tput(1): report or emit terminal-dependent
// capabilities from the terminfo database.
//
// Two things about tput are easy to get wrong, and the conformance suite tests
// both directly.
//
// First, the EXIT STATUS is the primary output, not a side channel. POSIX
// gives five distinct statuses and a script reads them: 0 the capability was
// emitted (or a boolean is true), 1 a boolean is false or the capability is
// absent from this terminal, 2 a usage error, 3 nothing is known about the
// terminal type, 4 the operand is not a capability name at all. In particular
// "this terminal does not have that capability" (1) and "no such capability
// exists" (4) are different answers, so the implementation must know the full
// set of capability names independently of any one terminal's entry — that is
// what the shared package's capability tables are for.
//
// Second, a numeric capability may legitimately be zero, so "absent" can never
// be modelled by a zero value. The decoded entry stores presence in the map's
// key set, and `tput -T x xmc` prints 0 and exits 0 when the entry says 0.
//
// The database is read directly by cmds/internal/terminfo (shared with tabs):
// the compiled format is parsed there, the %-directive parameter language is
// interpreted there, and nothing is ever shelled out to.
package tputcmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"golang.org/x/term"

	"github.com/qiangli/coreutils/cmds/internal/terminfo"
	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "tput",
	Synopsis: "Query the terminfo database; emit terminal capability strings.",
	Usage: `tput [-T type] capname [parm...]
  tput [-T type] clear|init|reset [clear|init|reset...]
  tput [-T type] longname`,
}

// POSIX permits an implementation-defined terminal type when neither -T nor
// TERM supplies one.  "dumb" is deliberately conservative and is always
// available from this package's compiled-in terminfo table.
const defaultTerminalType = "dumb"

func init() {
	cmd.Usage += "\n\nWhere no terminfo database is installed, these types are answered from a\ncompiled-in fallback table: " + terminfo.BuiltinNames()
	cmd.Run = run
	tool.Register(cmd)
}

// Exit statuses, named because every one of them is specified behaviour that
// scripts branch on.
const (
	exitOK        = 0 // capability emitted, or boolean true
	exitAbsent    = 1 // boolean false, or capability absent from this terminal
	exitUsage     = 2 // usage error
	exitUnknownTT = 3 // no information about the terminal type
	exitBadCap    = 4 // the operand is not a terminfo capability name
	exitError     = 5 // an output or capability-processing error occurred
)

func run(rc *tool.RunContext, args []string) int {
	args = tool.AliasHelpVersion(args)
	fs := tool.NewFlags(cmd.Name)
	termType := fs.StringP("terminal", "T", "", "terminal `type` (default: $TERM, then dumb)")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	if len(operands) == 0 {
		return tool.UsageError(rc, cmd, "missing capability operand")
	}

	name := *termType
	if name == "" {
		name = rc.Getenv("TERM")
	}
	if name == "" {
		name = defaultTerminalType
	}

	e, err := terminfo.Load(rc.Getenv, name)
	if err != nil {
		fmt.Fprintf(rc.Err, "tput: unknown terminal %q\n", name)
		return exitUnknownTT
	}

	// POSIX permits a sequence of clear, init, and reset operands.  Restrict
	// that interpretation to an operation in the first position so the
	// long-standing capname [parm...] extension remains unambiguous.
	if isPOSIXOperation(operands[0]) {
		for _, operation := range operands {
			if !isPOSIXOperation(operation) {
				fmt.Fprintf(rc.Err, "tput: %q is not a POSIX operation\n", operation)
				return exitBadCap
			}
			if code := emitPOSIXOperation(rc, e, operation); code != exitOK {
				return code
			}
		}
		return exitOK
	}

	capName, parms := operands[0], operands[1:]
	switch capName {
	case "longname":
		// An SVr4 tput operand rather than a capability: it reports the
		// entry's description field.
		if _, err := fmt.Fprintln(rc.Out, e.LongName()); err != nil {
			fmt.Fprintf(rc.Err, "tput: write error: %v\n", err)
			return exitError
		}
		return exitOK
	}

	return emitCapability(rc, e, capName, parms)
}

func isPOSIXOperation(operand string) bool {
	return operand == "clear" || operand == "init" || operand == "reset"
}

func emitPOSIXOperation(rc *tool.RunContext, e *terminfo.Entry, operation string) int {
	switch operation {
	case "clear":
		code := emitCapability(rc, e, operation, nil)
		// POSIX operations unsupported by the selected terminal are not an
		// error and do not prevent processing later operands.  Keep exit 1
		// for absent capabilities requested through the capname extension.
		if code == exitAbsent {
			return exitOK
		}
		return code
	case "init":
		return emitStartup(rc, e, false)
	case "reset":
		return emitStartup(rc, e, true)
	default:
		panic("unreachable POSIX operation")
	}
}

// emitCapability handles the capname form, including the POSIX `clear`
// operand — which needs no special case, because clearing the screen IS the
// `clear` string capability.
func emitCapability(rc *tool.RunContext, e *terminfo.Entry, capName string, parms []string) int {
	kind := terminfo.KindOf(capName)
	if kind == terminfo.KindUnknown {
		// An entry may still define it as a user-defined (extended)
		// capability, which is a real capability of this terminal even though
		// no standard array reserves a slot for it.
		if kind = e.ExtendedKind(capName); kind == terminfo.KindUnknown {
			fmt.Fprintf(rc.Err, "tput: unknown terminfo capability %q\n", capName)
			return exitBadCap
		}
	}

	switch kind {
	case terminfo.KindBool:
		if e.Bool(capName) {
			return exitOK
		}
		return exitAbsent

	case terminfo.KindNum:
		v, ok := screenSize(rc, e, capName)
		if !ok {
			return exitAbsent
		}
		if _, err := fmt.Fprintln(rc.Out, v); err != nil {
			fmt.Fprintf(rc.Err, "tput: write error: %v\n", err)
			return exitError
		}
		return exitOK

	default: // terminfo.KindStr
		s, ok := e.Str(capName)
		if !ok {
			return exitAbsent
		}
		out, err := terminfo.Instantiate(s, parms)
		if err != nil {
			fmt.Fprintf(rc.Err, "tput: %s: %v\n", capName, err)
			return exitError
		}
		if _, err := rc.Out.Write([]byte(out)); err != nil {
			fmt.Fprintf(rc.Err, "tput: write error: %v\n", err)
			return exitError
		}
		return exitOK
	}
}

// screenSize resolves a numeric capability, letting the live window size win
// for lines and cols.
//
// A terminfo entry records the terminal's DEFAULT geometry — 80x24 for almost
// every entry ever written — while `tput cols` is nearly always asked in order
// to lay something out in the window that exists right now. The reference
// implementation therefore prefers $COLUMNS/$LINES, then the window size the
// kernel reports, and falls back to the entry. Reporting 80 into a 200-column
// window would be a wrong answer that looks right.
func screenSize(rc *tool.RunContext, e *terminfo.Entry, capName string) (int, bool) {
	var envVar string
	switch capName {
	case "cols":
		envVar = "COLUMNS"
	case "lines":
		envVar = "LINES"
	default:
		v, ok := e.Num(capName)
		return v, ok
	}

	if s := rc.Getenv(envVar); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			return n, true
		}
	}
	if w, h, ok := windowSize(rc); ok {
		if capName == "cols" && w > 0 {
			return w, true
		}
		if capName == "lines" && h > 0 {
			return h, true
		}
	}
	v, ok := e.Num(capName)
	return v, ok
}

// windowSize asks the kernel for the size of whichever of the invocation's
// streams is a terminal. Streams are inspected in the order the reference
// implementation uses; an embedded caller whose streams are all buffers simply
// gets no answer, which is correct — there is no window.
func windowSize(rc *tool.RunContext) (int, int, bool) {
	for _, s := range []any{rc.Out, rc.Err, rc.In} {
		f, ok := s.(*os.File)
		if !ok {
			continue
		}
		fd := int(f.Fd())
		if !term.IsTerminal(fd) {
			continue
		}
		w, h, err := term.GetSize(fd)
		if err != nil {
			continue
		}
		return w, h, true
	}
	return 0, 0, false
}

// emitStartup implements the `init` and `reset` operands.
//
// Both send a three-part sequence plus an optional file of literal bytes. The
// only difference is which capabilities supply the parts: reset prefers the
// rs1/rs2/rf/rs3 set and falls back to the init set per part, because most
// entries define only the init strings and a `reset` that emitted nothing
// would look like it worked.
func emitStartup(rc *tool.RunContext, e *terminfo.Entry, isReset bool) int {
	pick := func(resetCap, initCap string) string {
		if isReset {
			if s, ok := e.Str(resetCap); ok {
				return s
			}
		}
		s, _ := e.Str(initCap)
		return s
	}

	var b strings.Builder
	b.WriteString(terminfo.StripPadding(pick("rs1", "is1")))
	b.WriteString(terminfo.StripPadding(pick("rs2", "is2")))

	// The init/reset FILE capability names a file whose contents are written
	// verbatim. An unreadable file is skipped rather than fatal: the rest of
	// the sequence is still worth sending, and this matches what a terminal
	// user sees from the reference implementation.
	if path := pick("rf", "if"); path != "" {
		if data, err := os.ReadFile(rc.Path(path)); err == nil {
			b.Write(data)
		}
	}

	b.WriteString(terminfo.StripPadding(pick("rs3", "is3")))

	// An unsupported init/reset operation is a successful no-op.  In
	// particular, do not manufacture a write error by writing an empty slice.
	if b.Len() == 0 {
		return exitOK
	}
	if _, err := rc.Out.Write([]byte(b.String())); err != nil {
		fmt.Fprintf(rc.Err, "tput: write error: %v\n", err)
		return exitError
	}
	return exitOK
}
