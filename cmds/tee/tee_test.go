package teecmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func runToolDir(t *testing.T, dir, stdin string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Stdio: tool.Stdio{In: strings.NewReader(stdin), Out: &out, Err: &errb},
	}
	code = cmd.Run(rc, args)
	return out.String(), errb.String(), code
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestTeeStdoutOnly(t *testing.T) {
	out, errb, code := runToolDir(t, t.TempDir(), "hello\nworld\n")
	if out != "hello\nworld\n" || errb != "" || code != 0 {
		t.Errorf("no files: out=%q err=%q code=%d", out, errb, code)
	}
}

func TestTeeWritesFiles(t *testing.T) {
	dir := t.TempDir()
	out, _, code := runToolDir(t, dir, "data\n", "f1", "f2")
	if out != "data\n" || code != 0 {
		t.Errorf("out=%q code=%d", out, code)
	}
	// relative operands resolve against rc.Dir
	for _, f := range []string{"f1", "f2"} {
		if got := readFile(t, filepath.Join(dir, f)); got != "data\n" {
			t.Errorf("%s = %q, want %q", f, got, "data\n")
		}
	}
}

func TestTeeIssue7AtLeastThirteenFileOperands(t *testing.T) {
	dir := t.TempDir()
	var names []string
	for i := 0; i < 13; i++ {
		names = append(names, fmt.Sprintf("out-%02d", i))
	}
	out, errb, code := runToolDir(t, dir, "thirteen\n", names...)
	if code != 0 || out != "thirteen\n" || errb != "" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errb)
	}
	for _, name := range names {
		if got := readFile(t, filepath.Join(dir, name)); got != "thirteen\n" {
			t.Errorf("%s = %q", name, got)
		}
	}
}

func TestTeeTruncatesByDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f")
	if err := os.WriteFile(path, []byte("old contents that are long\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, code := runToolDir(t, dir, "new\n", "f")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if got := readFile(t, path); got != "new\n" {
		t.Errorf("file = %q, want %q", got, "new\n")
	}
}

func TestTeeAppend(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f")
	if err := os.WriteFile(path, []byte("first\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, code := runToolDir(t, dir, "second\n", "-a", "f")
	if out != "second\n" || code != 0 {
		t.Errorf("out=%q code=%d", out, code)
	}
	if got := readFile(t, path); got != "first\nsecond\n" {
		t.Errorf("file = %q, want %q", got, "first\nsecond\n")
	}
}

func TestTeeIssue7VirtualUmaskAndExistingMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not expose POSIX permission bits")
	}
	dir := t.TempDir()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(), Dir: dir, Umask: 0o077, UmaskSet: true,
		Stdio: tool.Stdio{In: strings.NewReader("new\n"), Out: &out, Err: &errb},
	}
	if code := cmd.Run(rc, []string{"f"}); code != 0 || errb.String() != "" {
		t.Fatalf("new file: code=%d stderr=%q", code, errb.String())
	}
	info, err := os.Stat(filepath.Join(dir, "f"))
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("new file mode=%#o, want 0600", got)
	}

	if err := os.Chmod(filepath.Join(dir, "f"), 0o640); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errb.Reset()
	rc.In = strings.NewReader("replacement\n")
	if code := cmd.Run(rc, []string{"f"}); code != 0 || errb.String() != "" {
		t.Fatalf("existing file: code=%d stderr=%q", code, errb.String())
	}
	info, err = os.Stat(filepath.Join(dir, "f"))
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o640 {
		t.Fatalf("existing file mode=%#o, want unchanged 0640", got)
	}
}

func TestTeeIgnoreInterruptsAccepted(t *testing.T) {
	// -i is accepted (and now actually ignores interrupts during the run)
	dir := t.TempDir()
	out, errb, code := runToolDir(t, dir, "x\n", "-i", "f")
	if out != "x\n" || errb != "" || code != 0 {
		t.Errorf("-i: out=%q err=%q code=%d", out, errb, code)
	}
	if got := readFile(t, filepath.Join(dir, "f")); got != "x\n" {
		t.Errorf("file = %q", got)
	}
}

