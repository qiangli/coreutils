//go:build !windows

package ddcmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/qiangli/coreutils/multicall"
	"github.com/qiangli/coreutils/tool"
	"golang.org/x/sys/unix"
)

const (
	ddSignalHelperEnv      = "COREUTILS_DD_SIGNAL_HELPER"
	ddSignalHelperArgsEnv  = "COREUTILS_DD_SIGNAL_ARGS"
	ddSignalHelperDirEnv   = "COREUTILS_DD_SIGNAL_DIR"
	ddSignalHelperBoundEnv = "COREUTILS_DD_SIGNAL_BOUNDARY"
)

// TestMain keeps a SIGINT registration alive for the whole test binary. These
// tests raise SIGINT on the test process itself, and without a standing
// handler a delivery that lands in the window after dd has torn its own
// handler down would kill the test run rather than fail a case.
func TestMain(m *testing.M) {
	if os.Getenv(ddSignalHelperEnv) != "1" {
		trap := make(chan os.Signal, 64)
		signal.Notify(trap, syscall.SIGINT)
		go func() {
			for range trap {
			}
		}()
	}
	os.Exit(m.Run())
}

func TestDdSignalHelperProcess(t *testing.T) {
	if os.Getenv(ddSignalHelperEnv) != "1" {
		return
	}
	var args []string
	if err := json.Unmarshal([]byte(os.Getenv(ddSignalHelperArgsEnv)), &args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	rc := &tool.RunContext{
		Ctx: context.Background(), Dir: os.Getenv(ddSignalHelperDirEnv),
		Stdio: tool.Stdio{In: os.Stdin, Out: os.Stdout, Err: os.Stderr},
	}
	code := cmd.Run(rc, args)
	if os.Getenv(ddSignalHelperBoundEnv) == "1" && rc.ExitSignal != 0 {
		multicall.TerminateBySignal(rc.ExitSignal)
	}
	os.Exit(code)
}

func TestDdEmbeddedSIGINTReturnsStatusAndDoesNotSignalHost(t *testing.T) {
	requireInterruptibleNamedFIFOInput(t)
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "fifo")
	var errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: ioDiscardWriter{}, Err: &errb},
	}
	done := make(chan int, 1)
	go func() {
		done <- cmd.Run(rc, []string{"if=" + fifo, "status=noxfer"})
	}()
	// Attaching a writer completes dd's blocking open, leaving it parked in
	// read(2) with a live but silent writer.
	w := attachFIFOWriter(t, fifo)
	defer w.Close()
	signalSelfSIGINT(t)
	code := waitCode(t, done)
	if code != 130 {
		t.Fatalf("code=%d want 130; stderr=%q", code, errb.String())
	}
	if rc.ExitSignal != int(syscall.SIGINT) {
		t.Fatalf("ExitSignal=%d want SIGINT", rc.ExitSignal)
	}
	if got, want := errb.String(), "0+0 records in\n0+0 records out\n"; got != want {
		t.Fatalf("status=%q want %q", got, want)
	}
}

// No-writer FIFO input must remain cancellable after its pathname disappears
// or starts naming a different object. The implementation owns only the
// non-blocking read descriptor; no blocked open or release goroutine may
// outlive Run.
func TestDdSIGINTDuringNoWriterFIFOInputDoesNotLeak(t *testing.T) {
	requireInterruptibleNamedFIFOInput(t)
	for _, mutation := range []string{"unlink", "rename"} {
		t.Run(mutation, func(t *testing.T) {
			dir := t.TempDir()
			fifo := makeFIFO(t, dir, "fifo")
			beforeG := runtime.NumGoroutine()
			beforeFD := openDescriptorCount(t)
			var errb bytes.Buffer
			rc := &tool.RunContext{
				Ctx:   context.Background(),
				Dir:   dir,
				Stdio: tool.Stdio{In: strings.NewReader(""), Out: ioDiscardWriter{}, Err: &errb},
			}
			done := make(chan int, 1)
			go func() { done <- cmd.Run(rc, []string{"if=" + fifo, "status=noxfer"}) }()
			time.Sleep(100 * time.Millisecond)
			if mutation == "unlink" {
				if err := os.Remove(fifo); err != nil {
					t.Fatal(err)
				}
			} else if err := os.Rename(fifo, filepath.Join(dir, "moved")); err != nil {
				t.Fatal(err)
			}
			signalSelfSIGINT(t)
			if code := waitCode(t, done); code != 130 {
				t.Fatalf("code=%d want 130; stderr=%q", code, errb.String())
			}
			if rc.ExitSignal != int(syscall.SIGINT) {
				t.Fatalf("ExitSignal=%d want SIGINT", rc.ExitSignal)
			}
			if got, want := errb.String(), "0+0 records in\n0+0 records out\n"; got != want {
				t.Fatalf("status=%q want %q", got, want)
			}
			waitForGoroutines(t, beforeG)
			waitForDescriptors(t, beforeFD)
		})
	}
}

