//go:build !unix

package cpcmd

import (
	"errors"
	"time"
)

func preserveLinkTimes(string, time.Time, time.Time) error {
	return errors.New("symbolic link timestamps unsupported on this platform")
}
