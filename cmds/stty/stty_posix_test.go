//go:build linux || darwin

package sttycmd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/creack/pty/v2"
	"golang.org/x/sys/unix"

	"github.com/qiangli/coreutils/tool"
)

type sttyFailWriter struct{}

func (sttyFailWriter) Write([]byte) (int, error) { return 0, errors.New("injected write failure") }

// openTTY returns the terminal side of a fresh pseudo-terminal pair,
// skipping when the environment cannot allocate one.
func openTTY(t *testing.T) *os.File {
	t.Helper()
	ptm, tty, err := pty.Open()
	if err != nil {
		t.Skipf("cannot open pty: %v", err)
	}
	t.Cleanup(func() { ptm.Close(); tty.Close() })
	return tty
}

func runStty(t *testing.T, tty *os.File, args []string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   t.TempDir(),
		Stdio: tool.Stdio{In: tty, Out: &out, Err: &errb},
	}
	code := run(rc, args)
	return code, out.String(), errb.String()
}

func getWinsize(t *testing.T, tty *os.File) *unix.Winsize {
	t.Helper()
	win, err := unix.IoctlGetWinsize(int(tty.Fd()), unix.TIOCGWINSZ)
	if err != nil {
		t.Fatalf("IoctlGetWinsize: %v", err)
	}
	return win
}

// TestSttyRowsAppliesWindowSize is the "green" proof: stty rows N actually
// reaches the kernel's window size, not just validates the argument.
func TestSttyRowsAppliesWindowSize(t *testing.T) {
	tty := openTTY(t)
	if code, _, errb := runStty(t, tty, []string{"rows", "40"}); code != 0 {
		t.Fatalf("stty rows 40: code=%d err=%q", code, errb)
	}
	if got := getWinsize(t, tty).Row; got != 40 {
		t.Fatalf("rows not applied: kernel row=%d, want 40", got)
	}
}

// TestSttyColumnsAppliesWindowSize proves cols/columns set only the column
// count, leaving rows untouched, matching GNU stty's independent rows/cols.
func TestSttyColumnsAppliesWindowSize(t *testing.T) {
	tty := openTTY(t)
	if code, _, errb := runStty(t, tty, []string{"rows", "40"}); code != 0 {
		t.Fatalf("stty rows 40: code=%d err=%q", code, errb)
	}
	if code, _, errb := runStty(t, tty, []string{"columns", "100"}); code != 0 {
		t.Fatalf("stty columns 100: code=%d err=%q", code, errb)
	}
	win := getWinsize(t, tty)
	if win.Col != 100 {
		t.Fatalf("columns not applied: kernel col=%d, want 100", win.Col)
	}
	if win.Row != 40 {
		t.Fatalf("columns changed rows: kernel row=%d, want unchanged 40", win.Row)
	}

	if code, _, errb := runStty(t, tty, []string{"cols", "77"}); code != 0 {
		t.Fatalf("stty cols 77: code=%d err=%q", code, errb)
	}
	if got := getWinsize(t, tty).Col; got != 77 {
		t.Fatalf("cols not applied: kernel col=%d, want 77", got)
	}
}

// TestSttyRowsColsRejectsOverflow is the "red" proof for the overflow
// policy: GNU stty caps rows/cols arguments at INT_MAX and rejects larger
// values outright rather than silently truncating them into the kernel's
// unsigned-short window size fields.
func TestSttyRowsColsRejectsOverflow(t *testing.T) {
	tty := openTTY(t)
	if code, _, errb := runStty(t, tty, []string{"rows", "40"}); code != 0 {
		t.Fatalf("stty rows 40: code=%d err=%q", code, errb)
	}
	before := getWinsize(t, tty)

	// 3000000000 is < UINT32_MAX but > INT_MAX: GNU stty rejects it.
	code, _, errb := runStty(t, tty, []string{"cols", "3000000000"})
	if code == 0 {
		t.Fatalf("stty cols 3000000000 unexpectedly succeeded")
	}
	if errb == "" {
		t.Fatalf("expected a diagnostic for an out-of-range cols argument")
	}

	after := getWinsize(t, tty)
	if after.Col != before.Col || after.Row != before.Row {
		t.Fatalf("rejected overflow still mutated window size: before=%+v after=%+v", before, after)
	}
	t.Run("required modes reach termios", testSttyRequiredModesReachTermios)
	t.Run("all reports kernel settings", testSttyAllReportsKernelSettings)
	t.Run("save and restore round trip", testSttySaveAndRestoreRoundTrip)
	t.Run("speed and control operands", testSttySpeedAndControlCharacterOperands)
	t.Run("diagnostics and status", testSttyDiagnosticsAndFailureStatus)
	t.Run("invalid later operand is atomic", testSttyInvalidLaterOperandIsAtomic)
	t.Run("numeric operands are decimal", testSttyNumericOperandsAreDecimal)
	t.Run("platform vdisable", testSttyUsesPlatformVDisableAndPrintsUndef)
	if runtime.GOOS == "linux" {
		t.Run("Linux PTY speeds use Cflag", testLinuxPTYSpeedsUseCflag)
	}
}