// A writer that connects and closes without sending anything is end-of-input,
// not the initial no-writer state of a non-blocking FIFO descriptor.
func TestDdFIFORecognizesWriterThatSendsNothing(t *testing.T) {
	requireInterruptibleNamedFIFOInput(t)
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "fifo")
	var errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: ioDiscardWriter{}, Err: &errb},
	}
	done := make(chan int, 1)
	go func() { done <- cmd.Run(rc, []string{"if=" + fifo, "status=noxfer"}) }()
	w := attachFIFOWriter(t, fifo)
	w.Close()
	if code := waitCode(t, done); code != 0 {
		t.Fatalf("code=%d want 0; stderr=%q", code, errb.String())
	}
	if got, want := errb.String(), "0+0 records in\n0+0 records out\n"; got != want {
		t.Fatalf("status=%q want %q", got, want)
	}
}

// TestDdDrainedPipeInputReportsEOF is the `: | dd` case. A closed anonymous
// pipe is end-of-input immediately; treating a zero-length read as "no writer
// yet" would hang the copy forever.
func TestDdDrainedPipeInputReportsEOF(t *testing.T) {
	r, w := blockingPipe(t)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	var errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Stdio: tool.Stdio{In: r, Out: ioDiscardWriter{}, Err: &errb},
	}
	done := make(chan int, 1)
	go func() { done <- cmd.Run(rc, []string{"bs=4", "status=noxfer"}) }()
	if code := waitCode(t, done); code != 0 {
		t.Fatalf("code=%d want 0; stderr=%q", code, errb.String())
	}
	if got, want := errb.String(), "0+0 records in\n0+0 records out\n"; got != want {
		t.Fatalf("status=%q want %q", got, want)
	}
}

func TestDdStandaloneSIGINTWaitStatus(t *testing.T) {
	requireInterruptibleNamedFIFOInput(t)
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "fifo")
	child, stderr := startSignalHelper(t, dir, []string{"if=fifo", "status=noxfer"}, true)
	w := attachFIFOWriter(t, fifo)
	defer w.Close()
	if err := child.Process.Signal(syscall.SIGINT); err != nil {
		t.Fatal(err)
	}
	err := child.Wait()
	ee, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("err=%v (%T), want signal exit", err, err)
	}
	ws, ok := ee.ProcessState.Sys().(syscall.WaitStatus)
	if !ok || !ws.Signaled() || ws.Signal() != syscall.SIGINT {
		t.Fatalf("wait status=%v signaled=%v signal=%v", ee.ProcessState.Sys(), ok && ws.Signaled(), ws.Signal())
	}
	if got, want := stderr.String(), "0+0 records in\n0+0 records out\n"; got != want {
		t.Fatalf("status=%q want %q", got, want)
	}
}

// TestDdStandaloneSIGINTWaitStatusOnBlockedOutput is the same standalone
// boundary assertion for the output side: dd is wedged in a write to a FIFO
// nobody is draining, and the process still has to end up WIFSIGNALED.
func TestDdStandaloneSIGINTWaitStatusOnBlockedOutput(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "fifo")
	// A non-blocking reader that never reads: it lets dd's own open succeed and
	// then lets the FIFO fill up.
	stalled := openStalledFIFOReader(t, fifo)
	defer unix.Close(stalled)
	child, stderr := startSignalHelper(t, dir,
		[]string{"if=/dev/zero", "of=fifo", "bs=65536", "count=64", "status=noxfer"}, true)
	time.Sleep(200 * time.Millisecond)
	if err := child.Process.Signal(syscall.SIGINT); err != nil {
		t.Fatal(err)
	}
	err := child.Wait()
	ee, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("err=%v (%T), want signal exit; stderr=%q", err, err, stderr.String())
	}
	ws, ok := ee.ProcessState.Sys().(syscall.WaitStatus)
	if !ok || !ws.Signaled() || ws.Signal() != syscall.SIGINT {
		t.Fatalf("wait status=%v; stderr=%q", ee.ProcessState.Sys(), stderr.String())
	}
	if got := strings.Count(stderr.String(), "records in\n"); got != 1 {
		t.Fatalf("records-in blocks=%d stderr=%q", got, stderr.String())
	}
}

