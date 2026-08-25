//go:build windows

package ddcmd

import (
	"errors"
	"io"
	"os"
)

var errInterrupted = errors.New("interrupted")

// Windows has no POSIX signal delivery to re-raise and no descriptor-level
// non-blocking mode to cancel a read or write with, so dd runs its ordinary
// blocking path there: nothing is ever wrapped, and no descriptor flags are
// touched.
type interruptContext struct{}

func newInterruptContext() *interruptContext  { return &interruptContext{} }
func (c *interruptContext) Stop()             {}
func (c *interruptContext) Interrupted() bool { return false }
func (c *interruptContext) Signal() int       { return 0 }

func interruptSignalNumber() int { return 0 }

func interruptibleOpenRead(path string, _ *interruptContext) (*os.File, bool, error) {
	f, err := os.Open(path)
	return f, false, err
}

func interruptibleOpenWrite(path string, _ bool, _ *interruptContext) (*os.File, error) {
	return os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0o666)
}

type interruptReader struct{}

func interruptibleReader(io.Reader, *interruptContext) (*interruptReader, bool) {
	return nil, false
}

func newInterruptReader(*os.File, *interruptContext, bool) (*interruptReader, bool) {
	return nil, false
}

func (r *interruptReader) Read([]byte) (int, error) { return 0, errInterrupted }

func (r *interruptReader) Close() error { return nil }

type interruptWriter struct{}

func interruptibleWriter(io.Writer, *interruptContext) (*interruptWriter, bool) {
	return nil, false
}

func newInterruptWriter(*os.File, *interruptContext) (*interruptWriter, bool) {
	return nil, false
}

func (w *interruptWriter) Write([]byte) (int, error) { return 0, errInterrupted }

func (w *interruptWriter) Close() error { return nil }
