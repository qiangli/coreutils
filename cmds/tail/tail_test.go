package tailcmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/qiangli/coreutils/tool"
)

func runTool(t *testing.T, dir, stdin string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	if dir == "" {
		dir = t.TempDir()
	}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Stdio: tool.Stdio{In: strings.NewReader(stdin), Out: &out, Err: &errb},
	}
	code = cmd.Run(rc, args)
	return out.String(), errb.String(), code
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func appendFile(t *testing.T, dir, name, content string) {
	t.Helper()
	f, err := os.OpenFile(filepath.Join(dir, name), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(content); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()
}

func twelveLines() string {
	var b strings.Builder
	for i := 1; i <= 12; i++ {
		fmt.Fprintf(&b, "line%d\n", i)
	}
	return b.String()
}

func TestTail(t *testing.T) {
	cases := []struct {
		name  string
		stdin string
		args  []string
		want  string
	}{
		{"default ten lines", twelveLines(), nil,
			"line3\nline4\nline5\nline6\nline7\nline8\nline9\nline10\nline11\nline12\n"},
		{"n two", "a\nb\nc\n", []string{"-n", "2"}, "b\nc\n"},
		{"n minus two same", "a\nb\nc\n", []string{"-n", "-2"}, "b\nc\n"},
		{"obsolete shorthand", "a\nb\nc\n", []string{"-2"}, "b\nc\n"},
		{"n zero", "a\nb\n", []string{"-n", "0"}, ""},
		{"from line two", "a\nb\nc\n", []string{"-n", "+2"}, "b\nc\n"},
		{"plus one whole file", "a\nb\n", []string{"-n", "+1"}, "a\nb\n"},
		{"plus zero whole file", "a\nb\n", []string{"-n", "+0"}, "a\nb\n"},
		{"plus beyond eof", "a\nb\n", []string{"-n", "+9"}, ""},
		{"bytes", "abcdef", []string{"-c", "4"}, "cdef"},
		{"bytes beyond size", "ab", []string{"-c", "9"}, "ab"},
		{"bytes from start", "abcdef", []string{"-c", "+3"}, "cdef"},
		{"bytes plus one whole", "ab", []string{"-c", "+1"}, "ab"},
		{"final partial line counts", "a\nb\nc", []string{"-n", "2"}, "b\nc"},
		{"c after n wins", "abc\ndef\n", []string{"-n", "1", "-c", "2"}, "f\n"},
	}
	for _, c := range cases {
		out, errb, code := runTool(t, "", c.stdin, c.args...)
		if out != c.want || code != 0 {
			t.Errorf("%s: tail %v = (%q, %q, %d), want (%q, _, 0)", c.name, c.args, out, errb, code, c.want)
		}
	}
}

func TestTailHeaders(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a", "1\n2\n")
	writeFile(t, dir, "b", "3\n")

	out, _, code := runTool(t, dir, "", "a", "b")
	want := "==> a <==\n1\n2\n\n==> b <==\n3\n"
	if out != want || code != 0 {
		t.Errorf("two files: got (%q, %d), want %q", out, code, want)
	}

	out, _, _ = runTool(t, dir, "", "-q", "a", "b")
	if out != "1\n2\n3\n" {
		t.Errorf("-q: got %q", out)
	}

	out, _, _ = runTool(t, dir, "in\n", "-v", "-")
	if out != "==> standard input <==\nin\n" {
		t.Errorf("-v stdin: got %q", out)
	}
}

func TestTailZeroTerminated(t *testing.T) {
	out, _, code := runTool(t, "", "a\x00b\x00c\x00", "-z", "-n", "2")
	if code != 0 || out != "b\x00c\x00" {
		t.Errorf("tail -z -n 2: out=%q code=%d", out, code)
	}
	out, _, code = runTool(t, "", "a\x00b\x00c\x00", "-z", "-n", "+2")
	if code != 0 || out != "b\x00c\x00" {
		t.Errorf("tail -z -n +2: out=%q code=%d", out, code)
	}
}

func TestTailFollowNotSupported(t *testing.T) {
	_, errb, code := runTool(t, "", "x\n", "--follow=invalid")
	if code != 2 || !strings.Contains(errb, "valid arguments for -f") {
		t.Errorf("tail --follow=invalid: err=%q code=%d, want usage error exit 2", errb, code)
	}

	_, errb, code = runTool(t, "", "x\n", "--follow=name", "-")
	if code != 1 || !strings.Contains(errb, "cannot follow standard input") {
		t.Errorf("tail -f name with stdin: err=%q code=%d, want exit 1", errb, code)
	}
}

func TestTailErrors(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a", "1\n")

	_, errb, code := runTool(t, dir, "", "missing", "a")
	if code != 1 || !strings.Contains(errb, "cannot open 'missing' for reading") {
		t.Errorf("missing file: err=%q code=%d", errb, code)
	}

	_, errb, code = runTool(t, "", "", "-n", "x")
	if code != 2 || !strings.Contains(errb, "invalid number of lines") {
		t.Errorf("bad -n: err=%q code=%d", errb, code)
	}

	_, errb, code = runTool(t, "", "", "--frobnicate")
	if code != 2 || !strings.Contains(errb, "frobnicate") || !strings.Contains(errb, "pure-Go") {
		t.Errorf("unknown flag: err=%q code=%d", errb, code)
	}
}

func TestTailHelpVersion(t *testing.T) {
	out, _, code := runTool(t, "", "", "--help")
	if code != 0 || !strings.Contains(out, "Usage: tail") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	out, _, code = runTool(t, "", "", "--version")
	if code != 0 || !strings.Contains(out, "tail") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}
}

