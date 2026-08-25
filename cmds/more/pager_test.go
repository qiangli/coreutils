package morecmd

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/qiangli/coreutils/tool"
)

// A fake TTY replacement for tests
func mockTerminal(out io.Writer, mockTTYChannel *ttyChannel, size func() (int, int, error)) func() {
	origIsTerm := isTerminal
	origOpenTTY := openTTY
	origGetSize := getTerminalSize

	isTerminal = func(w io.Writer) bool { return out == w }
	openTTY = func(rc *tool.RunContext) (*ttyChannel, bool) {
		if mockTTYChannel == nil {
			return nil, false
		}
		return mockTTYChannel, true
	}
	getTerminalSize = func(fd int) (int, int, error) {
		if size != nil {
			return size()
		}
		return 80, 24, nil
	}

	return func() {
		isTerminal = origIsTerm
		openTTY = origOpenTTY
		getTerminalSize = origGetSize
	}
}

func TestPagerSizing(t *testing.T) {
	rc := &tool.RunContext{
		Env: []string{},
		Stdio: tool.Stdio{
			Out: new(bytes.Buffer),
			Err: new(bytes.Buffer),
		},
	}

	// Default size
	cleanup := mockTerminal(rc.Out, &ttyChannel{cmds: strings.NewReader("q"), hasFd: true, close: func() error { return nil }}, nil)
	defer cleanup()

	// -n
	r, w := terminalSize(rc, nil, 10)
	if r != 10 || w != 80 {
		t.Errorf("expected 10,80 got %d,%d", r, w)
	}

	// LINES / COLUMNS
	rc.Env = []string{"LINES=15", "COLUMNS=40"}
	r, w = terminalSize(rc, nil, 0)
	if r != 15 || w != 40 {
		t.Errorf("expected 15,40 got %d,%d", r, w)
	}
}

func TestPagerFirstNextPages(t *testing.T) {
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Stdio: tool.Stdio{Out: out, Err: errOut},
	}

	// 3 lines per screenful
	cleanup := mockTerminal(rc.Out, &ttyChannel{cmds: strings.NewReader(" q"), hasFd: true, close: func() error { return nil }}, func() (int, int, error) {
		return 80, 4, nil
	})
	defer cleanup()

	dir := t.TempDir()
	rc.Dir = dir
	f := filepath.Join(dir, "f1")
	os.WriteFile(f, []byte("1\n2\n3\n4\n5\n"), 0644)

	code := run(rc, []string{f})
	if code != 0 {
		t.Errorf("expected 0, got %d", code)
	}

	expected := "1\n2\n3\n\x1b[7m--More--\x1b[m\r\x1b[K4\n5\n\x1b[7m--More--(END)\x1b[m\r\x1b[K"
	if out.String() != expected {
		t.Errorf("got %q, expected %q", out.String(), expected)
	}
}

// runPager is a test helper: wires a fake tty channel (working unless
// openOK is false), a fixed rows/cols tty-size seam, and runs more.
func runPager(t *testing.T, rows, cols int, openOK bool, cmds string, args []string, setup func(rc *tool.RunContext)) (string, string, int) {
	t.Helper()
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Stdio: tool.Stdio{Out: out, Err: errOut},
	}
	if setup != nil {
		setup(rc)
	}

	var ch *ttyChannel
	if openOK {
		ch = &ttyChannel{cmds: strings.NewReader(cmds), hasFd: true, close: func() error { return nil }}
	}
	cleanup := mockTerminal(rc.Out, ch, func() (int, int, error) { return cols, rows, nil })
	defer cleanup()

	code := run(rc, args)
	return out.String(), errOut.String(), code
}

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPagerPromptNextFile(t *testing.T) {
	dir := t.TempDir()
	f1 := writeFile(t, dir, "f1", "a\n")
	f2 := writeFile(t, dir, "f2", "b\n")

	out, errb, code := runPager(t, 24, 80, true, " q", []string{f1, f2}, func(rc *tool.RunContext) { rc.Dir = dir })
	if code != 0 || errb != "" {
		t.Fatalf("next-file prompt: code=%d err=%q", code, errb)
	}
	want := "a\n\x1b[7m--More--(Next file: " + f2 + ")\x1b[m\r\x1b[Kb\n\x1b[7m--More--(END)\x1b[m\r\x1b[K"
	if out != want {
		t.Fatalf("next-file prompt:\n got %q\nwant %q", out, want)
	}
}

