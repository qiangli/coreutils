//go:build windows

package whocmd

import "os"

// ttyMessageStatus reports the terminal's message/writable status.
//
// The writable status is a Unix concept derived from the tty device's
// group-write permission bit. Windows has no such permission model: Go
// synthesizes os.FileMode bits solely from the read-only attribute
// (a writable file is always 0o666), so the group-write bit carries no
// real meaning and cannot distinguish writable from non-writable. The
// honest answer is therefore '?' (unknown) rather than a fabricated
// '+'/'-'.
func ttyMessageStatus(_ os.FileInfo) byte {
	return '?'
}