// --- Follow mode tests ---

func TestTailFollowDescriptor(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "test.log", "line1\nline2\nline3\n")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   ctx,
		Dir:   dir,
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
	}

	var wg sync.WaitGroup
	wg.Add(1)
	var code int
	go func() {
		defer wg.Done()
		code = cmd.Run(rc, []string{"-f", "-s", "0.1", "test.log"})
	}()

	// Give follow time to output initial content.
	time.Sleep(200 * time.Millisecond)

	// Append more data.
	appendFile(t, dir, "test.log", "line4\nline5\n")

	// Give follow time to pick it up.
	time.Sleep(300 * time.Millisecond)
	cancel()
	wg.Wait()

	outStr := out.String()
	if code != 0 {
		t.Errorf("follow exit code: got %d, want 0", code)
	}
	if !strings.Contains(outStr, "line3\n") {
		t.Errorf("initial content missing: %q", outStr)
	}
	if !strings.Contains(outStr, "line4\n") {
		t.Errorf("followed content missing (line4): %q", outStr)
	}
	if !strings.Contains(outStr, "line5\n") {
		t.Errorf("followed content missing (line5): %q", outStr)
	}
}

func TestTailFollowByName(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "test.log", "line1\nline2\nline3\n")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   ctx,
		Dir:   dir,
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
	}

	var wg sync.WaitGroup
	wg.Add(1)
	var code int
	go func() {
		defer wg.Done()
		code = cmd.Run(rc, []string{"--follow=name", "-s", "0.1", "test.log"})
	}()

	time.Sleep(200 * time.Millisecond)
	appendFile(t, dir, "test.log", "line4\n")
	time.Sleep(300 * time.Millisecond)
	cancel()
	wg.Wait()

	outStr := out.String()
	if code != 0 {
		t.Errorf("exit code: got %d, want 0", code)
	}
	if !strings.Contains(outStr, "line4\n") {
		t.Errorf("followed content missing: %q", outStr)
	}
}

