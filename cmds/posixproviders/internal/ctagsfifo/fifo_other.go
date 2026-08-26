//go:build !unix

package ctagsfifo

import (
	"context"
	"fmt"
	"io"
	"os"
)

func openFIFO(context.Context, string, os.FileInfo, bool) (*os.File, error) {
	return nil, fmt.Errorf("FIFO output is unsupported on this platform")
}

func openPrivateOutput(path string, original os.FileInfo) (*os.File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	current, statErr := f.Stat()
	if statErr != nil || !current.Mode().IsRegular() || !os.SameFile(original, current) {
		_ = f.Close()
		if statErr != nil {
			return nil, statErr
		}
		return nil, fmt.Errorf("private output changed during ctags execution")
	}
	return f, nil
}

func copyPrivateOutput(ctx context.Context, out, in *os.File) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	_, err := io.Copy(out, in)
	return err
}
