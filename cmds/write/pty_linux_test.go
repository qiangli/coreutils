//go:build linux

package writecmd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/qiangli/coreutils/tool"
)

func openPTYLinux() (*os.File, *os.File, string, error) {
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		return nil, nil, "", err
	}
	// TIOCSPTLCK takes a POINTER to the new lock value. IoctlSetInt passes the
	// value in the argument slot instead, so it fails with EFAULT and every
	// test here silently skips - which is how a PTY suite ends up proving
	// nothing at all.
	if err := unix.IoctlSetPointerInt(int(master.Fd()), unix.TIOCSPTLCK, 0); err != nil {
		master.Close()
		return nil, nil, "", err
	}
	ptyNum, err := unix.IoctlGetInt(int(master.Fd()), unix.TIOCGPTN)
	if err != nil {
		master.Close()
		return nil, nil, "", err
	}
	slavePath := fmt.Sprintf("/dev/pts/%d", ptyNum)
	slave, err := os.OpenFile(slavePath, os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		master.Close()
		return nil, nil, "", err
	}
	return master, slave, slavePath, nil
}

func TestDefaultSenderTerminalNameAndAlertSinkAreSamePTYLinux(t *testing.T) {
	master, slave, slavePath, err := openPTYLinux()
	if err != nil {
		t.Skipf("PTY creation skipped: %v", err)
	}
	defer master.Close()
	defer slave.Close()

	oldDev := devDir
	devDir = "/dev"
	defer func() { devDir = oldDev }()
	rc := &tool.RunContext{Stdio: tool.Stdio{In: slave, Out: io.Discard, Err: io.Discard}}
	name := defaultSenderTTY(rc)
	if name != strings.TrimPrefix(slavePath, "/dev/") {
		t.Fatalf("sender tty=%q want=%q", name, strings.TrimPrefix(slavePath, "/dev/"))
	}
	control, err := defaultOpenSenderControlTTY(rc, name)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeString(control, "\a\a"); err != nil {
		t.Fatal(err)
	}
	if err := control.Close(); err != nil {
		t.Fatal(err)
	}
	_ = master.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 16)
	n, err := master.Read(buf)
	if err != nil || !bytes.Contains(buf[:n], []byte("\a\a")) {
		t.Fatalf("authenticated PTY read n=%d err=%v data=%q", n, err, buf[:n])
	}
}

func TestDefaultSenderAlertSinkRejectsDifferentPTYLinux(t *testing.T) {
	firstMaster, firstSlave, _, err := openPTYLinux()
	if err != nil {
		t.Skipf("PTY creation skipped: %v", err)
	}
	defer firstMaster.Close()
	defer firstSlave.Close()
	secondMaster, secondSlave, secondPath, err := openPTYLinux()
	if err != nil {
		t.Skipf("second PTY creation skipped: %v", err)
	}
	defer secondMaster.Close()
	defer secondSlave.Close()

	oldDev := devDir
	devDir = "/dev"
	defer func() { devDir = oldDev }()
	rc := &tool.RunContext{Stdio: tool.Stdio{In: firstSlave}}
	if control, err := defaultOpenSenderControlTTY(rc, strings.TrimPrefix(secondPath, "/dev/")); err == nil {
		_ = control.Close()
		t.Fatal("unrelated PTY accepted as sender alert sink")
	}
}

func TestDefaultSenderTTYRejectsCharacterDeviceThatIsNotTerminalLinux(t *testing.T) {
	f, err := os.Open("/dev/null")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rc := &tool.RunContext{Stdio: tool.Stdio{In: f, Out: f, Err: f}}
	if got := defaultSenderTTY(rc); got != "" {
		t.Fatalf("/dev/null identified as sender terminal %q", got)
	}
}