func TestDdBlockedPipeReadSIGINTDoesNotLeakReader(t *testing.T) {
	r, w := blockingPipe(t)
	defer w.Close()
	before := runtime.NumGoroutine()
	var errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Stdio: tool.Stdio{In: r, Out: ioDiscardWriter{}, Err: &errb},
	}
	done := make(chan int, 1)
	go func() { done <- cmd.Run(rc, []string{"bs=4", "status=noxfer"}) }()
	time.Sleep(50 * time.Millisecond)
	signalSelfSIGINT(t)
	if code := waitCode(t, done); code != 130 {
		t.Fatalf("code=%d want 130; stderr=%q", code, errb.String())
	}
	waitForGoroutines(t, before)
}

// TestDdBlockedStdoutWriteIsCancellable pins that rc.Out is wrapped too: with
// the pipe's reader stalled dd parks in write(2), and only a cancellable write
// path can turn that into a 130 instead of a hang.
func TestDdBlockedStdoutWriteIsCancellable(t *testing.T) {
	pr, pw := blockingPipe(t)
	defer pr.Close()
	before := runtime.NumGoroutine()
	var errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Stdio: tool.Stdio{In: &repeatReader{remaining: 4 << 20}, Out: pw, Err: &errb},
	}
	done := make(chan int, 1)
	go func() { done <- cmd.Run(rc, []string{"bs=65536", "status=noxfer"}) }()
	time.Sleep(100 * time.Millisecond)
	signalSelfSIGINT(t)
	if code := waitCode(t, done); code != 130 {
		t.Fatalf("code=%d want 130; stderr=%q", code, errb.String())
	}
	if rc.ExitSignal != int(syscall.SIGINT) {
		t.Fatalf("ExitSignal=%d want SIGINT", rc.ExitSignal)
	}
	if got := strings.Count(errb.String(), "records in\n"); got != 1 {
		t.Fatalf("records-in blocks=%d stderr=%q", got, errb.String())
	}
	waitForGoroutines(t, before)
}

// TestDdOutputFIFOAppliesBackpressure is the normal-path counterpart: a slow
// reader must produce ordinary blocking behavior, not an EAGAIN failure.
func TestDdOutputFIFOAppliesBackpressure(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "out")
	const total = 1 << 20
	received := make(chan []byte, 1)
	readErr := make(chan error, 1)
	go func() {
		f, err := os.OpenFile(fifo, os.O_RDONLY, 0)
		if err != nil {
			readErr <- err
			return
		}
		defer f.Close()
		var got bytes.Buffer
		chunk := make([]byte, 4096)
		for {
			n, err := f.Read(chunk)
			got.Write(chunk[:n])
			if err != nil {
				break
			}
			// Deliberately slower than dd can write, so the FIFO fills and dd
			// has to wait for POLLOUT repeatedly.
			time.Sleep(200 * time.Microsecond)
		}
		received <- got.Bytes()
	}()
	var errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Stdio: tool.Stdio{In: &repeatReader{remaining: total}, Out: ioDiscardWriter{}, Err: &errb},
	}
	done := make(chan int, 1)
	go func() { done <- cmd.Run(rc, []string{"of=out", "bs=65536", "status=noxfer"}) }()
	code := waitCodeWithin(t, done, 30*time.Second)
	if code != 0 {
		t.Fatalf("code=%d want 0; stderr=%q", code, errb.String())
	}
	if got, want := errb.String(), "16+0 records in\n16+0 records out\n"; got != want {
		t.Fatalf("status=%q want %q", got, want)
	}
	select {
	case err := <-readErr:
		t.Fatal(err)
	case got := <-received:
		if len(got) != total {
			t.Fatalf("received %d bytes want %d", len(got), total)
		}
		if !bytes.Equal(got, bytes.Repeat([]byte{'x'}, total)) {
			t.Fatal("received bytes differ from what dd was given")
		}
	case <-time.After(30 * time.Second):
		t.Fatal("FIFO reader did not finish")
	}
}

