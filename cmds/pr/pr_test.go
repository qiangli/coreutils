package prcmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/coreutils/tool"
)

type firstWriteSignal struct{ ch chan struct{} }

func (w firstWriteSignal) Write(p []byte) (int, error) {
	select {
	case w.ch <- struct{}{}:
	default:
	}
	return len(p), nil
}

type orderedWriter struct {
	events *[]string
	label  string
}

func (w orderedWriter) Write(p []byte) (int, error) {
	*w.events = append(*w.events, w.label)
	return len(p), nil
}

func runPR(t *testing.T, dir, stdin string, args ...string) (string, string, int) {
	return runPREnv(t, dir, stdin, nil, args...)
}

func runPREnv(t *testing.T, dir, stdin string, env []string, args ...string) (string, string, int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Env:   env,
		Stdio: tool.Stdio{In: strings.NewReader(stdin), Out: &out, Err: &errb},
	}
	code := cmd.Run(rc, args)
	return out.String(), errb.String(), code
}

// writeFixed writes a file and pins its mtime so pr headers are
// deterministic in tests.
func writeFixed(t *testing.T, dir, name, content string) time.Time {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	stamp := time.Date(2020, 1, 2, 3, 4, 0, 0, time.Local)
	if err := os.Chtimes(path, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	return stamp
}

func TestPROmitHeaderPassesContent(t *testing.T) {
	out, errb, code := runPR(t, t.TempDir(), "a\nb\n", "-t")
	if out != "a\nb\n" || errb != "" || code != 0 {
		t.Fatalf("pr -t = (%q, %q, %d)", out, errb, code)
	}
}

func TestPRDefaultPageStructure(t *testing.T) {
	dir := t.TempDir()
	writeFixed(t, dir, "in", "l1\nl2\nl3\n")
	out, errb, code := runPR(t, dir, "", "in")
	if errb != "" || code != 0 {
		t.Fatalf("pr default = (%q, %d)", errb, code)
	}
	header := "Jan  2 03:04 2020 in Page 1"
	want := "\n\n" + header + "\n\n\n" + "l1\nl2\nl3\n" + strings.Repeat("\n", 58)
	if out != want {
		t.Fatalf("pr default page = %q, want %q", out, want)
	}
	if n := strings.Count(out, "\n"); n != 66 {
		t.Fatalf("pr default page has %d lines, want 66", n)
	}
}

func TestPRHeaderHonorsInvocationTZAndLCTime(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "in")
	if err := os.WriteFile(path, []byte("content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stamp := time.Date(2020, time.March, 2, 8, 4, 0, 0, time.UTC)
	if err := os.Chtimes(path, stamp, stamp); err != nil {
		t.Fatal(err)
	}

	out, errb, code := runPREnv(t, dir, "", []string{"TZ=EST5", "LC_TIME=de_DE.UTF-8"}, "in")
	if code != 0 || errb != "" || !strings.Contains(out, "Mär  2 03:04 2020 in Page 1") {
		t.Fatalf("localized header = (%q, %q, %d)", out, errb, code)
	}
	out, errb, code = runPREnv(t, dir, "", []string{"TZ=UTC0", "LC_TIME=de_DE.UTF-8", "LC_ALL=C"}, "in")
	if code != 0 || errb != "" || !strings.Contains(out, "Mar  2 08:04 2020 in Page 1") {
		t.Fatalf("LC_ALL/TZ header = (%q, %q, %d)", out, errb, code)
	}
	out, errb, code = runPREnv(t, dir, "", []string{"LC_TIME=fr_FR.UTF-8"}, "in")
	if code != 1 || out != "" || !strings.Contains(errb, "LC_TIME") {
		t.Fatalf("unsupported LC_TIME = (%q, %q, %d), want fail-closed", out, errb, code)
	}
}

func TestPRStreamsCompletePageBeforeEOF(t *testing.T) {
	// A pipe or FIFO producer may remain open while consuming pr's output.
	// GNU pr emits each completed page without waiting for input EOF.
	reader, writer := io.Pipe()
	wrote := make(chan struct{}, 1)
	done := make(chan int, 1)
	rc := &tool.RunContext{
		Ctx: context.Background(), Dir: t.TempDir(),
		Stdio: tool.Stdio{In: reader, Out: firstWriteSignal{ch: wrote}, Err: io.Discard},
	}
	go func() { done <- cmd.Run(rc, nil) }()

	// Default pages contain 56 body lines. Keep the input open after one page
	// to prove output does not depend on EOF.
	if _, err := io.WriteString(writer, strings.Repeat("x\n", 56)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-wrote:
	case <-time.After(time.Second):
		writer.Close()
		t.Fatal("pr produced no complete page while pipe remained open")
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("pr exited %d", code)
		}
	case <-time.After(time.Second):
		t.Fatal("pr did not finish after pipe EOF")
	}
}

func TestPRMultiColumnStreamsCompletePageBeforeEOF(t *testing.T) {
	for _, tt := range []struct {
		name  string
		args  []string
		lines int
	}{
		{name: "headerless-vertical", args: []string{"-t", "-l", "3", "-2"}, lines: 6},
		{name: "headerless-across", args: []string{"-t", "-l", "3", "-a", "-2"}, lines: 6},
		{name: "paginated-vertical", args: []string{"-l", "12", "-2"}, lines: 4},
		{name: "paginated-across", args: []string{"-l", "12", "-a", "-2"}, lines: 4},
	} {
		t.Run(tt.name, func(t *testing.T) {
			reader, writer := io.Pipe()
			wrote := make(chan struct{}, 1)
			done := make(chan int, 1)
			rc := &tool.RunContext{
				Ctx: context.Background(), Dir: t.TempDir(),
				Stdio: tool.Stdio{In: reader, Out: firstWriteSignal{ch: wrote}, Err: io.Discard},
			}
			go func() { done <- cmd.Run(rc, tt.args) }()

			// Keep the producer open after one full page: output must depend on
			// a page of lookahead, not on EOF.
			if _, err := io.WriteString(writer, strings.Repeat("x\n", tt.lines)); err != nil {
				t.Fatal(err)
			}
			select {
			case <-wrote:
			case <-time.After(time.Second):
				writer.Close()
				t.Fatal("pr produced no multi-column page while pipe remained open")
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			select {
			case code := <-done:
				if code != 0 {
					t.Fatalf("pr exited %d", code)
				}
			case <-time.After(time.Second):
				t.Fatal("pr did not finish after pipe EOF")
			}
		})
	}
}

func TestPRSingleColumnNeverTruncatedByDefault(t *testing.T) {
	dir := t.TempDir()
	writeFixed(t, dir, "in", "abcdef\n")
	out, _, code := runPR(t, dir, "", "-t", "-w", "3", "in")
	if out != "abcdef\n" || code != 0 {
		t.Fatalf("pr -w must not truncate single-column output = (%q, %d)", out, code)
	}
	out, _, code = runPR(t, dir, "", "-t", "-W", "3", "in")
	if out != "abc\n" || code != 0 {
		t.Fatalf("pr -W truncates = (%q, %d), want abc", out, code)
	}
}

func TestPRShortPageLengthImpliesOmitHeader(t *testing.T) {
	out, _, code := runPR(t, t.TempDir(), "a\nb\nc\n", "-l", "3")
	if out != "a\nb\nc\n" || code != 0 {
		t.Fatalf("pr -l3 (<=10 implies -t) = (%q, %d), want passthrough", out, code)
	}
}

func TestPRNumberIndentAndDoubleSpace(t *testing.T) {
	out, errb, code := runPR(t, t.TempDir(), "a\nb\n", "-t", "-n", "-o", "2", "-d")
	want := "      1\ta\n\n      2\tb\n\n"
	if out != want || errb != "" || code != 0 {
		t.Fatalf("pr line controls = (%q, %q, %d), want (%q, \"\", 0)", out, errb, code, want)
	}
}

func TestPRCustomHeaderAndTOmitPagination(t *testing.T) {
	out, _, code := runPR(t, t.TempDir(), "a\n", "-h", "TITLE", "-w", "50")
	if code != 0 || !strings.Contains(out, "TITLE") {
		t.Fatalf("pr custom header = (%q, %d), want TITLE", out, code)
	}

	out, _, code = runPR(t, t.TempDir(), "a\nb\nc\n", "-T", "-l", "2")
	if out != "a\nb\nc\n" || code != 0 {
		t.Fatalf("pr -T = (%q, %d), want passthrough", out, code)
	}
}

func TestPRPagesRangeAndDateFormat(t *testing.T) {
	out, _, code := runPR(t, t.TempDir(), "a\nb\nc\nd\n", "--pages", "2", "-l", "13", "-D", "%Y")
	if code != 0 {
		t.Fatalf("pr pages exited %d", code)
	}
	if strings.Contains(out, "Page 1") || !strings.Contains(out, "Page 2") || strings.Contains(out, "a\n") || !strings.Contains(out, "d\n") {
		t.Fatalf("pr pages = %q, want only page 2", out)
	}
	if n := strings.Count(out, "\n"); n != 13 {
		t.Fatalf("pr page 2 has %d lines, want 13", n)
	}
}

func TestPRPlusOperandPageRange(t *testing.T) {
	out, _, code := runPR(t, t.TempDir(), "a\nb\nc\nd\n", "-l", "13", "+2")
	if code != 0 || strings.Contains(out, "Page 1") || !strings.Contains(out, "Page 2") {
		t.Fatalf("pr +2 = (%q, %d), want only page 2", out, code)
	}

	_, errb, code := runPR(t, t.TempDir(), "", "+0")
	if code != 2 || !strings.Contains(errb, "invalid page range") {
		t.Fatalf("pr +0 code=%d err=%q", code, errb)
	}
}

func TestPRDoubleDashProtectsPlusOperand(t *testing.T) {
	dir := t.TempDir()
	writeFixed(t, dir, "+2", "protected\n")
	out, errb, code := runPR(t, dir, "", "-t", "--", "+2")
	if out != "protected\n" || errb != "" || code != 0 {
		t.Fatalf("pr -- +2 = (%q, %q, %d), want protected file content", out, errb, code)
	}
}

func TestPRFormFeedTrailer(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"-F", []string{"-F"}},
		{"-f", []string{"-f"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, _, code := runPR(t, t.TempDir(), "a\nb\n", tt.args...)
			if code != 0 || strings.Count(out, "\f") != 1 || !strings.HasSuffix(out, "b\n\f") {
				t.Fatalf("pr %s = (%q, %d), want single trailing form feed", tt.name, out, code)
			}
		})
	}
}

