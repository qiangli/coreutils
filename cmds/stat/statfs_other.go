//go:build !linux && !darwin

package statcmd

import (
	"errors"
)

func statfsFile(path string) (*fsStat, error) {
	return nil, errors.New("not supported on this platform")
}