// TestDdOutputFIFOBlockedWriteIsCancellable wedges dd in a write to a FIFO
// whose reader never drains, including the fill that happens right after the
// FIFO open succeeds.
func TestDdOutputFIFOBlockedWriteIsCancellable(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "out")
	stalled := openStalledFIFOReader(t, fifo)
	defer unix.Close(stalled)
	before := runtime.NumGoroutine()
	var errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Stdio: tool.Stdio{In: &repeatReader{remaining: 4 << 20}, Out: ioDiscardWriter{}, Err: &errb},
	}
	done := make(chan int, 1)
	go func() { done <- cmd.Run(rc, []string{"of=out", "bs=65536", "status=noxfer"}) }()
	time.Sleep(100 * time.Millisecond)
	signalSelfSIGINT(t)
	if code := waitCode(t, done); code != 130 {
		t.Fatalf("code=%d want 130; stderr=%q", code, errb.String())
	}
	if rc.ExitSignal != int(syscall.SIGINT) {
		t.Fatalf("ExitSignal=%d want SIGINT", rc.ExitSignal)
	}
	if got := strings.Count(errb.String(), "records in\n"); got != 1 {
		t.Fatalf("records-in blocks=%d stderr=%q", got, errb.String())
	}
	waitForGoroutines(t, before)
}

// TestDdRestoresCallerDescriptorFlagsOnInterrupt is the fcntl regression for
// the borrowed-stream rule: O_NONBLOCK lives on the shared open file
// description, so a host whose stdin or stdout came back non-blocking would
// see spurious EAGAIN failures from then on. The streams must also still be
// open afterwards.
func TestDdRestoresCallerDescriptorFlagsOnInterrupt(t *testing.T) {
	inR, inW := blockingPipe(t)
	outR, outW := blockingPipe(t)
	defer inW.Close()
	defer outR.Close()
	beforeIn, beforeOut := descriptorFlags(t, inR), descriptorFlags(t, outW)
	if beforeIn&unix.O_NONBLOCK != 0 || beforeOut&unix.O_NONBLOCK != 0 {
		t.Fatalf("test fixture is already non-blocking: in=%#x out=%#x", beforeIn, beforeOut)
	}
	var errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Stdio: tool.Stdio{In: inR, Out: outW, Err: &errb},
	}
	done := make(chan int, 1)
	go func() { done <- cmd.Run(rc, []string{"bs=4", "status=noxfer"}) }()
	time.Sleep(50 * time.Millisecond)
	signalSelfSIGINT(t)
	if code := waitCode(t, done); code != 130 {
		t.Fatalf("code=%d want 130; stderr=%q", code, errb.String())
	}
	if got := descriptorFlags(t, inR); got != beforeIn {
		t.Fatalf("stdin flags=%#x want %#x", got, beforeIn)
	}
	if got := descriptorFlags(t, outW); got != beforeOut {
		t.Fatalf("stdout flags=%#x want %#x", got, beforeOut)
	}
	assertStreamsStillUsable(t, inR, inW, outR, outW)
}

