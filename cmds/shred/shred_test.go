package shredcmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func runTool(t *testing.T, dir string, args ...string) (stdout, stderr string, code int) {
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

func TestShredOverwritesAndZeroes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file")
	if err := os.WriteFile(path, []byte("secret data"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, errb, code := runTool(t, dir, "-n", "1", "-z", "file")
	if code != 0 || out != "" || errb != "" {
		t.Fatalf("shred: code=%d out=%q err=%q", code, out, errb)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, bytes.Repeat([]byte{0}, len(got))) {
		t.Fatalf("file was not zeroed: %x", got)
	}
}

func TestShredRemove(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file")
	if err := os.WriteFile(path, []byte("secret data"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, errb, code := runTool(t, dir, "-n", "0", "-u", "file")
	if code != 0 {
		t.Fatalf("shred -u: code=%d err=%q", code, errb)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("file still exists or unexpected stat error: %v", err)
	}
}

func TestShredErrors(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}

	_, errb, code := runTool(t, dir)
	if code != 2 || !strings.Contains(errb, "missing file operand") {
		t.Fatalf("missing operand: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, dir, "subdir")
	if code != 1 || !strings.Contains(errb, "not a regular file") {
		t.Fatalf("directory operand: code=%d err=%q", code, errb)
	}
}

func TestShredNewFlags(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file")
	if err := os.WriteFile(path, []byte("secret data text"), 0o644); err != nil {
		t.Fatal(err)
	}

	// --size / -s: shred only first N bytes (rounded to block size without --exact)
	_, errb, code := runTool(t, dir, "-n", "1", "-z", "-s", "6", "file")
	if code != 0 {
		t.Fatalf("shred -s 6: code=%d err=%q", code, errb)
	}
	got, _ := os.ReadFile(path)
	if !bytes.Equal(got, bytes.Repeat([]byte{0}, len(got))) {
		t.Fatalf("shred -s 6: file not zeroed: %x", got)
	}

	// --exact
	path2 := filepath.Join(dir, "file2")
	if err := os.WriteFile(path2, []byte("secret data text"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, code = runTool(t, dir, "-n", "1", "-z", "--exact", "--size", "6", "file2")
	if code != 0 {
		t.Fatalf("shred --exact: code=%d", code)
	}
	got2, _ := os.ReadFile(path2)
	if string(got2[:6]) != "\x00\x00\x00\x00\x00\x00" {
		t.Fatalf("shred --exact: first 6 bytes not zeroed: %x", got2[:6])
	}

	// --random-source with /dev/zero: predictable zeros
	path3 := filepath.Join(dir, "file3")
	if err := os.WriteFile(path3, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, code = runTool(t, dir, "-n", "1", "--random-source", "/dev/zero", "file3")
	if code != 0 {
		t.Fatalf("shred --random-source: code=%d", code)
	}

	// -x (no-xdev) accepted
	path4 := filepath.Join(dir, "file4")
	if err := os.WriteFile(path4, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, code = runTool(t, dir, "-n", "1", "-x", "file4")
	if code != 0 {
		t.Fatalf("shred -x: code=%d", code)
	}
}
