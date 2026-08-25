//go:build windows

package mkdircmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func TestMkdirVirtualUmaskDoesNotApproximatePOSIXModesOnWindows(t *testing.T) {
	dir := t.TempDir()
	var chmodCalled bool
	m := &maker{
		rc: &tool.RunContext{
			Ctx: context.Background(), Dir: dir, Umask: 0o777, UmaskSet: true,
			Stdio: tool.Stdio{Err: &bytes.Buffer{}},
		},
		parents: true,
		deps:    defaultMkdirDeps,
	}
	m.deps.chmod = func(string, os.FileMode) error {
		chmodCalled = true
		return nil
	}
	m.make(filepath.Join("a", "b"))
	if m.failed {
		t.Fatal("mkdir -p failed")
	}
	if chmodCalled {
		t.Fatal("Windows virtual umask must not be approximated through os.Chmod")
	}
	if fi, err := os.Stat(filepath.Join(dir, "a", "b")); err != nil || !fi.IsDir() {
		t.Fatalf("directory not created: %v", err)
	}
}