func TestDdRestoresCallerDescriptorFlagsOnNormalExit(t *testing.T) {
	inR, inW := blockingPipe(t)
	outR, outW := blockingPipe(t)
	defer inW.Close()
	defer outR.Close()
	beforeIn, beforeOut := descriptorFlags(t, inR), descriptorFlags(t, outW)
	drained := make(chan []byte, 1)
	go func() {
		got, _ := io.ReadAll(outR)
		drained <- got
	}()
	go func() {
		_, _ = inW.Write([]byte("payload"))
		_ = inW.Close()
	}()
	var errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Stdio: tool.Stdio{In: inR, Out: outW, Err: &errb},
	}
	done := make(chan int, 1)
	go func() { done <- cmd.Run(rc, []string{"bs=4", "status=noxfer"}) }()
	if code := waitCode(t, done); code != 0 {
		t.Fatalf("code=%d want 0; stderr=%q", code, errb.String())
	}
	if got := descriptorFlags(t, inR); got != beforeIn {
		t.Fatalf("stdin flags=%#x want %#x", got, beforeIn)
	}
	if got := descriptorFlags(t, outW); got != beforeOut {
		t.Fatalf("stdout flags=%#x want %#x", got, beforeOut)
	}
	if err := outW.Close(); err != nil {
		t.Fatalf("dd closed a stream it only borrowed: %v", err)
	}
	if got := string(<-drained); got != "payload" {
		t.Fatalf("stdout=%q want %q", got, "payload")
	}
}

func TestDdRepeatedInvocationsRestoreCallerDescriptorFlags(t *testing.T) {
	inR, inW := blockingPipe(t)
	outR, outW := blockingPipe(t)
	defer inW.Close()
	defer outR.Close()
	beforeIn, beforeOut := descriptorFlags(t, inR), descriptorFlags(t, outW)
	for i := range 10 {
		var errb bytes.Buffer
		rc := &tool.RunContext{
			Ctx:   context.Background(),
			Stdio: tool.Stdio{In: inR, Out: outW, Err: &errb},
		}
		done := make(chan int, 1)
		go func() { done <- cmd.Run(rc, []string{"bs=4", "status=noxfer"}) }()
		time.Sleep(20 * time.Millisecond)
		signalSelfSIGINT(t)
		if code := waitCode(t, done); code != 130 {
			t.Fatalf("run %d: code=%d stderr=%q", i, code, errb.String())
		}
		if got := descriptorFlags(t, inR); got != beforeIn {
			t.Fatalf("run %d: stdin flags=%#x want %#x", i, got, beforeIn)
		}
		if got := descriptorFlags(t, outW); got != beforeOut {
			t.Fatalf("run %d: stdout flags=%#x want %#x", i, got, beforeOut)
		}
	}
	assertStreamsStillUsable(t, inR, inW, outR, outW)
}

func TestDdConcurrentInvocationsRestoreCallerDescriptorFlags(t *testing.T) {
	const runs = 8
	type stream struct {
		inR, inW   *os.File
		outR, outW *os.File
		flagsIn    int
		flagsOut   int
		code       chan int
	}
	streams := make([]*stream, runs)
	for i := range streams {
		inR, inW := blockingPipe(t)
		outR, outW := blockingPipe(t)
		s := &stream{inR: inR, inW: inW, outR: outR, outW: outW, code: make(chan int, 1)}
		s.flagsIn, s.flagsOut = descriptorFlags(t, inR), descriptorFlags(t, outW)
		streams[i] = s
	}
	for _, s := range streams {
		rc := &tool.RunContext{
			Ctx:   context.Background(),
			Stdio: tool.Stdio{In: s.inR, Out: s.outW, Err: ioDiscardWriter{}},
		}
		go func() { s.code <- cmd.Run(rc, []string{"bs=4", "status=none"}) }()
	}
	time.Sleep(100 * time.Millisecond)
	// One delivery reaches every concurrently running dd: os/signal fans a
	// signal out to each registered channel.
	signalSelfSIGINT(t)
	for i, s := range streams {
		if code := waitCode(t, s.code); code != 130 {
			t.Fatalf("run %d: code=%d want 130", i, code)
		}
	}
	for i, s := range streams {
		if got := descriptorFlags(t, s.inR); got != s.flagsIn {
			t.Fatalf("run %d: stdin flags=%#x want %#x", i, got, s.flagsIn)
		}
		if got := descriptorFlags(t, s.outW); got != s.flagsOut {
			t.Fatalf("run %d: stdout flags=%#x want %#x", i, got, s.flagsOut)
		}
		assertStreamsStillUsable(t, s.inR, s.inW, s.outR, s.outW)
	}
}