func TestPRFormFeedLowerEqualsUpper(t *testing.T) {
	outF, _, codeF := runPR(t, t.TempDir(), "a\nb\n", "-F")
	outf, _, codef := runPR(t, t.TempDir(), "a\nb\n", "-f")
	if codeF != 0 || codef != 0 {
		t.Fatalf("pr -F/-f exit codes: %d, %d", codeF, codef)
	}
	if outF != outf {
		t.Fatalf("pr -f output differs from pr -F:\n-F: %q\n-f: %q", outF, outf)
	}
}

// withTerminalStdout makes isTerminalFn report standard output as a
// terminal for the duration of the test, restoring it on cleanup. Every
// other -p/-f test in this file that does NOT call this proves the
// noninteractive path (a bytes.Buffer, exactly what runPR uses): with
// isTerminalFn left at its real default, that is always false.
func withTerminalStdout(t *testing.T) {
	t.Helper()
	old := isTerminalFn
	isTerminalFn = func(*tool.RunContext) bool { return true }
	t.Cleanup(func() { isTerminalFn = old })
}

// withControlTTY overrides the /dev/tty seam and returns a pointer to a
// counter of how many times it was opened, so tests can assert exactly one
// pause (-f) versus one per page (-p).
func withControlTTY(t *testing.T, open func() (io.ReadCloser, error)) *int {
	t.Helper()
	old := openControlTTYFn
	calls := 0
	openControlTTYFn = func() (io.ReadCloser, error) {
		calls++
		return open()
	}
	t.Cleanup(func() { openControlTTYFn = old })
	return &calls
}

