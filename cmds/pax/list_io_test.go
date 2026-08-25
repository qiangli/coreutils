package paxcmd

import (
	"archive/tar"
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func listArchiveBytes(t *testing.T) []byte {
	t.Helper()
	var archive bytes.Buffer
	tw := tar.NewWriter(&archive)
	if err := tw.WriteHeader(&tar.Header{Name: "member", Mode: 0o644, Size: 1, Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return archive.Bytes()
}

type listArchiveReader struct {
	*bytes.Reader
	closeErr error
}

func (r *listArchiveReader) Close() error { return r.closeErr }

type paxListFailWriter struct{ err error }

func (w paxListFailWriter) Write([]byte) (int, error) { return 0, w.err }

func TestPaxListModePropagatesOutputErrors(t *testing.T) {
	data := listArchiveBytes(t)
	for _, verbose := range []bool{false, true} {
		t.Run(map[bool]string{false: "plain", true: "verbose"}[verbose], func(t *testing.T) {
			var errb bytes.Buffer
			rc := &tool.RunContext{Stdio: tool.Stdio{Out: paxListFailWriter{errors.New("injected output failure")}, Err: &errb}}
			opener := func(*tool.RunContext, *options) (io.ReadCloser, error) {
				return &listArchiveReader{Reader: bytes.NewReader(data)}, nil
			}
			if code := listModeWithOpener(rc, &options{verbose: verbose}, nil, opener); code != 1 {
				t.Fatalf("exit %d, want 1", code)
			}
			if !strings.Contains(errb.String(), "write error") || !strings.Contains(errb.String(), "injected output failure") {
				t.Fatalf("stderr %q lacks output failure", errb.String())
			}
		})
	}
}

func TestPaxListModePropagatesArchiveCloseError(t *testing.T) {
	data := listArchiveBytes(t)
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Stdio: tool.Stdio{Out: &out, Err: &errb}}
	opener := func(*tool.RunContext, *options) (io.ReadCloser, error) {
		return &listArchiveReader{Reader: bytes.NewReader(data), closeErr: errors.New("injected close failure")}, nil
	}
	if code := listModeWithOpener(rc, &options{}, nil, opener); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if out.Len() != 0 {
		t.Fatalf("archive close failure produced listing %q", out.String())
	}
	if !strings.Contains(errb.String(), "close archive") || !strings.Contains(errb.String(), "injected close failure") {
		t.Fatalf("stderr %q lacks close failure", errb.String())
	}
}
