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
// what caps.go is for.
//
// Second, a numeric capability may legitimately be zero, so "absent" can never
// be modelled by a zero value. The decoded entry stores presence in the map's
// key set, and `tput -T x xmc` prints 0 and exits 0 when the entry says 0.
//
// The database is read directly: the compiled terminfo format is parsed here
// (terminfo.go), the %-directive parameter language is interpreted here
// (tparm.go), and nothing is ever shelled out to.
package tputcmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"golang.org/x/term"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "tput",
	Synopsis: "Query the terminfo database; emit terminal capability strings.",
	Usage: `tput [-T type] capname [parm...]
  tput [-T type] clear|init|reset|longname`,
}

func init() {
	cmd.Usage += "\n\nWhere no terminfo database is installed, these types are answered from a\ncompiled-in fallback table: " + builtinNames()
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
)

func run(rc *tool.RunContext, args []string) int {
	args = tool.AliasHelpVersion(args)
	fs := tool.NewFlags(cmd.Name)
	termType := fs.StringP("terminal", "T", "", "terminal `type` (default: $TERM)")
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
		return tool.UsageError(rc, cmd, "no value for $TERM and no -T specified")
	}

	e, err := loadEntry(rc.Getenv, name)
	if err != nil {
		fmt.Fprintf(rc.Err, "tput: unknown terminal %q\n", name)
		return exitUnknownTT
	}

	capName, parms := operands[0], operands[1:]

	switch capName {
	case "init":
		return emitStartup(rc, e, false)
	case "reset":
		return emitStartup(rc, e, true)
	case "longname":
		// An SVr4 tput operand rather than a capability: it reports the
		// entry's description field.
		fmt.Fprintln(rc.Out, e.longName())
		return exitOK
	}

	return emitCapability(rc, e, capName, parms)
}

// emitCapability handles the capname form, including the POSIX `clear`
// operand — which needs no special case, because clearing the screen IS the
// `clear` string capability.
func emitCapability(rc *tool.RunContext, e *entry, capName string, parms []string) int {
	kind := kindOf(capName)
	if kind == capUnknown {
		// An entry may still define it as a user-defined (extended)
		// capability, which is a real capability of this terminal even though
		// no standard array reserves a slot for it.
		if kind = extendedKind(e, capName); kind == capUnknown {
			fmt.Fprintf(rc.Err, "tput: unknown terminfo capability %q\n", capName)
			return exitBadCap
		}
	}

	switch kind {
	case capBool:
		if e.bools[capName] {
			return exitOK
		}
		return exitAbsent

	case capNum:
		v, ok := screenSize(rc, e, capName)
		if !ok {
			return exitAbsent
		}
		fmt.Fprintln(rc.Out, v)
		return exitOK

	default: // capStr
		s, ok := e.strs[capName]
		if !ok {
			return exitAbsent
		}
		out, err := instantiate(s, parms)
		if err != nil {
			fmt.Fprintf(rc.Err, "tput: %s: %v\n", capName, err)
			return exitUsage
		}
		if _, err := rc.Out.Write([]byte(out)); err != nil {
			fmt.Fprintf(rc.Err, "tput: write error: %v\n", err)
			return exitUsage
		}
		return exitOK
	}
}

// extendedKind classifies a name that no standard array reserves but this
// entry defines as a user-defined capability.
func extendedKind(e *entry, name string) capKind {
	switch {
	case containsKey(e.bools, name):
		return capBool
	case containsKey(e.nums, name):
		return capNum
	case containsKey(e.strs, name):
		return capStr
	}
	return capUnknown
}

