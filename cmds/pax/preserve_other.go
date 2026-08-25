//go:build !unix

package paxcmd

import "errors"

func defaultChownExtracted(string, int, int, bool) error {
	return errors.New("preserving ownership is not supported on this platform")
}