func TestRunContextNeverFallsBackToProcessGlobalTTYLinux(t *testing.T) {
	master, slave, slavePath, err := openPTYLinux()
	if err != nil {
		t.Skipf("PTY creation skipped: %v", err)
	}
	defer master.Close()
	defer slave.Close()

	oldIn, oldOut, oldErr := os.Stdin, os.Stdout, os.Stderr
	os.Stdin, os.Stdout, os.Stderr = slave, slave, slave
	defer func() { os.Stdin, os.Stdout, os.Stderr = oldIn, oldOut, oldErr }()
	oldDev := devDir
	devDir = "/dev"
	defer func() { devDir = oldDev }()

	rc := &tool.RunContext{Stdio: tool.Stdio{
		In: strings.NewReader("body\n"), Out: io.Discard, Err: io.Discard,
	}}
	if got := defaultSenderTTY(rc); got != "" {
		t.Fatalf("embedded RunContext inherited process-global tty %q", got)
	}
	expected := strings.TrimPrefix(slavePath, "/dev/")
	if control, err := defaultOpenSenderControlTTY(rc, expected); err == nil {
		_ = control.Close()
		t.Fatal("process-global tty authenticated an unrelated RunContext")
	}
}

func TestDefaultRecipientOpenAuthenticatesTerminalLinux(t *testing.T) {
	master, slave, slavePath, err := openPTYLinux()
	if err != nil {
		t.Skipf("PTY creation skipped: %v", err)
	}
	defer master.Close()
	defer slave.Close()
	f, err := defaultOpenTTY(slavePath)
	if err != nil {
		t.Fatalf("real PTY rejected: %v", err)
	}
	_ = f.Close()
	regular := filepath.Join(t.TempDir(), "not-a-tty")
	if err := os.WriteFile(regular, nil, 0o620); err != nil {
		t.Fatal(err)
	}
	if _, err := defaultOpenTTY(regular); err == nil {
		t.Fatal("regular file accepted as recipient terminal")
	}
}

func TestRecipientPIDMustOwnTheRecordedPTYLinux(t *testing.T) {
	master, slave, slavePath, err := openPTYLinux()
	if err != nil {
		t.Skipf("PTY creation skipped: %v", err)
	}
	defer master.Close()
	defer slave.Close()

	child := osexec.Command("sleep", "10")
	child.Stdin, child.Stdout, child.Stderr = slave, slave, slave
	child.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	if err := child.Start(); err != nil {
		t.Skipf("cannot create PTY-backed session: %v", err)
	}
	defer func() {
		_ = child.Process.Kill()
		_ = child.Wait()
	}()
	if !defaultSessionOwnsTerminal(child.Process.Pid, slavePath) {
		t.Fatalf("PID %d was not associated with its controlling PTY %s", child.Process.Pid, slavePath)
	}

	otherMaster, otherSlave, otherPath, err := openPTYLinux()
	if err != nil {
		t.Skipf("second PTY creation skipped: %v", err)
	}
	defer otherMaster.Close()
	defer otherSlave.Close()
	if defaultSessionOwnsTerminal(child.Process.Pid, otherPath) {
		t.Fatalf("PID %d falsely associated with unrelated PTY %s", child.Process.Pid, otherPath)
	}
}

func TestPTYBackedWriteLinux(t *testing.T) {
	master, slave, slavePath, err := openPTYLinux()
	if err != nil {
		t.Skipf("PTY creation skipped: %v", err)
	}
	defer master.Close()
	defer slave.Close()

	relLine := strings.TrimPrefix(slavePath, "/dev/")
	w := install(t, fixture{
		sender: "alice", uid: 1000, myTTY: "pts/999",
		logins: []login{{user: "bob", line: relLine, mode: 0o620, when: epoch}},
	})

	oldOpen := openTTYFn
	openTTYFn = func(path string) (io.WriteCloser, error) {
		return slave, nil
	}
	defer func() { openTTYFn = oldOpen }()

	out, errOut, code := exec(t, "pty test\n", "bob")
	if code != 0 {
		t.Fatalf("exit code %d: %s", code, errOut)
	}
	if out != "" {
		t.Errorf("stdout must be empty, got %q", out)
	}

	buf := make([]byte, 1024)
	_ = master.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	n, _ := master.Read(buf)
	got := string(buf[:n])
	if !strings.Contains(got, "Message from alice (pts/999) [") || strings.Contains(got, "to bob") {
		t.Errorf("master PTY received: %q, want the exact POSIX banner", got)
	}
	if !strings.Contains(got, "pty test\r\n") || !strings.Contains(got, "EOT\r\n") {
		t.Errorf("master PTY received: %q, want the body and the EOT marker", got)
	}
	_ = w
}

