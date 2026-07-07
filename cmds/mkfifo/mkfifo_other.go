//go:build !unix

package mkfifocmd

import "errors"

func makeFIFO(_ string, _ uint32) error {
	return errors.New("not supported on this platform: FIFOs require Unix named-pipe support")
}
