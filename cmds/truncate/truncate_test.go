package truncatecmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

// runTool is the canonical test harness shape for cmds packages.
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

func sizeOf(t *testing.T, path string) int64 {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return fi.Size()
}

func TestTruncateSizes(t *testing.T) {
	cases := []struct {
		initial string // file content before; "" with create=false means missing
		size    string
		want    int64
	}{
		{"hello world", "5", 5},
		{"hello", "0", 0},
		{"", "100", 100},
		{"0123456789", "+5", 15},
		{"0123456789", "-4", 6},
		{"abc", "-100", 0}, // clamps at zero
		{"", "1K", 1024},
		{"", "2KB", 2000},
		{"", "1M", 1 << 20},
		{"", "1MiB", 1 << 20},
	}
	for _, c := range cases {
		dir := t.TempDir()
		f := filepath.Join(dir, "f")
		if err := os.WriteFile(f, []byte(c.initial), 0o644); err != nil {
			t.Fatal(err)
		}
		_, errb, code := runTool(t, dir, "-s", c.size, "f")
		if code != 0 {
			t.Errorf("-s %s: code=%d err=%q", c.size, code, errb)
			continue
		}
		if got := sizeOf(t, f); got != c.want {
			t.Errorf("-s %s on %q: size=%d want %d", c.size, c.initial, got, c.want)
		}
	}
}

func TestTruncateCreatesMissing(t *testing.T) {
	dir := t.TempDir()
	_, errb, code := runTool(t, dir, "-s", "5", "newf")
	if code != 0 || errb != "" {
		t.Fatalf("create: code=%d err=%q", code, errb)
	}
	if got := sizeOf(t, filepath.Join(dir, "newf")); got != 5 {
		t.Errorf("size=%d want 5", got)
	}
}

func TestTruncateNoCreate(t *testing.T) {
	dir := t.TempDir()
	_, errb, code := runTool(t, dir, "-c", "-s", "5", "missing")
	if code != 0 || errb != "" {
		t.Fatalf("-c missing: code=%d err=%q", code, errb)
	}
	if _, err := os.Stat(filepath.Join(dir, "missing")); !os.IsNotExist(err) {
		t.Error("-c created the file")
	}
}

func TestTruncateReferenceAndIoBlocks(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ref"), []byte("1234567"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "f"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "-r", "ref", "f")
	if code != 0 {
		t.Fatalf("truncate -r: code=%d err=%q", code, errb)
	}
	if got := sizeOf(t, filepath.Join(dir, "f")); got != 7 {
		t.Fatalf("reference size=%d want 7", got)
	}
	_, errb, code = runTool(t, dir, "-r", "ref", "-s", "+5", "f")
	if code != 0 {
		t.Fatalf("truncate -r -s +5: code=%d err=%q", code, errb)
	}
	if got := sizeOf(t, filepath.Join(dir, "f")); got != 12 {
		t.Fatalf("reference relative size=%d want 12", got)
	}
	_, errb, code = runTool(t, dir, "-o", "-s", "2", "f")
	if code != 0 {
		t.Fatalf("truncate -o: code=%d err=%q", code, errb)
	}
	if got := sizeOf(t, filepath.Join(dir, "f")); got != 1024 {
		t.Fatalf("io-block size=%d want 1024", got)
	}
}

func TestTruncateErrors(t *testing.T) {
	dir := t.TempDir()
	_, errb, code := runTool(t, dir, "f")
	if code != 2 || !strings.Contains(errb, "you must specify either '--size' or '--reference'") {
		t.Errorf("no -s: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, dir, "-s", "5")
	if code != 2 || !strings.Contains(errb, "missing file operand") {
		t.Errorf("no file: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, dir, "-s", "1X", "f")
	if code != 2 || !strings.Contains(errb, "invalid number: '1X'") {
		t.Errorf("bad suffix: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, dir, "-s", "abc", "f")
	if code != 2 || !strings.Contains(errb, "invalid number: 'abc'") {
		t.Errorf("no digits: code=%d err=%q", code, errb)
	}
	// GNU's <, >, /, % prefixes are deliberately unsupported.
	_, errb, code = runTool(t, dir, "-s", "<5", "f")
	if code != 2 || !strings.Contains(errb, "not supported") {
		t.Errorf("'<' prefix: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, dir, "--frobnicate", "f")
	if code != 2 || !strings.Contains(errb, "frobnicate") || !strings.Contains(errb, "pure-Go") {
		t.Errorf("unknown flag: code=%d err=%q", code, errb)
	}
}

func TestTruncateHelpAndVersion(t *testing.T) {
	out, _, code := runTool(t, t.TempDir(), "--help")
	if code != 0 || !strings.Contains(out, "Usage: truncate") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	out, _, code = runTool(t, t.TempDir(), "--version")
	if code != 0 || !strings.Contains(out, "truncate") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}
}
