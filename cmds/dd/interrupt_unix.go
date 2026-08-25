//go:build !windows

package ddcmd

import (
	"errors"
	"io"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

var errInterrupted = errors.New("interrupted")

var errUnsupportedInterruptibleNamedFIFOInput = errors.New(
	"interruptible named FIFO input is not supported on " + runtime.GOOS,
)

// pollSliceMS bounds every wait on a descriptor. The self-pipe already sits in
// each poll set, so cancellation does not depend on the timeout. The bounded
// retry also covers the unlikely case that creation of the self-pipe failed.
const pollSliceMS = 25

// interruptContext turns SIGINT into two things dd can wait on: an atomic flag
// for the copy loop, and a self-pipe that joins every poll set so a descriptor
// wait unblocks the moment the signal lands.
//
// The signal handler runs in exactly one goroutine, which is also the only
// writer to the self-pipe. Stop waits for that goroutine to return before the
// pipe is closed, so a wakeup write can never be delivered to a descriptor the
// process has since reused for something else.
type interruptContext struct {
	sigc     chan os.Signal
	done     chan struct{}
	exited   chan struct{}
	pipe     [2]int
	once     sync.Once
	stopOnce sync.Once
	closed   sync.Once
	sig      atomic.Int32
}

func newInterruptContext() *interruptContext {
	c := &interruptContext{
		sigc:   make(chan os.Signal, 1),
		done:   make(chan struct{}),
		exited: make(chan struct{}),
		pipe:   [2]int{-1, -1},
	}
	if err := unix.Pipe(c.pipe[:]); err != nil {
		c.pipe = [2]int{-1, -1}
	} else {
		c.pipe[0] = moveDescriptor(c.pipe[0])
		c.pipe[1] = moveDescriptor(c.pipe[1])
		for _, fd := range c.pipe {
			if fd >= 0 {
				_ = unix.SetNonblock(fd, true)
				_, _ = unix.FcntlInt(uintptr(fd), unix.F_SETFD, unix.FD_CLOEXEC)
			}
		}
	}
	signal.Notify(c.sigc, syscall.SIGINT)
	go func() {
		defer close(c.exited)
		select {
		case sig := <-c.sigc:
			n := interruptSignalNumber()
			if s, ok := sig.(syscall.Signal); ok && int(s) != 0 {
				n = int(s)
			}
			c.interrupt(n)
		case <-c.done:
			// signal.Stop waits for sends already in flight. If completion
			// and SIGINT raced, prefer the signal when it reached our channel
			// before Stop established the normal-completion boundary.
			select {
			case sig := <-c.sigc:
				n := interruptSignalNumber()
				if s, ok := sig.(syscall.Signal); ok && int(s) != 0 {
					n = int(s)
				}
				c.interrupt(n)
			default:
			}
		}
	}()
	return c
}

// Stop is idempotent so it is safe both as a deferred cleanup and as an
// explicit teardown.
func (c *interruptContext) Stop() {
	c.stopOnce.Do(func() {
		signal.Stop(c.sigc)
		close(c.done)
		// The notifier goroutine is the only writer to the self-pipe; wait for
		// it to return so closePipe cannot race a pending wakeup write.
		<-c.exited
		c.closePipe()
	})
}

func (c *interruptContext) Interrupted() bool {
	return c.sig.Load() != 0
}

func (c *interruptContext) Signal() int {
	return int(c.sig.Load())
}

func (c *interruptContext) interrupt(sig int) {
	c.once.Do(func() {
		c.sig.Store(int32(sig))
		if c.pipe[1] >= 0 {
			_, _ = unix.Write(c.pipe[1], []byte{1})
		}
	})
}

func (c *interruptContext) closePipe() {
	c.closed.Do(func() {
		for _, fd := range c.pipe {
			if fd >= 0 {
				_ = unix.Close(fd)
			}
		}
	})
}

func interruptSignalNumber() int {
	return int(syscall.SIGINT)
}

func moveDescriptor(fd int) int {
	if fd >= 0 && fd < 10 {
		nfd, err := unix.FcntlInt(uintptr(fd), unix.F_DUPFD_CLOEXEC, 10)
		if err == nil {
			_ = unix.Close(fd)
			return nfd
		}
	}
	return fd
}

// nonblockGuard remembers a descriptor's original O_NONBLOCK state. dd borrows
// rc.In and rc.Out from the embedding host, and O_NONBLOCK lives on the shared
// open file description, not on the descriptor: leaving a host's stdin or
// stdout non-blocking would break every later reader or writer of that stream.
// Every wrapper therefore restores what it found, on every exit path, and never
// closes a stream it does not own.
type nonblockGuard struct {
	fd      int
	orig    int
	changed bool
}

func setNonblockSaving(fd int) (nonblockGuard, error) {
	flags, err := unix.FcntlInt(uintptr(fd), unix.F_GETFL, 0)
	if err != nil {
		return nonblockGuard{fd: fd}, err
	}
	g := nonblockGuard{fd: fd, orig: flags}
	if flags&unix.O_NONBLOCK != 0 {
		// Already non-blocking (a Go-poller-managed pipe, or a FIFO we opened
		// that way ourselves): nothing to change and nothing to restore.
		return g, nil
	}
	if _, err := unix.FcntlInt(uintptr(fd), unix.F_SETFL, flags|unix.O_NONBLOCK); err != nil {
		return g, err
	}
	g.changed = true
	return g, nil
}

func (g *nonblockGuard) restore() {
	if !g.changed {
		return
	}
	g.changed = false
	_, _ = unix.FcntlInt(uintptr(g.fd), unix.F_SETFL, g.orig)
}

// rawDescriptor returns f's descriptor without os.File.Fd's side effect of
// detaching the file from the runtime poller: that would leave a stream the
// caller still owns permanently in thread-blocking mode after dd returns.
func rawDescriptor(f *os.File) (int, bool) {
	sc, err := f.SyscallConn()
	if err != nil {
		return -1, false
	}
	fd := -1
	if err := sc.Control(func(v uintptr) { fd = int(v) }); err != nil || fd < 0 {
		return -1, false
	}
	return fd, true
}

// blockable reports whether fd names an object whose reads or writes can block
// indefinitely, and therefore has to go through the cancellable path. Regular
// files and directories never block, so they are left exactly as they are.
func blockable(fd int) bool {
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return false
	}
	switch uint32(st.Mode) & unix.S_IFMT {
	case unix.S_IFIFO, unix.S_IFSOCK, unix.S_IFCHR:
		return true
	}
	return false
}