// TestDdInterruptedRunsDoNotLeakDescriptors covers the internally owned files:
// the FIFO dd opens itself, its output file, and the interrupt context's own
// self-pipe all have to be gone by the time Run returns.
func TestDdInterruptedRunsDoNotLeakDescriptors(t *testing.T) {
	requireInterruptibleNamedFIFOInput(t)
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "fifo")
	run := func() {
		var errb bytes.Buffer
		rc := &tool.RunContext{
			Ctx:   context.Background(),
			Dir:   dir,
			Stdio: tool.Stdio{In: strings.NewReader(""), Out: ioDiscardWriter{}, Err: &errb},
		}
		done := make(chan int, 1)
		go func() { done <- cmd.Run(rc, []string{"if=fifo", "of=out", "status=none"}) }()
		w := attachFIFOWriter(t, fifo)
		time.Sleep(10 * time.Millisecond)
		signalSelfSIGINT(t)
		if code := waitCode(t, done); code != 130 {
			t.Fatalf("code=%d want 130; stderr=%q", code, errb.String())
		}
		_ = w.Close()
	}
	run() // warm up: first call allocates whatever is allocated once
	before := openDescriptorCount(t)
	for range 20 {
		run()
	}
	// Allow the abandoned-open release goroutines their moment to finish.
	deadline := time.Now().Add(2 * time.Second)
	for {
		after := openDescriptorCount(t)
		if after <= before+2 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("open descriptors after 20 interrupted runs=%d before=%d", after, before)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestDdSIGINTStatusEmittedOnceUnderRepeatedSignals(t *testing.T) {
	r, w := blockingPipe(t)
	defer w.Close()
	var errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Stdio: tool.Stdio{In: r, Out: ioDiscardWriter{}, Err: &errb},
	}
	done := make(chan int, 1)
	go func() { done <- cmd.Run(rc, []string{"bs=4", "status=noxfer"}) }()
	time.Sleep(50 * time.Millisecond)
	for range 5 {
		signalSelfSIGINT(t)
	}
	if code := waitCode(t, done); code != 130 {
		t.Fatalf("code=%d want 130", code)
	}
	if got := strings.Count(errb.String(), "records in\n"); got != 1 {
		t.Fatalf("records-in blocks=%d stderr=%q", got, errb.String())
	}
	if rc.ExitSignal != int(syscall.SIGINT) {
		t.Fatalf("ExitSignal=%d want SIGINT", rc.ExitSignal)
	}
}

func TestDdSIGINTEOFRaceStress(t *testing.T) {
	for i := range 50 {
		r, w := blockingPipe(t)
		var errb bytes.Buffer
		rc := &tool.RunContext{
			Ctx:   context.Background(),
			Stdio: tool.Stdio{In: r, Out: ioDiscardWriter{}, Err: &errb},
		}
		done := make(chan int, 1)
		go func() { done <- cmd.Run(rc, []string{"bs=4", "status=noxfer"}) }()
		time.Sleep(time.Millisecond)
		signalSelfSIGINT(t)
		_ = w.Close()
		code := waitCode(t, done)
		if code != 0 && code != 130 {
			t.Fatalf("iteration %d: code=%d stderr=%q", i, code, errb.String())
		}
		if (code == 130) != (rc.ExitSignal != 0) {
			t.Fatalf("iteration %d: code=%d ExitSignal=%d disagree", i, code, rc.ExitSignal)
		}
		if strings.Count(errb.String(), "records in\n") != 1 {
			t.Fatalf("iteration %d status emitted %d times: %q",
				i, strings.Count(errb.String(), "records in\n"), errb.String())
		}
	}
}

