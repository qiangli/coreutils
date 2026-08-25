// Package tabscmd implements tabs(1): clear the terminal's hardware tab stops
// and set new ones at the requested positions.
//
// The utility is a WRITER, not a query: everything it does is bytes on stdout
// addressed to the terminal, so the implementation is "resolve the requested
// column list, then render it with this terminal's capabilities". Two things
// follow from that and are easy to get wrong.
//
// First, THE COLUMN LISTS ARE THE SPECIFICATION. The nine preset formats
// (-a, -a2, -c, -c2, -c3, -f, -p, -s, -u) are literal tables from POSIX, and a
// transcription slip there produces plausible output that no self-consistent
// test can catch — so presetsTable below is pinned by a test that spells every
// list out a second time, transcribed independently.
//
// Second, A TAB STOP IS SET WHERE THE CURSOR IS. terminfo's set-tab capability
// (hts) has no column parameter, so the emitted sequence is: return to column
// one, clear every stop (tbc), then for each requested column advance the
// cursor with spaces and set a stop. That is also why the left margin composes
// for free — once the margin moves, a carriage return lands on it and every
// subsequent column is counted from there, which is exactly what "move all the
// tabs over n columns" means.
//
// The terminfo database is read through cmds/internal/terminfo, the same
// reader tput uses; nothing is ever shelled out to.
package tabscmd

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
	Name:     "tabs",
	Synopsis: "Clear the terminal's hardware tab stops and set new ones.",
	Usage: `tabs [-n|-a|-a2|-c|-c2|-c3|-f|-p|-s|-u] [+m[n]] [-T type]
  tabs [-T type] [+[n]] n1[,n2,...]`,
}

func init() {
	cmd.Run = run
	tool.Register(cmd)
}

const (
	exitOK    = 0
	exitError = 1 // POSIX: >0 if an error occurred
)

// defaultRepeat is what `tabs` with no operands means: POSIX says the default
// usage "shall be equivalent to tabs -8".
const defaultRepeat = 8

// defaultColumns is the width assumed when neither $COLUMNS, the live window,
// nor the terminfo entry says otherwise. Repetitive stops run out to the
// screen width, so a width is always needed.
const defaultColumns = 80

// presetsTable is the POSIX preset formats, transcribed from the standard's
// OPTIONS section. Each row is the option spelling, its tab-stop columns, and
// the language it was defined for.
//
// These lists are DATA, not behaviour: getting one wrong yields a working
// program that sets the wrong stops.
var presetsTable = []struct {
	flag  string
	stops []int
	why   string
}{
	{"-a", []int{1, 10, 16, 36, 72}, "assembler"},
	{"-a2", []int{1, 10, 16, 40, 72}, "assembler, alternate"},
	{"-c", []int{1, 8, 12, 16, 20, 55}, "COBOL, normal format"},
	{"-c2", []int{1, 6, 10, 14, 49}, "COBOL, compact format"},
	{"-c3", []int{1, 6, 10, 14, 18, 22, 26, 30, 34, 38, 42, 46, 50, 54, 58, 62, 67}, "COBOL, compact format, more tabs"},
	{"-f", []int{1, 7, 11, 15, 19, 23}, "FORTRAN"},
	{"-p", []int{1, 5, 9, 13, 17, 21, 25, 29, 33, 37, 41, 45, 49, 53, 57, 61}, "PL/1"},
	{"-s", []int{1, 10, 55}, "SNOBOL"},
	{"-u", []int{1, 12, 20, 44}, "assembler, alternate"},
}

func preset(flag string) ([]int, bool) {
	for _, p := range presetsTable {
		if p.flag == flag {
			return p.stops, true
		}
	}
	return nil, false
}

// spec is the parsed request: WHAT stops were asked for, independent of how any
// terminal renders them.
type spec struct {
	// stops is an explicit column list (from a preset, or from the operand
	// list). nil means the repetitive form applies.
	stops []int
	// repeat is the -n uniform spacing. 0 means "clear the stops and set
	// none", which is what -0 asks for.
	repeat int
	// margin is the +m[n] request; -1 when no margin was asked for.
	margin int
	// source names the format option the stops came from, for diagnostics.
	// Empty means no format option was given.
	source string
}

func run(rc *tool.RunContext, args []string) int {
	sp, rest, code := prescan(rc, args)
	if code >= 0 {
		return code
	}

	fs := tool.NewFlags(cmd.Name)
	termType := fs.StringP("terminal", "T", "", "terminal `type` (default: $TERM, then ansi)")
	operands, code := tool.Parse(rc, cmd, fs, rest)
	if code >= 0 {
		return code
	}

	if len(operands) > 0 {
		if sp.source != "" {
			return tool.UsageError(rc, cmd, "cannot combine a tab-stop list with %s", sp.source)
		}
		stops, err := parseStopList(operands)
		if err != nil {
			fmt.Fprintf(rc.Err, "tabs: %v\n", err)
			return exitError
		}
		sp.stops = stops
	}

	name := *termType
	if name == "" {
		name = rc.Getenv("TERM")
	}
	if name == "" {
		name = "ansi"
	}
	e, err := terminfo.Load(rc.Getenv, name)
	if err != nil {
		fmt.Fprintf(rc.Err, "tabs: unknown terminal %q\n", name)
		return exitError
	}

	return emit(rc, e, sp)
}