type interruptReader struct {
	file               *os.File
	fd                 int
	ctx                *interruptContext
	guard              nonblockGuard
	awaitingFIFOWriter bool
	closeOnce          sync.Once
}

func interruptibleReader(r io.Reader, sigctx *interruptContext) (*interruptReader, bool) {
	f, ok := r.(*os.File)
	if !ok {
		return nil, false
	}
	return newInterruptReader(f, sigctx, false)
}

func newInterruptReader(f *os.File, sigctx *interruptContext, awaitingFIFOWriter bool) (*interruptReader, bool) {
	fd, ok := rawDescriptor(f)
	if !ok || !blockable(fd) {
		return nil, false
	}
	g, err := setNonblockSaving(fd)
	if err != nil {
		g.restore()
		return nil, false
	}
	return &interruptReader{
		file:               f,
		fd:                 fd,
		ctx:                sigctx,
		guard:              g,
		awaitingFIFOWriter: awaitingFIFOWriter,
	}, true
}

// Close restores the descriptor flags dd changed. It never closes file: the
// stream is either the host's (rc.In) or one copyDD closes itself, so that a
// close error still reaches the caller.
func (r *interruptReader) Close() error {
	r.closeOnce.Do(r.guard.restore)
	return nil
}

func (r *interruptReader) Read(p []byte) (int, error) {
	for {
		if r.ctx.Interrupted() {
			return 0, errInterrupted
		}
		n, err := unix.Read(r.fd, p)
		if n > 0 {
			r.awaitingFIFOWriter = false
			return n, nil
		}
		if err == nil {
			if r.awaitingFIFOWriter {
				revents, err := pollDescriptorOrInterrupt(r.fd, unix.POLLIN|unix.POLLHUP, r.ctx)
				if err != nil {
					return 0, err
				}
				if revents&unix.POLLHUP == 0 {
					// A non-blocking FIFO read returns zero before its first
					// writer arrives. A bounded poll followed by another read
					// preserves blocking-open semantics without stranding a
					// goroutine in open(2). Retrying after the timeout is also
					// required on Darwin, where FIFO readiness can be missed.
					continue
				}
				r.awaitingFIFOWriter = false
			}
			if r.ctx.Interrupted() {
				return 0, errInterrupted
			}
			return 0, io.EOF
		}
		switch err {
		case unix.EINTR:
			continue
		case unix.EAGAIN:
			// For a FIFO, EAGAIN proves that a writer is currently
			// connected but has supplied no bytes. A later POLLHUP then
			// distinguishes its close from the initial no-writer state.
			r.awaitingFIFOWriter = false
			if _, err := pollDescriptorOrInterrupt(r.fd, unix.POLLIN|unix.POLLHUP, r.ctx); err != nil {
				return 0, err
			}
		default:
			return 0, err
		}
	}
}

