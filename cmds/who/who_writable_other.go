//go:build !windows

package whocmd

import "os"

// ttyMessageStatus reports the terminal's message/writable status ('+'/'-')
// from the Unix group-write permission bit of the tty device.
func ttyMessageStatus(fi os.FileInfo) byte {
	if fi.Mode()&0o020 != 0 {
		return '+'
	}
	return '-'
}
