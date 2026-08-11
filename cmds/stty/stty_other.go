//go:build !linux && !darwin && !freebsd && !netbsd && !openbsd && !windows

package sttycmd

import "fmt"

func applyMode(fd int, mode string) error {
	return fmt.Errorf("%s is not supported on this platform", mode)
}

func applyValue(fd int, name string, value uint8) error {
	return fmt.Errorf("%s is not supported on this platform", name)
}

func applyWindowSize(fd int, rows, cols int) error {
	return fmt.Errorf("window size is not supported on this platform")
}