// withNoInterrupt overrides the interrupt seam with a channel that never
// fires, so a pause under test runs to normal completion.
func withNoInterrupt(t *testing.T) {
	t.Helper()
	old := watchInterruptFn
	watchInterruptFn = func() (chan os.Signal, func()) { return make(chan os.Signal), func() {} }
	t.Cleanup(func() { watchInterruptFn = old })
}

type failingReader struct{ err error }

func (f failingReader) Read([]byte) (int, error) { return 0, f.err }

type failingWriter struct{ err error }

func (f failingWriter) Write([]byte) (int, error) { return 0, f.err }

type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) { return len(p) - 1, nil }

func TestPRMergeAssumesPOSIXTabExpansionAndReplacement(t *testing.T) {
	dir := t.TempDir()
	writeFixed(t, dir, "one", "a\tb\n")
	writeFixed(t, dir, "two", "x\n")
	out, errb, code := runPR(t, dir, "", "-m", "-t", "-w", "10", "-s|", "one", "two")
	if code != 0 || errb != "" || out != "a    |x\n" {
		t.Fatalf("pr -m implicit -e/-i = (%q, %q, %d), want %q", out, errb, code, "a    |x\n")
	}
}

func TestPRTerminalDefersFileDiagnosticsUntilOutputCompletes(t *testing.T) {
	withTerminalStdout(t)
	dir := t.TempDir()
	writeFixed(t, dir, "good", "content\n")
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"serial", []string{"missing", "good"}},
		{"merge", []string{"-m", "missing", "good"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var events []string
			rc := &tool.RunContext{
				Ctx: context.Background(), Dir: dir,
				Stdio: tool.Stdio{
					In:  strings.NewReader(""),
					Out: orderedWriter{events: &events, label: "output"},
					Err: orderedWriter{events: &events, label: "diagnostic"},
				},
			}
			if code := cmd.Run(rc, tc.args); code != 1 {
				t.Fatalf("pr terminal diagnostic status = %d, want 1", code)
			}
			if len(events) < 2 || events[len(events)-1] != "diagnostic" {
				t.Fatalf("events = %v, want all output before the deferred diagnostic", events)
			}
			for _, event := range events[:len(events)-1] {
				if event != "output" {
					t.Fatalf("events = %v, diagnostic was not deferred", events)
				}
			}
		})
	}
}

func TestPRStreamReadAndShortWriteFailuresAreNonzero(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   io.Reader
		out  io.Writer
		want string
	}{
		{"read", failingReader{err: errors.New("input failure")}, io.Discard, "input failure"},
		{"short-write", strings.NewReader("content\n"), shortWriter{}, io.ErrShortWrite.Error()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var errb bytes.Buffer
			rc := &tool.RunContext{
				Ctx: context.Background(), Dir: t.TempDir(),
				Stdio: tool.Stdio{In: tc.in, Out: tc.out, Err: &errb},
			}
			if code := cmd.Run(rc, []string{"-t"}); code == 0 || !strings.Contains(strings.ToLower(errb.String()), strings.ToLower(tc.want)) {
				t.Fatalf("pr %s failure = code %d, stderr %q; want nonzero and %q", tc.name, code, errb.String(), tc.want)
			}
		})
	}
}

func TestPRPausePerPageWritesAlertAndReadsDevTTY(t *testing.T) {
	withTerminalStdout(t)
	withNoInterrupt(t)
	calls := withControlTTY(t, func() (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader("\n")), nil
	})

	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(), Dir: t.TempDir(),
		Stdio: tool.Stdio{In: strings.NewReader("a\nb\n"), Out: &out, Err: &errb},
	}
	// -l 11 -> a 1-line body (11 - the 10-line header/trailer), so two input
	// lines make exactly two pages: one pause expected per page.
	if code := cmd.Run(rc, []string{"-p", "-l", "11"}); code != 0 {
		t.Fatalf("pr -p = code %d, stderr %q", code, errb.String())
	}
	if *calls != 2 {
		t.Fatalf("pr -p opened /dev/tty %d times, want 2 (one per page)", *calls)
	}
	if got := strings.Count(errb.String(), "\a"); got != 2 {
		t.Fatalf("pr -p wrote %d alerts to stderr, want 2: %q", got, errb.String())
	}
	if !strings.Contains(out.String(), "a\n") || !strings.Contains(out.String(), "b\n") {
		t.Fatalf("pr -p output missing paginated content: %q", out.String())
	}
}

func TestPRMergePauseFlushesEachPageBeforeNextAlert(t *testing.T) {
	withTerminalStdout(t)
	withNoInterrupt(t)
	withControlTTY(t, func() (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader("\n")), nil
	})

	dir := t.TempDir()
	writeFixed(t, dir, "one", "a\nb\n")
	writeFixed(t, dir, "two", "c\nd\n")
	var events []string
	rc := &tool.RunContext{
		Ctx: context.Background(), Dir: dir,
		Stdio: tool.Stdio{
			In:  strings.NewReader(""),
			Out: orderedWriter{events: &events, label: "page"},
			Err: orderedWriter{events: &events, label: "pause"},
		},
	}
	if code := cmd.Run(rc, []string{"-m", "-p", "-l", "11", "one", "two"}); code != 0 {
		t.Fatalf("pr -m -p exited %d; events=%v", code, events)
	}
	want := []string{"pause", "page", "pause", "page"}
	if len(events) != len(want) {
		t.Fatalf("merge pause/write events=%v, want %v", events, want)
	}
	for i := range want {
		if events[i] != want[i] {
			t.Fatalf("merge pause/write events=%v, want %v", events, want)
		}
	}
}