func TestTailFollowF(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "test.log", "line1\nline2\nline3\n")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   ctx,
		Dir:   dir,
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
	}

	var wg sync.WaitGroup
	wg.Add(1)
	var code int
	go func() {
		defer wg.Done()
		code = cmd.Run(rc, []string{"-F", "-s", "0.1", "test.log"})
	}()

	time.Sleep(200 * time.Millisecond)
	appendFile(t, dir, "test.log", "line4\n")
	time.Sleep(300 * time.Millisecond)
	cancel()
	wg.Wait()

	outStr := out.String()
	if code != 0 {
		t.Errorf("exit code: got %d, want 0", code)
	}
	if !strings.Contains(outStr, "line4\n") {
		t.Errorf("followed content missing with -F: %q", outStr)
	}
}

func TestTailFollowRetry(t *testing.T) {
	dir := t.TempDir()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   ctx,
		Dir:   dir,
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
	}

	var wg sync.WaitGroup
	wg.Add(1)
	var code int
	go func() {
		defer wg.Done()
		code = cmd.Run(rc, []string{"--follow=name", "--retry", "-s", "0.1", "test.log"})
	}()

	// File doesn't exist yet; tail should retry.
	time.Sleep(400 * time.Millisecond)

	// Create the file.
	writeFile(t, dir, "test.log", "line1\nline2\n")

	time.Sleep(400 * time.Millisecond)
	appendFile(t, dir, "test.log", "line3\n")
	time.Sleep(300 * time.Millisecond)
	cancel()
	wg.Wait()

	outStr := out.String()
	if code != 0 {
		t.Errorf("exit code: got %d, want 0", code)
	}
	if !strings.Contains(outStr, "line1\n") {
		t.Errorf("initial content missing after retry: %q", outStr)
	}
	if !strings.Contains(outStr, "line3\n") {
		t.Errorf("followed content missing after retry: %q", outStr)
	}
}

func TestTailFollowSleepInterval(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "test.log", "start\n")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   ctx,
		Dir:   dir,
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
	}

	var wg sync.WaitGroup
	wg.Add(1)
	var code int
	go func() {
		defer wg.Done()
		code = cmd.Run(rc, []string{"-f", "-s", "0.05", "test.log"})
	}()

	time.Sleep(150 * time.Millisecond)
	appendFile(t, dir, "test.log", "more\n")
	time.Sleep(200 * time.Millisecond)
	cancel()
	wg.Wait()

	if code != 0 {
		t.Errorf("exit code: got %d, want 0", code)
	}
	if !strings.Contains(out.String(), "more\n") {
		t.Errorf("content missing with custom sleep interval: %q", out.String())
	}
}

func TestTailFollowSleepIntervalInvalid(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "test.log", "data\n")

	_, errb, code := runTool(t, dir, "", "-f", "-s", "-1", "test.log")
	if code != 2 || !strings.Contains(errb, "invalid sleep interval") {
		t.Errorf("negative sleep interval: err=%q code=%d, want exit 2", errb, code)
	}

	_, errb, code = runTool(t, dir, "", "-f", "-s", "0", "test.log")
	if code != 2 || !strings.Contains(errb, "invalid sleep interval") {
		t.Errorf("zero sleep interval: err=%q code=%d, want exit 2", errb, code)
	}
}

func TestTailFollowPid(t *testing.T) {
	// Create a child process we can signal.
	dir := t.TempDir()
	writeFile(t, dir, "test.log", "line1\nline2\n")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   ctx,
		Dir:   dir,
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
	}

	// Use PID 1 (init), which always exists and won't die.
	// The test verifies that the --pid flag is accepted and doesn't crash.
	var wg sync.WaitGroup
	wg.Add(1)
	var code int
	go func() {
		defer wg.Done()
		code = cmd.Run(rc, []string{"-f", "--pid=1", "-s", "0.1", "test.log"})
	}()

	time.Sleep(300 * time.Millisecond)
	cancel()
	wg.Wait()

	outStr := out.String()
	if code != 0 {
		t.Errorf("exit code: got %d, want 0", code)
	}
	if !strings.Contains(outStr, "line2\n") {
		t.Errorf("initial content missing with --pid: %q", outStr)
	}
}

