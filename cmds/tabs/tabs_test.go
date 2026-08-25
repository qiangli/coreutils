package tabscmd

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/cmds/internal/terminfo"
	"github.com/qiangli/coreutils/tool"
)

type tabsFailWriter struct{}

func (tabsFailWriter) Write([]byte) (int, error) { return 0, errors.New("injected write failure") }

// The fixture terminal's capabilities are the real xterm ones, so a rendered
// sequence in a test below is byte-for-byte what a terminal would receive.
const (
	capCR  = "\r"
	capTBC = "\x1b[3g"
	capHTS = "\x1bH"
	capMGC = "\x1b[?69l"
	capSML = "\x1b[s" // stand-in: xterm has no smgl, so the fixture supplies one
)

// demoTerm is the terminal every test drives: it can clear tabs, set tabs,
// return the carriage, and move its left margin. Tests that need a terminal
// LACKING one of those build a narrower fixture.
func demoTerm() terminfo.Fixture {
	return terminfo.Fixture{
		Names: []string{"demo", "a demo terminal"},
		Nums:  map[string]int{"cols": 80},
		Strs: map[string]string{
			"cr":   capCR,
			"tbc":  capTBC,
			"hts":  capHTS,
			"mgc":  capMGC,
			"smgl": capSML,
		},
	}
}

func fixtureDir(t *testing.T, extra ...terminfo.Fixture) string {
	t.Helper()
	dir := t.TempDir()
	writeEntry(t, dir, "demo", demoTerm())
	for _, f := range extra {
		writeEntry(t, dir, f.Names[0], f)
	}
	return dir
}

func writeEntry(t *testing.T, dir, name string, f terminfo.Fixture) {
	t.Helper()
	if err := terminfo.WriteEntry(dir, name, f, false); err != nil {
		t.Fatal(err)
	}
}

// runIn drives the command against a private terminfo database, so no test
// depends on which ncurses the host happens to ship.
//
// Note the order: run FIRST, then read the buffers. Returning
// `out.String(), errb.String(), run(...)` would evaluate the strings before
// the command had written anything, and every assertion would silently
// compare against "".
func runIn(t *testing.T, dir string, env []string, args ...string) (string, string, int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Dir:   t.TempDir(),
		Env:   append([]string{"TERMINFO=" + dir}, env...),
		Stdio: tool.Stdio{Out: &out, Err: &errb},
	}
	code := run(rc, args)
	return out.String(), errb.String(), code
}

// want renders the sequence a terminal should receive for a column list: a
// carriage return, a clear-all-tabs, then the cursor walked to each column
// with spaces and a set-tab there, then a final carriage return.
func want(stops ...int) string {
	var b strings.Builder
	b.WriteString(capCR)
	b.WriteString(capTBC)
	col := 1
	for _, s := range stops {
		if s > col {
			b.WriteString(strings.Repeat(" ", s-col))
			col = s
		}
		b.WriteString(capHTS)
	}
	b.WriteString(capCR)
	return b.String()
}

// THE PRESET LISTS ARE THE SPECIFICATION, and this is the check that they were
// not mistyped. The columns below are transcribed a SECOND time, directly from
// the POSIX tabs OPTIONS section, so agreement between the table and this test
// is agreement between two independent readings of the standard rather than a
// program agreeing with itself.
func TestPresetColumnsMatchPOSIX(t *testing.T) {
	posix := map[string][]int{
		"-a":  {1, 10, 16, 36, 72},
		"-a2": {1, 10, 16, 40, 72},
		"-c":  {1, 8, 12, 16, 20, 55},
		"-c2": {1, 6, 10, 14, 49},
		"-c3": {1, 6, 10, 14, 18, 22, 26, 30, 34, 38, 42, 46, 50, 54, 58, 62, 67},
		"-f":  {1, 7, 11, 15, 19, 23},
		"-p":  {1, 5, 9, 13, 17, 21, 25, 29, 33, 37, 41, 45, 49, 53, 57, 61},
		"-s":  {1, 10, 55},
		"-u":  {1, 12, 20, 44},
	}
	if len(presetsTable) != len(posix) {
		t.Fatalf("presetsTable has %d formats, POSIX defines %d", len(presetsTable), len(posix))
	}
	for _, p := range presetsTable {
		wantList, ok := posix[p.flag]
		if !ok {
			t.Errorf("%s is not a POSIX preset format", p.flag)
			continue
		}
		if !equal(p.stops, wantList) {
			t.Errorf("%s (%s) = %v, POSIX says %v", p.flag, p.why, p.stops, wantList)
		}
		// Every preset is a strictly ascending list starting at column 1;
		// a slip that still parses would usually break one of those.
		if len(p.stops) == 0 || p.stops[0] != 1 {
			t.Errorf("%s must start at column 1, got %v", p.flag, p.stops)
		}
		for i := 1; i < len(p.stops); i++ {
			if p.stops[i] <= p.stops[i-1] {
				t.Errorf("%s is not ascending at index %d: %v", p.flag, i, p.stops)
			}
		}
	}
}

