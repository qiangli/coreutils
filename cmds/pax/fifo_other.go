//go:build !unix

package paxcmd

import "errors"

func fifoSupported() bool { return false }

func makeFIFO(_ string, _ uint32) error {
	return errors.New("FIFO extraction is not supported on this platform")
}
