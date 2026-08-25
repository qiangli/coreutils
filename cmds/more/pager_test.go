package morecmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/qiangli/coreutils/tool"
)

func withPagerTTY(t *testing.T, out io.Writer, tty *ttyChannel, rows, cols int) {
	t.Helper()
	origTerm, origOpen, origSize := isTerminal, openTTY, getTerminalSize
	isTerminal = func(w io.Writer) bool { return w == out }
	openTTY = func(*tool.RunContext) (*ttyChannel, error) { return tty, nil }
	getTerminalSize = func(int) (int, int, error) { return cols, rows, nil }
	t.Cleanup(func() { isTerminal, openTTY, getTerminalSize = origTerm, origOpen, origSize })
}

func commandTTY(commands string) *ttyChannel {
	var mu sync.Mutex
	i := 0
	return &ttyChannel{
		hasFd: true,
		readCommand: func(context.Context) (byte, error) {
			mu.Lock()
			defer mu.Unlock()
			if i == len(commands) {
				return 0, io.EOF
			}
			b := commands[i]
			i++
			return b, nil
		},
		close: func() error { return nil },
	}
}

func TestPagerSeparatesContentAndTerminalUI(t *testing.T) {
	var out, errOut bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{In: strings.NewReader("a\nb\n"), Out: &out, Err: &errOut}}
	withPagerTTY(t, rc.Out, commandTTY("q"), 2, 80)
	if code := run(rc, nil); code != 0 {
		t.Fatalf("code=%d err=%q", code, errOut.String())
	}
	if out.String() != "a\n" {
		t.Fatalf("content channel = %q", out.String())
	}
	if got := errOut.String(); got != "\x1b[7m--More--\x1b[m\r\x1b[K" {
		t.Fatalf("terminal channel = %q", got)
	}
}

type gatedSource struct {
	first []byte
	gate  <-chan struct{}
	done  bool
}

func (r *gatedSource) Read(p []byte) (int, error) {
	if len(r.first) != 0 {
		n := copy(p, r.first)
		r.first = r.first[n:]
		return n, nil
	}
	<-r.gate
	if r.done {
		return 0, io.EOF
	}
	r.done = true
	return copy(p, []byte("later\n")), nil
}

func TestPagerStreamsFirstScreenBeforeSourceEOF(t *testing.T) {
	var out, errOut bytes.Buffer
	gate := make(chan struct{})
	rc := &tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{In: &gatedSource{first: []byte("one\n"), gate: gate}, Out: &out, Err: &errOut}}
	read := make(chan struct{})
	release := make(chan struct{})
	tty := commandTTY("q")
	origRead := tty.readCommand
	tty.readCommand = func(ctx context.Context) (byte, error) {
		close(read)
		<-release
		return origRead(ctx)
	}
	withPagerTTY(t, rc.Out, tty, 2, 80)
	done := make(chan int, 1)
	go func() { done <- run(rc, nil) }()
	select {
	case <-read:
		gotOut, gotErr := out.String(), errOut.String()
		close(release)
		if gotOut != "one\n" || !strings.Contains(gotErr, "--More--") {
			t.Fatalf("before EOF out=%q err=%q", gotOut, gotErr)
		}
	case <-time.After(2 * time.Second):
		close(release)
		close(gate)
		t.Fatal("first screen was not emitted before source requested more input")
	}
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("code=%d", code)
		}
	case <-time.After(2 * time.Second):
		close(gate)
		t.Fatal("q attempted another source read")
	}
	close(gate)
}

type orderedTerminal struct {
	mu      sync.Mutex
	visible bool
}

func (w *orderedTerminal) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.visible = w.visible || bytes.Contains(p, []byte("--More--"))
	return len(p), nil
}
func (w *orderedTerminal) Flush() error { return nil }

func TestPagerPromptVisibleBeforeRead(t *testing.T) {
	var out bytes.Buffer
	errOut := &orderedTerminal{}
	rc := &tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{In: strings.NewReader("x\ny\n"), Out: &out, Err: errOut}}
	tty := commandTTY("q")
	tty.readCommand = func(context.Context) (byte, error) {
		errOut.mu.Lock()
		defer errOut.mu.Unlock()
		if !errOut.visible {
			return 0, errors.New("read before prompt")
		}
		return 'q', nil
	}
	withPagerTTY(t, rc.Out, tty, 2, 80)
	if code := run(rc, nil); code != 0 {
		t.Fatalf("code=%d", code)
	}
}

type failPromptWriter struct {
	bytes.Buffer
	fail bool
}

func (w *failPromptWriter) Write(p []byte) (int, error) {
	if w.fail && bytes.Contains(p, []byte("--More--")) {
		w.fail = false
		return 0, errors.New("prompt write failed")
	}
	return w.Buffer.Write(p)
}

func (w *failPromptWriter) WriteString(s string) (int, error) {
	return w.Write([]byte(s))
}

func TestPagerPromptFailureReturnsBeforeRead(t *testing.T) {
	var out bytes.Buffer
	errOut := &failPromptWriter{fail: true}
	reads := 0
	tty := commandTTY("q")
	tty.readCommand = func(context.Context) (byte, error) { reads++; return 'q', nil }
	rc := &tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{In: strings.NewReader("x\ny\n"), Out: &out, Err: errOut}}
	withPagerTTY(t, rc.Out, tty, 2, 80)
	if code := run(rc, nil); code == 0 || reads != 0 || !strings.Contains(errOut.String(), "prompt write failed") {
		t.Fatalf("code=%d reads=%d err=%q", code, reads, errOut.String())
	}
}

type failFlushWriter struct {
	bytes.Buffer
	fail bool
}

func (w *failFlushWriter) Flush() error {
	if w.fail {
		w.fail = false
		return errors.New("prompt flush failed")
	}
	return nil
}