func TestTabsOutputWriteErrorIsReported(t *testing.T) {
	dir := fixtureDir(t)
	var errb bytes.Buffer
	rc := &tool.RunContext{
		Dir: t.TempDir(), Env: []string{"TERMINFO=" + dir},
		Stdio: tool.Stdio{Out: tabsFailWriter{}, Err: &errb},
	}
	if code := run(rc, []string{"-T", "demo", "8"}); code != exitError {
		t.Fatalf("exit %d, want %d", code, exitError)
	}
	if !strings.Contains(errb.String(), "write error") {
		t.Errorf("stderr %q lacks write failure", errb.String())
	}
}

// Each preset must actually REACH the wire, not merely sit in a table.
func TestPresetFormats(t *testing.T) {
	dir := fixtureDir(t)
	for _, p := range presetsTable {
		t.Run(p.flag, func(t *testing.T) {
			out, errb, code := runIn(t, dir, nil, "-T", "demo", p.flag)
			if code != exitOK {
				t.Fatalf("exit %d, stderr %q", code, errb)
			}
			if got := want(p.stops...); out != got {
				t.Errorf("output %q, want %q", out, got)
			}
		})
	}
}

// The repetitive form: a stop every n columns starting at 1, out to the screen
// width plus the first stop past it (a tab typed in the last cell still has
// somewhere to go).
func TestRepetitiveSpec(t *testing.T) {
	dir := fixtureDir(t)
	for _, c := range []struct {
		name  string
		args  []string
		env   []string
		stops []int
	}{
		{"-8 on a 40-column screen", []string{"-8"}, []string{"COLUMNS=40"}, []int{1, 9, 17, 25, 33, 41}},
		{"-4 on a 12-column screen", []string{"-4"}, []string{"COLUMNS=12"}, []int{1, 5, 9, 13}},
		{"-16 on a 40-column screen", []string{"-16"}, []string{"COLUMNS=40"}, []int{1, 17, 33, 49}},
		{"the default is -8", nil, []string{"COLUMNS=40"}, []int{1, 9, 17, 25, 33, 41}},
		{"the width comes from the entry when unset", []string{"-40"}, nil, []int{1, 41, 81}},
	} {
		t.Run(c.name, func(t *testing.T) {
			args := append([]string{"-T", "demo"}, c.args...)
			out, errb, code := runIn(t, dir, c.env, args...)
			if code != exitOK {
				t.Fatalf("exit %d, stderr %q", code, errb)
			}
			if got := want(c.stops...); out != got {
				t.Errorf("output %q, want %q", out, got)
			}
		})
	}
}

// -0 is the one spec that sets nothing: it clears and stops there.
func TestZeroClearsWithoutSetting(t *testing.T) {
	dir := fixtureDir(t)
	out, errb, code := runIn(t, dir, nil, "-T", "demo", "-0")
	if code != exitOK {
		t.Fatalf("exit %d, stderr %q", code, errb)
	}
	if got := capCR + capTBC + capCR; out != got {
		t.Errorf("output %q, want %q — -0 must not set a stop", out, got)
	}
}

func TestExplicitList(t *testing.T) {
	dir := fixtureDir(t)
	for _, c := range []struct {
		name  string
		args  []string
		stops []int
	}{
		{"comma separated", []string{"1,5,9"}, []int{1, 5, 9}},
		{"blanks inside one argument", []string{"1 5 9"}, []int{1, 5, 9}},
		{"blanks split by the shell", []string{"1", "5", "9"}, []int{1, 5, 9}},
		{"a single stop", []string{"12"}, []int{12}},
		{"not starting at column 1", []string{"7,14,21"}, []int{7, 14, 21}},
	} {
		t.Run(c.name, func(t *testing.T) {
			args := append([]string{"-T", "demo"}, c.args...)
			out, errb, code := runIn(t, dir, nil, args...)
			if code != exitOK {
				t.Fatalf("exit %d, stderr %q", code, errb)
			}
			if got := want(c.stops...); out != got {
				t.Errorf("output %q, want %q", out, got)
			}
		})
	}
}