func TestPagerQuitMidScreen(t *testing.T) {
	dir := t.TempDir()
	f := writeFile(t, dir, "f", "a\nb\nc\n")

	// rows=2 -> screenful=1: a mid-screen prompt fires after the first line.
	out, errb, code := runPager(t, 2, 80, true, "q", []string{f}, func(rc *tool.RunContext) { rc.Dir = dir })
	if code != 0 || errb != "" {
		t.Fatalf("quit mid-screen: code=%d err=%q", code, errb)
	}
	want := "a\n\x1b[7m--More--\x1b[m\r\x1b[K"
	if out != want {
		t.Fatalf("quit mid-screen:\n got %q\nwant %q", out, want)
	}
}

func TestPagerUnavailableTTY(t *testing.T) {
	dir := t.TempDir()
	f := writeFile(t, dir, "f", "a\nb\nc\n")

	// isTerminal true, openTTY fails -> fail closed to the non-interactive
	// copy path rather than hanging on a command channel that isn't there.
	out, errb, code := runPager(t, 2, 80, false, "", []string{f}, func(rc *tool.RunContext) { rc.Dir = dir })
	if code != 0 || errb != "" {
		t.Fatalf("unavailable tty: code=%d err=%q", code, errb)
	}
	if out != "a\nb\nc\n" {
		t.Fatalf("unavailable tty: got %q, want byte-exact passthrough", out)
	}
}

func TestPagerChannelEOF(t *testing.T) {
	dir := t.TempDir()
	f := writeFile(t, dir, "f", "a\nb\n")

	// rows=2 -> screenful=1: prompt fires after "a\n"; the command channel
	// is empty, so the very first read hits EOF.
	out, errb, code := runPager(t, 2, 80, true, "", []string{f}, func(rc *tool.RunContext) { rc.Dir = dir })
	if code != 0 || errb != "" {
		t.Fatalf("channel EOF: code=%d err=%q", code, errb)
	}
	// EOF on the command channel returns without clearing the prompt
	// (unlike an explicit quit, which erases it first).
	want := "a\n\x1b[7m--More--\x1b[m"
	if out != want {
		t.Fatalf("channel EOF:\n got %q\nwant %q", out, want)
	}
}

func TestPagerExitOnEOF(t *testing.T) {
	dir := t.TempDir()
	f := writeFile(t, dir, "f", "a\nb\n")

	out, errb, code := runPager(t, 24, 80, true, "", []string{"-e", f}, func(rc *tool.RunContext) { rc.Dir = dir })
	if code != 0 || errb != "" {
		t.Fatalf("-e exit-on-eof: code=%d err=%q", code, errb)
	}
	if out != "a\nb\n" {
		t.Fatalf("-e exit-on-eof: got %q, want plain content with no final prompt", out)
	}
}

func TestPagerPlainBackspaceCR(t *testing.T) {
	dir := t.TempDir()
	f := writeFile(t, dir, "f", "a\bb\rc\n")

	out, errb, code := runPager(t, 24, 80, true, "q", []string{"-u", f}, func(rc *tool.RunContext) { rc.Dir = dir })
	if code != 0 || errb != "" {
		t.Fatalf("-u plain: code=%d err=%q", code, errb)
	}
	// -u: backspace/CR are printable, not column-control -> content passes
	// through unmodified, same as the non-terminal path would render it.
	want := "a\bb\rc\n\x1b[7m--More--(END)\x1b[m\r\x1b[K"
	if out != want {
		t.Fatalf("-u plain: got %q, want %q", out, want)
	}
}

func TestPagerSqueezeInteractive(t *testing.T) {
	dir := t.TempDir()
	f := writeFile(t, dir, "f", "one\n\n\ntwo\n")

	out, errb, code := runPager(t, 24, 80, true, "q", []string{"-s", f}, func(rc *tool.RunContext) { rc.Dir = dir })
	if code != 0 || errb != "" {
		t.Fatalf("-s squeeze: code=%d err=%q", code, errb)
	}
	want := "one\n\ntwo\n\x1b[7m--More--(END)\x1b[m\r\x1b[K"
	if out != want {
		t.Fatalf("-s squeeze:\n got %q\nwant %q", out, want)
	}
}

