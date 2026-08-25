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
	return invokeEnv(t, dir, stdin, []string{"LANG=C.UTF-8"}, args...)
}

func invokeEnv(t *testing.T, dir, stdin string, env []string, args ...string) (string, string, int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Env: env, Stdio: tool.Stdio{In: strings.NewReader(stdin), Out: &out, Err: &errb}}
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
		{"elf", []byte{0x7f, 'E', 'L', 'F', 2, 1}, "ELF 64-bit LSB"}, {"script", []byte("#!/bin/sh\necho ok\n"), "/bin/sh commands text"},
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
	if err := os.Symlink("absent", filepath.Join(dir, "dangling")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("loop", filepath.Join(dir, "loop")); err != nil {
		t.Fatal(err)
	}
	out, _, code := invoke(t, dir, "", "link")
	if code != 0 || out != "link: ASCII text\n" {
		t.Fatalf("link: %q code=%d", out, code)
	}
	out, _, code = invoke(t, dir, "", "-h", "link")
	if code != 0 || out != "link: symbolic link to target\n" {
		t.Fatalf("no-follow: %q code=%d", out, code)
	}
	out, _, code = invoke(t, dir, "", "dangling")
	if code != 0 || out != "dangling: symbolic link to absent\n" {
		t.Fatalf("dangling: %q code=%d", out, code)
	}
	out, errb, code := invoke(t, dir, "", "loop")
	if code != 0 || errb != "" || !strings.Contains(out, "loop: cannot open") {
		t.Fatalf("loop: out=%q err=%q code=%d", out, errb, code)
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

func TestDefaultTestsAndMagicOptionOrder(t *testing.T) {
	dir := t.TempDir()
	put(t, dir, "png", []byte("\x89PNG\r\n\x1a\nrest"))
	put(t, dir, "text", []byte("ordinary text\n"))
	put(t, dir, "match", []byte("0 string \\211PNG custom png\n"))
	put(t, dir, "miss", []byte("0 string NOPE custom\n"))
	put(t, dir, "textmatch", []byte("0 string ordinary custom text\n"))

	tests := []struct {
		name string
		args []string
		want string
	}{
		{"default-before-additional", []string{"-d", "-m", "match", "png"}, "png: PNG image data\n"},
		{"context-default-is-deferred", []string{"-d", "-m", "textmatch", "text"}, "text: custom text\n"},
		{"combined-default-before-additional", []string{"-dm", "match", "png"}, "png: PNG image data\n"},
		{"additional-before-default", []string{"-m", "match", "-d", "png"}, "png: custom png\n"},
		{"lone-additional-falls-through", []string{"-m", "miss", "png"}, "png: PNG image data\n"},
		{"replacement-omits-all-defaults", []string{"-M", "miss", "text"}, "text: data\n"},
		{"replacement-before-default", []string{"-M", "match", "-d", "png"}, "png: custom png\n"},
		{"default-before-replacement", []string{"-d", "-M", "match", "png"}, "png: PNG image data\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, errb, code := invoke(t, dir, "", tc.args...)
			if out != tc.want || errb != "" || code != 0 {
				t.Fatalf("file %v = (%q, %q, %d), want %q", tc.args, out, errb, code, tc.want)
			}
		})
	}
}

func TestMinimalIdentification(t *testing.T) {
	dir := t.TempDir()
	put(t, dir, "empty", nil)
	put(t, dir, "png", []byte("\x89PNG\r\n\x1a\n"))
	out, errb, code := invoke(t, dir, "", "-i", "empty", "png", ".")
	if want := "empty: regular file\npng: regular file\n.: directory\n"; out != want || errb != "" || code != 0 {
		t.Fatalf("file -i = (%q, %q, %d), want %q", out, errb, code, want)
	}
	_, errb, code = invoke(t, dir, "", "-i", "-d", "empty")
	if code != 2 || !strings.Contains(errb, "cannot be combined") {
		t.Fatalf("file -i -d = (_, %q, %d)", errb, code)
	}
}

func TestMinimalIdentificationSymlinkAndStandardInput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs privileges on some Windows hosts")
	}
	dir := t.TempDir()
	put(t, dir, "target", []byte("payload\n"))
	if err := os.Symlink("target", filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("missing", filepath.Join(dir, "dangling")); err != nil {
		t.Fatal(err)
	}

	// -i still follows a usable symbolic link by default, but -h (and a
	// dangling link) identify the link itself. This is the portable way for a
	// shell script to distinguish a regular file from the other file types.
	out, errb, code := invoke(t, dir, "", "-i", "link", "dangling")
	want := "link: regular file\ndangling: symbolic link to missing\n"
	if out != want || errb != "" || code != 0 {
		t.Fatalf("file -i links = (%q, %q, %d), want %q", out, errb, code, want)
	}
	out, errb, code = invoke(t, dir, "", "-i", "-h", "link")
	if out != "link: symbolic link to target\n" || errb != "" || code != 0 {
		t.Fatalf("file -i -h link = (%q, %q, %d)", out, errb, code)
	}
	out, errb, code = invoke(t, dir, "stream", "-i", "-")
	if out != "-: regular file\n" || errb != "" || code != 0 {
		t.Fatalf("file -i - = (%q, %q, %d)", out, errb, code)
	}
}