// A '+' inside the list is an increment on the previous stop, so 1,+4,+4 is
// the same request as 1,5,9.
func TestIncrementForm(t *testing.T) {
	dir := fixtureDir(t)
	for _, c := range []struct {
		name  string
		args  []string
		stops []int
	}{
		{"all increments", []string{"1,+4,+4"}, []int{1, 5, 9}},
		{"mixed absolute and increment", []string{"1,+9,20,+5"}, []int{1, 10, 20, 25}},
		{"increment as its own argument", []string{"1", "+4", "+4"}, []int{1, 5, 9}},
	} {
		t.Run(c.name, func(t *testing.T) {
			args := append([]string{"-T", "demo"}, c.args...)
			out, errb, code := runIn(t, dir, nil, args...)
			if code != exitOK {
				t.Fatalf("exit %d, stderr %q", code, errb)
			}
			if got := want(c.stops...); out != got {
				t.Errorf("output %q, want %q", out, got)
			}
		})
	}
}

// A list that does not strictly ascend is REFUSED. Silently sorting it would
// hand the caller stops they did not ask for, and setting them in the given
// order is impossible — the cursor cannot go backwards.
func TestNonAscendingListIsRefused(t *testing.T) {
	dir := fixtureDir(t)
	for _, c := range []struct{ name, arg, msg string }{
		{"decreasing", "5,3", "strictly ascending"},
		{"repeated", "5,5", "strictly ascending"},
		{"descending after an increment", "1,+9,4", "strictly ascending"},
		{"zero", "0,5", "positive column"},
		{"negative", "-5x", ""}, // not a number and not a known flag
		{"a zero increment", "1,+0", "must be positive"},
		{"a negative increment", "1,+-3", "must be positive"},
		{"a list cannot open with an increment", "+5,10", "invalid margin"},
		{"not a number", "1,abc", "invalid tab stop"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errb, code := runIn(t, dir, nil, "-T", "demo", c.arg)
			if code == exitOK {
				t.Fatalf("exit 0 for %q, want a refusal (stdout %q)", c.arg, out)
			}
			if errb == "" {
				t.Errorf("%q was refused silently", c.arg)
			}
			if c.msg != "" && !strings.Contains(errb, c.msg) {
				t.Errorf("stderr = %q, want it to mention %q", errb, c.msg)
			}
		})
	}
}

// An increment can never be the FIRST value: POSIX says the plus-sign applies
// to every stop "except the first one", and before the list starts a '+' word
// is the margin option instead.
func TestLeadingIncrementIsNotAStop(t *testing.T) {
	dir := fixtureDir(t)
	// "+5" alone is the margin form, so the default -8 stops are still set.
	out, errb, code := runIn(t, dir, []string{"COLUMNS=16"}, "-T", "demo", "+5")
	if code != exitOK {
		t.Fatalf("exit %d, stderr %q", code, errb)
	}
	if !strings.HasPrefix(out, capCR+strings.Repeat(" ", 5)+capSML) {
		t.Errorf("output %q does not begin with a margin move", out)
	}
	// After the margin, the ordinary default sequence follows.
	if !strings.HasSuffix(out, want(1, 9, 17)) {
		t.Errorf("output %q does not end with the default -8 stops", out)
	}
}

// Past "--" a '+' word can no longer be the margin, so the list parser's own
// "the first stop cannot be an increment" guard is what answers.
func TestIncrementCannotOpenTheList(t *testing.T) {
	dir := fixtureDir(t)
	out, errb, code := runIn(t, dir, nil, "-T", "demo", "--", "+5")
	if code == exitOK {
		t.Fatalf("exit 0 (stdout %q)", out)
	}
	if !strings.Contains(errb, "cannot be an increment") {
		t.Errorf("stderr = %q", errb)
	}
}