// prescan pulls out the arguments pflag structurally cannot parse — the preset
// format flags (-a2, -c3, …), the repetitive spec (-8), and the +m margin —
// and hands the rest to the standard flag machinery, which is what keeps
// --help, --version and -T behaving like they do in every other tool here.
//
// Returns (spec, remaining args, -1) to proceed, or an exit code to return.
func prescan(rc *tool.RunContext, args []string) (spec, []string, int) {
	sp := spec{repeat: defaultRepeat, margin: -1}
	var rest []string
	endOfFlags := false
	sawOperand := false

	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case endOfFlags:
			rest = append(rest, a)
			sawOperand = true

		case a == "--":
			endOfFlags = true
			rest = append(rest, a)

		// -T takes a separate value that must not be mistaken for a flag.
		case a == "-T":
			rest = append(rest, a)
			if i+1 < len(args) {
				i++
				rest = append(rest, args[i])
			}

		case isFormatFlag(a):
			if sp.source != "" {
				return sp, nil, tool.UsageError(rc, cmd, "%s conflicts with %s", a, sp.source)
			}
			sp.source = a
			if stops, ok := preset(a); ok {
				sp.stops = stops
				break
			}
			// A repetitive spec: -0 clears without setting, -n sets a stop
			// every n columns. POSIX specifies a single digit; every historic
			// implementation accepts more, and refusing `-16` would reject a
			// request whose meaning is unambiguous.
			n, err := strconv.Atoi(a[1:])
			if err != nil || n < 0 {
				return sp, nil, tool.UsageError(rc, cmd, "invalid repetitive tab spec %q", a)
			}
			sp.stops = nil
			sp.repeat = n

		case strings.HasPrefix(a, "+"):
			// A +-prefixed word means two different things by POSITION. Before
			// the tab-stop list it is the left-margin request (+m[n], or its
			// bare +[n] spelling); once the list has started it is an
			// INCREMENT on the previous stop, which parseStopList owns. There
			// is no other way to tell them apart, and reading an increment as
			// a margin would silently drop a stop.
			if sawOperand {
				rest = append(rest, a)
				break
			}
			n, err := parseMargin(a)
			if err != nil {
				return sp, nil, tool.UsageError(rc, cmd, "%v", err)
			}
			sp.margin = n

		default:
			rest = append(rest, a)
			if !strings.HasPrefix(a, "-") {
				sawOperand = true
			}
		}
	}
	return sp, rest, -1
}

// isFormatFlag reports whether a is one of the mutually exclusive format
// options: a preset language, or the repetitive -n spec.
func isFormatFlag(a string) bool {
	if _, ok := preset(a); ok {
		return true
	}
	if len(a) < 2 || a[0] != '-' {
		return false
	}
	for _, c := range a[1:] {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// parseMargin reads +m[n] / +[n]. An omitted value is 10, per the option's
// historic definition; +m0 asks for the leftmost margin.
func parseMargin(a string) (int, error) {
	body := strings.TrimPrefix(a, "+")
	body = strings.TrimPrefix(body, "m")
	if body == "" {
		return 10, nil
	}
	n, err := strconv.Atoi(body)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("invalid margin %q", a)
	}
	return n, nil
}

// parseStopList reads the explicit tab-stop operand.
//
// POSIX allows a <comma> OR a <blank> as the separator inside the single
// argument; the shell has usually already split on the blanks, so several
// operands are accepted as one continued list. A value prefixed with '+' is an
// INCREMENT on the previous stop rather than an absolute column, and the whole
// list must be strictly ascending — a request that goes backwards is refused
// rather than silently reordered, because the stops the caller would then get
// are not the ones asked for.
func parseStopList(operands []string) ([]int, error) {
	var fields []string
	for _, op := range operands {
		for _, part := range strings.Split(op, ",") {
			fields = append(fields, strings.Fields(part)...)
		}
	}
	if len(fields) == 0 {
		return nil, fmt.Errorf("empty tab-stop list")
	}

	stops := make([]int, 0, len(fields))
	prev := 0
	for i, f := range fields {
		if strings.HasPrefix(f, "+") {
			if i == 0 {
				return nil, fmt.Errorf("the first tab stop %q cannot be an increment", f)
			}
			d, err := strconv.Atoi(f[1:])
			if err != nil {
				return nil, fmt.Errorf("invalid tab stop %q", f)
			}
			if d <= 0 {
				return nil, fmt.Errorf("tab-stop increment %q must be positive", f)
			}
			prev += d
			stops = append(stops, prev)
			continue
		}
		n, err := strconv.Atoi(f)
		if err != nil {
			return nil, fmt.Errorf("invalid tab stop %q", f)
		}
		if n <= 0 {
			return nil, fmt.Errorf("tab stop %q must be a positive column", f)
		}
		if n <= prev {
			return nil, fmt.Errorf("tab stops must be strictly ascending: %d follows %d", n, prev)
		}
		prev = n
		stops = append(stops, n)
	}
	return stops, nil
}

// resolveStops turns the spec into the column list to set on this terminal.
func resolveStops(rc *tool.RunContext, e *terminfo.Entry, sp spec) []int {
	if sp.stops != nil {
		return sp.stops
	}
	if sp.repeat == 0 {
		return nil
	}
	// Repetitive stops run out to the screen width, and the first stop PAST
	// the edge is included: a tab typed in the last cell still has somewhere
	// to go. That is also what the historic implementations emit.
	cols := columns(rc, e)
	var stops []int
	for c := 1; ; c += sp.repeat {
		stops = append(stops, c)
		if c > cols {
			break
		}
	}
	return stops
}

// columns resolves the screen width the way tput does: the environment first,
// then the live window, then the entry's declared default. A terminfo entry
// records the terminal's DEFAULT geometry, which is not the window the caller
// is actually laying out.
func columns(rc *tool.RunContext, e *terminfo.Entry) int {
	if s := rc.Getenv("COLUMNS"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			return n
		}
	}
	if w, ok := windowWidth(rc); ok && w > 0 {
		return w
	}
	if v, ok := e.Num("cols"); ok && v > 0 {
		return v
	}
	return defaultColumns
}

