package tool

import (
	"runtime"

	"mvdan.cc/sh/v3/pathconv"
)

// The two seams below exist so an applet's OWN package tests can state what
// its operand handling does on Windows while running on a Unix developer
// host. They are test-only; nothing in an applet's production path calls
// them. The alternative — asserting Windows behavior only on the Windows CI
// leg — is what let Story #682's failures ship: `go test` on darwin cannot
// see a \-spelled operand at all, because filepath there is the POSIX
// implementation.

// operandWindows is the spelling mode OperandJoin, OperandDir, OperandClean
// and OperandSeparator apply. It is the host's by default; SetOperandWindows
// pins it for a test.
var operandWindows = runtime.GOOS == "windows"

// SetOperandWindows pins the operand-spelling mode and returns a function
// restoring the previous value (`defer tool.SetOperandWindows(true)()`).
// It is not safe for concurrent use: a test that calls it must not be
// parallel.
func SetOperandWindows(windows bool) (restore func()) {
	prev := operandWindows
	operandWindows = windows
	return func() { operandWindows = prev }
}

// ResolveOperandMode is RunContext.Path's conversion step with an explicit
// mount table and windows flag: it answers "where would this operand land on
// a Windows host with these mounts?". dir supplies the volume for a
// drive-relative /foo and may be empty. An applet test uses it to assert
// that the operand it BUILT still resolves under the mounted /tmp — which is
// exactly what broke when the operand was rebuilt with filepath.Join.
func ResolveOperandMode(m *pathconv.Mounts, dir, operand string, windows bool) string {
	return shellAbsMode(m, dir, operand, windows)
}