func TestTailFollowDeadPid(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "test.log", "line1\n")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   ctx,
		Dir:   dir,
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
	}

	// Use a very high PID that almost certainly doesn't exist.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		cmd.Run(rc, []string{"-f", "--pid=999999", "-s", "0.1", "test.log"})
	}()

	time.Sleep(500 * time.Millisecond)
	cancel()
	wg.Wait()

	// Should exit cleanly after detecting dead PID.
	outStr := out.String()
	if !strings.Contains(outStr, "line1\n") {
		t.Errorf("initial content missing with dead --pid: %q", outStr)
	}
}

func TestTailFollowDebug(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "test.log", "line1\n")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   ctx,
		Dir:   dir,
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		cmd.Run(rc, []string{"-f", "--debug", "-s", "0.1", "test.log"})
	}()

	time.Sleep(300 * time.Millisecond)
	cancel()
	wg.Wait()

	errStr := errb.String()
	if !strings.Contains(errStr, "==> tail: following") {
		t.Errorf("--debug output missing: %q", errStr)
	}
}

func TestTailFollowUsePolling(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "test.log", "line1\n")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   ctx,
		Dir:   dir,
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
	}

	var wg sync.WaitGroup
	wg.Add(1)
	var code int
	go func() {
		defer wg.Done()
		code = cmd.Run(rc, []string{"-f", "--use-polling", "-s", "0.1", "test.log"})
	}()

	time.Sleep(200 * time.Millisecond)
	appendFile(t, dir, "test.log", "line2\n")
	time.Sleep(300 * time.Millisecond)
	cancel()
	wg.Wait()

	if code != 0 {
		t.Errorf("exit code: got %d, want 0", code)
	}
	if !strings.Contains(out.String(), "line2\n") {
		t.Errorf("followed content missing with --use-polling: %q", out.String())
	}
}

func TestTailFollowMaxUnchangedStats(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "test.log", "line1\nline2\n")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   ctx,
		Dir:   dir,
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
	}

	var wg sync.WaitGroup
	wg.Add(1)
	var code int
	go func() {
		defer wg.Done()
		code = cmd.Run(rc, []string{"--follow=name", "--max-unchanged-stats=2", "-s", "0.1", "test.log"})
	}()

	time.Sleep(200 * time.Millisecond)
	appendFile(t, dir, "test.log", "line3\n")
	time.Sleep(500 * time.Millisecond)
	appendFile(t, dir, "test.log", "line4\n")
	time.Sleep(300 * time.Millisecond)
	cancel()
	wg.Wait()

	outStr := out.String()
	if code != 0 {
		t.Errorf("exit code: got %d, want 0", code)
	}
	if !strings.Contains(outStr, "line3\n") {
		t.Errorf("content missing with --max-unchanged-stats: %q", outStr)
	}
}

func TestTailFollowMultiFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.log", "a1\na2\n")
	writeFile(t, dir, "b.log", "b1\nb2\n")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   ctx,
		Dir:   dir,
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
	}

	var wg sync.WaitGroup
	wg.Add(1)
	var code int
	go func() {
		defer wg.Done()
		code = cmd.Run(rc, []string{"-f", "-s", "0.1", "a.log", "b.log"})
	}()

	time.Sleep(200 * time.Millisecond)
	appendFile(t, dir, "a.log", "a3\n")
	time.Sleep(200 * time.Millisecond)
	appendFile(t, dir, "b.log", "b3\n")
	time.Sleep(300 * time.Millisecond)
	cancel()
	wg.Wait()

	outStr := out.String()
	if code != 0 {
		t.Errorf("exit code: got %d, want 0", code)
	}
	if !strings.Contains(outStr, "==> a.log <==") {
		t.Errorf("header for a.log missing: %q", outStr)
	}
	if !strings.Contains(outStr, "==> b.log <==") {
		t.Errorf("header for b.log missing: %q", outStr)
	}
}
