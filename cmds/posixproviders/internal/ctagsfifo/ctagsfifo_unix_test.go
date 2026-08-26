//go:build unix

package ctagsfifo

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/coreutils/tool"
	"golang.org/x/sys/unix"
)

func makeFIFO(t *testing.T, path string) {
	t.Helper()
	if err := unix.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}
}

func readFIFO(path string) <-chan struct {
	data []byte
	err  error
} {
	done := make(chan struct {
		data []byte
		err  error
	}, 1)
	go func() {
		data, err := os.ReadFile(path)
		done <- struct {
			data []byte
			err  error
		}{data, err}
	}()
	return done
}

func outputWriter(t *testing.T, payload string, check func([]string)) ExecFunc {
	t.Helper()
	return func(_ *tool.RunContext, _, _ string, args []string) int {
		if check != nil {
			check(args)
		}
		p := inspectArgs(args)
		info, err := os.Stat(p.output)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
			t.Errorf("private provider output mode=%v err=%v", modeOf(info), err)
			return 2
		}
		flags := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
		for _, arg := range args {
			if strings.HasPrefix(arg, "-") && strings.Contains(strings.TrimPrefix(arg, "-"), "a") {
				flags = os.O_WRONLY | os.O_CREATE | os.O_APPEND
				break
			}
		}
		f, err := os.OpenFile(p.output, flags, 0o600)
		if err != nil {
			t.Errorf("fake provider open: %v", err)
			return 2
		}
		_, err = io.WriteString(f, payload)
		if closeErr := f.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			t.Errorf("fake provider write: %v", err)
			return 2
		}
		return 0
	}
}

func modeOf(info os.FileInfo) os.FileMode {
	if info == nil {
		return 0
	}
	return info.Mode()
}

func TestFIFOOutputVariantsAndAppend(t *testing.T) {
	tests := []struct {
		name      string
		makeArgs  func(string) []string
		wantShape func([]string) bool
	}{
		{
			name:     "separate -f operand with spaces",
			makeArgs: func(f string) []string { return []string{"-f", f, "source.c"} },
			wantShape: func(a []string) bool {
				return len(a) == 3 && a[0] == "-f" && a[1] != "" && a[1] != "tags pipe" && a[2] == "source.c"
			},
		},
		{
			name:     "attached -f operand",
			makeArgs: func(f string) []string { return []string{"-f" + f, "source.c"} },
			wantShape: func(a []string) bool {
				return len(a) == 2 && strings.HasPrefix(a[0], "-f/") && a[1] == "source.c"
			},
		},
		{
			name:     "grouped append",
			makeArgs: func(f string) []string { return []string{"-af" + f, "source.c"} },
			wantShape: func(a []string) bool {
				return len(a) == 2 && strings.HasPrefix(a[0], "-af/") && a[1] == "source.c"
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			fifo := filepath.Join(dir, "tags pipe")
			makeFIFO(t, fifo)
			read := readFIFO(fifo)
			var errOut bytes.Buffer
			rc := &tool.RunContext{Stdio: tool.Stdio{In: bytes.NewReader(nil), Out: io.Discard, Err: &errOut}}
			exec := outputWriter(t, "tag\tfile\t1\n", func(args []string) {
				if !tt.wantShape(args) {
					t.Errorf("rewritten args = %#v", args)
				}
			})
			if got := Run(rc, "ctags", "/provider/ctags", tt.makeArgs(fifo), exec); got != 0 {
				t.Fatalf("Run() = %d, stderr %q", got, errOut.String())
			}
			result := <-read
			if result.err != nil || string(result.data) != "tag\tfile\t1\n" {
				t.Fatalf("FIFO read = %q, %v", result.data, result.err)
			}
			info, err := os.Stat(fifo)
			if err != nil || info.Mode()&os.ModeNamedPipe == 0 {
				t.Fatalf("output FIFO was replaced: mode=%v err=%v", modeOf(info), err)
			}
		})
	}
}

func TestDefaultTagsFIFOUsesRunDirectory(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "tags")
	makeFIFO(t, fifo)
	read := readFIFO(fifo)
	rc := &tool.RunContext{Dir: dir, Stdio: tool.Stdio{In: bytes.NewReader(nil), Out: io.Discard, Err: io.Discard}}
	if got := Run(rc, "ctags", "/provider/ctags", []string{"source.c"}, outputWriter(t, "default\n", func(args []string) {
		if len(args) != 3 || args[0] != "-f" || args[2] != "source.c" {
			t.Errorf("rewritten args = %#v", args)
		}
	})); got != 0 {
		t.Fatalf("Run() = %d", got)
	}
	result := <-read
	if result.err != nil || string(result.data) != "default\n" {
		t.Fatalf("FIFO read = %q, %v", result.data, result.err)
	}
}

