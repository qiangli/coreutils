//go:build darwin || freebsd || netbsd || openbsd

package sttycmd

import "golang.org/x/sys/unix"

func getTermios(fd int) (*unix.Termios, error) { return unix.IoctlGetTermios(fd, unix.TIOCGETA) }
func setTermios(fd int, t *unix.Termios) error { return unix.IoctlSetTermios(fd, unix.TIOCSETA, t) }

func baudToNative(baud uint64) (uint64, bool) {
	switch baud {
	case 0, 50, 75, 110, 134, 150, 200, 300, 600, 1200, 1800, 2400, 4800, 9600, 19200, 38400:
		return baud, true
	default:
		return 0, false
	}
}

func nativeToBaud(native uint64) uint64 { return native }

func nativeSpeeds(t *unix.Termios) (uint64, uint64) {
	return uint64(t.Ispeed), uint64(t.Ospeed)
}