func TestPagerCommandOptionQuit(t *testing.T) {
	dir := t.TempDir()
	f := writeFile(t, dir, "f", "a\nb\n")

	// -p is executed at the file's first screen before any content is
	// printed; a quit command there means the file is never shown.
	out, errb, code := runPager(t, 24, 80, true, "", []string{"-p", "q", f}, func(rc *tool.RunContext) { rc.Dir = dir })
	if code != 0 || errb != "" {
		t.Fatalf("-p q: code=%d err=%q", code, errb)
	}
	if out != "" {
		t.Fatalf("-p q: got %q, want no content printed", out)
	}
}

func TestPagerCommandOptionDeferredPerFile(t *testing.T) {
	dir := t.TempDir()
	f1 := writeFile(t, dir, "f1", "a\n")
	f2 := writeFile(t, dir, "f2", "b\n")

	// -p runs at every new file's first screen, not just the first file's.
	out, errb, code := runPager(t, 24, 80, true, " q", []string{"-p", "z", f1, f2}, func(rc *tool.RunContext) { rc.Dir = dir })
	if code != 0 {
		t.Fatalf("-p z: code=%d", code)
	}
	if n := strings.Count(errb, "more: unknown command: z (deferred)"); n != 2 {
		t.Fatalf("-p z: expected 2 deferred warnings (one per file), got %d in %q", n, errb)
	}
	want := "a\n\x1b[7m--More--(Next file: " + f2 + ")\x1b[m\r\x1b[Kb\n\x1b[7m--More--(END)\x1b[m\r\x1b[K"
	if out != want {
		t.Fatalf("-p z:\n got %q\nwant %q", out, want)
	}
}

func TestPagerOutputError(t *testing.T) {
	dir := t.TempDir()
	f := writeFile(t, dir, "f", "a\n")

	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Stdio: tool.Stdio{Out: shortWriter{}, Err: errOut},
	}
	ch := &ttyChannel{cmds: strings.NewReader("q"), hasFd: true, close: func() error { return nil }}
	cleanup := mockTerminal(rc.Out, ch, func() (int, int, error) { return 80, 24, nil })
	defer cleanup()

	code := run(rc, []string{f})
	if code == 0 {
		t.Fatalf("expected non-zero exit on output error, got 0")
	}
	if !strings.Contains(errOut.String(), "simulated short write") {
		t.Fatalf("expected write error message, got %q", errOut.String())
	}
	_ = out
}

func TestPagerReadError(t *testing.T) {
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   t.TempDir(),
		Stdio: tool.Stdio{In: errorReader{}, Out: out, Err: errOut},
	}
	ch := &ttyChannel{cmds: strings.NewReader("q"), hasFd: true, close: func() error { return nil }}
	cleanup := mockTerminal(rc.Out, ch, func() (int, int, error) { return 80, 24, nil })
	defer cleanup()

	code := run(rc, []string{})
	if code == 0 {
		t.Fatalf("expected non-zero exit on read error, got 0")
	}
	if !strings.Contains(strings.ToLower(errOut.String()), "simulated read error") {
		t.Fatalf("expected read error message, got %q", errOut.String())
	}
}

func TestPagerContextCancellation(t *testing.T) {
	dir := t.TempDir()
	f := writeFile(t, dir, "f", "a\n")

	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	rc := &tool.RunContext{
		Ctx:   ctx,
		Dir:   dir,
		Stdio: tool.Stdio{Out: out, Err: errOut},
	}
	// The command channel would hang forever if read; a canceled context
	// must short-circuit before it is ever touched.
	ch := &ttyChannel{cmds: blockingReader{}, hasFd: true, close: func() error { return nil }}
	cleanup := mockTerminal(rc.Out, ch, func() (int, int, error) { return 80, 24, nil })
	defer cleanup()

	code := run(rc, []string{f})
	if code != 0 {
		t.Fatalf("canceled context: expected exit 0, got %d", code)
	}
	if out.String() != "" {
		t.Fatalf("canceled context: expected no output, got %q", out.String())
	}
}

func TestPagerCleanPrintRedraw(t *testing.T) {
	dir := t.TempDir()
	f := writeFile(t, dir, "f", "a\n")

	out, errb, code := runPager(t, 24, 80, true, "q", []string{"-c", f}, func(rc *tool.RunContext) { rc.Dir = dir })
	if code != 0 || errb != "" {
		t.Fatalf("-c clean-print: code=%d err=%q", code, errb)
	}
	if !strings.HasPrefix(out, "\x1b[H\x1b[2J") {
		t.Fatalf("-c clean-print: expected leading redraw escape, got %q", out)
	}
}

