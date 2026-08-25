//go:build linux

package renicecmd

import (
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// The raw getpriority(2) syscall cannot report a negative nice value (the
// raw-syscall ABI reserves negative returns for errno), so the Linux kernel
// returns 20-nice and libc converts back. x/sys unix.Getpriority on Linux
// is a raw wrapper with no such conversion; without this, every read is
// mirrored around 20 (nice 0 reads as 20, nice 19 as 1) and the computed
// target nice value is garbage. Setpriority takes the true nice value on
// every platform and needs no counterpart.
func niceFromGetpriority(raw int) int { return 20 - raw }

// members expands a collective selector to a stable, ordered snapshot of
// process IDs. Relative renicing must read and adjust each process separately:
// Linux's collective priority syscalls otherwise replace heterogeneous member
// values with (minimum member nice)+increment.
func (hostScheduler) members(which, id int) ([]int, error) {
	switch which {
	case whichPGroup:
		if id == 0 {
			id = unix.Getpgrp()
		}
		return linuxMembers(os.DirFS("/proc"), which, uint32(id))
	case whichUser:
		return linuxMembers(os.DirFS("/proc"), which, uint32(id))
	default:
		return nil, fmt.Errorf("unsupported priority selector %d", which)
	}
}

func linuxMembers(proc fs.FS, which int, id uint32) ([]int, error) {
	entries, err := fs.ReadDir(proc, ".")
	if err != nil {
		return nil, fmt.Errorf("cannot enumerate /proc: %w", err)
	}
	var pids []int
	for _, entry := range entries {
		pid64, parseErr := strconv.ParseUint(entry.Name(), 10, 31)
		if parseErr != nil {
			continue
		}
		pid := int(pid64)
		matches, matchErr := linuxProcessMatches(proc, entry.Name(), which, id)
		if matchErr != nil {
			if os.IsNotExist(matchErr) {
				// The process exited during the snapshot and no longer needs an
				// adjustment.
				continue
			}
			return nil, fmt.Errorf("cannot inspect /proc/%d: %w", pid, matchErr)
		}
		if matches {
			pids = append(pids, pid)
		}
	}
	sort.Ints(pids)
	if len(pids) == 0 {
		return nil, unix.ESRCH
	}
	return pids, nil
}

func linuxProcessMatches(proc fs.FS, name string, which int, id uint32) (bool, error) {
	switch which {
	case whichPGroup:
		data, err := fs.ReadFile(proc, name+"/stat")
		if err != nil {
			return false, err
		}
		pgrp, err := linuxStatPGroup(data)
		return pgrp == id, err
	case whichUser:
		data, err := fs.ReadFile(proc, name+"/status")
		if err != nil {
			return false, err
		}
		saved, err := linuxStatusSavedUID(data)
		return saved == id, err
	default:
		return false, fmt.Errorf("unsupported priority selector %d", which)
	}
}

// /proc/PID/stat's comm field is parenthesized but may itself contain spaces
// and ')' characters. The delimiter is therefore the last ')'; the process
// group is field 5, or index 2 in the fields following comm.
func linuxStatPGroup(data []byte) (uint32, error) {
	end := strings.LastIndexByte(string(data), ')')
	if end < 0 {
		return 0, fmt.Errorf("malformed stat record")
	}
	fields := strings.Fields(string(data[end+1:]))
	if len(fields) < 3 {
		return 0, fmt.Errorf("malformed stat record")
	}
	n, err := strconv.ParseUint(fields[2], 10, 31)
	if err != nil {
		return 0, fmt.Errorf("malformed process group ID %q", fields[2])
	}
	return uint32(n), nil
}

func linuxStatusSavedUID(data []byte) (uint32, error) {
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 5 && fields[0] == "Uid:" {
			n, err := strconv.ParseUint(fields[3], 10, 32)
			if err != nil {
				return 0, fmt.Errorf("malformed saved user ID %q", fields[3])
			}
			return uint32(n), nil
		}
	}
	return 0, fmt.Errorf("status record has no Uid field")
}