func containsKey[V any](m map[string]V, k string) bool {
	_, ok := m[k]
	return ok
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
func screenSize(rc *tool.RunContext, e *entry, capName string) (int, bool) {
	var envVar string
	switch capName {
	case "cols":
		envVar = "COLUMNS"
	case "lines":
		envVar = "LINES"
	default:
		v, ok := e.nums[capName]
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
	v, ok := e.nums[capName]
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

// instantiate applies the operands to a capability string and strips padding.
//
// With NO operands the string is emitted verbatim rather than run through the
// parameter engine. That is the reference behaviour and it is deliberate:
// `tput cup` with no arguments prints the uninstantiated `\E[%i%p1%d;%p2%dH`,
// which is how a script asks for the raw template. Running the engine on it
// instead would silently substitute zeros and print `\E[1;1H` — a valid-looking
// escape sequence that is not what was asked for.
func instantiate(s string, parms []string) (string, error) {
	if len(parms) == 0 {
		return stripPadding(s), nil
	}
	out, err := tparm(s, parseParams(parms))
	if err != nil {
		return "", err
	}
	return stripPadding(out), nil
}

// parseParams converts command-line operands to stack values. An operand that
// reads as a decimal integer is passed as a number, because that is what the
// arithmetic directives in a capability like `cup` expect; everything else is
// passed as a string, which is what capabilities like `pfkey` expect.
func parseParams(parms []string) []value {
	if len(parms) > 9 {
		parms = parms[:9]
	}
	out := make([]value, len(parms))
	for i, p := range parms {
		if n, err := strconv.Atoi(p); err == nil {
			out[i] = numValue(n)
			continue
		}
		out[i] = strValue(p)
	}
	return out
}

// stripPadding removes `$<...>` delay specifications.
//
// A delay is an instruction to the OUTPUT DRIVER — wait this many
// milliseconds, or send this many pad characters at the current line speed —
// and it is meaningful only when writing to a real terminal at a known baud
// rate. tput's output is overwhelmingly captured into a shell variable, where
// emitting the literal text "$<5>" would corrupt it and where sleeping would
// just make the script slower. Modern terminals declare no delays at all, so
// this is invisible for them; for the ones that do, dropping the delay is the
// behaviour a script sees from the reference implementation writing to a pipe.
//
// Text that merely looks like a delay is left alone: the closing '>' and a
// well-formed body are both required.
func stripPadding(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] != '$' || i+1 >= len(s) || s[i+1] != '<' {
			b.WriteByte(s[i])
			i++
			continue
		}
		end := strings.IndexByte(s[i+2:], '>')
		if end < 0 || !isDelayBody(s[i+2:i+2+end]) {
			b.WriteByte(s[i])
			i++
			continue
		}
		i += 2 + end + 1
	}
	return b.String()
}

// isDelayBody reports whether body is a delay: a decimal number with at most
// one fractional digit, optionally followed by '*' (per affected line) and/or
// '/' (mandatory even with xon/xoff), in either order.
func isDelayBody(body string) bool {
	i := 0
	digits := 0
	for i < len(body) && body[i] >= '0' && body[i] <= '9' {
		i++
		digits++
	}
	if digits == 0 {
		return false
	}
	if i < len(body) && body[i] == '.' {
		i++
		for i < len(body) && body[i] >= '0' && body[i] <= '9' {
			i++
		}
	}
	for i < len(body) && (body[i] == '*' || body[i] == '/') {
		i++
	}
	return i == len(body)
}

// emitStartup implements the `init` and `reset` operands.
//
// Both send a three-part sequence plus an optional file of literal bytes. The
// only difference is which capabilities supply the parts: reset prefers the
// rs1/rs2/rf/rs3 set and falls back to the init set per part, because most
// entries define only the init strings and a `reset` that emitted nothing
// would look like it worked.
func emitStartup(rc *tool.RunContext, e *entry, isReset bool) int {
	pick := func(resetCap, initCap string) string {
		if isReset {
			if s, ok := e.strs[resetCap]; ok {
				return s
			}
		}
		return e.strs[initCap]
	}

	var b strings.Builder
	b.WriteString(stripPadding(pick("rs1", "is1")))
	b.WriteString(stripPadding(pick("rs2", "is2")))

	// The init/reset FILE capability names a file whose contents are written
	// verbatim. An unreadable file is skipped rather than fatal: the rest of
	// the sequence is still worth sending, and this matches what a terminal
	// user sees from the reference implementation.
	if path := pick("rf", "if"); path != "" {
		if data, err := os.ReadFile(rc.Path(path)); err == nil {
			b.Write(data)
		}
	}

	b.WriteString(stripPadding(pick("rs3", "is3")))

	if _, err := rc.Out.Write([]byte(b.String())); err != nil {
		fmt.Fprintf(rc.Err, "tput: write error: %v\n", err)
		return exitUsage
	}
	return exitOK
}