func windowWidth(rc *tool.RunContext) (int, bool) {
	for _, s := range []any{rc.Out, rc.Err, rc.In} {
		f, ok := s.(*os.File)
		if !ok {
			continue
		}
		fd := int(f.Fd())
		if !term.IsTerminal(fd) {
			continue
		}
		if w, _, err := term.GetSize(fd); err == nil {
			return w, true
		}
	}
	return 0, false
}

// emit renders the request with this terminal's capabilities.
func emit(rc *tool.RunContext, e *terminfo.Entry, sp spec) int {
	stops := resolveStops(rc, e, sp)

	tbc, hasClear := capString(e, "tbc")
	if !hasClear {
		fmt.Fprintln(rc.Err, "tabs: terminal cannot clear tabs")
		return exitError
	}
	hts, hasSet := capString(e, "hts")
	if len(stops) > 0 && !hasSet {
		fmt.Fprintln(rc.Err, "tabs: terminal cannot set tabs")
		return exitError
	}
	cr, hasCR := capString(e, "cr")
	if !hasCR {
		cr = "\r"
	}

	var b strings.Builder
	status := exitOK

	if sp.margin >= 0 {
		if !writeMargin(&b, e, cr, sp.margin) {
			// The stops are still set: the margin is an optional adjustment,
			// the stops are the job. But the failure is REPORTED — and carried
			// into the exit status — because a request that did not happen
			// must never look like one that did.
			fmt.Fprintln(rc.Err, "tabs: terminal cannot set left margin")
			status = exitError
		}
	}

	b.WriteString(cr)
	b.WriteString(tbc)
	col := 1
	for _, stop := range stops {
		if stop > col {
			b.WriteString(strings.Repeat(" ", stop-col))
			col = stop
		}
		b.WriteString(hts)
	}
	b.WriteString(cr)

	if _, err := rc.Out.Write([]byte(b.String())); err != nil {
		fmt.Fprintf(rc.Err, "tabs: write error: %v\n", err)
		return exitError
	}
	return status
}

// writeMargin moves the left margin to column n+1, reporting whether the
// terminal can do it at all.
//
// The cursor-positioned capability is preferred over the parameterized one
// because positioning is unambiguous: a carriage return plus n spaces puts the
// cursor in column n+1, and smgl makes wherever the cursor is the margin.
func writeMargin(b *strings.Builder, e *terminfo.Entry, cr string, n int) bool {
	if n == 0 {
		if mgc, ok := capString(e, "mgc"); ok {
			b.WriteString(mgc)
			return true
		}
	}
	if smgl, ok := capString(e, "smgl"); ok {
		b.WriteString(cr)
		if n > 0 {
			b.WriteString(strings.Repeat(" ", n))
		}
		b.WriteString(smgl)
		return true
	}
	if smglp, ok := e.Str("smglp"); ok {
		// set_left_margin_parm addresses the margin column directly, and
		// terminfo counts that parameter from zero — so column n+1 is n.
		out, err := terminfo.Instantiate(smglp, []string{strconv.Itoa(n)})
		if err != nil {
			return false
		}
		b.WriteString(out)
		return true
	}
	return false
}

// capString reads a string capability with its padding removed. A delay is an
// instruction to the output driver, not text, and tabs' output is routinely
// captured into a variable or a file.
func capString(e *terminfo.Entry, name string) (string, bool) {
	s, ok := e.Str(name)
	if !ok {
		return "", false
	}
	return terminfo.StripPadding(s), true
}
