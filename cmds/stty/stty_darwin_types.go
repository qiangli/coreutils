//go:build darwin

package sttycmd

import "golang.org/x/sys/unix"

type termiosUint = uint64

const posixVDisable = byte(0xff)

func stateToTermios(t *unix.Termios, state *terminalState) {
	t.Iflag, t.Oflag, t.Cflag, t.Lflag = state.Iflag, state.Oflag, state.Cflag, state.Lflag
	t.Ispeed, t.Ospeed = state.Ispeed, state.Ospeed
}

func setNativeSpeed(t *unix.Termios, which string, speed uint64) {
	if which == "speed" || which == "ospeed" {
		t.Ospeed = speed
	}
	if which == "speed" || which == "ispeed" {
		t.Ispeed = speed
	}
}