func TestPTYCanonicalVEOLDiscoveryLinux(t *testing.T) {
	master, slave, _, err := openPTYLinux()
	if err != nil {
		t.Skipf("PTY creation skipped: %v", err)
	}
	defer master.Close()
	defer slave.Close()
	term, err := getTermios(int(slave.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	const veol = byte(0x1d)
	term.Cc[unix.VEOL] = veol
	if err := unix.IoctlSetTermios(int(slave.Fd()), unix.TCSETS, term); err != nil {
		t.Fatal(err)
	}
	if got := defaultGetVEOL(slave); got != veol {
		t.Fatalf("defaultGetVEOL(real PTY) = %#x, want %#x", got, veol)
	}
}

// readMaster drains whatever the recipient's terminal has been sent. A real
// PTY delivers in whatever chunks the driver chooses, so a single Read can
// return the banner alone; keep reading until the closing EOT arrives or the
// deadline passes.
func readMaster(t *testing.T, master *os.File) string {
	t.Helper()
	var got strings.Builder
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		_ = master.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
		buf := make([]byte, 4096)
		n, err := master.Read(buf)
		got.Write(buf[:n])
		if strings.Contains(got.String(), "EOT") {
			break
		}
		if err != nil && n == 0 {
			continue
		}
	}
	return got.String()
}

// The BEL pass-through is a claim about what arrives at a REAL terminal
// device, so it is worth proving against one: a tty driver is the thing that
// would interpret or drop the byte.
func TestPTYBELAndUTF8ReachTheRecipientUnchangedLinux(t *testing.T) {
	master, slave, slavePath, err := openPTYLinux()
	if err != nil {
		t.Skipf("PTY creation skipped: %v", err)
	}
	defer master.Close()
	defer slave.Close()

	relLine := strings.TrimPrefix(slavePath, "/dev/")
	install(t, fixture{
		sender: "alice", uid: 1000, myTTY: "pts/999",
		logins: []login{{user: "bob", line: relLine, mode: 0o620, when: epoch}},
	})
	openTTYFn = func(string) (io.WriteCloser, error) { return nopWriteCloser{slave}, nil }

	out, errOut, code := execEnv(t, []string{"LC_ALL=en_US.UTF-8"}, "wake\aup caf\u00e9 \u2615\n", "bob")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if out != "" {
		t.Errorf("standard output must stay empty, got %q", out)
	}

	got := readMaster(t, master)
	if !strings.Contains(got, "wake\aup caf\u00e9 \u2615\r\n") {
		t.Errorf("real terminal received %q; BEL and the UTF-8 bytes must arrive unchanged", got)
	}
	if strings.Contains(got, "^G") {
		t.Errorf("BEL was rewritten to caret notation on a real terminal: %q", got)
	}
}

// POSIX routes the informational message to stdout and only the two alerts to
// the authenticated sender terminal.
func TestPTYSenderAlertsReachTheControllingTerminalLinux(t *testing.T) {
	master, slave, _, err := openPTYLinux()
	if err != nil {
		t.Skipf("PTY creation skipped: %v", err)
	}
	defer master.Close()
	defer slave.Close()

	w := install(t, fixture{
		sender: "alice", uid: 1000, myTTY: "pts/999",
		logins: []login{
			{user: "bob", line: "pts/2", mode: 0o620, when: epoch},
			{user: "bob", line: "pts/4", mode: 0o620, when: epoch.Add(time.Hour)},
		},
	})
	openSenderControlTTYFn = func(*tool.RunContext, string) (io.WriteCloser, error) {
		return nopWriteCloser{slave}, nil
	}

	out, errOut, code := exec(t, "hi\n", "bob")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if want := "write: bob is logged in on more than one line; using pts/4\n"; out != want {
		t.Errorf("standard output = %q, want %q", out, want)
	}

	var sender strings.Builder
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && !strings.Contains(sender.String(), "\a\a") {
		_ = master.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
		buf := make([]byte, 1024)
		n, _ := master.Read(buf)
		sender.Write(buf[:n])
	}
	got := sender.String()
	if strings.Contains(got, "bob is logged in on more than one line") {
		t.Errorf("sender terminal received stdout informational message: %q", got)
	}
	if !strings.Contains(got, "\a\a") {
		t.Errorf("sender terminal received %q, want two alerts", got)
	}
	if body := w.read(t, "pts/4"); !strings.Contains(body, "hi\nEOT\n") {
		t.Errorf("recipient terminal = %q", body)
	}
}

// defaultSenderTTY names the sender's terminal by matching device numbers
// under /dev. Only a real device exercises that: a temp-dir stand-in has no
// rdev to match.
func TestPTYSenderTerminalNameResolvesToARealDeviceLinux(t *testing.T) {
	_, slave, slavePath, err := openPTYLinux()
	if err != nil {
		t.Skipf("PTY creation skipped: %v", err)
	}
	defer slave.Close()

	old := devDir
	devDir = "/dev"
	defer func() { devDir = old }()

	rc := &tool.RunContext{Stdio: tool.Stdio{In: slave}}
	want := strings.TrimPrefix(slavePath, "/dev/")
	if got := defaultSenderTTY(rc); got != want {
		t.Errorf("defaultSenderTTY = %q, want %q", got, want)
	}
}

// An interrupt has to be honoured while the sender is parked in a read on a
// REAL terminal — the case the poll(2) path exists for, and the one a pipe
// cannot reproduce.
func TestPTYInterruptDuringTerminalReadLinux(t *testing.T) {
	master, slave, _, err := openPTYLinux()
	if err != nil {
		t.Skipf("PTY creation skipped: %v", err)
	}
	defer master.Close()
	defer slave.Close()

	w := install(t, fixture{
		sender: "alice", uid: 1000, myTTY: "pts/999",
		logins: []login{{user: "bob", line: "pts/9", mode: 0o620, when: epoch}},
	})

	before := goroutineCount()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Dir: t.TempDir(), Stdio: tool.Stdio{In: slave, Out: &out, Err: &errb}}
	done := make(chan int, 1)
	go func() { done <- run(rc, []string{"bob"}) }()

	if _, err := master.Write([]byte("typed on a tty\n")); err != nil {
		t.Fatal(err)
	}
	waitForBanner(t, w, "pts/9")
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		if strings.Contains(w.read(t, "pts/9"), "typed on a tty\n") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	p, _ := os.FindProcess(os.Getpid())
	if err := p.Signal(os.Interrupt); err != nil {
		t.Skipf("cannot send signal: %v", err)
	}
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("exit = %d, want 0 on SIGINT", code)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("write stayed blocked in a terminal read after SIGINT")
	}

	if got := w.read(t, "pts/9"); !strings.Contains(got, "typed on a tty\nEOT\n") {
		t.Errorf("recipient terminal = %q, want the typed line then EOT", got)
	}
	if _, err := slave.Stat(); err != nil {
		t.Errorf("the caller's terminal was closed: %v", err)
	}
	if after := settledGoroutineCount(before); after > before {
		t.Errorf("goroutines: %d before, %d after; a reader was left blocked on the tty", before, after)
	}
}
