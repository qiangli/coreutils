//go:build linux || darwin

package sttycmd

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/creack/pty/v2"
	"golang.org/x/sys/unix"

	"github.com/qiangli/coreutils/tool"
)

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
}
