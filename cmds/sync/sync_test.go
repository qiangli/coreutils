package synccmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func runIn(t *testing.T, dir string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
	}
	code = cmd.Run(rc, args)
	return out.String(), errb.String(), code
}

func TestSyncFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows FlushFileBuffers needs a writable handle; sync's read-only fsync path is a tracked windows-port item")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f1"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "f2"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	// relative operands resolve against rc.Dir, not the process cwd
	out, errb, code := runIn(t, dir, "f1", "f2")
	if code != 0 || out != "" || errb != "" {
		t.Errorf("sync f1 f2 = (%q, %q, %d), want clean success", out, errb, code)
	}
}

func TestSyncMissingFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ok"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runIn(t, dir, "gone", "ok")
	if code != 1 || !strings.Contains(errb, "error opening 'gone'") {
		t.Errorf("sync gone ok = (code=%d, err=%q), want code 1 + open error", code, errb)
	}
}

func TestSyncBare(t *testing.T) {
	_, errb, code := runIn(t, t.TempDir())
	if runtime.GOOS == "windows" {
		if code != 1 || !strings.Contains(errb, "not supported on windows") {
			t.Errorf("bare sync on windows = (code=%d, err=%q), want clear not-supported error", code, errb)
		}
		return
	}
	if code != 0 || errb != "" {
		t.Errorf("bare sync = (code=%d, err=%q), want success", code, errb)
	}
}

func TestSyncErrors(t *testing.T) {
	_, errb, code := runIn(t, t.TempDir(), "--data", "x")
	if code != 2 || !strings.Contains(errb, "data") || !strings.Contains(errb, "pure-Go") {
		t.Errorf("unimplemented flag: code=%d err=%q", code, errb)
	}
	_, errb, code = runIn(t, t.TempDir(), "--frobnicate")
	if code != 2 || !strings.Contains(errb, "frobnicate") {
		t.Errorf("unknown flag: code=%d err=%q", code, errb)
	}
}

func TestSyncHelpAndVersion(t *testing.T) {
	out, _, code := runIn(t, t.TempDir(), "--help")
	if code != 0 || !strings.Contains(out, "Usage: sync") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	out, _, code = runIn(t, t.TempDir(), "--version")
	if code != 0 || !strings.Contains(out, "sync") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}
}
