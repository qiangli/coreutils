//go:build !linux && !darwin && !windows

package ttycmd

import "os"

// ttyName on unprobed platforms: never a terminal, so tty reports
// "not a tty" (exit 1) rather than guessing.
func ttyName(_ *os.File) (string, bool) {
	return "", false
}
