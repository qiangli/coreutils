//go:build windows

package timeoutcmd

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// On Windows there are no POSIX signals to deliver to another process; timeout
// maps any signal request to job termination or process kill.
func signalByName(name string) os.Signal {
	n := strings.TrimPrefix(strings.ToUpper(strings.TrimSpace(name)), "SIG")
	if n == "" {
		return nil
	}
	if _, err := strconv.Atoi(n); err == nil {
		return os.Kill
	}
	// accept known names (validation only); delivery is always Kill
	switch n {
	case "HUP", "INT", "QUIT", "KILL", "TERM", "ABRT", "USR1", "USR2", "STOP", "CONT":
		return os.Kill
	}
	return nil
}

func killSignal() os.Signal { return os.Kill }

type procTree struct {
	foreground bool
	job        windows.Handle
	once       sync.Once
}

func createProcTree(c *exec.Cmd, foreground bool) *procTree {
	t := &procTree{foreground: foreground}
	if foreground {
		return t
	}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return t
	}
	var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE | windows.JOB_OBJECT_LIMIT_BREAKAWAY_OK
	_, err = windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	)
	if err != nil {
		_ = windows.CloseHandle(job)
		return t
	}
	t.job = job
	return t
}

func (t *procTree) started(c *exec.Cmd) {
	if t == nil || t.job == 0 || c == nil || c.Process == nil {
		return
	}
	var assignErr error
	err := c.Process.WithHandle(func(h uintptr) {
		assignErr = windows.AssignProcessToJobObject(t.job, windows.Handle(h))
	})
	if err != nil || assignErr != nil {
		if h, oerr := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(c.Process.Pid)); oerr == nil {
			_ = windows.AssignProcessToJobObject(t.job, h)
			_ = windows.CloseHandle(h)
		}
	}
}

func (t *procTree) signal(c *exec.Cmd, sig os.Signal) {
	if t != nil && t.job != 0 {
		_ = windows.TerminateJobObject(t.job, 1)
	}
	if c != nil && c.Process != nil {
		_ = c.Process.Kill()
	}
}

func (t *procTree) close() {
	if t == nil {
		return
	}
	t.once.Do(func() {
		if t.job != 0 {
			_ = windows.CloseHandle(t.job)
			t.job = 0
		}
	})
}

func setProcGroup(c *exec.Cmd) {}

func signalCmd(c *exec.Cmd, sig os.Signal, foreground bool) {
	if c != nil && c.Process != nil {
		_ = c.Process.Kill()
	}
}