func TestDdFIFOEOFSIGINTOrdering(t *testing.T) {
	requireInterruptibleNamedFIFOInput(t)
	t.Run("signal wins", func(t *testing.T) {
		for i := range 20 {
			dir := t.TempDir()
			fifo := makeFIFO(t, dir, "fifo")
			var errb bytes.Buffer
			rc := &tool.RunContext{
				Ctx: context.Background(), Dir: dir,
				Stdio: tool.Stdio{In: strings.NewReader(""), Out: ioDiscardWriter{}, Err: &errb},
			}
			done := make(chan int, 1)
			go func() { done <- cmd.Run(rc, []string{"if=fifo", "status=noxfer"}) }()
			w := attachFIFOWriter(t, fifo)
			time.Sleep(2 * pollSliceMS * time.Millisecond)
			signalSelfSIGINT(t)
			// Establish delivery before creating EOF; this makes the
			// ordering deterministic instead of testing scheduler latency in
			// the runtime's os/signal relay.
			time.Sleep(pollSliceMS * time.Millisecond)
			_ = w.Close()
			if code := waitCode(t, done); code != 130 {
				t.Fatalf("iteration %d: code=%d want 130; stderr=%q", i, code, errb.String())
			}
			if got := strings.Count(errb.String(), "records in\n"); got != 1 {
				t.Fatalf("iteration %d: status count=%d stderr=%q", i, got, errb.String())
			}
		}
	})
	t.Run("EOF wins after completion", func(t *testing.T) {
		dir := t.TempDir()
		fifo := makeFIFO(t, dir, "fifo")
		var errb bytes.Buffer
		rc := &tool.RunContext{
			Ctx: context.Background(), Dir: dir,
			Stdio: tool.Stdio{In: strings.NewReader(""), Out: ioDiscardWriter{}, Err: &errb},
		}
		done := make(chan int, 1)
		go func() { done <- cmd.Run(rc, []string{"if=fifo", "status=noxfer"}) }()
		w := attachFIFOWriter(t, fifo)
		time.Sleep(2 * pollSliceMS * time.Millisecond)
		_ = w.Close()
		if code := waitCode(t, done); code != 0 {
			t.Fatalf("code=%d want 0; stderr=%q", code, errb.String())
		}
		if rc.ExitSignal != 0 {
			t.Fatalf("ExitSignal=%d want 0", rc.ExitSignal)
		}
	})
}

func TestDdNormalPathResetsExitSignal(t *testing.T) {
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:        context.Background(),
		ExitSignal: int(syscall.SIGINT),
		Stdio:      tool.Stdio{In: strings.NewReader("abc"), Out: &out, Err: &errb},
	}
	if code := cmd.Run(rc, []string{"bs=3", "count=1", "status=none"}); code != 0 {
		t.Fatalf("code=%d stderr=%q", code, errb.String())
	}
	if rc.ExitSignal != 0 {
		t.Fatalf("ExitSignal=%d want reset", rc.ExitSignal)
	}
	if out.String() != "abc" {
		t.Fatalf("out=%q want abc", out.String())
	}
}

func startSignalHelper(t *testing.T, dir string, args []string, boundary bool) (*exec.Cmd, *bytes.Buffer) {
	t.Helper()
	encoded, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	child := exec.Command(os.Args[0], "-test.run=^TestDdSignalHelperProcess$")
	child.Env = append(os.Environ(),
		ddSignalHelperEnv+"=1",
		ddSignalHelperDirEnv+"="+dir,
		ddSignalHelperArgsEnv+"="+string(encoded),
	)
	if boundary {
		child.Env = append(child.Env, ddSignalHelperBoundEnv+"=1")
	}
	child.Stderr = &stderr
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if child.Process != nil && child.ProcessState == nil {
			_ = child.Process.Kill()
			_, _ = child.Process.Wait()
		}
	})
	return child, &stderr
}

func makeFIFO(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := unix.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func requireInterruptibleNamedFIFOInput(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("exact interruptible named-FIFO input is currently supported only on Linux")
	}
}

// attachFIFOWriter opens the write side of a FIFO. Since dd opens its read side
// non-blocking, success establishes that dd owns the input descriptor.
func attachFIFOWriter(t *testing.T, path string) *os.File {
	t.Helper()
	type result struct {
		f   *os.File
		err error
	}
	ch := make(chan result, 1)
	go func() {
		f, err := os.OpenFile(path, os.O_WRONLY, 0)
		ch <- result{f, err}
	}()
	select {
	case r := <-ch:
		if r.err != nil {
			t.Fatal(r.err)
		}
		return r.f
	case <-time.After(5 * time.Second):
		t.Fatal("dd did not open the FIFO for reading before the deadline")
		return nil
	}
}

// openStalledFIFOReader opens a FIFO for reading and never reads from it, so a
// writer's own open succeeds and then fills the pipe buffer.
func openStalledFIFOReader(t *testing.T, path string) int {
	t.Helper()
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NONBLOCK, 0)
	if err != nil {
		t.Fatal(err)
	}
	return fd
}

