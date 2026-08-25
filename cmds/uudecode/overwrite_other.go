//go:build !darwin && !linux && !freebsd && !netbsd && !openbsd

package uudecodecmd

import (
	"errors"
	"fmt"
	"os"
)

func checkOverwrite(path string) error {
	_, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	// Do not silently overwrite an existing file on platforms where this
	// build cannot ask the kernel about effective-ID write access without
	// opening the target (which can block on FIFOs or affect devices).
	return fmt.Errorf("cannot safely verify effective write access on this platform")
}