func TestTeeOutputErrorOptionsAccepted(t *testing.T) {
	dir := t.TempDir()
	for _, args := range [][]string{
		{"-p", "f"},
		{"--output-error", "f"},
		{"--output-error=warn", "f"},
		{"--output-error=exit-nopipe", "f"},
	} {
		out, errb, code := runToolDir(t, dir, "x\n", args...)
		if code != 0 || errb != "" || out != "x\n" {
			t.Errorf("tee %v: out=%q err=%q code=%d", args, out, errb, code)
		}
	}
	_, errb, code := runToolDir(t, dir, "", "--output-error=bad")
	if code != 2 || !strings.Contains(errb, "invalid argument") {
		t.Errorf("bad --output-error: err=%q code=%d", errb, code)
	}
}

func TestTeeOpenErrorContinues(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skipf("relies on opening a path under a missing directory failing the same way")
	}
	dir := t.TempDir()
	// unopenable path (missing parent dir): diagnose, keep copying to the rest
	out, errb, code := runToolDir(t, dir, "x\n", "missing/sub/f", "ok")
	if code != 1 || out != "x\n" || !strings.Contains(errb, "tee: missing/sub/f:") {
		t.Errorf("open error: out=%q err=%q code=%d", out, errb, code)
	}
	if got := readFile(t, filepath.Join(dir, "ok")); got != "x\n" {
		t.Errorf("ok file = %q", got)
	}
}

func TestTeeDashIsLiteralFileName(t *testing.T) {
	// GNU tee does not special-case "-": it names a file
	dir := t.TempDir()
	out, _, code := runToolDir(t, dir, "x\n", "-")
	if out != "x\n" || code != 0 {
		t.Errorf("dash: out=%q code=%d", out, code)
	}
	if got := readFile(t, filepath.Join(dir, "-")); got != "x\n" {
		t.Errorf("dash file = %q", got)
	}
}

func TestTeeIssue7FirstOperandEndsOptionRecognition(t *testing.T) {
	// A lone "-" is the first file operand, not an option. Under the
	// Issue 7 utility-syntax rules, every following argument is therefore
	// an operand as well, even when it looks like an option or delimiter.
	dir := t.TempDir()
	out, errb, code := runToolDir(t, dir, "payload\n", "-", "-i", "-h", "--")
	if code != 0 || errb != "" || out != "payload\n" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errb)
	}
	for _, name := range []string{"-", "-i", "-h", "--"} {
		if got := readFile(t, filepath.Join(dir, name)); got != "payload\n" {
			t.Errorf("operand %q contains %q, want payload", name, got)
		}
	}
}

func TestTeeUnknownFlag(t *testing.T) {
	_, errb, code := runToolDir(t, t.TempDir(), "", "--frobnicate")
	if code != 2 || !strings.Contains(errb, "frobnicate") || !strings.Contains(errb, "pure-Go") {
		t.Errorf("unknown flag: code=%d err=%q", code, errb)
	}
}

func TestTeeHelpAndVersion(t *testing.T) {
	out, _, code := runToolDir(t, t.TempDir(), "", "--help")
	if code != 0 || !strings.Contains(out, "Usage: tee") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	out, _, code = runToolDir(t, t.TempDir(), "", "--version")
	if code != 0 || !strings.Contains(out, "tee") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}
}

// errWriter is an io.Writer that always returns err.
type errWriter struct{ err error }

func (e errWriter) Write(p []byte) (int, error) { return 0, e.err }

type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) { return len(p) - 1, nil }

type testWriteCloser struct {
	bytes.Buffer
	writeErr error
	closeErr error
	short    bool
	closed   bool
}

func (w *testWriteCloser) Write(p []byte) (int, error) {
	if w.writeErr != nil {
		return 0, w.writeErr
	}
	if w.short {
		return len(p) - 1, nil
	}
	return w.Buffer.Write(p)
}

func (w *testWriteCloser) Close() error {
	w.closed = true
	return w.closeErr
}

type readResult struct {
	data string
	err  error
}

type stagedReader struct{ results <-chan readResult }

func (r stagedReader) Read(p []byte) (int, error) {
	result, ok := <-r.results
	if !ok {
		return 0, io.EOF
	}
	return copy(p, result.data), result.err
}

type channelWriter chan string

func (w channelWriter) Write(p []byte) (int, error) {
	w <- string(p)
	return len(p), nil
}

// pipeErrWriter wraps an io.Writer and reports itself as a pipe via the
// local pipeMarker interface, letting tests exercise --output-error pipe
// behavior without creating real OS pipes.
type pipeErrWriter struct {
	io.Writer
	err error
}