func TestPRMergeFlushErrorStopsBeforeAnotherPause(t *testing.T) {
	withTerminalStdout(t)
	withNoInterrupt(t)
	calls := withControlTTY(t, func() (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader("\n")), nil
	})

	dir := t.TempDir()
	writeFixed(t, dir, "one", "a\nb\n")
	writeFixed(t, dir, "two", "c\nd\n")
	var errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(), Dir: dir,
		Stdio: tool.Stdio{
			In:  strings.NewReader(""),
			Out: failingWriter{err: errors.New("broken pipe")},
			Err: &errb,
		},
	}
	code := cmd.Run(rc, []string{"-m", "-p", "-l", "11", "one", "two"})
	if code == 0 || !strings.Contains(strings.ToLower(errb.String()), "broken pipe") {
		t.Fatalf("merge flush failure: code=%d stderr=%q", code, errb.String())
	}
	if *calls != 1 {
		t.Fatalf("merge flush failure prompted %d times, want 1", *calls)
	}
}

func TestPRPauseAcceptsCarriageReturnFromDevTTY(t *testing.T) {
	withTerminalStdout(t)
	withNoInterrupt(t)
	calls := withControlTTY(t, func() (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader("\r")), nil
	})

	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(), Dir: t.TempDir(),
		Stdio: tool.Stdio{In: strings.NewReader("a\n"), Out: &out, Err: &errb},
	}
	if code := cmd.Run(rc, []string{"-p"}); code != 0 {
		t.Fatalf("pr -p with carriage return = code %d, stderr %q", code, errb.String())
	}
	if *calls != 1 {
		t.Fatalf("pr -p opened /dev/tty %d times, want 1", *calls)
	}
	if got := strings.Count(errb.String(), "\a"); got != 1 {
		t.Fatalf("pr -p wrote %d alerts to stderr, want 1: %q", got, errb.String())
	}
	if !strings.Contains(out.String(), "a\n") {
		t.Fatalf("pr -p output missing content after carriage return: %q", out.String())
	}
}

func TestPRFormFeedLowerPausesOnlyBeforeFirstPageOnTerminal(t *testing.T) {
	withTerminalStdout(t)
	withNoInterrupt(t)
	calls := withControlTTY(t, func() (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader("\n")), nil
	})

	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(), Dir: t.TempDir(),
		Stdio: tool.Stdio{In: strings.NewReader("a\nb\n"), Out: &out, Err: &errb},
	}
	if code := cmd.Run(rc, []string{"-f", "-l", "11"}); code != 0 {
		t.Fatalf("pr -f = code %d, stderr %q", code, errb.String())
	}
	if *calls != 1 {
		t.Fatalf("pr -f (XSI) opened /dev/tty %d times, want 1 (first page only)", *calls)
	}
	if got := strings.Count(errb.String(), "\a"); got != 1 {
		t.Fatalf("pr -f wrote %d alerts to stderr, want 1: %q", got, errb.String())
	}
	if strings.Count(out.String(), "\f") == 0 {
		t.Fatalf("pr -f output has no form-feed page separators: %q", out.String())
	}
}

func TestPRPauseUnavailableControllingTerminal(t *testing.T) {
	withTerminalStdout(t)
	withNoInterrupt(t)
	withControlTTY(t, func() (io.ReadCloser, error) {
		return nil, errors.New("no such device or address")
	})

	dir := t.TempDir()
	writeFixed(t, dir, "in", "a\n")
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(), Dir: dir,
		Stdio: tool.Stdio{In: nil, Out: &out, Err: &errb},
	}
	code := cmd.Run(rc, []string{"-p", "in"})
	if code == 0 {
		t.Fatalf("pr -p with an unavailable controlling terminal must fail, got code 0, out=%q", out.String())
	}
	if !strings.Contains(errb.String(), "in") || !strings.Contains(errb.String(), "/dev/tty") {
		t.Fatalf("pr -p diagnostic = %q, want it to name the file and /dev/tty", errb.String())
	}
	if out.String() != "" {
		t.Fatalf("pr -p must not print page content when the pause before it fails: %q", out.String())
	}
}

func TestPRPauseDevTTYReadError(t *testing.T) {
	withTerminalStdout(t)
	withNoInterrupt(t)
	withControlTTY(t, func() (io.ReadCloser, error) {
		return io.NopCloser(failingReader{err: errors.New("input/output error")}), nil
	})

	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(), Dir: t.TempDir(),
		Stdio: tool.Stdio{In: strings.NewReader("a\n"), Out: &out, Err: &errb},
	}
	code := cmd.Run(rc, []string{"-p"})
	if code == 0 {
		t.Fatalf("pr -p with a failing /dev/tty read must fail, got code 0")
	}
	if !strings.Contains(errb.String(), "/dev/tty") {
		t.Fatalf("pr -p read-error diagnostic = %q, want it to name /dev/tty", errb.String())
	}
	if out.String() != "" {
		t.Fatalf("pr -p must not print page content when the pause read fails: %q", out.String())
	}
}

func TestPRPauseAlertWriteError(t *testing.T) {
	withTerminalStdout(t)
	withNoInterrupt(t)
	calls := withControlTTY(t, func() (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader("\n")), nil
	})

	var out bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(), Dir: t.TempDir(),
		Stdio: tool.Stdio{In: strings.NewReader("a\n"), Out: &out, Err: failingWriter{err: errors.New("broken pipe")}},
	}
	code := cmd.Run(rc, []string{"-p"})
	if code == 0 {
		t.Fatalf("pr -p with a failing stderr alert write must fail, got code 0")
	}
	if *calls != 0 {
		t.Fatalf("pr -p must not open /dev/tty once the alert write already failed, got %d opens", *calls)
	}
	if out.String() != "" {
		t.Fatalf("pr -p must not print page content when the alert write fails: %q", out.String())
	}
}