// interruptWriter is the output-side counterpart of interruptReader. Without
// it a blocked write — a FIFO or pipe whose reader has stopped draining — is
// uncancellable, and a non-blocking descriptor turns ordinary backpressure into
// an EAGAIN failure. Write drains p completely, waiting for POLLOUT (or SIGINT)
// whenever the object is full, so the io.Writer contract still holds.
type interruptWriter struct {
	file      *os.File
	fd        int
	ctx       *interruptContext
	guard     nonblockGuard
	closeOnce sync.Once
}

func interruptibleWriter(w io.Writer, sigctx *interruptContext) (*interruptWriter, bool) {
	f, ok := w.(*os.File)
	if !ok {
		return nil, false
	}
	return newInterruptWriter(f, sigctx)
}

func newInterruptWriter(f *os.File, sigctx *interruptContext) (*interruptWriter, bool) {
	fd, ok := rawDescriptor(f)
	if !ok || !blockable(fd) {
		return nil, false
	}
	g, err := setNonblockSaving(fd)
	if err != nil {
		g.restore()
		return nil, false
	}
	return &interruptWriter{file: f, fd: fd, ctx: sigctx, guard: g}, true
}

// Close restores the descriptor flags dd changed, leaving the stream itself to
// its owner. It is idempotent, so copyDD can both defer it and call it
// explicitly before closing an output file it opened.
func (w *interruptWriter) Close() error {
	w.closeOnce.Do(w.guard.restore)
	return nil
}

func (w *interruptWriter) Write(p []byte) (int, error) {
	total := 0
	for total < len(p) {
		if w.ctx.Interrupted() {
			return total, errInterrupted
		}
		n, err := unix.Write(w.fd, p[total:])
		if n > 0 {
			total += n
			continue
		}
		if err == nil {
			return total, io.ErrShortWrite
		}
		switch err {
		case unix.EINTR:
			continue
		case unix.EAGAIN:
			if _, err := pollDescriptorOrInterrupt(w.fd, unix.POLLOUT, w.ctx); err != nil {
				return total, err
			}
		default:
			return total, err
		}
	}
	return total, nil
}

