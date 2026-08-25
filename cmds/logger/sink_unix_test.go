//go:build unix

package loggercmd

import (
	"bytes"
	"fmt"
	"log/syslog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/coreutils/tool"
)

// realReceiver is a genuine local syslog-shaped listener: an AF_UNIX
// datagram socket, exactly the transport log/syslog.Dial uses for a local
// network/raddr pair. It lets these tests exercise dialSystemLog, and
// syslogSink.Send/Close, for real, instead of only through the fakeSink
// seam the rest of logger_test.go uses — this package's actual production
// transport otherwise has zero coverage. No host syslog daemon is involved:
// the socket is created and owned by the test.
type realReceiver struct {
	t    *testing.T
	conn *net.UnixConn
	path string
}

func newRealReceiver(t *testing.T) *realReceiver {
	t.Helper()
	// AF_UNIX socket paths share one small fixed-size buffer (sun_path, 104
	// bytes on Darwin/BSD, 108 on Linux); t.TempDir() nests under a per-test
	// directory that can alone exceed that on macOS, so the socket lives
	// directly under /tmp with a short, unique name instead.
	dir, err := os.MkdirTemp("/tmp", "logger-audit")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	path := filepath.Join(dir, "log.sock")

	conn, err := net.ListenUnixgram("unixgram", &net.UnixAddr{Name: path, Net: "unixgram"})
	if err != nil {
		t.Fatalf("ListenUnixgram: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return &realReceiver{t: t, conn: conn, path: path}
}

// dial points dialSystemLog at this receiver for the lifetime of the test.
func (r *realReceiver) dial(t *testing.T) {
	t.Helper()
	oldDial := dialSyslog
	dialSyslog = func(_, _ string, pri syslog.Priority, tag string) (*syslog.Writer, error) {
		return syslog.Dial("unixgram", r.path, pri, tag)
	}
	t.Cleanup(func() { dialSyslog = oldDial })
}

// read blocks for one datagram, failing the test if none arrives in time.
func (r *realReceiver) read() string {
	r.t.Helper()
	buf := make([]byte, 4096)
	r.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	n, err := r.conn.Read(buf)
	if err != nil {
		r.t.Fatalf("reading from the real receiver: %v", err)
	}
	return string(buf[:n])
}

// The local (unix-network) wire format log/syslog writes is
// "<PRI>TIMESTAMP TAG[PID]: MSG\n" — no hostname field, unlike the
// RFC3164-over-network form. Asserting that shape end to end is what proves
// this is the real transport and not a stand-in for it.
func wantLocalWireMessage(t *testing.T, got string, pri priority, tag, msg string) {
	t.Helper()
	prefix := fmt.Sprintf("<%d>", int(pri))
	if !strings.HasPrefix(got, prefix) {
		t.Errorf("wire message = %q, want it to start with %q", got, prefix)
	}
	suffix := fmt.Sprintf("%s[%d]: %s\n", tag, os.Getpid(), msg)
	if !strings.HasSuffix(got, suffix) {
		t.Errorf("wire message = %q, want it to end with %q", got, suffix)
	}
}

// End-to-end: run() itself, unmodified, dialing a real local socket. This is
// the strongest evidence available that open/send/close actually deliver a
// message, since nothing about run()'s own code is faked.
func TestLoggerCommandDeliversOverARealLocalSyslogSocket(t *testing.T) {
	r := newRealReceiver(t)
	r.dial(t)

	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Dir:   t.TempDir(),
		Env:   []string{"LOGNAME=alice"},
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
	}
	code := run(rc, []string{"-t", "audittag", "hello", "real", "receiver"})
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errb.String())
	}

	wantLocalWireMessage(t, r.read(), defaultPriority, "audittag", "hello real receiver")
}

// The documented fidelity gap in dialSystemLog's comment says the pid is
// stamped into every record unconditionally, -i or not, because log/syslog's
// writer always includes it. This proves that with a real receiver rather
// than only asserting it in prose.
func TestRealSyslogSinkAlwaysStampsPIDRegardlessOfDashI(t *testing.T) {
	r := newRealReceiver(t)
	r.dial(t)

	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Dir:   t.TempDir(),
		Env:   []string{"LOGNAME=alice"},
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
	}
	// No -i at all: the pid must still be present on the wire.
	if code := run(rc, []string{"-t", "nopid", "msg"}); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errb.String())
	}
	wantLocalWireMessage(t, r.read(), defaultPriority, "nopid", "msg")
}

// dialSystemLog's own contract, exercised directly rather than through run(),
// so the open/send/close life cycle and the severity-routing branch in
// syslogSink.Send (taken only when a record's priority differs from the
// dial-time priority) both get real coverage.
func TestRealSyslogSinkOpenSendCloseLifecycle(t *testing.T) {
	r := newRealReceiver(t)
	r.dial(t)

	dialedPrio := priority(19<<3 | 5) // local3.notice
	s, err := dialSystemLog(&tool.RunContext{}, dialedPrio, "lifecycle")
	if err != nil {
		t.Fatalf("dialSystemLog: %v", err)
	}

	// Same priority as dial time: Send takes the plain Write path.
	if err := s.Send(record{Priority: dialedPrio, Tag: "lifecycle", Message: "same priority"}); err != nil {
		t.Fatalf("Send (same priority): %v", err)
	}
	wantLocalWireMessage(t, r.read(), dialedPrio, "lifecycle", "same priority")

	// A different priority takes the per-severity helper path. The facility
	// cannot change after Dial (log/syslog has no API for it), so the wire
	// facility must stay the DIALED facility even though the record below
	// names a different one — this is the documented constraint that is why
	// -p is resolved before the sink is opened.
	for _, tc := range []struct {
		name     string
		severity priority
	}{
		{"emerg", 0}, {"alert", 1}, {"crit", 2}, {"err", 3},
		{"warning", 4}, {"notice", 5}, {"info", 6}, {"debug", 7},
	} {
		recPrio := 3<<3 | tc.severity // facility "daemon", deliberately not local3
		if err := s.Send(record{Priority: recPrio, Tag: "lifecycle", Message: tc.name}); err != nil {
			t.Fatalf("Send (%s): %v", tc.name, err)
		}
		wantWire := dialedPrio.facility() | tc.severity
		wantLocalWireMessage(t, r.read(), wantWire, "lifecycle", tc.name)
	}

	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// A real, deterministic OPEN failure: nothing is listening at this path, so
// net.Dial itself fails — no fakeSink stand-in for the error.
func TestRealSyslogSinkOpenFailureIsReported(t *testing.T) {
	dir := t.TempDir()
	oldDial := dialSyslog
	missing := filepath.Join(dir, "nothing-listens-here.sock")
	dialSyslog = func(_, _ string, pri syslog.Priority, tag string) (*syslog.Writer, error) {
		return syslog.Dial("unixgram", missing, pri, tag)
	}
	t.Cleanup(func() { dialSyslog = oldDial })

	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Dir:   t.TempDir(),
		Env:   []string{"LOGNAME=alice"},
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
	}
	code := run(rc, []string{"msg"})
	if code == 0 {
		t.Fatalf("a socket nothing listens on must fail, got exit 0")
	}
	if !strings.Contains(errb.String(), "logger:") {
		t.Errorf("stderr = %q, want a logger diagnostic", errb.String())
	}
}