func (pipeErrWriter) isPipe() bool { return true }

func (p pipeErrWriter) Write(b []byte) (int, error) {
	if p.err != nil {
		return 0, p.err
	}
	return p.Writer.Write(b)
}

// pipeWriter is a non-failing pipe-marked writer.
type pipeWriter struct{ io.Writer }

func (pipeWriter) isPipe() bool { return true }

func runToolRaw(t *testing.T, dir string, in io.Reader, out, errOut io.Writer, args ...string) int {
	t.Helper()
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Stdio: tool.Stdio{In: in, Out: out, Err: errOut},
	}
	return cmd.Run(rc, args)
}

func TestTeeStdoutWriteErrorPOSIX(t *testing.T) {
	// POSIX default: an error writing to standard output is fatal and uses
	// the Utility Description Defaults, which require a diagnostic.
	var errb bytes.Buffer
	code := runToolRaw(t, t.TempDir(), strings.NewReader("x\n"), errWriter{errors.New("broken")}, &errb)
	if code != 1 {
		t.Errorf("stdout error: code=%d, want 1", code)
	}
	if !strings.Contains(errb.String(), "tee: standard output: Broken") {
		t.Errorf("stdout error: stderr=%q, want diagnostic", errb.String())
	}
}

func TestTeeIssue7ShortWriteFails(t *testing.T) {
	var errb bytes.Buffer
	code := runToolRaw(t, t.TempDir(), strings.NewReader("x\n"), shortWriter{}, &errb)
	if code != 1 || !strings.Contains(errb.String(), "Short write") {
		t.Fatalf("short stdout write: code=%d stderr=%q", code, errb.String())
	}
}

func TestTeeIssue7InputReadFailurePreservesReadBytes(t *testing.T) {
	var out, errb bytes.Buffer
	in := &oneReadError{data: []byte("kept\n"), err: errors.New("input failed")}
	code := runToolRaw(t, t.TempDir(), in, &out, &errb)
	if code != 1 || out.String() != "kept\n" || !strings.Contains(errb.String(), "tee: read error: input failed") {
		t.Fatalf("read error: code=%d stdout=%q stderr=%q", code, out.String(), errb.String())
	}
}

type oneReadError struct {
	data []byte
	err  error
	done bool
}

func (r *oneReadError) Read(p []byte) (int, error) {
	if r.done {
		return 0, io.EOF
	}
	r.done = true
	return copy(p, r.data), r.err
}

func TestTeeIssue7StreamsBeforeEOF(t *testing.T) {
	results := make(chan readResult)
	writes := make(channelWriter, 1)
	done := make(chan int, 1)
	dir := t.TempDir()
	go func() {
		rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Stdio: tool.Stdio{In: stagedReader{results: results}, Out: writes, Err: io.Discard}}
		done <- cmd.Run(rc, nil)
	}()
	results <- readResult{data: "first\n"}
	if got := <-writes; got != "first\n" {
		t.Fatalf("first streamed write = %q", got)
	}
	close(results)
	if code := <-done; code != 0 {
		t.Fatalf("code=%d", code)
	}
}

func TestTeeIssue7OutputFailuresContinue(t *testing.T) {
	tests := []struct {
		name       string
		configure  func(bad *testWriteCloser)
		wantDetail string
	}{
		{"write error", func(bad *testWriteCloser) { bad.writeErr = errors.New("write failed") }, "Write failed"},
		{"short write", func(bad *testWriteCloser) { bad.short = true }, "Short write"},
		{"close error", func(bad *testWriteCloser) { bad.closeErr = errors.New("close failed") }, "Close failed"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bad, good := &testWriteCloser{}, &testWriteCloser{}
			tc.configure(bad)
			opened := map[string]*testWriteCloser{"bad": bad, "good": good}
			var out, errb bytes.Buffer
			rc := &tool.RunContext{Ctx: context.Background(), Dir: t.TempDir(), Stdio: tool.Stdio{In: strings.NewReader("data\n"), Out: &out, Err: &errb}}
			code := runWithOpen(rc, []string{"bad", "good"}, func(path string, _ int, _ os.FileMode) (io.WriteCloser, error) {
				return opened[filepath.Base(path)], nil
			})
			if code != 1 || out.String() != "data\n" || good.String() != "data\n" {
				t.Fatalf("code=%d stdout=%q good=%q stderr=%q", code, out.String(), good.String(), errb.String())
			}
			if !strings.Contains(errb.String(), "tee: bad: "+tc.wantDetail) || !bad.closed || !good.closed {
				t.Fatalf("stderr=%q bad.closed=%v good.closed=%v", errb.String(), bad.closed, good.closed)
			}
		})
	}
}