func TestSttyRequiredReportsPropagateWriteErrors(t *testing.T) {
	tty := openTTY(t)
	for _, args := range [][]string{nil, {"-a"}, {"-g"}} {
		var errb bytes.Buffer
		rc := &tool.RunContext{
			Ctx: context.Background(), Dir: t.TempDir(),
			Stdio: tool.Stdio{In: tty, Out: sttyFailWriter{}, Err: &errb},
		}
		if code := run(rc, args); code == 0 {
			t.Errorf("stty %v ignored stdout failure", args)
		}
		if !strings.Contains(errb.String(), "write error") {
			t.Errorf("stty %v stderr %q lacks write diagnostic", args, errb.String())
		}
	}
}

func testSttyRequiredModesReachTermios(t *testing.T) {
	tty := openTTY(t)
	code, _, errb := runStty(t, tty, []string{"ignbrk", "ocrnl", "tostop", "clocal"})
	if code != 0 {
		t.Fatalf("set required modes: code=%d err=%q", code, errb)
	}
	state, err := getTermios(int(tty.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	if state.Iflag&unix.IGNBRK == 0 || state.Oflag&unix.OCRNL == 0 || state.Lflag&unix.TOSTOP == 0 || state.Cflag&unix.CLOCAL == 0 {
		t.Fatalf("required modes not applied: iflag=%#x oflag=%#x lflag=%#x cflag=%#x", state.Iflag, state.Oflag, state.Lflag, state.Cflag)
	}
	code, _, errb = runStty(t, tty, []string{"-ignbrk", "-ocrnl", "-tostop", "-clocal"})
	if code != 0 {
		t.Fatalf("clear required modes: code=%d err=%q", code, errb)
	}
	state, err = getTermios(int(tty.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	if state.Iflag&unix.IGNBRK != 0 || state.Oflag&unix.OCRNL != 0 || state.Lflag&unix.TOSTOP != 0 || state.Cflag&unix.CLOCAL != 0 {
		t.Fatalf("required modes not cleared: iflag=%#x oflag=%#x lflag=%#x cflag=%#x", state.Iflag, state.Oflag, state.Lflag, state.Cflag)
	}
}

func testSttyAllReportsKernelSettings(t *testing.T) {
	tty := openTTY(t)
	if code, _, errb := runStty(t, tty, []string{"-echo", "ignbrk", "ocrnl", "tostop", "intr", "^A", "min", "2", "time", "3"}); code != 0 {
		t.Fatalf("prepare settings: code=%d err=%q", code, errb)
	}
	code, out, errb := runStty(t, tty, []string{"-a"})
	if code != 0 {
		t.Fatalf("stty -a: code=%d err=%q", code, errb)
	}
	for _, want := range []string{"speed ", "-echo ", "ignbrk ", "ocrnl ", "tostop;", "intr = ^A", "min = 2; time = 3;", "cr", "tab", "bs", "ff", "vt"} {
		if !strings.Contains(out, want) {
			t.Errorf("stty -a missing %q:\n%s", want, out)
		}
	}
}

func testSttySaveAndRestoreRoundTrip(t *testing.T) {
	tty := openTTY(t)
	code, before, errb := runStty(t, tty, []string{"-g"})
	if code != 0 {
		t.Fatalf("initial stty -g: code=%d err=%q", code, errb)
	}
	saved := strings.TrimSpace(before)
	if !strings.HasPrefix(saved, "v1:") {
		t.Fatalf("saved form is not versioned: %q", saved)
	}
	if code, _, errb = runStty(t, tty, []string{"-echo", "ignbrk", "intr", "^A", "9600"}); code != 0 {
		t.Fatalf("mutate settings: code=%d err=%q", code, errb)
	}
	if code, _, errb = runStty(t, tty, []string{saved}); code != 0 {
		t.Fatalf("restore settings: code=%d err=%q", code, errb)
	}
	code, after, errb := runStty(t, tty, []string{"-g"})
	if code != 0 {
		t.Fatalf("final stty -g: code=%d err=%q", code, errb)
	}
	if strings.TrimSpace(after) != saved {
		t.Fatalf("settings did not round trip:\nbefore %s\nafter  %s", saved, strings.TrimSpace(after))
	}
}

func testSttySpeedAndControlCharacterOperands(t *testing.T) {
	tty := openTTY(t)
	if code, _, errb := runStty(t, tty, []string{"9600", "ispeed", "4800", "ospeed", "9600", "eof", "^E", "eol", "x", "erase", "^?", "intr", "^B", "kill", "^K", "quit", "^Q", "susp", "^S", "start", "^A", "stop", "^D"}); code != 0 {
		t.Fatalf("speed/control operands: code=%d err=%q", code, errb)
	}
	state, err := getTermios(int(tty.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]uint8{"eof": 5, "eol": 'x', "erase": 0x7f, "intr": 2, "kill": 11, "quit": 17, "susp": 19, "start": 1, "stop": 4} {
		index, _ := controlCharIndex(name)
		if got := state.Cc[index]; got != want {
			t.Errorf("%s=%d, want %d", name, got, want)
		}
	}
	code, out, errb := runStty(t, tty, []string{"baud"})
	if code != 0 || strings.TrimSpace(out) != "9600" {
		t.Fatalf("baud report: code=%d out=%q err=%q", code, out, errb)
	}
}

func testSttyDiagnosticsAndFailureStatus(t *testing.T) {
	tty := openTTY(t)
	for _, args := range [][]string{{"ospeed", "12345"}, {"intr", "long"}, {"v1:broken"}, {"-cr1"}} {
		code, _, errb := runStty(t, tty, args)
		if code == 0 || strings.TrimSpace(errb) == "" {
			t.Errorf("stty %q: code=%d err=%q, want non-zero with diagnostic", args, code, errb)
		}
	}
}

func testSttyInvalidLaterOperandIsAtomic(t *testing.T) {
	tty := openTTY(t)
	before, err := getTerminalState(int(tty.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	beforeWindow := *getWinsize(t, tty)

	code, _, errb := runStty(t, tty, []string{"-echo", "rows", "52", "not-a-mode"})
	if code == 0 || strings.TrimSpace(errb) == "" {
		t.Fatalf("invalid operand list: code=%d err=%q", code, errb)
	}
	after, err := getTerminalState(int(tty.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	afterWindow := *getWinsize(t, tty)
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("invalid later operand changed termios:\nbefore=%+v\nafter=%+v", before, after)
	}
	if afterWindow != beforeWindow {
		t.Fatalf("invalid later operand changed window: before=%+v after=%+v", beforeWindow, afterWindow)
	}
}

func testSttyNumericOperandsAreDecimal(t *testing.T) {
	tty := openTTY(t)
	if code, _, errb := runStty(t, tty, []string{"min", "010"}); code != 0 {
		t.Fatalf("decimal min operand: code=%d err=%q", code, errb)
	}
	state, err := getTermios(int(tty.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	if got := state.Cc[unix.VMIN]; got != 10 {
		t.Fatalf("min 010 parsed as %d, want decimal 10", got)
	}
	before := state.Cc[unix.VTIME]
	if code, _, _ := runStty(t, tty, []string{"time", "0x10"}); code == 0 {
		t.Fatal("hexadecimal numeric operand unexpectedly accepted")
	}
	state, err = getTermios(int(tty.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	if state.Cc[unix.VTIME] != before {
		t.Fatalf("invalid numeric operand changed VTIME: before=%d after=%d", before, state.Cc[unix.VTIME])
	}
}

func testSttyUsesPlatformVDisableAndPrintsUndef(t *testing.T) {
	tty := openTTY(t)
	if code, _, errb := runStty(t, tty, []string{"intr", "undef"}); code != 0 {
		t.Fatalf("disable intr: code=%d err=%q", code, errb)
	}
	state, err := getTermios(int(tty.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	if got := state.Cc[unix.VINTR]; got != posixVDisable {
		t.Fatalf("VINTR=%d, want platform _POSIX_VDISABLE %d", got, posixVDisable)
	}
	code, out, errb := runStty(t, tty, []string{"-a"})
	if code != 0 || !strings.Contains(out, "intr = undef") {
		t.Fatalf("stty -a disabled character: code=%d out=%q err=%q", code, out, errb)
	}
}

func testLinuxPTYSpeedsUseCflag(t *testing.T) {
	if posixVDisable != 0 {
		t.Fatalf("Linux _POSIX_VDISABLE=%d, want 0", posixVDisable)
	}
	tty := openTTY(t)
	if code, _, errb := runStty(t, tty, []string{"9600"}); code != 0 {
		t.Fatalf("set speed: code=%d err=%q", code, errb)
	}

	tio, err := getTermios(int(tty.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	if tio.Ispeed != 0 || tio.Ospeed != 0 {
		t.Fatalf("TCGETS unexpectedly populated speed fields: ispeed=%d ospeed=%d", tio.Ispeed, tio.Ospeed)
	}
	state, err := getTerminalState(int(tty.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	if nativeToBaud(state.Ispeed) != 9600 || nativeToBaud(state.Ospeed) != 9600 {
		t.Fatalf("decoded speeds=(%d,%d), want 9600", nativeToBaud(state.Ispeed), nativeToBaud(state.Ospeed))
	}

	input, ok := baudToNative(4800)
	if !ok {
		t.Fatal("platform unexpectedly lacks B4800")
	}
	output, ok := baudToNative(9600)
	if !ok {
		t.Fatal("platform unexpectedly lacks B9600")
	}
	state.Ispeed, state.Ospeed = input, output
	if err := setTerminalState(int(tty.Fd()), state); err != nil {
		t.Fatal(err)
	}
	state, err = getTerminalState(int(tty.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	if nativeToBaud(state.Ispeed) != 4800 || nativeToBaud(state.Ospeed) != 9600 {
		t.Fatalf("encoded speeds=(%d,%d), want (4800,9600)", nativeToBaud(state.Ispeed), nativeToBaud(state.Ospeed))
	}
}
