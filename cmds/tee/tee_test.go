package teecmd

import (
	"bytes"
	"context"
	"errors"
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
	// POSIX default: an error writing to standard output is fatal and
	// must not produce a diagnostic on standard error.
	var errb bytes.Buffer
	code := runToolRaw(t, t.TempDir(), strings.NewReader("x\n"), errWriter{errors.New("broken")}, &errb)
	if code != 1 {
		t.Errorf("stdout error: code=%d, want 1", code)
	}
	if errb.String() != "" {
		t.Errorf("stdout error: stderr=%q, want empty", errb.String())
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