func TestTeeIssue7OpenFailureContinuesPortably(t *testing.T) {
	good := &testWriteCloser{}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: t.TempDir(), Stdio: tool.Stdio{In: strings.NewReader("data\n"), Out: &out, Err: &errb}}
	code := runWithOpen(rc, []string{"bad", "good"}, func(path string, _ int, _ os.FileMode) (io.WriteCloser, error) {
		if filepath.Base(path) == "bad" {
			return nil, errors.New("open failed")
		}
		return good, nil
	})
	if code != 1 || out.String() != "data\n" || good.String() != "data\n" || !good.closed || !strings.Contains(errb.String(), "tee: bad: Open failed") {
		t.Fatalf("code=%d stdout=%q good=%q closed=%v stderr=%q", code, out.String(), good.String(), good.closed, errb.String())
	}
}

func TestTeeExtensionsRemainAvailableWithPOSIXEnvironment(t *testing.T) {
	for _, args := range [][]string{{"-p"}, {"--output-error=warn"}, {"--append"}} {
		var out, errb bytes.Buffer
		rc := &tool.RunContext{Ctx: context.Background(), Dir: t.TempDir(), Env: []string{"POSIXLY_CORRECT="}, Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb}}
		if code := cmd.Run(rc, args); code != 0 || errb.String() != "" || out.String() != "" {
			t.Errorf("args=%v code=%d stdout=%q stderr=%q", args, code, out.String(), errb.String())
		}
	}
	var helpOut, helpErr bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: t.TempDir(), Env: []string{"POSIXLY_CORRECT="}, Stdio: tool.Stdio{In: strings.NewReader(""), Out: &helpOut, Err: &helpErr}}
	if code := cmd.Run(rc, []string{"--help"}); code != 0 || helpErr.String() != "" || helpOut.Len() == 0 {
		t.Fatalf("--help: code=%d stdout=%q stderr=%q", code, helpOut.String(), helpErr.String())
	}
}

// TestTeeIssue7OptionTerminatorAndShortBundles pins the XBD Utility
// Syntax Guidelines clauses behind the Issue 7 synopsis "tee [-ai]
// [file...]": "--" ends option parsing so later option-shaped tokens are
// file operands, and single-character options may be bundled.
func TestTeeIssue7OptionTerminatorAndShortBundles(t *testing.T) {
	dir := t.TempDir()

	// Guideline 10: after "--", "-a" is a pathname, not the append option.
	// The file is therefore created and truncated, not appended.
	out, errb, code := runToolDir(t, dir, "x", "--", "-a")
	if code != 0 || errb != "" || out != "x" {
		t.Fatalf("terminator: code=%d stdout=%q stderr=%q", code, out, errb)
	}
	if got := readFile(t, filepath.Join(dir, "-a")); got != "x" {
		t.Fatalf("literal -a operand: file content %q, want \"x\"", got)
	}

	// A long-option-shaped token after the terminator is a pathname too.
	out, errb, code = runToolDir(t, dir, "y", "--", "--output-error")
	if code != 0 || errb != "" || out != "y" {
		t.Fatalf("long token after terminator: code=%d stdout=%q stderr=%q", code, out, errb)
	}
	if got := readFile(t, filepath.Join(dir, "--output-error")); got != "y" {
		t.Fatalf("long token operand: file content %q, want \"y\"", got)
	}

	// A bare terminator with no operands is a pure copy to standard output.
	out, errb, code = runToolDir(t, dir, "z", "--")
	if code != 0 || errb != "" || out != "z" {
		t.Fatalf("bare terminator: code=%d stdout=%q stderr=%q", code, out, errb)
	}
	if _, err := os.Stat(filepath.Join(dir, "--")); !os.IsNotExist(err) {
		t.Fatalf("bare terminator created a file named \"--\": %v", err)
	}

	// Guideline 5-style bundling: -ai and -ia are -a plus -i, appending.
	for _, bundle := range []string{"-ai", "-ia"} {
		f := filepath.Join(dir, "f"+bundle)
		if err := os.WriteFile(f, []byte("old"), 0o666); err != nil {
			t.Fatal(err)
		}
		out, errb, code = runToolDir(t, dir, "new", bundle, f)
		if code != 0 || errb != "" || out != "new" {
			t.Fatalf("bundle %s: code=%d stdout=%q stderr=%q", bundle, code, out, errb)
		}
		if got := readFile(t, f); got != "oldnew" {
			t.Fatalf("bundle %s: file content %q, want \"oldnew\"", bundle, got)
		}
	}
}

