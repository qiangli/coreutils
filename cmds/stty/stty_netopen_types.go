//go:build netbsd || openbsd

package sttycmd

import "golang.org/x/sys/unix"

type termiosUint = uint32

const posixVDisable = byte(0xff)

func stateToTermios(t *unix.Termios, state *terminalState) {
	t.Iflag, t.Oflag, t.Cflag, t.Lflag = uint32(state.Iflag), uint32(state.Oflag), uint32(state.Cflag), uint32(state.Lflag)
	t.Ispeed, t.Ospeed = int32(state.Ispeed), int32(state.Ospeed)
}

func setNativeSpeed(t *unix.Termios, which string, speed uint64) {
	if which == "speed" || which == "ospeed" {
		t.Ospeed = int32(speed)
	}
	if which == "speed" || which == "ispeed" {
		t.Ispeed = int32(speed)
	}
}