// blockingReader never returns, standing in for a command channel that a
// real terminal would leave open indefinitely; used only to prove context
// cancellation is checked before any read is attempted.
type blockingReader struct{}

func (blockingReader) Read(p []byte) (n int, err error) {
	select {}
}

// TestPagerNewlineDeferred pins that <newline> — a more(1) command that
// scrolls ONE line, not a screenful — is refused as deferred rather than
// quietly aliased to <space>. Answering a different command than the one
// typed is the silent-approximation failure the contract forbids.
func TestPagerNewlineDeferred(t *testing.T) {
	dir := t.TempDir()
	f := writeFile(t, dir, "f", "a\nb\n")

	// rows=2 -> screenful=1: the prompt fires after "a\n".
	out, errb, code := runPager(t, 2, 80, true, "\nq", []string{f}, func(rc *tool.RunContext) { rc.Dir = dir })
	if code != 0 {
		t.Fatalf("newline deferred: code=%d", code)
	}
	if !strings.Contains(errb, "more: newline: command not supported yet (deferred)") {
		t.Fatalf("newline deferred: want deferred diagnostic, got %q", errb)
	}
	// The prompt is cleared and reissued; "b" is never reached, because
	// newline did not advance the screen.
	p := "\x1b[7m--More--\x1b[m"
	want := "a\n" + p + "\r\x1b[K" + p + "\r\x1b[K"
	if out != want {
		t.Fatalf("newline deferred:\n got %q\nwant %q", out, want)
	}
}

// TestPagerDeferredVsUnknown separates the two refusals: a POSIX more
// command this slice has not implemented is named as deferred, while a key
// that is no more(1) command at all is named as unknown. Both are loud.
func TestPagerDeferredVsUnknown(t *testing.T) {
	dir := t.TempDir()
	f := writeFile(t, dir, "f", "a\n")

	_, errb, code := runPager(t, 24, 80, true, "b\x06zq", []string{f}, func(rc *tool.RunContext) { rc.Dir = dir })
	if code != 0 {
		t.Fatalf("deferred vs unknown: code=%d", code)
	}
	for _, want := range []string{
		"more: b: command not supported yet (deferred)",
		"more: ^F: command not supported yet (deferred)",
		"more: unknown command: z (deferred)",
	} {
		if !strings.Contains(errb, want) {
			t.Fatalf("deferred vs unknown: missing %q in %q", want, errb)
		}
	}
}

// TestPagerFoldsLongLine checks plain-character column accounting: a line
// longer than the screen width folds at the margin and the folded segment
// costs the screenful a line.
func TestPagerFoldsLongLine(t *testing.T) {
	dir := t.TempDir()
	f := writeFile(t, dir, "f", "abcdefgh\n")

	out, errb, code := runPager(t, 24, 4, true, "q", []string{f}, func(rc *tool.RunContext) { rc.Dir = dir })
	if code != 0 || errb != "" {
		t.Fatalf("fold: code=%d err=%q", code, errb)
	}
	want := "abcd\nefgh\n\x1b[7m--More--(END)\x1b[m\r\x1b[K"
	if out != want {
		t.Fatalf("fold:\n got %q\nwant %q", out, want)
	}
}

