//go:build !darwin && !linux

package uudecodecmd

import (
	"errors"
	"os"
)

func checkOverwrite(path string) error {
	_, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
