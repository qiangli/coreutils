package mkfifocmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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

func TestMkfifoCreatesFIFO(t *testing.T) {
	dir := t.TempDir()
	out, errb, code := runTool(t, dir, "pipe")
	if runtime.GOOS == "windows" {
		if code != 1 || !strings.Contains(strings.ToLower(errb), "not supported") {
			t.Fatalf("windows mkfifo: code=%d err=%q", code, errb)
		}
		return
	}
	if code != 0 || out != "" || errb != "" {
		t.Fatalf("mkfifo pipe: code=%d out=%q err=%q", code, out, errb)
	}
	fi, err := os.Lstat(filepath.Join(dir, "pipe"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeNamedPipe == 0 {
		t.Fatalf("mode=%v, want named pipe", fi.Mode())
	}
}

func TestMkfifoMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("native FIFO creation is unsupported on windows")
	}
	dir := t.TempDir()
	_, errb, code := runTool(t, dir, "-m", "600", "pipe")
	if code != 0 {
		t.Fatalf("mkfifo -m: code=%d err=%q", code, errb)
	}
	fi, err := os.Lstat(filepath.Join(dir, "pipe"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode=%o, want 600", fi.Mode().Perm())
	}
}

func TestMkfifoSymbolicMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("native FIFO creation is unsupported on windows")
	}
	tests := []struct {
		mode   string
		want   os.FileMode
		setuid bool
		setgid bool
		sticky bool
	}{
		{mode: "u=rw,go=", want: 0o600},
		{mode: "a=rw,u+x", want: 0o766},
		{mode: "+x", want: 0o777},
		{mode: "a+X", want: 0o666},
		{mode: "u=rw,g=u,o=g", want: 0o666},
		{mode: "u+=", want: 0o066},
		{mode: "=", want: 0},
		{mode: "u+s", want: 0o666, setuid: true},
		{mode: "g+s", want: 0o666, setgid: true},
		{mode: "a=t", want: 0, sticky: true},
	}
	for i, tc := range tests {
		name := fmt.Sprintf("pipe%d", i)
		dir := t.TempDir()
		_, errb, code := runTool(t, dir, "-m", tc.mode, name)
		if code != 0 {
			t.Fatalf("mkfifo -m %q: code=%d err=%q", tc.mode, code, errb)
		}
		fi, err := os.Lstat(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if got := fi.Mode().Perm(); got != tc.want {
			t.Errorf("mkfifo -m %q mode=%03o, want %03o", tc.mode, got, tc.want)
		}
		if got := fi.Mode()&os.ModeSetuid != 0; got != tc.setuid {
			t.Errorf("mkfifo -m %q setuid=%v, want %v", tc.mode, got, tc.setuid)
		}
		if got := fi.Mode()&os.ModeSetgid != 0; got != tc.setgid {
			t.Errorf("mkfifo -m %q setgid=%v, want %v", tc.mode, got, tc.setgid)
		}
		if got := fi.Mode()&os.ModeSticky != 0; got != tc.sticky {
			t.Errorf("mkfifo -m %q sticky=%v, want %v", tc.mode, got, tc.sticky)
		}
	}
}

func TestMkfifoContextNoop(t *testing.T) {
	dir := t.TempDir()
	out, errb, code := runTool(t, dir, "--context", "system_u:object_r:mkfifo_t:s0", "pipe")
	if runtime.GOOS == "windows" {
		if code != 1 || !strings.Contains(errb, "not supported") {
			t.Fatalf("windows mkfifo --context: code=%d err=%q", code, errb)
		}
		return
	}
	if code != 0 || out != "" {
		t.Fatalf("mkfifo --context: code=%d out=%q err=%q", code, out, errb)
	}
	if _, err := os.Lstat(filepath.Join(dir, "pipe")); err != nil {
		t.Fatalf("context no-op did not create fifo: %v", err)
	}
	// The -Z short form must behave identically to --context.
	dir2 := t.TempDir()
	_, errb, code = runTool(t, dir2, "-Z", "system_u:object_r:mkfifo_t:s0", "pipe2")
	if code != 0 {
		t.Fatalf("mkfifo -Z: code=%d err=%q", code, errb)
	}
	if _, err := os.Lstat(filepath.Join(dir2, "pipe2")); err != nil {
		t.Fatalf("context -Z no-op did not create fifo: %v", err)
	}
}

