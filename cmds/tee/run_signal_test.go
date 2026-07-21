package teecmd

import (
	"context"
	"io"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/qiangli/coreutils/tool"
)

type blockingReader struct {
	ch chan string
}

func (b *blockingReader) Read(p []byte) (n int, err error) {
	s, ok := <-b.ch
	if !ok {
		return 0, io.EOF
	}
	n = copy(p, s)
	return n, nil
}

func TestTeeIgnoreInterruptsActual(t *testing.T) {
	dir := t.TempDir()
	out, errb := new(strings.Builder), new(strings.Builder)

	in := &blockingReader{ch: make(chan string)}
	
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Stdio: tool.Stdio{In: in, Out: out, Err: errb},
	}
	
	done := make(chan int)
	go func() {
		done <- cmd.Run(rc, []string{"-i"})
	}()

	time.Sleep(100 * time.Millisecond)

	proc, err := os.FindProcess(os.Getpid())
	if err == nil {
		proc.Signal(syscall.SIGINT)
	}

	time.Sleep(50 * time.Millisecond) // Give signal time to be processed

	in.ch <- "hello\n"
	close(in.ch)

	code := <-done
	if code != 0 {
		t.Errorf("code = %d", code)
	}
	if out.String() != "hello\n" {
		t.Errorf("out = %q", out.String())
	}
}
