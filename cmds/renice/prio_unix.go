//go:build unix

package renicecmd

import "golang.org/x/sys/unix"

const (
	whichProcess = unix.PRIO_PROCESS
	whichPGroup  = unix.PRIO_PGRP
	whichUser    = unix.PRIO_USER
)

// hostScheduler is the real kernel seam.
//
// Getpriority legitimately reports negative nice values, so the error must
// be inspected rather than the value compared against -1 (x/sys does this
// correctly on every unix target).
type hostScheduler struct{}

func (hostScheduler) get(which, id int) (int, error) {
	raw, err := unix.Getpriority(which, id)
	if err != nil {
		return 0, err
	}
	return niceFromGetpriority(raw), nil
}

func (hostScheduler) set(which, id, prio int) error {
	return unix.Setpriority(which, id, prio)
}

func newHostScheduler() (scheduler, error) { return hostScheduler{}, nil }
