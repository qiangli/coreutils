//go:build linux

package sttycmd

import "golang.org/x/sys/unix"

type termiosUint = uint32

const posixVDisable = byte(0)

func stateToTermios(t *unix.Termios, state *terminalState) {
	t.Iflag = uint32(state.Iflag)
	t.Oflag = uint32(state.Oflag)
	t.Cflag = uint32(state.Cflag)
	t.Lflag = uint32(state.Lflag)
	setNativeSpeed(t, "ospeed", state.Ospeed)
	setNativeSpeed(t, "ispeed", state.Ispeed)
}

func nativeSpeeds(t *unix.Termios) (uint64, uint64) {
	ospeed := uint64(t.Cflag) & uint64(unix.CBAUD)
	ispeed := (uint64(t.Cflag) & uint64(unix.CIBAUD)) >> 16
	if ispeed == 0 {
		ispeed = ospeed
	}
	return ispeed, ospeed
}

func getTermios(fd int) (*unix.Termios, error) { return unix.IoctlGetTermios(fd, unix.TCGETS) }
func setTermios(fd int, t *unix.Termios) error { return unix.IoctlSetTermios(fd, unix.TCSETS, t) }

func baudToNative(baud uint64) (uint64, bool) {
	for _, pair := range linuxBaudRates {
		if pair.baud == baud {
			return pair.native, true
		}
	}
	return 0, false
}

func nativeToBaud(native uint64) uint64 {
	for _, pair := range linuxBaudRates {
		if pair.native == native {
			return pair.baud
		}
	}
	return native
}

func setNativeSpeed(t *unix.Termios, which string, code uint64) {
	native := termiosUint(code)
	if which == "speed" {
		t.Cflag = (t.Cflag &^ termiosUint(unix.CBAUD|unix.CIBAUD)) | native
		return
	}
	if which == "ospeed" {
		// A zero CIBAUD field means "use the output speed".  Materialize
		// that logical input speed before changing CBAUD, otherwise changing
		// only ospeed silently changes ispeed as well.
		input := t.Cflag & termiosUint(unix.CIBAUD)
		if input == 0 {
			input = (t.Cflag & termiosUint(unix.CBAUD)) << 16
		}
		t.Cflag = (t.Cflag &^ termiosUint(unix.CBAUD|unix.CIBAUD)) | native
		if input != native<<16 {
			t.Cflag |= input
		}
	}
	if which == "ispeed" {
		output := t.Cflag & termiosUint(unix.CBAUD)
		t.Cflag &^= termiosUint(unix.CIBAUD)
		if native != 0 && native != output {
			t.Cflag |= native << 16
		}
	}
}

type baudRate struct{ baud, native uint64 }

var linuxBaudRates = []baudRate{
	{0, unix.B0}, {50, unix.B50}, {75, unix.B75}, {110, unix.B110},
	{134, unix.B134}, {150, unix.B150}, {200, unix.B200}, {300, unix.B300},
	{600, unix.B600}, {1200, unix.B1200}, {1800, unix.B1800}, {2400, unix.B2400},
	{4800, unix.B4800}, {9600, unix.B9600}, {19200, unix.B19200}, {38400, unix.B38400},
}