// waitDescriptorOrInterrupt waits for fd to become ready for events, for SIGINT
// to arrive, or for the poll slice to expire. Expiry returns nil: the caller
// retries the syscall, which either makes progress or blocks again here.
func pollDescriptorOrInterrupt(fd int, events int16, sigctx *interruptContext) (int16, error) {
	fds := []unix.PollFd{{Fd: int32(fd), Events: events}}
	if sigctx.pipe[0] >= 0 {
		fds = append(fds, unix.PollFd{Fd: int32(sigctx.pipe[0]), Events: unix.POLLIN})
	}
	for {
		if sigctx.Interrupted() {
			return 0, errInterrupted
		}
		n, err := unix.Poll(fds, pollSliceMS)
		if err == unix.EINTR {
			continue
		}
		if err != nil {
			return 0, err
		}
		if len(fds) > 1 && fds[1].Revents != 0 {
			return 0, errInterrupted
		}
		if n == 0 || fds[0].Revents != 0 {
			return fds[0].Revents, nil
		}
	}
}

// waitForInterrupt sleeps up to timeoutMS on the self-pipe alone. It is the
// retry pacing for waits that are not descriptor-readiness waits — opening a
// FIFO for writing, which reports "no reader yet" as ENXIO.
func waitForInterrupt(sigctx *interruptContext, timeoutMS int) error {
	if sigctx.Interrupted() {
		return errInterrupted
	}
	if sigctx.pipe[0] < 0 {
		time.Sleep(time.Duration(timeoutMS) * time.Millisecond)
		return nil
	}
	fds := []unix.PollFd{{Fd: int32(sigctx.pipe[0]), Events: unix.POLLIN}}
	for {
		if sigctx.Interrupted() {
			return errInterrupted
		}
		_, err := unix.Poll(fds, timeoutMS)
		if err == unix.EINTR {
			continue
		}
		if err != nil {
			return err
		}
		if fds[0].Revents != 0 {
			return errInterrupted
		}
		return nil
	}
}

// interruptibleOpenRead opens Linux FIFO input non-blocking so cancellation
// never leaves a goroutine stranded in open(2). interruptReader preserves the
// blocking-open boundary by waiting through initial zero-byte reads until
// Linux's latched POLLHUP proves that a writer connected and then closed.
//
// XNU does not retain an observable writer transition when the writer opens
// and closes between our non-blocking open and first read. In that state EOF is
// indistinguishable from "no writer has arrived", and a blocking open cannot
// be cancelled safely after unlink or rename. Other kernels are left disabled
// until the same transition and cancellation properties are proved there.
func interruptibleOpenRead(path string, sigctx *interruptContext) (*os.File, bool, error) {
	fi, statErr := os.Stat(path)
	isFIFO := statErr == nil && fi.Mode()&os.ModeNamedPipe != 0
	if !isFIFO {
		f, err := os.Open(path)
		return f, false, err
	}
	if runtime.GOOS != "linux" {
		return nil, true, errUnsupportedInterruptibleNamedFIFOInput
	}
	if sigctx.Interrupted() {
		return nil, true, errInterrupted
	}
	for {
		fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
		if err == unix.EINTR {
			if sigctx.Interrupted() {
				return nil, true, errInterrupted
			}
			continue
		}
		if err != nil {
			return nil, true, &os.PathError{Op: "open", Path: path, Err: err}
		}
		fd = moveDescriptor(fd)
		return os.NewFile(uintptr(fd), path), true, nil
	}
}

// interruptibleOpenWrite opens path for writing. A FIFO reports "no reader yet"
// as ENXIO under O_NONBLOCK, so the blocking open GNU dd performs is emulated by
// retrying between cancellable waits. The descriptor stays non-blocking on
// purpose: interruptWriter owns the backpressure from there on.
func interruptibleOpenWrite(path string, isFIFO bool, sigctx *interruptContext) (*os.File, error) {
	if !isFIFO {
		return os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0o666)
	}
	for {
		if sigctx.Interrupted() {
			return nil, errInterrupted
		}
		fd, err := unix.Open(path, unix.O_WRONLY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0o666)
		if err == nil {
			fd = moveDescriptor(fd)
			return os.NewFile(uintptr(fd), path), nil
		}
		if err != unix.ENXIO && err != unix.EINTR {
			return nil, &os.PathError{Op: "open", Path: path, Err: err}
		}
		if err := waitForInterrupt(sigctx, pollSliceMS); err != nil {
			return nil, err
		}
	}
}
