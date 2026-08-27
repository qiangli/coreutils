//go:build !linux && !darwin && !dragonfly && !freebsd && !netbsd && !openbsd

package talkcmd

import "errors"

type controlChars struct{ erase, kill, intr, eof byte }

func enterTerminalMode(int) (controlChars, func() error, error) {
	return controlChars{}, nil, errors.New("character-at-a-time terminal mode is unsupported on this platform")
}