func TestPRPauseInterrupted(t *testing.T) {
	withTerminalStdout(t)
	// A real pipe that nothing ever writes to: the pause's read blocks until
	// either data arrives or the pipe is closed, so this proves the select
	// on the interrupt channel actually races a live, hanging read rather
	// than a read that happens to return immediately.
	pr, pw := io.Pipe()
	t.Cleanup(func() { pw.Close() })
	withControlTTY(t, func() (io.ReadCloser, error) { return pr, nil })

	sig := make(chan os.Signal, 1)
	sig <- os.Interrupt
	old := watchInterruptFn
	watchInterruptFn = func() (chan os.Signal, func()) { return sig, func() {} }
	t.Cleanup(func() { watchInterruptFn = old })

	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(), Dir: t.TempDir(),
		Stdio: tool.Stdio{In: strings.NewReader("a\nb\n"), Out: &out, Err: &errb},
	}
	done := make(chan int, 1)
	go func() { done <- cmd.Run(rc, []string{"-p"}) }()
	select {
	case code := <-done:
		if code != interruptExit {
			t.Fatalf("pr -p interrupted while paused = code %d, want %d: stderr=%q", code, interruptExit, errb.String())
		}
	case <-time.After(time.Second):
		t.Fatal("pr -p did not return after an interrupt while paused; it hung waiting on /dev/tty")
	}
	if out.String() != "" {
		t.Fatalf("pr -p must not print page content once interrupted before it: %q", out.String())
	}
}

func TestPRPauseFlagsNoOpWhenStdoutIsNotATerminal(t *testing.T) {
	// isTerminalFn is deliberately left at its real default here: runPR's
	// bytes.Buffer is never a terminal, matching every other test in this
	// file, so this proves -p/-f's pause never engages in the gated
	// noninteractive path all the other option coverage depends on.
	var ttyCalls, sigCalls int
	oldOpen, oldWatch := openControlTTYFn, watchInterruptFn
	openControlTTYFn = func() (io.ReadCloser, error) {
		ttyCalls++
		return io.NopCloser(strings.NewReader("\n")), nil
	}
	watchInterruptFn = func() (chan os.Signal, func()) {
		sigCalls++
		return make(chan os.Signal), func() {}
	}
	t.Cleanup(func() { openControlTTYFn, watchInterruptFn = oldOpen, oldWatch })

	// -p adds nothing but the pause, so its non-terminal output must equal
	// plain pr exactly. -f also aliases -F's form feeds regardless of
	// terminal status (that half is not gated), so it is compared against
	// -F rather than against plain pr; TestPRFormFeedLowerEqualsUpper covers
	// that pairing too, but repeating it here keeps this test self-contained
	// against the same tty/sig call counters checked below.
	plain, _, codePlain := runPR(t, t.TempDir(), "a\nb\n", "-l", "11")
	paused, errb, codePaused := runPR(t, t.TempDir(), "a\nb\n", "-l", "11", "-p")
	if codePlain != 0 || codePaused != 0 {
		t.Fatalf("pr exit codes: plain=%d paused=%d", codePlain, codePaused)
	}
	if plain != paused {
		t.Fatalf("pr -p on a non-terminal changed output:\nplain=%q\npaused=%q", plain, paused)
	}
	if errb != "" {
		t.Fatalf("pr -p on a non-terminal must write no alert: stderr=%q", errb)
	}
	upperF, _, codeUpperF := runPR(t, t.TempDir(), "a\nb\n", "-F")
	lowerF, _, codeLowerF := runPR(t, t.TempDir(), "a\nb\n", "-f")
	if codeUpperF != 0 || codeLowerF != 0 {
		t.Fatalf("pr exit codes: -F=%d -f=%d", codeUpperF, codeLowerF)
	}
	if upperF != lowerF {
		t.Fatalf("pr -f on a non-terminal changed output vs -F:\n-F=%q\n-f=%q", upperF, lowerF)
	}
	if ttyCalls != 0 || sigCalls != 0 {
		t.Fatalf("pr -p -f on a non-terminal must not touch /dev/tty or install an interrupt handler: ttyCalls=%d sigCalls=%d", ttyCalls, sigCalls)
	}
}

func TestPRInputFormFeedsBreakPages(t *testing.T) {
	// -t keeps input form feeds as page breaks.
	out, _, code := runPR(t, t.TempDir(), "a\fb\n", "-t")
	if out != "a\n\fb\n" || code != 0 {
		t.Fatalf("pr -t form feed = (%q, %d), want %q", out, code, "a\n\fb\n")
	}
	// -T eliminates them.
	out, _, code = runPR(t, t.TempDir(), "a\fb\n", "-T")
	if out != "a\nb\n" || code != 0 {
		t.Fatalf("pr -T form feed = (%q, %d), want %q", out, code, "a\nb\n")
	}
	// Paginated: the form feed starts a new page.
	out, _, code = runPR(t, t.TempDir(), "a\n\fb\n", "-l", "20")
	if code != 0 || !strings.Contains(out, "Page 2") || strings.Contains(out, "Page 3") {
		t.Fatalf("pr paginated form feed = (%q, %d), want 2 pages", out, code)
	}
	if n := strings.Count(out, "\n"); n != 40 {
		t.Fatalf("pr paginated form feed has %d lines, want 40", n)
	}
	// Consecutive form feeds produce an empty page.
	out, _, code = runPR(t, t.TempDir(), "a\n\f\fb\n", "-l", "20")
	if code != 0 || !strings.Contains(out, "Page 3") {
		t.Fatalf("pr double form feed = (%q, %d), want 3 pages", out, code)
	}
	if n := strings.Count(out, "\n"); n != 60 {
		t.Fatalf("pr double form feed has %d lines, want 60", n)
	}
	// A leading form feed represents an empty first page once data follows.
	out, _, code = runPR(t, t.TempDir(), "\fb\n", "-l", "20")
	if code != 0 || !strings.Contains(out, "Page 2") || strings.Count(out, "\n") != 40 {
		t.Fatalf("pr leading form feed = (%q, %d), want empty page then data page", out, code)
	}
}

func TestPRVerticalColumns(t *testing.T) {
	input := "a\nbb\nc\ndd\ne\n"
	want := "a    dd\nbb   e\nc\n"
	for _, args := range [][]string{{"-t", "-w", "10", "-2"}, {"-t", "-w", "10", "--columns=2"}, {"-t", "-w", "10", "--column", "2"}} {
		out, errb, code := runPR(t, t.TempDir(), input, args...)
		if out != want || errb != "" || code != 0 {
			t.Errorf("pr %v = (%q, %q, %d), want (%q, \"\", 0)", args, out, errb, code, want)
		}
	}
}