func TestMagicGrammarComparisonsContinuationsAndFormatting(t *testing.T) {
	dir := t.TempDir()
	put(t, dir, "strings", []byte("hi there!"))
	put(t, dir, "numeric", []byte{0xab, 201, 0xff, 7})
	put(t, dir, "high", []byte{1, 201})
	put(t, dir, "signed", []byte{1, 0, 0xff})
	put(t, dir, "low", []byte{1, 9})
	put(t, dir, "allbits", []byte{1, 20, 0, 7})
	put(t, dir, "missingbit", []byte{1, 20, 0, 1})
	put(t, dir, "continued", []byte("AB\x07"))
	put(t, dir, "tabbed", []byte("ZZ"))
	put(t, dir, "magic", []byte(strings.Join([]string{
		"0x0 string hi\\ there string=%s",
		"0 byte&0xf0 0xa0 masked=%u",
		"1 u1 >200 high=%u",
		"2 byte =-1 signed=%d",
		"1 uC <10 low=%u",
		"3 uC &0x05 allbits=%u",
		"3 uC ^0x08 missingbit=%u",
		"0\tstring\tZZ\t leading message",
		"0 string AB root",
		">02 byte x ; byte=%u",
	}, "\n")+"\n"))

	out, errb, code := invoke(t, dir, "", "-M", "magic", "strings", "numeric", "high", "signed", "low", "allbits", "missingbit", "continued", "tabbed")
	want := "strings: string=hi there\nnumeric: masked=160\nhigh: high=201\nsigned: signed=-1\nlow: low=9\nallbits: allbits=7\nmissingbit: missingbit=1\ncontinued: root; byte=7\ntabbed:  leading message\n"
	if out != want || errb != "" || code != 0 {
		t.Fatalf("portable magic = (%q, %q, %d), want %q", out, errb, code, want)
	}
}

func TestMagicBareDecimalUsesCIntWidth(t *testing.T) {
	dir := t.TempDir()
	// A bare d is the C int type. On a 64-bit Go host it must still read
	// four bytes, rather than silently becoming an eight-byte Go int test.
	word := make([]byte, 4)
	binary.NativeEndian.PutUint32(word, 0x01020304)
	put(t, dir, "word", word)
	put(t, dir, "magic", []byte("0\td\t0x01020304\tc int=%u\n"))
	out, errb, code := invoke(t, dir, "", "-M", "magic", "word")
	if out != "word: c int=16909060\n" || errb != "" || code != 0 {
		t.Fatalf("file bare d = (%q, %q, %d)", out, errb, code)
	}
}

func TestMagicContinuationGatingAndShortValues(t *testing.T) {
	dir := t.TempDir()
	put(t, dir, "gated", []byte("BA\x07"))
	put(t, dir, "short", []byte("A"))
	put(t, dir, "magic", []byte(strings.Join([]string{
		"0\tstring\tAB\twrong",
		">2\tbyte\tx\t must-not-print-%u",
		"0\tstring\tBA\tright",
		">2\tbyte\tx\t byte=%u",
		"0\tstring\tABCD\ttoo-short",
	}, "\n")+"\n"))

	// A continuation is gated by its immediately preceding primary test, and
	// both string and numeric tests fail when their full value is not present.
	out, errb, code := invoke(t, dir, "", "-M", "magic", "gated", "short")
	want := "gated: right byte=7\nshort: data\n"
	if out != want || errb != "" || code != 0 {
		t.Fatalf("file continuation/short value = (%q, %q, %d), want %q", out, errb, code, want)
	}
}

