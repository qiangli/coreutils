//go:build unix

package renicecmd

import "golang.org/x/sys/unix"

const (
	whichProcess = unix.PRIO_PROCESS
	whichPGroup  = unix.PRIO_PGRP
	whichUser    = unix.PRIO_USER
)

// getPriority reads the current nice value. Getpriority legitimately returns
// -1 for a process at nice -1, so the error must be inspected rather than the
// value compared against -1.
func getPriority(which, id int) (int, error) { return unix.Getpriority(which, id) }

func setPriority(which, id, prio int) error { return unix.Setpriority(which, id, prio) }