func TestPRVerticalColumnsUnevenFinalPage(t *testing.T) {
	out, errb, code := runPR(t, t.TempDir(), "a\nb\nc\nd\ne\nf\ng\n", "-t", "-l", "3", "-w", "10", "--columns=2")
	want := "a    d\nb    e\nc    f\ng\n"
	if out != want || errb != "" || code != 0 {
		t.Fatalf("pr vertical uneven final page = (%q, %q, %d), want (%q, \"\", 0)", out, errb, code, want)
	}
}

func TestPRColumnsReserveBlankBetweenFullWidthCells(t *testing.T) {
	input := strings.Repeat("1", 40) + "\n" + strings.Repeat("2", 40) + "\n"
	for _, args := range [][]string{
		{"-t", "-w", "10", "-2"},
		{"-t", "-w", "10", "-2", "-a"},
	} {
		out, errb, code := runPR(t, t.TempDir(), input, args...)
		if code != 0 || errb != "" || !strings.Contains(out, "1111 ") {
			t.Fatalf("pr %v did not reserve a blank column separator: (%q, %q, %d)", args, out, errb, code)
		}
	}
	dir := t.TempDir()
	writeFixed(t, dir, "one", strings.Repeat("1", 40)+"\n")
	writeFixed(t, dir, "two", strings.Repeat("2", 40)+"\n")
	out, errb, code := runPR(t, dir, "", "-m", "-t", "-w", "10", "one", "two")
	if out != "1111 2222\n" || errb != "" || code != 0 {
		t.Fatalf("pr -m did not reserve a blank column separator: (%q, %q, %d)", out, errb, code)
	}
}

func TestPRAcrossColumns(t *testing.T) {
	input := "a\nbb\nc\ndd\ne\n"
	out, errb, code := runPR(t, t.TempDir(), input, "-t", "-w", "10", "-2", "-a")
	if want := "a    bb\nc    dd\ne\n"; out != want || errb != "" || code != 0 {
		t.Fatalf("pr -a = (%q, %q, %d), want (%q, \"\", 0)", out, errb, code, want)
	}
	_, errb, code = runPR(t, t.TempDir(), "a\n", "-a", "-m")
	if code != 1 || !strings.Contains(errb, "cannot specify both printing across and printing in parallel") {
		t.Fatalf("pr -am code=%d err=%q", code, errb)
	}
}

func TestPRClusteredColumnWithFlags(t *testing.T) {
	out, errb, code := runPR(t, t.TempDir(), "a\nb\nc\nd\n", "-t", "-w", "10", "-3d")
	if want := "a  c\n\nb  d\n\n"; out != want || errb != "" || code != 0 {
		t.Fatalf("pr -3d = (%q, %q, %d), want (%q, \"\", 0)", out, errb, code, want)
	}
}

func TestPRVerticalColumnsInteractions(t *testing.T) {
	out, errb, code := runPR(t, t.TempDir(), "a\nb\n", "-t", "-w", "10", "-o", "2", "-s:", "-2")
	if want := "  a:b\n"; out != want || errb != "" || code != 0 {
		t.Fatalf("pr vertical separator/margin = (%q, %q, %d), want (%q, \"\", 0)", out, errb, code, want)
	}

	out, errb, code = runPR(t, t.TempDir(), "a\nb\nc\nd\n", "-t", "-w", "10", "-d", "-2")
	if want := "a    c\n\nb    d\n\n"; out != want || errb != "" || code != 0 {
		t.Fatalf("pr vertical double-space = (%q, %q, %d), want (%q, \"\", 0)", out, errb, code, want)
	}
}

func TestPRCustomSeparatorTruncatesColumns(t *testing.T) {
	line := strings.Repeat("x", 20)
	out, errb, code := runPR(t, t.TempDir(), line+"\n"+line+"\n", "-t", "-w", "10", "-sX", "-2", "-a")
	if want := "xxxxXxxxx\n"; out != want || errb != "" || code != 0 {
		t.Fatalf("pr -sX = (%q, %q, %d), want truncated columns %q", out, errb, code, want)
	}
}

func TestPRSeparatorDefaultWidthIs512(t *testing.T) {
	line := strings.Repeat("x", 300)
	out, errb, code := runPR(t, t.TempDir(), line+"\n"+line+"\n", "-t", "-s:", "-2", "-a")
	want := strings.Repeat("x", 255) + ":" + strings.Repeat("x", 255) + "\n"
	if out != want || errb != "" || code != 0 {
		t.Fatalf("pr -s default width = len %d err %q code %d, want len %d", len(out), errb, code, len(want))
	}
}

func TestPRMerge(t *testing.T) {
	dir := t.TempDir()
	writeFixed(t, dir, "left", "a\nbb\nc\n")
	writeFixed(t, dir, "right", "1\n")
	out, errb, code := runPR(t, dir, "", "-m", "-t", "-w", "20", "-s:", "left", "right")
	if want := "a:1\nbb:\nc:\n"; out != want || errb != "" || code != 0 {
		t.Fatalf("pr -m = (%q, %q, %d), want (%q, \"\", 0)", out, errb, code, want)
	}
	_, errb, code = runPR(t, dir, "", "-m", "-2", "left", "right")
	if code != 1 || !strings.Contains(errb, "cannot specify number of columns") {
		t.Fatalf("pr -m -2 code=%d err=%q", code, errb)
	}
}

func TestPRMergeSeparatorOddWidthDoesNotReservePadding(t *testing.T) {
	dir := t.TempDir()
	writeFixed(t, dir, "left", "1234567890\n")
	writeFixed(t, dir, "right", "x\n")
	for _, tt := range []struct {
		width string
		want  string
	}{
		{width: "19", want: "123456789:x\n"},
		{width: "21", want: "1234567890:x\n"},
	} {
		out, errb, code := runPR(t, dir, "", "-m", "-t", "-s:", "-w", tt.width, "left", "right")
		if out != tt.want || errb != "" || code != 0 {
			t.Fatalf("pr -m -s -w %s = (%q, %q, %d), want (%q, \"\", 0)", tt.width, out, errb, code, tt.want)
		}
	}
}