func TestMargin(t *testing.T) {
	dir := fixtureDir(t)
	for _, c := range []struct {
		name   string
		arg    string
		prefix string
	}{
		{"+m defaults to 10", "+m", capCR + strings.Repeat(" ", 10) + capSML},
		{"+m4 moves four columns", "+m4", capCR + strings.Repeat(" ", 4) + capSML},
		{"+m0 clears the margin", "+m0", capMGC},
		{"the bare +n spelling", "+4", capCR + strings.Repeat(" ", 4) + capSML},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errb, code := runIn(t, dir, []string{"COLUMNS=16"}, "-T", "demo", c.arg, "-0")
			if code != exitOK {
				t.Fatalf("exit %d, stderr %q", code, errb)
			}
			if wantAll := c.prefix + capCR + capTBC + capCR; out != wantAll {
				t.Errorf("output %q, want %q", out, wantAll)
			}
		})
	}
}

// When the terminal has no margin capability the stops are still set — but the
// margin request is REPORTED and carried into the exit status, never dropped.
func TestMarginOnATerminalThatCannotDoIt(t *testing.T) {
	f := demoTerm()
	f.Names = []string{"nomargin", "no margin support"}
	delete(f.Strs, "smgl")
	delete(f.Strs, "mgc")
	dir := fixtureDir(t, f)

	out, errb, code := runIn(t, dir, []string{"COLUMNS=16"}, "-T", "nomargin", "+m4", "-0")
	if code == exitOK {
		t.Errorf("exit 0 — an unsatisfied margin request must not look satisfied")
	}
	if !strings.Contains(errb, "left margin") {
		t.Errorf("stderr = %q, want a margin diagnostic", errb)
	}
	if got := capCR + capTBC + capCR; out != got {
		t.Errorf("output %q, want the stops to still be set (%q)", out, got)
	}
}

// A parameterized margin capability is used when the cursor-positioned one is
// absent; terminfo counts that parameter from zero, so column n+1 is n.
func TestMarginViaParameterizedCapability(t *testing.T) {
	f := demoTerm()
	f.Names = []string{"parm", "parameterized margin"}
	delete(f.Strs, "smgl")
	delete(f.Strs, "mgc")
	f.Strs["smglp"] = "\x1b[%p1%ds"
	dir := fixtureDir(t, f)

	out, errb, code := runIn(t, dir, nil, "-T", "parm", "+m4", "-0")
	if code != exitOK {
		t.Fatalf("exit %d, stderr %q", code, errb)
	}
	if wantAll := "\x1b[4s" + capCR + capTBC + capCR; out != wantAll {
		t.Errorf("output %q, want %q", out, wantAll)
	}
}

// A terminal with no clear-all-tabs cannot do the job at all, and says so
// rather than emitting a sequence that sets stops on top of the old ones.
func TestTerminalWithoutTabCapabilities(t *testing.T) {
	noClear := terminfo.Fixture{
		Names: []string{"noclear", "cannot clear tabs"},
		Strs:  map[string]string{"cr": capCR, "hts": capHTS},
	}
	noSet := terminfo.Fixture{
		Names: []string{"noset", "cannot set tabs"},
		Strs:  map[string]string{"cr": capCR, "tbc": capTBC},
	}
	dir := fixtureDir(t, noClear, noSet)

	out, errb, code := runIn(t, dir, nil, "-T", "noclear", "-8")
	if code == exitOK || !strings.Contains(errb, "cannot clear tabs") {
		t.Errorf("noclear: got (%q, %q, %d)", out, errb, code)
	}
	if out != "" {
		t.Errorf("noclear: wrote %q — nothing should reach a terminal that cannot be cleared", out)
	}

	out, errb, code = runIn(t, dir, nil, "-T", "noset", "-8")
	if code == exitOK || !strings.Contains(errb, "cannot set tabs") {
		t.Errorf("noset: got (%q, %q, %d)", out, errb, code)
	}

	// …but a terminal that cannot SET tabs can still be asked to clear them.
	out, errb, code = runIn(t, dir, nil, "-T", "noset", "-0")
	if code != exitOK {
		t.Fatalf("noset -0: exit %d, stderr %q", code, errb)
	}
	if got := capCR + capTBC + capCR; out != got {
		t.Errorf("noset -0: output %q, want %q", out, got)
	}
}