func TestPagerPromptFlushFailureReturnsBeforeRead(t *testing.T) {
	var out bytes.Buffer
	errOut := &failFlushWriter{fail: true}
	reads := 0
	tty := commandTTY("q")
	tty.readCommand = func(context.Context) (byte, error) { reads++; return 'q', nil }
	rc := &tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{In: strings.NewReader("x\ny\n"), Out: &out, Err: errOut}}
	withPagerTTY(t, rc.Out, tty, 2, 80)
	if code := run(rc, nil); code == 0 || reads != 0 || !strings.Contains(errOut.String(), "prompt flush failed") {
		t.Fatalf("code=%d reads=%d err=%q", code, reads, errOut.String())
	}
}

func TestPagerCancellationNeedsNoReaderGoroutine(t *testing.T) {
	var out, errOut bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())
	tty := commandTTY("")
	tty.readCommand = func(ctx context.Context) (byte, error) { cancel(); <-ctx.Done(); return 0, ctx.Err() }
	rc := &tool.RunContext{Ctx: ctx, Stdio: tool.Stdio{In: strings.NewReader("x\ny\n"), Out: &out, Err: &errOut}}
	withPagerTTY(t, rc.Out, tty, 2, 80)
	if code := run(rc, nil); code != 0 {
		t.Fatalf("code=%d err=%q", code, errOut.String())
	}
}

func TestPagerPreservesBytesLongLinesAndSqueeze(t *testing.T) {
	input := append([]byte("a\r\x00"), bytes.Repeat([]byte{'z'}, 10000)...)
	input = append(input, '\n', '\n', '\n', 'b')
	var out, errOut bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{In: bytes.NewReader(input), Out: &out, Err: &errOut}}
	withPagerTTY(t, rc.Out, commandTTY(strings.Repeat(" ", 3000)+"q"), 1000, 5)
	if code := run(rc, []string{"-s"}); code != 0 {
		t.Fatalf("code=%d err=%q", code, errOut.String())
	}
	want := bytes.Replace(input, []byte("\n\n\n"), []byte("\n\n"), 1)
	if !bytes.Equal(out.Bytes(), want) {
		t.Fatalf("byte preservation: got %d bytes want %d", out.Len(), len(want))
	}
}

func TestPagerTerminalAndSourceErrors(t *testing.T) {
	tests := []struct {
		name string
		in   io.Reader
		tty  *ttyChannel
		want string
	}{
		{"tty eof", strings.NewReader("x\n"), commandTTY(""), "terminal read error"},
		{"tty read", strings.NewReader("x\n"), &ttyChannel{readCommand: func(context.Context) (byte, error) { return 0, errors.New("tty bad") }, close: func() error { return nil }}, "tty bad"},
		{"source read", errorReader{}, commandTTY("q"), "Simulated read error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			rc := &tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{In: tt.in, Out: &out, Err: &errOut}}
			withPagerTTY(t, rc.Out, tt.tty, 24, 80)
			if code := run(rc, nil); code == 0 || !strings.Contains(errOut.String(), tt.want) {
				t.Fatalf("code=%d err=%q", code, errOut.String())
			}
		})
	}
}

func TestPagerTTYCloseError(t *testing.T) {
	var out, errOut bytes.Buffer
	tty := commandTTY("q")
	tty.close = func() error { return errors.New("close bad") }
	rc := &tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{In: strings.NewReader("x\n"), Out: &out, Err: &errOut}}
	withPagerTTY(t, rc.Out, tty, 24, 80)
	if code := run(rc, nil); code == 0 || !strings.Contains(errOut.String(), "Close bad") {
		t.Fatalf("code=%d err=%q", code, errOut.String())
	}
}

type closeErrorReader struct{ io.Reader }

func (closeErrorReader) Close() error { return errors.New("source close bad") }

func TestPagerSourceCloseError(t *testing.T) {
	orig := openInput
	openInput = func(*tool.RunContext, string) (io.Reader, io.Closer, error) {
		r := closeErrorReader{Reader: strings.NewReader("x\n")}
		return r, r, nil
	}
	t.Cleanup(func() { openInput = orig })
	var out, errOut bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &errOut}}
	withPagerTTY(t, rc.Out, commandTTY(" "), 24, 80)
	if code := run(rc, []string{"-e"}); code == 0 || !strings.Contains(errOut.String(), "Source close bad") {
		t.Fatalf("code=%d err=%q", code, errOut.String())
	}
}

func TestPagerOpenTTYFailureIsNotCopyFallback(t *testing.T) {
	origTerm, origOpen := isTerminal, openTTY
	var out, errOut bytes.Buffer
	isTerminal = func(io.Writer) bool { return true }
	openTTY = func(*tool.RunContext) (*ttyChannel, error) { return nil, errors.New("no controlling tty") }
	t.Cleanup(func() { isTerminal, openTTY = origTerm, origOpen })
	rc := &tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{In: strings.NewReader("secret\n"), Out: &out, Err: &errOut}}
	if code := run(rc, nil); code == 0 || out.Len() != 0 || !strings.Contains(errOut.String(), "No controlling tty") {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), errOut.String())
	}
}

func TestPagerUnsupportedCommandFails(t *testing.T) {
	for _, command := range []string{"b", "z", "Q", "\n"} {
		var out, errOut bytes.Buffer
		rc := &tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{In: strings.NewReader("x\ny\n"), Out: &out, Err: &errOut}}
		withPagerTTY(t, rc.Out, commandTTY(command), 2, 80)
		if code := run(rc, nil); code == 0 || !strings.Contains(errOut.String(), "command") {
			t.Fatalf("command %q: code=%d err=%q", command, code, errOut.String())
		}
	}
}