func TestPRMergeOpenErrorContinuesAndNewline(t *testing.T) {
	dir := t.TempDir()
	writeFixed(t, dir, "left", "a\n")
	out, errb, code := runPR(t, dir, "", "-m", "-t", "left", "missing")
	if out != "a\n" || code != 1 {
		t.Fatalf("pr -m missing = (%q, %q, %d), want readable file and exit 1", out, errb, code)
	}
	if !strings.HasSuffix(errb, "\n") || strings.Contains(errb, `\n`) || !strings.Contains(errb, "missing") {
		t.Fatalf("pr -m missing diagnostic malformed: %q", errb)
	}
}

func TestPRBareSeparatorDoesNotConsumeFile(t *testing.T) {
	dir := t.TempDir()
	writeFixed(t, dir, "in", "from-file\n")
	for _, separator := range []string{"-s", "--separator"} {
		out, errb, code := runPR(t, dir, "from-stdin\n", "-m", "-t", separator, "in")
		if out != "from-file\n" || errb != "" || code != 0 {
			t.Fatalf("pr %s consumed the file operand = (%q, %q, %d)", separator, out, errb, code)
		}
	}
}

func TestPRMergeThreeFilesAndPagination(t *testing.T) {
	dir := t.TempDir()
	writeFixed(t, dir, "one", "a\nb\nc\nd\n")
	writeFixed(t, dir, "two", "1\n2\n")
	writeFixed(t, dir, "three", "x\ny\nz\n")
	out, errb, code := runPR(t, dir, "", "-m", "-t", "-w", "18", "one", "two", "three")
	if want := "a     1\t    x\nb     2\t    y\nc     \t    z\nd     \t    \n"; out != want || errb != "" || code != 0 {
		t.Fatalf("pr -m three files = (%q, %q, %d), want (%q, \"\", 0)", out, errb, code, want)
	}
	out, errb, code = runPR(t, dir, "", "-m", "-l", "13", "-D", "%Y", "one", "two")
	if errb != "" || code != 0 || !strings.Contains(out, "Page 1") || !strings.Contains(out, "Page 2") || strings.Count(out, "\n") != 26 {
		t.Fatalf("paginated pr -m = (%q, %q, %d)", out, errb, code)
	}
}

func TestPRMergeLineNumbering(t *testing.T) {
	dir := t.TempDir()
	writeFixed(t, dir, "one", "alpha\nbeta\n")
	writeFixed(t, dir, "two", "gamma\n")
	out, errb, code := runPR(t, dir, "", "-m", "-t", "-w", "30", "-nX", "one", "two")
	if code != 0 || errb != "" || !strings.HasPrefix(out, "    1X") || !strings.Contains(out, "\n    2X") {
		t.Fatalf("pr -m -nX = (%q, %q, %d), want one numbered prefix per merged row", out, errb, code)
	}
}

func TestPRClusteredOptionalOutputTabs(t *testing.T) {
	out, errb, code := runPR(t, t.TempDir(), "a\n", "-adFrtiX", "-2")
	if code != 0 || errb != "" || out == "" {
		t.Fatalf("pr clustered -iX = (%q, %q, %d)", out, errb, code)
	}
}

func TestPRRejectsMoreColumnsThanPageWidth(t *testing.T) {
	_, errb, code := runPR(t, t.TempDir(), "a\n", "-500", "-w", "40")
	if code != 2 || !strings.Contains(errb, "page width too narrow") {
		t.Fatalf("pr -500 -w40 = (%q, %d), want width error", errb, code)
	}
}

func TestPRExpandTabs(t *testing.T) {
	out, _, code := runPR(t, t.TempDir(), "a\tb\n", "-t", "-e")
	if want := "a       b\n"; out != want || code != 0 {
		t.Fatalf("pr expand tabs = (%q, %d), want (%q, 0)", out, code, want)
	}
}

func TestPROptionalExpandArgument(t *testing.T) {
	dir := t.TempDir()
	writeFixed(t, dir, "in", "a\tbXc\n")
	for _, tt := range []struct {
		name string
		arg  string
		want string
	}{
		{name: "gap", arg: "-e4", want: "a   bXc\n"},
		{name: "char-gap", arg: "-eX4", want: "a\tb c\n"},
		{name: "char-default-gap", arg: "-eX", want: "a\tb     c\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			out, errb, code := runPR(t, dir, "from-stdin\n", "-t", tt.arg, "in")
			if out != tt.want || errb != "" || code != 0 {
				t.Fatalf("pr %s = (%q, %q, %d), want (%q, \"\", 0)", tt.arg, out, errb, code, tt.want)
			}
		})
	}
}

func TestPROptionalGapZeroMeansDefault(t *testing.T) {
	for _, tt := range []struct {
		arg  string
		want string
	}{
		{arg: "-e0", want: "a       b\n"},
		{arg: "-eX0", want: "a\tb     c\n"},
		{arg: "-i0", want: "a\tb\n"},
		{arg: "-iX0", want: "aXb\n"},
	} {
		input := "a\tb\n"
		if strings.HasPrefix(tt.arg, "-i") {
			input = "a       b\n"
		} else if strings.HasPrefix(tt.arg, "-eX") {
			input = "a\tbXc\n"
		}
		out, errb, code := runPR(t, t.TempDir(), input, "-t", tt.arg)
		if out != tt.want || errb != "" || code != 0 {
			t.Fatalf("pr %s = (%q, %q, %d), want (%q, \"\", 0)", tt.arg, out, errb, code, tt.want)
		}
	}
}

func TestPRMultiColumnImpliesExpandAndOutputTabs(t *testing.T) {
	out, errb, code := runPR(t, t.TempDir(), "a\tb\nc       d\n", "-t", "-2", "-w", "20")
	if want := "a\tb c\t  d\n"; out != want || errb != "" || code != 0 {
		t.Fatalf("pr multicolumn implied tab handling = (%q, %q, %d), want %q", out, errb, code, want)
	}
}