func TestMagicFileErrorsAreDiagnostics(t *testing.T) {
	dir := t.TempDir()
	put(t, dir, "data", []byte("x"))
	for _, tc := range []struct {
		name, contents, diagnostic string
	}{
		{"bad-type", "0 float 1 nope\n", "unsupported magic type"},
		{"orphan", ">0 byte 1 nope\n", "continuation has no preceding"},
		{"bad-escape", "0 string \\q nope\n", "unsupported magic escape"},
		{"signed-offset", "+0 string x nope\n", "invalid magic offset"},
		{"signed-width", "0 u+1 1 nope\n", "unsupported magic type width"},
		{"signed-mask", "0 uC&+1 1 nope\n", "invalid magic mask"},
		{"bad-message-escape", "0 string x bad\\q\n", "unsupported magic message escape"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			put(t, dir, "magic", []byte(tc.contents))
			out, errb, code := invoke(t, dir, "", "-M", "magic", "data")
			if out != "" || code != 1 || !strings.Contains(errb, tc.diagnostic) || !strings.Contains(errb, "magic:1") {
				t.Fatalf("file malformed magic = (%q, %q, %d)", out, errb, code)
			}
		})
	}
}

func TestRequiredProgramTextAndRunContextLocale(t *testing.T) {
	dir := t.TempDir()
	put(t, dir, "shell", []byte("#!/bin/sh\necho ok\n"))
	put(t, dir, "source.c", []byte("#include <stdio.h>\nint main(void) { return 0; }\n"))
	put(t, dir, "source.f", []byte("      PROGRAM HELLO\n      END\n"))
	put(t, dir, "utf8", []byte("héllo\n"))
	out, errb, code := invokeEnv(t, dir, "", []string{"LANG=C.UTF-8", "LC_CTYPE=C", "LC_ALL=C.UTF-8"}, "shell", "source.c", "source.f", "utf8")
	want := "shell: /bin/sh commands text\nsource.c: c program text\nsource.f: fortran program text\nutf8: Unicode text, UTF-8 text\n"
	if out != want || errb != "" || code != 0 {
		t.Fatalf("UTF-8 locale = (%q, %q, %d), want %q", out, errb, code, want)
	}
	out, errb, code = invokeEnv(t, dir, "", []string{"LANG=C.UTF-8", "LC_CTYPE=C"}, "utf8")
	if out != "utf8: data\n" || errb != "" || code != 0 {
		t.Fatalf("LC_CTYPE override = (%q, %q, %d)", out, errb, code)
	}
	// Invocation-owned environments can contain repeated assignments; the last
	// one wins, and must not leak in process-global locale state.
	out, errb, code = invokeEnv(t, dir, "", []string{"LC_CTYPE=C", "LC_CTYPE=C.UTF-8"}, "utf8")
	if out != "utf8: Unicode text, UTF-8 text\n" || errb != "" || code != 0 {
		t.Fatalf("RunContext LC_CTYPE = (%q, %q, %d)", out, errb, code)
	}
}

func TestMagicMessageUsesPrintfFormatSemantics(t *testing.T) {
	dir := t.TempDir()
	put(t, dir, "data", []byte{7})
	put(t, dir, "escaped", []byte(`A\nB\cTAIL`))
	put(t, dir, "magic", []byte("0\tuC\tx\tvalue=%03u\\tfirst=%u second=%u\\012\n"))
	out, errb, code := invoke(t, dir, "", "-M", "magic", "data")
	want := "data: value=007\tfirst=0 second=0\n\n"
	if out != want || errb != "" || code != 0 {
		t.Fatalf("file magic printf message = (%q, %q, %d), want %q", out, errb, code, want)
	}
	put(t, dir, "magic", []byte("0\tstring\tA\\\\nB\\\\cTAIL\tdecoded=%b SHOULD-NOT-PRINT\n"))
	out, errb, code = invoke(t, dir, "", "-M", "magic", "escaped")
	if want := "escaped: decoded=A\nB\n"; out != want || errb != "" || code != 0 {
		t.Fatalf("file magic %%b message = (%q, %q, %d), want %q", out, errb, code, want)
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