func TestTeeStdoutWriteErrorGNUWarn(t *testing.T) {
	// GNU --output-error=warn: diagnose errors writing to any output,
	// including standard output.
	var errb bytes.Buffer
	code := runToolRaw(t, t.TempDir(), strings.NewReader("x\n"), errWriter{errors.New("broken")}, &errb, "--output-error=warn")
	if code != 1 {
		t.Errorf("stdout warn: code=%d, want 1", code)
	}
	if !strings.Contains(errb.String(), "tee: standard output: Broken") {
		t.Errorf("stdout warn: stderr=%q, want diagnostic", errb.String())
	}
}

func TestTeeStdoutPipeErrorIgnoredWithP(t *testing.T) {
	// -p (--output-error=warn-nopipe) ignores write errors to pipes,
	// including when standard output itself is a pipe.
	var errb bytes.Buffer
	out := pipeErrWriter{Writer: io.Discard, err: errors.New("broken pipe")}
	code := runToolRaw(t, t.TempDir(), strings.NewReader("x\n"), out, &errb, "-p")
	if code != 0 {
		t.Errorf("stdout pipe -p: code=%d, want 0", code)
	}
	if errb.String() != "" {
		t.Errorf("stdout pipe -p: stderr=%q, want empty", errb.String())
	}
}

func TestTeePWithNormalFile(t *testing.T) {
	// -p must not interfere with normal (non-pipe) file output.
	dir := t.TempDir()
	var errb bytes.Buffer
	out := &bytes.Buffer{}
	code := runToolRaw(t, dir, strings.NewReader("data\n"), out, &errb, "-p", "f")
	if code != 0 || errb.String() != "" || out.String() != "data\n" {
		t.Errorf("-p normal file: code=%d out=%q err=%q", code, out.String(), errb.String())
	}
	if got := readFile(t, filepath.Join(dir, "f")); got != "data\n" {
		t.Errorf("file content = %q", got)
	}
}

func TestTeeOutputErrorPipeModes(t *testing.T) {
	broken := errors.New("broken pipe")
	tests := []struct {
		name      string
		args      []string
		wantCode  int
		wantErr   bool
		wantEmpty bool
	}{
		{"warn-nopipe pipe", []string{"--output-error=warn-nopipe"}, 0, false, true},
		{"exit-nopipe pipe", []string{"--output-error=exit-nopipe"}, 0, false, true},
		{"warn pipe", []string{"--output-error=warn"}, 1, true, false},
		{"exit pipe", []string{"--output-error=exit"}, 1, true, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var errb bytes.Buffer
			out := pipeErrWriter{Writer: io.Discard, err: broken}
			code := runToolRaw(t, t.TempDir(), strings.NewReader("x\n"), out, &errb, tc.args...)
			if code != tc.wantCode {
				t.Errorf("code=%d, want %d", code, tc.wantCode)
			}
			hasErr := strings.Contains(errb.String(), "tee: standard output:")
			if hasErr != tc.wantErr {
				t.Errorf("stderr=%q, wantErr=%v", errb.String(), tc.wantErr)
			}
			if tc.wantEmpty && errb.String() != "" {
				t.Errorf("stderr=%q, want empty", errb.String())
			}
		})
	}
}

func TestTeeOutputErrorExitNoPipeNonPipe(t *testing.T) {
	// A non-pipe write error with --output-error=exit-nopipe should be
	// diagnosed and cause immediate exit.
	var errb bytes.Buffer
	code := runToolRaw(t, t.TempDir(), strings.NewReader("x\n"), errWriter{errors.New("broken")}, &errb, "--output-error=exit-nopipe")
	if code != 1 {
		t.Errorf("code=%d, want 1", code)
	}
	if !strings.Contains(errb.String(), "tee: standard output: Broken") {
		t.Errorf("stderr=%q, want diagnostic", errb.String())
	}
}