func TestPROptionalNumberArgument(t *testing.T) {
	for _, tt := range []struct {
		name string
		arg  string
		want string
	}{
		{name: "width", arg: "-n4", want: "   1\ta\n"},
		{name: "char-width", arg: "-nX4", want: "   1Xa\n"},
		{name: "char-default-width", arg: "-nX", want: "    1Xa\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			out, errb, code := runPR(t, t.TempDir(), "a\n", "-t", tt.arg)
			if out != tt.want || errb != "" || code != 0 {
				t.Fatalf("pr %s = (%q, %q, %d), want (%q, \"\", 0)", tt.arg, out, errb, code, tt.want)
			}
		})
	}
}

func TestPRNoFileWarnings(t *testing.T) {
	out, errb, code := runPR(t, t.TempDir(), "", "-r", "missing")
	if out != "" || errb != "" || code != 1 {
		t.Fatalf("pr -r missing = (%q, %q, %d), want quiet exit 1", out, errb, code)
	}
}

func TestPRRejectsBadLength(t *testing.T) {
	_, errb, code := runPR(t, t.TempDir(), "", "-l", "0")
	if code != 2 || !strings.Contains(errb, "invalid page length") {
		t.Fatalf("pr bad length code=%d err=%q", code, errb)
	}
}

func TestPRRejectsBadIndent(t *testing.T) {
	_, errb, code := runPR(t, t.TempDir(), "", "-o", "-1")
	if code != 2 || !strings.Contains(errb, "invalid indent") {
		t.Fatalf("pr bad indent code=%d err=%q", code, errb)
	}
}

func TestPRFirstLineNumber(t *testing.T) {
	out, _, code := runPR(t, t.TempDir(), "a\nb\n", "-t", "-n", "-N", "10")
	want := "   10\ta\n   11\tb\n"
	if out != want || code != 0 {
		t.Fatalf("pr -N 10 = (%q, %d), want (%q, 0)", out, code, want)
	}

	_, errb, code := runPR(t, t.TempDir(), "", "-N", "0", "a")
	if code != 2 || !strings.Contains(errb, "invalid first line number") {
		t.Fatalf("pr -N 0 code=%d err=%q", code, errb)
	}
}

func TestPRNewFlagAliases(t *testing.T) {
	// --column is an alias for --columns
	out, errb, code := runPR(t, t.TempDir(), "a\nb\n", "-t", "-w", "10", "--column", "2")
	if out != "a    b\n" || errb != "" || code != 0 {
		t.Fatalf("pr --column 2 = (%q, %q, %d)", out, errb, code)
	}

	// -J is accepted (join-lines, no-op)
	out, _, code = runPR(t, t.TempDir(), "a\nb\n", "-J", "-t")
	if out != "a\nb\n" || code != 0 {
		t.Fatalf("pr -J = (%q, %d)", out, code)
	}

	// -i is accepted (indent style, no-op)
	out, _, code = runPR(t, t.TempDir(), "a\n", "-i", "-t")
	if out != "a\n" || code != 0 {
		t.Fatalf("pr -i = (%q, %d)", out, code)
	}
}

func TestPROutputTabsAtAdjacentStop(t *testing.T) {
	o := options{outputTabs: true, outputTabChar: '\t', outputTabWidth: 8}
	if got := tabifyLine("1234567 90\n", o); got != "1234567\t90\n" {
		t.Fatalf("one-space tab-stop replacement = %q", got)
	}
}

func TestPRStopsOptionAndPageParsingAtFirstOperand(t *testing.T) {
	dir := t.TempDir()
	writeFixed(t, dir, "first", "one\n")
	writeFixed(t, dir, "-x", "two\n")
	writeFixed(t, dir, "+2", "three\n")
	writeFixed(t, dir, "--wid", "four\n")
	out, errb, code := runPR(t, dir, "", "-t", "first", "-x", "+2", "--wid")
	if code != 0 || errb != "" || out != "one\ntwo\nthree\nfour\n" {
		t.Fatalf("pr option-looking operands = (%q, %q, %d)", out, errb, code)
	}
}

func TestPRPOSIXLYCorrectDifferentials(t *testing.T) {
	if os.Getenv("COREUTILS_SYSTEM_DIFFERENTIALS") != "1" {
		t.Skip("set COREUTILS_SYSTEM_DIFFERENTIALS=1 to compare with host pr")
	}
	if runtime.GOOS != "linux" {
		t.Skip("GNU POSIXLY_CORRECT differential is only required on Linux/Ubuntu hosts")
	}
	if out, err := exec.Command("/usr/bin/pr", "--version").CombinedOutput(); err != nil || !strings.Contains(string(out), "GNU coreutils") {
		t.Skip("GNU pr reference not available")
	}
	dir := t.TempDir()
	writeFixed(t, dir, "one", "a\nbb\n")
	writeFixed(t, dir, "two", "1\n")
	cases := []struct {
		name  string
		stdin string
		args  []string
	}{
		{name: "header", args: []string{"one"}},
		{name: "merge-s", args: []string{"-m", "-t", "-w", "20", "-s:", "one", "two"}},
		{name: "columns", stdin: "a\tb\nc       d\n", args: []string{"-t", "-2", "-w", "20"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wantCmd := exec.Command("/usr/bin/pr", tc.args...)
			wantCmd.Dir = dir
			wantCmd.Stdin = strings.NewReader(tc.stdin)
			wantCmd.Env = append(os.Environ(), "POSIXLY_CORRECT=1", "LC_ALL=C")
			want, err := wantCmd.CombinedOutput()
			if err != nil {
				t.Fatalf("GNU pr failed: %v output=%q", err, want)
			}
			out, errb, code := runPR(t, dir, tc.stdin, tc.args...)
			if code != 0 || errb != "" || out != string(want) {
				t.Fatalf("candidate differs from GNU POSIXLY_CORRECT:\nout=%q\nerr=%q\ncode=%d\nwant=%q", out, errb, code, want)
			}
		})
	}
}
