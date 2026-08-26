// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

//go:build unix

package posixproviderscmd

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// TestCtagsDefaultFIFOThroughRegisteredProvider covers the full product route:
// registry -> provenance resolver -> external provider -> FIFO semantic adapter.
// The synthetic provider intentionally uses ordinary shell redirection; unlike
// Universal Ctags, it cannot deadlock before producing the private output.
func TestCtagsDefaultFIFOThroughRegisteredProvider(t *testing.T) {
	root := t.TempDir()
	provision(t, root, "ctags", `#!/bin/sh
out=tags
while [ "$#" -gt 0 ]; do
  case $1 in
    -f) out=$2; shift 2 ;;
    -f*) out=${1#-f}; shift ;;
    *) shift ;;
  esac
done
printf 'symbol\tsource.c\t1\n' >"$out"
`)
	rc, out, errb := newRC(t, root)
	fifo := filepath.Join(rc.Dir, "tags")
	if err := unix.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	read := make(chan struct {
		data []byte
		err  error
	}, 1)
	go func() {
		data, err := os.ReadFile(fifo)
		read <- struct {
			data []byte
			err  error
		}{data, err}
	}()

	code, _, stderr := run(t, "ctags", rc, out, errb, "source.c")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	select {
	case result := <-read:
		if result.err != nil || string(result.data) != "symbol\tsource.c\t1\n" {
			t.Fatalf("FIFO read = %q, %v", result.data, result.err)
		}
	case <-time.After(time.Second):
		t.Fatal("registered ctags provider did not write the default FIFO")
	}
	info, err := os.Stat(fifo)
	if err != nil || info.Mode()&os.ModeNamedPipe == 0 {
		if info == nil {
			t.Fatalf("output FIFO missing: %v", err)
		}
		t.Fatalf("output FIFO replaced: mode=%v err=%v", info.Mode(), err)
	}
}

// TestCtagsPublicProbeFIFOShapeThroughRegisteredProvider pins the exact
// suite-free readiness invocation: --options=NONE disables ambient Universal
// Ctags configuration, while the following -f FIFO must still enter the Go
// adapter rather than being mistaken for an opaque long-option argument.
func TestCtagsPublicProbeFIFOShapeThroughRegisteredProvider(t *testing.T) {
	root := t.TempDir()
	provision(t, root, "ctags", `#!/bin/sh
out=tags
options_none=no
while [ "$#" -gt 0 ]; do
  case $1 in
    --options=NONE) options_none=yes; shift ;;
    -f) out=$2; shift 2 ;;
    -f*) out=${1#-f}; shift ;;
    *) shift ;;
  esac
done
[ "$options_none" = yes ] || exit 9
printf 'symbol\tsource.c\t1\n' >"$out"
`)
	rc, out, errb := newRC(t, root)
	fifo := filepath.Join(rc.Dir, "probe-tags")
	if err := unix.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	read := make(chan struct {
		data []byte
		err  error
	}, 1)
	go func() {
		data, err := os.ReadFile(fifo)
		read <- struct {
			data []byte
			err  error
		}{data, err}
	}()

	code, _, stderr := run(t, "ctags", rc, out, errb,
		"--options=NONE", "-f", fifo, "source.c")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	select {
	case result := <-read:
		if result.err != nil || string(result.data) != "symbol\tsource.c\t1\n" {
			t.Fatalf("FIFO read = %q, %v", result.data, result.err)
		}
	case <-time.After(time.Second):
		t.Fatal("registered ctags provider did not satisfy the public FIFO probe shape")
	}
}
