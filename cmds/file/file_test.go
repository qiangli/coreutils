package filecmd

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func invoke(t *testing.T, dir, stdin string, args ...string) (string, string, int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Stdio: tool.Stdio{In: strings.NewReader(stdin), Out: &out, Err: &errb}}
	code := cmd.Run(rc, args)
	return out.String(), errb.String(), code
}

func put(t *testing.T, dir, name string, data []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestTextEmptyDataAndStdin(t *testing.T) {
	dir := t.TempDir()
	put(t, dir, "empty", nil)
	put(t, dir, "ascii", []byte("hello\n"))
	put(t, dir, "utf8", []byte("héllo\n"))
	put(t, dir, "binary", []byte{'a', 0, 'b'})
	out, errb, code := invoke(t, dir, "", "empty", "ascii", "utf8", "binary")
	want := "empty: empty\nascii: ASCII text\nutf8: Unicode text, UTF-8 text\nbinary: data\n"
	if code != 0 || errb != "" || out != want {
		t.Fatalf("out=%q err=%q code=%d", out, errb, code)
	}
	out, _, code = invoke(t, dir, "stdin text\n", "-b", "-")
	if code != 0 || out != "ASCII text\n" {
		t.Fatalf("stdin: out=%q code=%d", out, code)
	}
}

func TestPortableSignatures(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want string
	}{
		{"png", []byte("\x89PNG\r\n\x1a\nrest"), "PNG image data"}, {"jpeg", []byte("\xff\xd8\xff\xe0"), "JPEG image data"},
		{"gif", []byte("GIF89a"), "GIF image data, version 89a"}, {"zip", []byte("PK\x03\x04"), "Zip archive data"},
		{"gzip", []byte("\x1f\x8b\x08"), "gzip compressed data"}, {"pdf", []byte("%PDF-1.7\n"), "PDF document, version 1.7"},
		{"elf", []byte{0x7f, 'E', 'L', 'F', 2, 1}, "ELF 64-bit LSB"}, {"script", []byte("#!/bin/sh\necho ok\n"), "/bin/sh script, text executable"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := classify(tc.data, nil)
			if err != nil || got != tc.want {
				t.Fatalf("got=%q err=%v want=%q", got, err, tc.want)
			}
		})
	}
	tar := make([]byte, 512)
	copy(tar[257:], "ustar")
	if got, _ := classify(tar, nil); got != "POSIX tar archive" {
		t.Fatalf("tar: %q", got)
	}
}

func TestDirectorySymlinkAndFollow(t *testing.T) {
	dir := t.TempDir()
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs privileges on some Windows hosts")
	}
	put(t, dir, "target", []byte("hello\n"))
	if err := os.Symlink("target", filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	out, _, code := invoke(t, dir, "", "link")
	if code != 0 || out != "link: symbolic link to target\n" {
		t.Fatalf("link: %q code=%d", out, code)
	}
	out, _, code = invoke(t, dir, "", "-L", "link")
	if code != 0 || out != "link: ASCII text\n" {
		t.Fatalf("follow: %q code=%d", out, code)
	}
	out, _, code = invoke(t, dir, "", "-b", ".")
	if code != 0 || out != "directory\n" {
		t.Fatalf("dir: %q code=%d", out, code)
	}
}

func TestNoDereferenceShortOptionAndTrailingSlash(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs privileges on some Windows hosts")
	}
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "target"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target", filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	out, errb, code := invoke(t, dir, "", "-h", "link")
	if out != "link: symbolic link to target\n" || errb != "" || code != 0 {
		t.Fatalf("file -h = (%q, %q, %d)", out, errb, code)
	}
	out, errb, code = invoke(t, dir, "", "link/")
	if out != "link/: directory\n" || errb != "" || code != 0 {
		t.Fatalf("file link/ = (%q, %q, %d)", out, errb, code)
	}
}

func TestAdditionalMagicFileFallsBackToBuiltins(t *testing.T) {
	dir := t.TempDir()
	put(t, dir, "magic", []byte("0 s /* custom-c-source\n"))
	put(t, dir, "source", []byte("/* hello */\n"))
	put(t, dir, "text", []byte("hello\n"))
	out, errb, code := invoke(t, dir, "", "-m", "magic", "source", "text")
	if want := "source: custom-c-source\ntext: ASCII text\n"; out != want || errb != "" || code != 0 {
		t.Fatalf("file -m = (%q, %q, %d), want %q", out, errb, code, want)
	}
}

func TestArchiveAndExecutableSignatures(t *testing.T) {
	elf := make([]byte, 18)
	copy(elf, []byte{0x7f, 'E', 'L', 'F', 2, 1})
	binary.LittleEndian.PutUint16(elf[16:], 2)
	for _, tc := range []struct {
		data []byte
		want string
	}{
		{elf, "executable"},
		{[]byte("!<arch>\n"), "archive"},
		{[]byte("070701"), "cpio archive"},
	} {
		got, err := classify(tc.data, nil)
		if err != nil || !strings.Contains(got, tc.want) {
			t.Fatalf("classify(%q) = (%q, %v), want %q", tc.data, got, err, tc.want)
		}
	}
}

func TestErrorsAndStrictFlags(t *testing.T) {
	dir := t.TempDir()
	out, errb, code := invoke(t, dir, "", "missing")
	if !strings.Contains(out, "missing: cannot open") || code != 0 || errb != "" {
		t.Fatalf("out=%q err=%q code=%d", out, errb, code)
	}
	_, errb, code = invoke(t, dir, "", "--mime", "missing")
	if code != 2 || !strings.Contains(errb, "mime") {
		t.Fatalf("err=%q code=%d", errb, code)
	}
	_, _, code = invoke(t, dir, "")
	if code != 2 {
		t.Fatalf("no operand code=%d", code)
	}
}
