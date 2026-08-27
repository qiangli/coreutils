package cpcmd

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

type cpFailReader struct {
	data []byte
	err  error
}

func (r *cpFailReader) Read(p []byte) (int, error) {
	if len(r.data) > 0 {
		n := copy(p, r.data)
		r.data = r.data[n:]
		return n, nil
	}
	return 0, r.err
}

type cpShortWriter struct{ bytes.Buffer }

func (w *cpShortWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return w.Buffer.Write(p[:len(p)-1])
}

func TestCopyRegularDataClassifiesReadAndWriteErrors(t *testing.T) {
	readFailure := errors.New("source read failure")
	var dst bytes.Buffer
	rerr, werr := copyRegularData(&dst, &cpFailReader{data: []byte("partial"), err: readFailure})
	if !errors.Is(rerr, readFailure) || werr != nil || dst.String() != "partial" {
		t.Fatalf("read failure: read=%v write=%v output=%q", rerr, werr, dst.String())
	}

	short := new(cpShortWriter)
	rerr, werr = copyRegularData(short, bytes.NewBufferString("payload"))
	if rerr != nil || !errors.Is(werr, io.ErrShortWrite) {
		t.Fatalf("short write: read=%v write=%v", rerr, werr)
	}
}
