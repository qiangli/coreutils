//go:build unix

package writecmd

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
	"golang.org/x/term"

	"github.com/qiangli/coreutils/tool"
)

// defaultSenderTTY names the sender's own terminal, as the banner reports it
// and as the self-write guard compares it. It returns "" when the caller has
// no terminal at all (a pipeline, a cron job, an agent harness), which is not
// an error: write is still useful from a script.
//
// The streams are examined in the order in/out/err so that a caller who
// redirected only one of them is still identified by another. rc's streams win
// over the process's: an embedding shell routinely hands a tool something other
// than the process's own file descriptors, and the process's would name the
// wrong device.
func defaultSenderTTY(rc *tool.RunContext) string {
	var candidates []*os.File
	if rc != nil {
		for _, s := range []any{rc.In, rc.Out, rc.Err} {
			if f, ok := s.(*os.File); ok {
				candidates = append(candidates, f)
			}
		}
	} else {
		candidates = append(candidates, os.Stdin, os.Stdout, os.Stderr)
	}

	for _, f := range candidates {
		if f == nil {
			continue
		}
		fi, err := f.Stat()
		if err != nil || fi.Mode()&os.ModeCharDevice == 0 || !term.IsTerminal(int(f.Fd())) {
			continue
		}
		if name := deviceNameOf(fi); name != "" {
			return name
		}
	}
	return ""
}

// deviceNameOf maps an open character device onto its name under devDir by
// matching device numbers. Go exposes no portable ttyname(3), and assuming
// /dev/tty would name the controlling terminal even when the caller's streams
// point somewhere else entirely. Same approach as cmds/mesg's ttyPath.
func deviceNameOf(want fs.FileInfo) string {
	wantDev, ok := rdevOf(want)
	if !ok {
		return ""
	}
	for _, sub := range []string{"pts", ""} {
		dir := filepath.Join(devDir, sub)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			info, err := e.Info()
			if err != nil || info.Mode()&os.ModeCharDevice == 0 {
				continue
			}
			if dev, ok := rdevOf(info); ok && dev == wantDev {
				return filepath.Join(sub, e.Name())
			}
		}
	}
	return ""
}

func rdevOf(fi fs.FileInfo) (uint64, bool) {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return uint64(st.Rdev), true
}

func defaultOpenSenderControlTTY(rc *tool.RunContext, expected string) (io.WriteCloser, error) {
	path := ttyDevice(expected)
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return nil, err
	}
	if !terminalFile(f) {
		_ = f.Close()
		return nil, errors.New("not a terminal")
	}
	fi, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	if name := deviceNameOf(fi); normalizeTTY(name) != normalizeTTY(expected) || !senderStreamMatches(rc, expected, fi) {
		_ = f.Close()
		return nil, errors.New("terminal is not the sender's authenticated stream")
	}
	return f, nil
}

func senderStreamMatches(rc *tool.RunContext, expected string, opened fs.FileInfo) bool {
	wantRdev, ok := rdevOf(opened)
	if !ok {
		return false
	}
	var candidates []*os.File
	if rc != nil {
		for _, stream := range []any{rc.In, rc.Out, rc.Err} {
			if f, ok := stream.(*os.File); ok {
				candidates = append(candidates, f)
			}
		}
	} else {
		candidates = append(candidates, os.Stdin, os.Stdout, os.Stderr)
	}
	for _, candidate := range candidates {
		if candidate == nil || !term.IsTerminal(int(candidate.Fd())) {
			continue
		}
		fi, err := candidate.Stat()
		if err != nil {
			continue
		}
		rdev, ok := rdevOf(fi)
		if ok && rdev == wantRdev && normalizeTTY(deviceNameOf(fi)) == normalizeTTY(expected) {
			return true
		}
	}
	return false
}

func terminalFile(f *os.File) bool { return f != nil && term.IsTerminal(int(f.Fd())) }

func defaultTerminalDevice(path string) bool {
	f, err := os.OpenFile(path, os.O_WRONLY|unix.O_NOCTTY|unix.O_NONBLOCK, 0)
	if err != nil {
		return false
	}
	defer f.Close()
	return terminalFile(f)
}

func defaultSessionActive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := unix.Kill(pid, 0)
	return err == nil || errors.Is(err, unix.EPERM)
}

func defaultSessionOwnsTerminal(pid int, path string) bool {
	if !defaultSessionActive(pid) {
		return false
	}
	if runtime.GOOS != "linux" {
		return false
	}
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return false
	}
	end := strings.LastIndexByte(string(data), ')')
	if end < 0 {
		return false
	}
	fields := strings.Fields(string(data[end+1:]))
	if len(fields) < 5 {
		return false
	}
	ttyNR, err := strconv.ParseInt(fields[4], 10, 64)
	if err != nil || ttyNR <= 0 {
		return false
	}
	fi, err := os.Stat(path)
	if err != nil {
		return false
	}
	rdev, ok := rdevOf(fi)
	return ok && uint64(ttyNR) == rdev
}

func defaultGetVEOL(in io.Reader) byte {
	if f, ok := in.(*os.File); ok {
		if term, err := getTermios(int(f.Fd())); err == nil {
			veol := term.Cc[unix.VEOL]
			if veol != 0 && veol != 0xff {
				return veol
			}
		}
	}
	return 0
}

func duplicateInputFile(in *os.File) (*os.File, error) {
	fd, err := unix.Dup(int(in.Fd()))
	if err != nil {
		return nil, err
	}
	dup := os.NewFile(uintptr(fd), in.Name())
	if dup == nil {
		_ = unix.Close(fd)
		return nil, errors.New("cannot own duplicated input descriptor")
	}
	return dup, nil
}

func waitInputReadable(in *os.File, timeout time.Duration) (bool, error) {
	fds := []unix.PollFd{{Fd: int32(in.Fd()), Events: unix.POLLIN | unix.POLLHUP}}
	n, err := unix.Poll(fds, int(timeout.Milliseconds()))
	if err == unix.EINTR {
		return false, nil
	}
	return n > 0, err
}
