package odcmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func runOD(t *testing.T, dir, stdin string, args ...string) (string, string, int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Stdio: tool.Stdio{In: strings.NewReader(stdin), Out: &out, Err: &errb},
	}
	code := cmd.Run(rc, args)
	return out.String(), errb.String(), code
}

func TestODDefaultOctalWords(t *testing.T) {
	out, errb, code := runOD(t, t.TempDir(), "ABCD")
	want := "0000000 041101 042103\n0000004\n"
	if out != want || errb != "" || code != 0 {
		t.Fatalf("od default = (%q, %q, %d), want (%q, \"\", 0)", out, errb, code, want)
	}
}

func TestODFormatsAndOffsets(t *testing.T) {
	out, _, code := runOD(t, t.TempDir(), "abc\n", "-A", "x", "-t", "x1", "-N", "3")
	if want := "0000000 61 62 63\n0000003\n"; out != want || code != 0 {
		t.Fatalf("od x1 = (%q, %d), want (%q, 0)", out, code, want)
	}
	out, _, code = runOD(t, t.TempDir(), "a\n", "-A", "n", "-t", "c")
	if want := "   a  \\n\n"; out != want || code != 0 {
		t.Fatalf("od c no addresses = (%q, %d), want (%q, 0)", out, code, want)
	}
}

func TestODShortAliasesAndWidth(t *testing.T) {
	out, _, code := runOD(t, t.TempDir(), "abcd", "-b", "-w", "2")
	if want := "0000000 141 142\n0000002 143 144\n0000004\n"; out != want || code != 0 {
		t.Fatalf("od -b -w = (%q, %d), want (%q, 0)", out, code, want)
	}

	out, _, code = runOD(t, t.TempDir(), "AB", "-x")
	if want := "0000000 4241\n0000002\n"; out != want || code != 0 {
		t.Fatalf("od -x = (%q, %d), want (%q, 0)", out, code, want)
	}

	out, _, code = runOD(t, t.TempDir(), "AB", "-d")
	if want := "0000000 16961\n0000002\n"; out != want || code != 0 {
		t.Fatalf("od -d = (%q, %d), want (%q, 0)", out, code, want)
	}
}

func TestODSkipAndFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "in"), []byte("abcd"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, code := runOD(t, dir, "", "-t", "o1", "-j", "2", "in")
	if want := "0000002 143 144\n0000004\n"; out != want || code != 0 {
		t.Fatalf("od skip file = (%q, %d), want (%q, 0)", out, code, want)
	}
}

func TestODMultiFormatEndianStringsAndTraditionalSkip(t *testing.T) {
	out, _, code := runOD(t, t.TempDir(), "ABCD", "-A", "x", "-t", "x2", "-t", "u1", "--endian", "big", "-w", "4")
	want := "0000000 4142 4344\n         65  66  67  68\n0000004\n"
	if out != want || code != 0 {
		t.Fatalf("od multi/endian = (%q, %d), want (%q, 0)", out, code, want)
	}

	out, _, code = runOD(t, t.TempDir(), "\x00abc\x00de\x00", "-A", "d", "-S", "3")
	want = "0000001 abc\n0000008\n"
	if out != want || code != 0 {
		t.Fatalf("od strings = (%q, %d), want (%q, 0)", out, code, want)
	}

	out, _, code = runOD(t, t.TempDir(), "abcd", "+2")
	want = "0000002 062143\n0000004\n"
	if out != want || code != 0 {
		t.Fatalf("od traditional skip = (%q, %d), want (%q, 0)", out, code, want)
	}
}

func TestODRejectsBadFormat(t *testing.T) {
	_, errb, code := runOD(t, t.TempDir(), "", "-t", "x4")
	if code != 0 || errb != "" {
		t.Fatalf("od x4 should now be supported code=%d err=%q", code, errb)
	}

	_, errb, code = runOD(t, t.TempDir(), "", "-t", "z9")
	if code != 2 || !strings.Contains(errb, "unsupported output format") {
		t.Fatalf("od bad format code=%d err=%q", code, errb)
	}
}

func TestODRejectsBadWidth(t *testing.T) {
	_, errb, code := runOD(t, t.TempDir(), "", "-w", "0")
	if code != 2 || !strings.Contains(errb, "invalid output width") {
		t.Fatalf("od bad width code=%d err=%q", code, errb)
	}
}