func TestMkfifoMultipleOperands(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("native FIFO creation is unsupported on windows")
	}
	dir := t.TempDir()
	_, errb, code := runTool(t, dir, "-m", "600", "a", "b", "c")
	if code != 0 || errb != "" {
		t.Fatalf("mkfifo a b c: code=%d err=%q", code, errb)
	}
	for _, name := range []string{"a", "b", "c"} {
		fi, err := os.Lstat(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
		if fi.Mode()&os.ModeNamedPipe == 0 {
			t.Errorf("%s is not a FIFO: %v", name, fi.Mode())
		}
		if fi.Mode().Perm() != 0o600 {
			t.Errorf("%s mode=%o, want 600", name, fi.Mode().Perm())
		}
	}
}

func TestMkfifoPartialFailureContinues(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("native FIFO creation is unsupported on windows")
	}
	dir := t.TempDir()
	// Pre-create an entry so the middle operand fails; the others must still
	// succeed and the exit code is 1, not 2 (system error, not usage error).
	if err := os.WriteFile(filepath.Join(dir, "mid"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "good", "mid", "good2")
	if code != 1 {
		t.Fatalf("partial failure: code=%d, want 1 (err=%q)", code, errb)
	}
	if !strings.Contains(errb, "cannot create fifo 'mid'") {
		t.Errorf("missing mid error in %q", errb)
	}
	for _, name := range []string{"good", "good2"} {
		fi, err := os.Lstat(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("expected %s created despite partial failure: %v", name, err)
		}
		if fi.Mode()&os.ModeNamedPipe == 0 {
			t.Errorf("%s is not a FIFO", name)
		}
	}
}

func TestMkfifoOctalSpecialBits(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("native FIFO creation is unsupported on windows")
	}
	tests := []struct {
		mode   string
		perm   os.FileMode
		setuid bool
		setgid bool
		sticky bool
	}{
		{mode: "4755", perm: 0o755, setuid: true},
		{mode: "2755", perm: 0o755, setgid: true},
		{mode: "1755", perm: 0o755, sticky: true},
		{mode: "4644", perm: 0o644, setuid: true},
		{mode: "7777", perm: 0o777, setuid: true, setgid: true, sticky: true},
		{mode: "0666", perm: 0o666},
	}
	for i, tc := range tests {
		name := fmt.Sprintf("sp%d", i)
		dir := t.TempDir()
		_, errb, code := runTool(t, dir, "-m", tc.mode, name)
		if code != 0 {
			t.Fatalf("mkfifo -m %s: code=%d err=%q", tc.mode, code, errb)
		}
		fi, err := os.Lstat(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if got := fi.Mode().Perm(); got != tc.perm {
			t.Errorf("mkfifo -m %s perm=%03o, want %03o", tc.mode, got, tc.perm)
		}
		if got := fi.Mode()&os.ModeSetuid != 0; got != tc.setuid {
			t.Errorf("mkfifo -m %s setuid=%v, want %v", tc.mode, got, tc.setuid)
		}
		if got := fi.Mode()&os.ModeSetgid != 0; got != tc.setgid {
			t.Errorf("mkfifo -m %s setgid=%v, want %v", tc.mode, got, tc.setgid)
		}
		if got := fi.Mode()&os.ModeSticky != 0; got != tc.sticky {
			t.Errorf("mkfifo -m %s sticky=%v, want %v", tc.mode, got, tc.sticky)
		}
	}
}

func TestMkfifoErrors(t *testing.T) {
	dir := t.TempDir()
	_, errb, code := runTool(t, dir)
	if code != 2 || !strings.Contains(errb, "missing operand") {
		t.Errorf("no args: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, dir, "-m", "999", "pipe")
	if code != 2 || !strings.Contains(errb, "invalid mode '999'") {
		t.Errorf("invalid mode: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, dir, "-m", "u+q", "pipe")
	if code != 2 || !strings.Contains(errb, "invalid mode 'u+q'") {
		t.Errorf("invalid symbolic mode: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, dir, "--frobnicate", "pipe")
	if code != 2 || !strings.Contains(errb, "frobnicate") || !strings.Contains(errb, "pure-Go") {
		t.Errorf("unknown flag: code=%d err=%q", code, errb)
	}
}

func TestMkfifoHelpAndVersion(t *testing.T) {
	out, _, code := runTool(t, t.TempDir(), "--help")
	if code != 0 || !strings.Contains(out, "Usage: mkfifo") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	out, _, code = runTool(t, t.TempDir(), "--version")
	if code != 0 || !strings.Contains(out, "mkfifo") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}
}
