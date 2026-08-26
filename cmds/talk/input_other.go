//go:build !unix

package talkcmd

import "io"

func newTerminalInput(r io.Reader) (terminalInput, error) { return newAsyncInput(r) }