func TestXBypassesFIFOAdapter(t *testing.T) {
	dir := t.TempDir()
	makeFIFO(t, filepath.Join(dir, "tags"))
	want := []string{"-x", "source.c"}
	called := false
	rc := &tool.RunContext{Dir: dir, Stdio: tool.Stdio{In: bytes.NewReader(nil), Out: io.Discard, Err: io.Discard}}
	got := Run(rc, "ctags", "/provider/ctags", want, func(_ *tool.RunContext, _, _ string, args []string) int {
		called = true
		if fmt.Sprint(args) != fmt.Sprint(want) {
			t.Errorf("args = %#v, want %#v", args, want)
		}
		return 19
	})
	if got != 19 || !called {
		t.Fatalf("Run() = %d, called=%v", got, called)
	}
}

func TestProviderFailurePreservesStatusSignalAndUnblocksWaitingReader(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "tags")
	makeFIFO(t, fifo)
	read := readFIFO(fifo)
	rc := &tool.RunContext{Stdio: tool.Stdio{In: bytes.NewReader(nil), Out: io.Discard, Err: io.Discard}}
	got := Run(rc, "ctags", "/provider/ctags", []string{"-f", fifo, "bad.c"}, func(rc *tool.RunContext, _, _ string, _ []string) int {
		rc.ExitSignal = 9
		return 137
	})
	if got != 137 || rc.ExitSignal != 9 {
		t.Fatalf("status/signal = %d/%d, want 137/9", got, rc.ExitSignal)
	}
	select {
	case result := <-read:
		if result.err != nil || len(result.data) != 0 {
			t.Fatalf("reader = %q, %v", result.data, result.err)
		}
	case <-time.After(time.Second):
		t.Fatal("waiting FIFO reader was not released on provider failure")
	}
}

func TestNoReaderAndCancellationAreBounded(t *testing.T) {
	t.Run("product timeout", func(t *testing.T) {
		dir := t.TempDir()
		fifo := filepath.Join(dir, "tags")
		makeFIFO(t, fifo)
		old := fifoOpenTimeout
		fifoOpenTimeout = 40 * time.Millisecond
		defer func() { fifoOpenTimeout = old }()
		var errOut bytes.Buffer
		start := time.Now()
		got := Run(&tool.RunContext{Stdio: tool.Stdio{Err: &errOut, Out: io.Discard}}, "ctags", "/provider/ctags", []string{"-f", fifo, "source.c"}, outputWriter(t, "tags\n", nil))
		if got == 0 || time.Since(start) > time.Second || !strings.Contains(errOut.String(), "deadline exceeded") {
			t.Fatalf("Run()=%d elapsed=%v stderr=%q", got, time.Since(start), errOut.String())
		}
	})

	t.Run("caller cancellation", func(t *testing.T) {
		dir := t.TempDir()
		fifo := filepath.Join(dir, "tags")
		makeFIFO(t, fifo)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		var errOut bytes.Buffer
		got := Run(&tool.RunContext{Ctx: ctx, Stdio: tool.Stdio{Err: &errOut, Out: io.Discard}}, "ctags", "/provider/ctags", []string{"-f", fifo, "source.c"}, outputWriter(t, "tags\n", nil))
		if got == 0 || !strings.Contains(errOut.String(), "context canceled") {
			t.Fatalf("Run()=%d stderr=%q", got, errOut.String())
		}
	})
}

func TestCancellationAfterRendezvousWithStalledReaderIsBounded(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "tags")
	makeFIFO(t, fifo)

	opened := make(chan *os.File, 1)
	readerErr := make(chan error, 1)
	go func() {
		f, err := os.Open(fifo)
		if err != nil {
			readerErr <- err
			return
		}
		opened <- f
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var errOut bytes.Buffer
	var privatePath string
	runDone := make(chan int, 1)
	go func() {
		runDone <- Run(
			&tool.RunContext{Ctx: ctx, Stdio: tool.Stdio{Err: &errOut, Out: io.Discard}},
			"ctags", "/provider/ctags", []string{"-f", fifo, "source.c"},
			outputWriter(t, strings.Repeat("tag\tfile\t1\n", 1<<20), func(args []string) {
				privatePath = inspectArgs(args).output
			}),
		)
	}()

	var reader *os.File
	select {
	case reader = <-opened:
	case err := <-readerErr:
		t.Fatalf("reader open: %v", err)
	case <-time.After(time.Second):
		t.Fatal("reader did not rendezvous")
	}
	defer reader.Close()

	// Give the writer enough time to fill the pipe while the reader deliberately
	// consumes nothing, then require cancellation to break the poll/write loop.
	time.Sleep(40 * time.Millisecond)
	start := time.Now()
	cancel()
	select {
	case got := <-runDone:
		if got == 0 || !strings.Contains(errOut.String(), "context canceled") {
			t.Fatalf("Run()=%d stderr=%q", got, errOut.String())
		}
	case <-time.After(time.Second):
		t.Fatal("cancellation did not stop stalled FIFO writer")
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("cancellation took %v", elapsed)
	}
	if privatePath == "" {
		t.Fatal("provider private output path was not captured")
	}
	if _, err := os.Stat(privatePath); !os.IsNotExist(err) {
		t.Fatalf("private output remains after cancellation: %v", err)
	}
}

