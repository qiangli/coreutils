//go:build !linux && !darwin && !windows

package ttycmd

import (
	"fmt"
	"os"
)

// ttyName on unprobed platforms: never a terminal, so tty reports
// "not a tty" (exit 1) rather than guessing.
func ttyName(f *os.File) (string, bool, error) {
	st, err := f.Stat()
	if err != nil {
		return "", false, err
	}
	if st.Mode()&os.ModeCharDevice == 0 {
		return "", false, nil
	}
	return "", false, fmt.Errorf("terminal pathname lookup is unsupported on this platform")
}