// TestPagerTabColumnAccounting checks that a tab advances to the next
// 8-column stop and that overflowing the margin folds AND counts a line.
func TestPagerTabColumnAccounting(t *testing.T) {
	dir := t.TempDir()

	// The fold costs the screenful a line: with screenful=1 it is the
	// wrapped tab, not a newline, that makes the next character prompt.
	f := writeFile(t, dir, "f", strings.Repeat("a", 17)+"\tX\n")
	out, errb, code := runPager(t, 2, 20, true, " q", []string{f}, func(rc *tool.RunContext) { rc.Dir = dir })
	if code != 0 || errb != "" {
		t.Fatalf("tab accounting: code=%d err=%q", code, errb)
	}
	want := strings.Repeat("a", 17) + "\n\t" +
		"\x1b[7m--More--\x1b[m\r\x1b[K" + "X\n" +
		"\x1b[7m--More--(END)\x1b[m\r\x1b[K"
	if out != want {
		t.Fatalf("tab accounting:\n got %q\nwant %q", out, want)
	}

	// And the column the fold lands on is exact: 17 columns + a tab stops
	// at 24, which is 4 columns past a 20-column margin, so exactly 16
	// more characters fit on the folded line before the next fold.
	f2 := writeFile(t, dir, "f2", strings.Repeat("a", 17)+"\t"+strings.Repeat("b", 17)+"\n")
	out, errb, code = runPager(t, 24, 20, true, "q", []string{f2}, func(rc *tool.RunContext) { rc.Dir = dir })
	if code != 0 || errb != "" {
		t.Fatalf("tab fold column: code=%d err=%q", code, errb)
	}
	want = strings.Repeat("a", 17) + "\n\t" + strings.Repeat("b", 16) + "\nb\n" +
		"\x1b[7m--More--(END)\x1b[m\r\x1b[K"
	if out != want {
		t.Fatalf("tab fold column:\n got %q\nwant %q", out, want)
	}
}

// TestPagerBackspaceColumnAccounting contrasts the default rendering —
// backspace moves the cursor back a column — with -u, where it is just
// another printable character and so pushes the line over the margin.
func TestPagerBackspaceColumnAccounting(t *testing.T) {
	dir := t.TempDir()
	f := writeFile(t, dir, "f", "ab\bcde\n")

	out, errb, code := runPager(t, 24, 4, true, "q", []string{f}, func(rc *tool.RunContext) { rc.Dir = dir })
	if code != 0 || errb != "" {
		t.Fatalf("backspace accounting: code=%d err=%q", code, errb)
	}
	if want := "ab\bcde\n\x1b[7m--More--(END)\x1b[m\r\x1b[K"; out != want {
		t.Fatalf("backspace accounting:\n got %q\nwant %q", out, want)
	}

	out, errb, code = runPager(t, 24, 4, true, "q", []string{"-u", f}, func(rc *tool.RunContext) { rc.Dir = dir })
	if code != 0 || errb != "" {
		t.Fatalf("-u backspace accounting: code=%d err=%q", code, errb)
	}
	if want := "ab\bc\nde\n\x1b[7m--More--(END)\x1b[m\r\x1b[K"; out != want {
		t.Fatalf("-u backspace accounting:\n got %q\nwant %q", out, want)
	}
}

// promptWatcher is an out sink that closes seen once the prompt has been
// flushed to it, so a test can act at the exact moment the pager is parked
// on the command channel.
type promptWatcher struct {
	mu   sync.Mutex
	buf  bytes.Buffer
	seen chan struct{}
	once sync.Once
}

func (w *promptWatcher) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n, err := w.buf.Write(p)
	if strings.Contains(w.buf.String(), "--More--") {
		w.once.Do(func() { close(w.seen) })
	}
	return n, err
}

func (w *promptWatcher) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

// TestPagerCancelWhileAwaitingCommand is the no-hang guarantee: the pager
// is parked on a command channel that will never produce a keystroke (what
// a real terminal looks like while nobody types), and cancellation alone
// must unblock it.
func TestPagerCancelWhileAwaitingCommand(t *testing.T) {
	dir := t.TempDir()
	f := writeFile(t, dir, "f", "a\nb\n")

	out := &promptWatcher{seen: make(chan struct{})}
	errOut := new(bytes.Buffer)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rc := &tool.RunContext{
		Ctx:   ctx,
		Dir:   dir,
		Stdio: tool.Stdio{Out: out, Err: errOut},
	}
	ch := &ttyChannel{cmds: blockingReader{}, hasFd: true, close: func() error { return nil }}
	cleanup := mockTerminal(rc.Out, ch, func() (int, int, error) { return 80, 2, nil })
	defer cleanup()

	go func() {
		<-out.seen
		cancel()
	}()

	done := make(chan int, 1)
	go func() { done <- run(rc, []string{f}) }()

	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("cancel while awaiting command: code=%d err=%q", code, errOut.String())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("cancel while awaiting command: more did not return — the command channel read ignored cancellation")
	}
	if got := out.String(); !strings.HasPrefix(got, "a\n\x1b[7m--More--\x1b[m") {
		t.Fatalf("cancel while awaiting command: got %q", got)
	}
}