func TestFIFOIdentityChangeFailsWithoutWritingReplacement(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "tags")
	makeFIFO(t, fifo)
	var errOut bytes.Buffer
	rc := &tool.RunContext{Stdio: tool.Stdio{Err: &errOut, Out: io.Discard}}
	got := Run(rc, "ctags", "/provider/ctags", []string{"-f", fifo, "source.c"}, func(_ *tool.RunContext, _, _ string, args []string) int {
		p := inspectArgs(args)
		if err := os.WriteFile(p.output, []byte("generated\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(fifo); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fifo, []byte("replacement\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		return 0
	})
	if got == 0 || !strings.Contains(errOut.String(), "changed during ctags execution") {
		t.Fatalf("Run()=%d stderr=%q", got, errOut.String())
	}
	data, err := os.ReadFile(fifo)
	if err != nil || string(data) != "replacement\n" {
		t.Fatalf("replacement = %q, %v", data, err)
	}
}

func TestPrivateOutputIdentityChangeIsRejected(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "tags")
	makeFIFO(t, fifo)
	var errOut bytes.Buffer
	rc := &tool.RunContext{Stdio: tool.Stdio{Err: &errOut, Out: io.Discard}}
	got := Run(rc, "ctags", "/provider/ctags", []string{"-f", fifo, "source.c"}, func(_ *tool.RunContext, _, _ string, args []string) int {
		p := inspectArgs(args)
		if err := os.Remove(p.output); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p.output, []byte("replacement\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		return 0
	})
	if got == 0 || !strings.Contains(errOut.String(), "private output changed") {
		t.Fatalf("Run()=%d stderr=%q", got, errOut.String())
	}
}

func TestPrivateOutputFIFOReplacementIsRejectedWithoutBlocking(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "tags")
	makeFIFO(t, fifo)
	var errOut bytes.Buffer
	start := time.Now()
	got := Run(&tool.RunContext{Stdio: tool.Stdio{Err: &errOut, Out: io.Discard}}, "ctags", "/provider/ctags", []string{"-f", fifo, "source.c"}, func(_ *tool.RunContext, _, _ string, args []string) int {
		private := inspectArgs(args).output
		if err := os.Remove(private); err != nil {
			t.Fatal(err)
		}
		makeFIFO(t, private)
		return 0
	})
	if got == 0 || !strings.Contains(errOut.String(), "private output changed") {
		t.Fatalf("Run()=%d stderr=%q", got, errOut.String())
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("FIFO replacement rejection took %v", elapsed)
	}
}

func TestPrivateOutputSymlinkReplacementIsRejected(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "tags")
	makeFIFO(t, fifo)
	replacement := filepath.Join(dir, "replacement")
	if err := os.WriteFile(replacement, []byte("replacement\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var errOut bytes.Buffer
	got := Run(&tool.RunContext{Stdio: tool.Stdio{Err: &errOut, Out: io.Discard}}, "ctags", "/provider/ctags", []string{"-f", fifo, "source.c"}, func(_ *tool.RunContext, _, _ string, args []string) int {
		private := inspectArgs(args).output
		if err := os.Remove(private); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(replacement, private); err != nil {
			t.Fatal(err)
		}
		return 0
	})
	if got == 0 || !strings.Contains(errOut.String(), "private output changed") {
		t.Fatalf("Run()=%d stderr=%q", got, errOut.String())
	}
	data, err := os.ReadFile(replacement)
	if err != nil || string(data) != "replacement\n" {
		t.Fatalf("replacement = %q, %v", data, err)
	}
}

func TestBrokenFIFOReaderPropagatesWriteFailure(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "tags")
	makeFIFO(t, fifo)
	opened := make(chan struct{})
	go func() {
		f, err := os.Open(fifo)
		if err == nil {
			close(opened)
			_ = f.Close()
		}
	}()
	var errOut bytes.Buffer
	payload := strings.Repeat("tag\tfile\t1\n", 1<<18)
	got := Run(&tool.RunContext{Stdio: tool.Stdio{Err: &errOut, Out: io.Discard}}, "ctags", "/provider/ctags", []string{"-f", fifo, "source.c"}, outputWriter(t, payload, nil))
	select {
	case <-opened:
	case <-time.After(time.Second):
		t.Fatal("reader did not rendezvous")
	}
	if got == 0 || errOut.Len() == 0 {
		t.Fatalf("Run()=%d stderr=%q, want write failure", got, errOut.String())
	}
}