// blockingPipe returns a pipe that the Go runtime has not put into
// non-blocking mode, so a test can observe exactly which flags dd changes.
func blockingPipe(t *testing.T) (*os.File, *os.File) {
	t.Helper()
	var fds [2]int
	if err := unix.Pipe(fds[:]); err != nil {
		t.Fatal(err)
	}
	r := os.NewFile(uintptr(fds[0]), "pipe-r")
	w := os.NewFile(uintptr(fds[1]), "pipe-w")
	t.Cleanup(func() {
		_ = r.Close()
		_ = w.Close()
	})
	return r, w
}

// settableFlagMask covers the status flags F_SETFL can actually change. The
// rest of what F_GETFL reports includes kernel bookkeeping (Darwin sets an
// internal "has been written" bit, for instance) that moves for reasons which
// have nothing to do with the descriptor state dd is responsible for.
const settableFlagMask = unix.O_NONBLOCK | unix.O_APPEND

// descriptorFlags reads F_GETFL without os.File.Fd's poller side effects, so
// the observation itself cannot change what is being observed.
func descriptorFlags(t *testing.T, f *os.File) int {
	t.Helper()
	sc, err := f.SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	flags, ferr := 0, error(nil)
	if err := sc.Control(func(fd uintptr) {
		flags, ferr = unix.FcntlInt(fd, unix.F_GETFL, 0)
	}); err != nil {
		t.Fatal(err)
	}
	if ferr != nil {
		t.Fatal(ferr)
	}
	return flags & settableFlagMask
}

func assertStreamsStillUsable(t *testing.T, inR, inW, outR, outW *os.File) {
	t.Helper()
	if _, err := inW.Write([]byte("ping")); err != nil {
		t.Fatalf("borrowed stdin pipe unusable: %v", err)
	}
	buf := make([]byte, 4)
	if _, err := io.ReadFull(inR, buf); err != nil {
		t.Fatalf("borrowed stdin unreadable: %v", err)
	}
	if string(buf) != "ping" {
		t.Fatalf("stdin round trip=%q want ping", buf)
	}
	if _, err := outW.Write([]byte("pong")); err != nil {
		t.Fatalf("borrowed stdout unusable: %v", err)
	}
	if _, err := io.ReadFull(outR, buf); err != nil {
		t.Fatalf("borrowed stdout unreadable: %v", err)
	}
	if string(buf) != "pong" {
		t.Fatalf("stdout round trip=%q want pong", buf)
	}
}

func openDescriptorCount(t *testing.T) int {
	t.Helper()
	entries, err := os.ReadDir("/dev/fd")
	if err != nil {
		t.Skipf("/dev/fd unavailable: %v", err)
	}
	return len(entries)
}

func waitForGoroutines(t *testing.T, before int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		after := runtime.NumGoroutine()
		if after <= before+4 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("goroutines after dd returned=%d before=%d", after, before)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func waitForDescriptors(t *testing.T, before int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		after := openDescriptorCount(t)
		if after <= before {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("open descriptors after dd returned=%d before=%d", after, before)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func signalSelfSIGINT(t *testing.T) {
	t.Helper()
	if err := syscall.Kill(syscall.Getpid(), syscall.SIGINT); err != nil {
		t.Fatal(err)
	}
}

func waitCode(t *testing.T, done <-chan int) int {
	t.Helper()
	return waitCodeWithin(t, done, 5*time.Second)
}

func waitCodeWithin(t *testing.T, done <-chan int, limit time.Duration) int {
	t.Helper()
	select {
	case code := <-done:
		return code
	case <-time.After(limit):
		t.Fatal("dd did not return within the deadline")
		return -1
	}
}

type ioDiscardWriter struct{}

func (ioDiscardWriter) Write(p []byte) (int, error) { return len(p), nil }

// repeatReader supplies a fixed number of identical bytes without allocating
// the whole payload.
type repeatReader struct{ remaining int64 }

func (r *repeatReader) Read(p []byte) (int, error) {
	if r.remaining <= 0 {
		return 0, io.EOF
	}
	n := int64(len(p))
	if n > r.remaining {
		n = r.remaining
	}
	for i := range p[:n] {
		p[i] = 'x'
	}
	r.remaining -= n
	return int(n), nil
}
