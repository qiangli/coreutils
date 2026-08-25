//go:build !darwin && !linux && !freebsd && !netbsd && !openbsd

package uudecodecmd

import (
	"errors"
	"fmt"
	"os"
)

func openWritableRegular(path string) (*os.File, error) {
	file, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, fmt.Errorf("output changed while opening and is not a regular file")
	}
	return file, nil
}

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