// The carriage return falls back to a literal \r when the entry omits cr,
// because returning to column one is what makes the space-counting correct.
func TestCarriageReturnFallback(t *testing.T) {
	f := terminfo.Fixture{
		Names: []string{"nocr", "no cr capability"},
		Strs:  map[string]string{"tbc": capTBC, "hts": capHTS},
	}
	dir := fixtureDir(t, f)
	out, errb, code := runIn(t, dir, nil, "-T", "nocr", "1,3")
	if code != exitOK {
		t.Fatalf("exit %d, stderr %q", code, errb)
	}
	if got := "\r" + capTBC + capHTS + "  " + capHTS + "\r"; out != got {
		t.Errorf("output %q, want %q", out, got)
	}
}

func TestTerminalTypeResolution(t *testing.T) {
	dir := fixtureDir(t)

	t.Run("-T wins over $TERM", func(t *testing.T) {
		_, errb, code := runIn(t, dir, []string{"TERM=definitely-not-a-terminal"}, "-T", "demo", "-0")
		if code != exitOK {
			t.Errorf("exit %d, stderr %q", code, errb)
		}
	})

	t.Run("$TERM is used when -T is absent", func(t *testing.T) {
		_, errb, code := runIn(t, dir, []string{"TERM=demo"}, "-0")
		if code != exitOK {
			t.Errorf("exit %d, stderr %q", code, errb)
		}
	})

	t.Run("an unknown terminal is an error", func(t *testing.T) {
		out, errb, code := runIn(t, dir, nil, "-T", "definitely-not-a-terminal", "-8")
		if code == exitOK {
			t.Errorf("exit 0 for an unknown terminal")
		}
		if !strings.Contains(errb, "unknown terminal") {
			t.Errorf("stderr = %q", errb)
		}
		if out != "" {
			t.Errorf("wrote %q for an unknown terminal", out)
		}
	})

	t.Run("no terminal at all defaults to ansi", func(t *testing.T) {
		_, _, code := runIn(t, dir, nil, "-8")
		if code != exitOK {
			t.Errorf("exit %d, want 2", code)
		}
	})
}

// Two format options, or a format option plus a list, are contradictory
// requests — and a contradictory request is refused, not resolved by
// last-one-wins.
func TestConflictingRequests(t *testing.T) {
	dir := fixtureDir(t)
	for _, c := range []struct {
		name string
		args []string
	}{
		{"two presets", []string{"-a", "-c"}},
		{"a preset and a repetitive spec", []string{"-a", "-4"}},
		{"a preset and a list", []string{"-a", "1,5"}},
		{"a repetitive spec and a list", []string{"-4", "1,5"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			args := append([]string{"-T", "demo"}, c.args...)
			out, errb, code := runIn(t, dir, nil, args...)
			if code != 2 {
				t.Errorf("exit %d, want a usage error (stdout %q)", code, out)
			}
			if errb == "" {
				t.Errorf("refused silently")
			}
		})
	}
}

// An option this implementation does not know must fail loudly rather than be
// ignored — a silently dropped flag is a wrong answer that looks right.
func TestUnknownOptionIsRefused(t *testing.T) {
	dir := fixtureDir(t)
	for _, arg := range []string{"-z", "--nope", "-a3", "+mx"} {
		t.Run(arg, func(t *testing.T) {
			out, errb, code := runIn(t, dir, nil, "-T", "demo", arg)
			if code == exitOK {
				t.Errorf("exit 0 for %q (stdout %q)", arg, out)
			}
			if errb == "" {
				t.Errorf("%q was refused silently", arg)
			}
		})
	}
}

func TestHelpAndVersion(t *testing.T) {
	dir := fixtureDir(t)
	for _, arg := range []string{"--help", "-h"} {
		out, _, code := runIn(t, dir, nil, arg)
		if code != 0 || !strings.Contains(out, "tabs") {
			t.Errorf("%s: got (%q, %d)", arg, out, code)
		}
	}
	for _, arg := range []string{"--version", "-V"} {
		out, _, code := runIn(t, dir, nil, arg)
		if code != 0 || out == "" {
			t.Errorf("%s: got (%q, %d)", arg, out, code)
		}
	}
}

// The certification harness rebuilds the measured PATH from the multicall's own
// inventory, so an applet that is not registered is invisible to the run.
func TestRegisteredUnderItsOwnName(t *testing.T) {
	got := tool.Lookup("tabs")
	if got == nil {
		t.Fatal("tabs is not in the tool registry")
	}
	if got.Run == nil {
		t.Error("tabs is registered with no Run")
	}
	if !strings.Contains(got.Usage, "+m") {
		t.Errorf("usage does not document the margin option: %q", got.Usage)
	}
}

func equal(a, b []int) bool {
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
