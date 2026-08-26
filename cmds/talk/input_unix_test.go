//go:build unix

package talkcmd

import (
	"os"
	"testing"
)

type fdWrapper struct{ file *os.File }

func (w fdWrapper) Read(p []byte) (int, error) { return w.file.Read(p) }
func (w fdWrapper) Fd() uintptr                { return w.file.Fd() }

func TestFdWrapperUsesSynchronousPolledInput(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	in, err := newTerminalInput(fdWrapper{file: r})
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	if _, ok := in.(*polledTerminalInput); !ok {
		t.Fatalf("input type = %T, want synchronous fd poller", in)
	}
}
