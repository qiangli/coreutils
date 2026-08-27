package filecmd

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

type fileFailWriter struct {
	n   int
	err error
}

func (w fileFailWriter) Write(p []byte) (int, error) {
	if w.n > len(p) {
		return len(p), w.err
	}
	return w.n, w.err
}

type panicReader struct{}

func (panicReader) Read([]byte) (int, error) { panic("stdin was read") }

type endlessReader struct {
	read       int
	maxRequest int
}

func (r *endlessReader) Read(p []byte) (int, error) {
	if len(p) > r.maxRequest {
		r.maxRequest = len(p)
	}
	for i := range p {
		p[i] = 'x'
	}
	r.read += len(p)
	return len(p), nil
}

type trackingRegularFile struct {
	*os.File
	maxRequest int
}

func (f *trackingRegularFile) ReadAt(p []byte, off int64) (int, error) {
	if len(p) > f.maxRequest {
		f.maxRequest = len(p)
	}
	return f.File.ReadAt(p, off)
}

type readCloseStub struct {
	readerAt io.ReaderAt
	readErr  error
	closeErr error
	closed   bool
}

func (r *readCloseStub) ReadAt(p []byte, off int64) (int, error) {
	if r.readErr != nil {
		return 0, r.readErr
	}
	return r.readerAt.ReadAt(p, off)
}

func (r *readCloseStub) Close() error {
	r.closed = true
	return r.closeErr
}

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
	// dangling link) identify the link itself using the STDOUT alternative
	// format: "%s: %s %s\n", <file>, "symbolic link to", <contents>. -i only
	// restricts classification of regular files.
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

func TestMinimalStandardInputDoesNotRead(t *testing.T) {
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{In: panicReader{}, Out: &out, Err: &errb}}
	if code := cmd.Run(rc, []string{"-i", "-"}); code != 0 || out.String() != "-: regular file\n" || errb.Len() != 0 {
		t.Fatalf("file -i - = (%q, %q, %d)", out.String(), errb.String(), code)
	}
}

func TestInspectionReadsOnlyTheDerivedBound(t *testing.T) {
	reader := new(endlessReader)
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(), Env: []string{"LC_ALL=C"},
		Stdio: tool.Stdio{In: reader, Out: &out, Err: &errb},
	}
	if code := cmd.Run(rc, []string{"-"}); code != 0 || out.String() != "-: ASCII text\n" || errb.Len() != 0 {
		t.Fatalf("bounded stdin = (%q, %q, %d)", out.String(), errb.String(), code)
	}
	if reader.read != contextPrefixBytes {
		t.Fatalf("stdin bytes read = %d, want exact derived bound %d", reader.read, contextPrefixBytes)
	}
}

func TestLoadedMagicExtendsTheDerivedBoundWithoutUnboundedReading(t *testing.T) {
	dir := t.TempDir()
	offset := uint64(20*1024*1024 + 7)
	put(t, dir, "magic", []byte(fmt.Sprintf("%d\tstring\tx\thigh-offset\n", offset)))
	reader := new(endlessReader)
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Dir: dir, Stdio: tool.Stdio{In: reader, Out: &out, Err: &errb}}
	if code := cmd.Run(rc, []string{"-M", "magic", "-"}); code != 0 || out.String() != "-: high-offset\n" || errb.Len() != 0 {
		t.Fatalf("high-offset magic = (%q, %q, %d)", out.String(), errb.String(), code)
	}
	if uint64(reader.read) != offset+1 {
		t.Fatalf("stdin bytes read = %d, want magic offset plus width %d", reader.read, offset+1)
	}
	if reader.maxRequest > readChunkSize {
		t.Fatalf("largest hostile-reader request = %d, want <= %d", reader.maxRequest, readChunkSize)
	}
}

func TestHugeSparseRegularFileUsesBoundedPrefix(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sparse")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(1 << 40); err != nil {
		_ = f.Close()
		t.Skipf("sparse files unavailable: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	out, errb, code := invoke(t, dir, "", "sparse")
	if out != "sparse: data\n" || errb != "" || code != 0 {
		t.Fatalf("sparse file = (%q, %q, %d)", out, errb, code)
	}
	const farOffset = 32*1024*1024 + 3
	f, err = os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteAt([]byte{'Q'}, farOffset); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	put(t, dir, "magic", []byte(fmt.Sprintf("%d\tstring\tQ\tsparse-high-offset\n", farOffset)))
	var sparseOut, sparseErr bytes.Buffer
	var tracked *trackingRegularFile
	rc := &tool.RunContext{Dir: dir, Stdio: tool.Stdio{In: strings.NewReader(""), Out: &sparseOut, Err: &sparseErr}}
	open := func(name string) (regularFile, error) {
		file, err := os.Open(name)
		if err != nil {
			return nil, err
		}
		tracked = &trackingRegularFile{File: file}
		return tracked, nil
	}
	code = runWithOpener(rc, []string{"-M", "magic", "sparse"}, open)
	if sparseOut.String() != "sparse: sparse-high-offset\n" || sparseErr.Len() != 0 || code != 0 {
		t.Fatalf("sparse high-offset magic = (%q, %q, %d)", sparseOut.String(), sparseErr.String(), code)
	}
	if tracked == nil || tracked.maxRequest != 1 {
		t.Fatalf("largest sparse-file ReadAt request = %v, want 1", tracked)
	}
}

func TestSparsePEOffsetUsesADynamicBuiltInRange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "far.exe")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(1 << 40); err != nil {
		_ = file.Close()
		t.Skipf("sparse files unavailable: %v", err)
	}
	const signatureOffset = 24*1024*1024 + 5
	header := make([]byte, 0x40)
	copy(header, "MZ")
	binary.LittleEndian.PutUint32(header[0x3c:], signatureOffset)
	if _, err := file.WriteAt(header, 0); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if _, err := file.WriteAt([]byte("PE\x00\x00"), signatureOffset); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	var out, errb bytes.Buffer
	var tracked *trackingRegularFile
	rc := &tool.RunContext{Dir: dir, Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb}}
	open := func(name string) (regularFile, error) {
		opened, err := os.Open(name)
		if err != nil {
			return nil, err
		}
		tracked = &trackingRegularFile{File: opened}
		return tracked, nil
	}
	if code := runWithOpener(rc, []string{"far.exe"}, open); code != 0 || out.String() != "far.exe: PE executable\n" || errb.Len() != 0 {
		t.Fatalf("sparse PE = (%q, %q, %d)", out.String(), errb.String(), code)
	}
	if tracked == nil || tracked.maxRequest > contextPrefixBytes {
		t.Fatalf("largest sparse PE ReadAt request = %v, want <= %d", tracked, contextPrefixBytes)
	}
}

func TestReadAndCloseErrorsRemainSuccessfulClassifications(t *testing.T) {
	dir := t.TempDir()
	put(t, dir, "data", []byte("placeholder"))
	for _, tc := range []struct {
		name string
		file *readCloseStub
		want string
	}{
		{"read", &readCloseStub{readErr: errors.New("input failed")}, "input failed"},
		{"close", &readCloseStub{readerAt: bytes.NewReader([]byte("text\n")), closeErr: errors.New("close failed")}, "close failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out, errb bytes.Buffer
			rc := &tool.RunContext{Dir: dir, Env: []string{"LANG=C.UTF-8"}, Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb}}
			open := func(string) (regularFile, error) { return tc.file, nil }
			code := runWithOpener(rc, []string{"data"}, open)
			if code != 0 || errb.Len() != 0 || !strings.Contains(out.String(), "data: cannot open") || !strings.Contains(strings.ToLower(out.String()), tc.want) {
				t.Fatalf("%s error = (%q, %q, %d)", tc.name, out.String(), errb.String(), code)
			}
			if !tc.file.closed {
				t.Fatalf("%s path did not close the regular file", tc.name)
			}
		})
	}
}

func TestOutputAndDiagnosticWriteFailuresAreFailures(t *testing.T) {
	dir := t.TempDir()
	put(t, dir, "data", []byte("text\n"))
	for _, out := range []io.Writer{
		fileFailWriter{err: errors.New("output failed")},
		fileFailWriter{n: 1},
	} {
		var errb bytes.Buffer
		rc := &tool.RunContext{Dir: dir, Stdio: tool.Stdio{In: strings.NewReader(""), Out: out, Err: &errb}}
		if code := cmd.Run(rc, []string{"data"}); code != 1 || !strings.Contains(errb.String(), "file: write error:") {
			t.Fatalf("stdout failure = (%q, %d)", errb.String(), code)
		}
	}

	put(t, dir, "bad.magic", []byte("invalid\n"))
	rc := &tool.RunContext{Dir: dir, Stdio: tool.Stdio{
		In: strings.NewReader(""), Out: io.Discard,
		Err: fileFailWriter{err: errors.New("diagnostic failed")},
	}}
	if code := cmd.Run(rc, []string{"-M", "bad.magic", "data"}); code != 1 {
		t.Fatalf("stderr failure exit = %d, want 1", code)
	}
}

func TestHostileZeroProgressReaderIsReported(t *testing.T) {
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Stdio: tool.Stdio{In: zeroProgressReader{}, Out: &out, Err: &errb}}
	if code := cmd.Run(rc, []string{"-"}); code != 0 || errb.Len() != 0 || !strings.Contains(strings.ToLower(out.String()), strings.ToLower(io.ErrNoProgress.Error())) {
		t.Fatalf("zero-progress reader = (%q, %q, %d)", out.String(), errb.String(), code)
	}
}

type zeroProgressReader struct{}

func (zeroProgressReader) Read([]byte) (int, error) { return 0, nil }

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
	put(t, dir, "letters", []byte("hello"))
	put(t, dir, "magic", []byte("0\tstring\thello\tfirst=%c\n"))
	out, errb, code = invoke(t, dir, "", "-M", "magic", "letters")
	if want := "letters: first=h\n"; out != want || errb != "" || code != 0 || strings.Contains(out, "%!") {
		t.Fatalf("file string %%c message = (%q, %q, %d), want %q", out, errb, code, want)
	}
}

func TestMagicMessageFormattingIsByteAccurate(t *testing.T) {
	dir := t.TempDir()
	put(t, dir, "utf8", []byte("é"))
	for _, tc := range []struct {
		name, format string
		want         []byte
	}{
		{"string-c", "%c", []byte{'v', '=', 0xc3, '\n'}},
		{"string-precision", "%.1s", []byte{'v', '=', 0xc3, '\n'}},
		{"b-precision", "%.1b", []byte{'v', '=', 0xc3, '\n'}},
		{"left-width-after-byte-precision", "%-4.1s", []byte{'v', '=', 0xc3, ' ', ' ', ' ', '\n'}},
		{"right-width-after-byte-precision", "%4.1b", []byte{'v', '=', ' ', ' ', ' ', 0xc3, '\n'}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			put(t, dir, "magic", []byte("0\tstring\té\tv="+tc.format+"\n"))
			out, errb, code := invoke(t, dir, "", "-b", "-M", "magic", "utf8")
			if code != 0 || errb != "" || !bytes.Equal([]byte(out), tc.want) {
				t.Fatalf("format %q = (% x, %q, %d), want % x", tc.format, []byte(out), errb, code, tc.want)
			}
		})
	}
}

func TestMagicMessageCrossTypeConversions(t *testing.T) {
	dir := t.TempDir()
	put(t, dir, "decimal", []byte("123"))
	put(t, dir, "hex", []byte("0x2a"))
	put(t, dir, "signed", []byte("-7"))
	put(t, dir, "numeric-s", []byte{7})
	put(t, dir, "numeric-c", []byte{65})
	put(t, dir, "numeric-b", []byte{8})
	put(t, dir, "missing", []byte("miss"))
	put(t, dir, "magic", []byte(strings.Join([]string{
		"0\tstring\t123\tdecimal=%d",
		"0\tstring\t0x2a\thex=%x",
		"0\tstring\t-7\tsigned=%i",
		"0\tuC\t=7\tstring=%s",
		"0\tuC\t=65\tbyte=%c",
		"0\tuC\t=8\tb=%b",
		"0\tstring\tmiss\tmissing=%s/%s/%u",
	}, "\n")+"\n"))

	out, errb, code := invoke(t, dir, "", "-M", "magic", "decimal", "hex", "signed", "numeric-s", "numeric-c", "numeric-b", "missing")
	want := "decimal: decimal=123\nhex: hex=2a\nsigned: signed=-7\nnumeric-s: string=7\nnumeric-c: byte=A\nnumeric-b: b=8\nmissing: missing=miss//0\n"
	if out != want || errb != "" || code != 0 || strings.Contains(out, "%!") {
		t.Fatalf("cross-type formats = (%q, %q, %d), want %q", out, errb, code, want)
	}
}

func TestMagicMessageAllPortableCrossTypeConversions(t *testing.T) {
	for _, tc := range []struct {
		name string
		arg  any
		want map[byte]string
	}{
		{
			name: "string",
			arg:  "65",
			want: map[byte]string{'b': "65", 'c': "6", 's': "65", 'd': "65", 'i': "65", 'o': "101", 'u': "65", 'x': "41", 'X': "41"},
		},
		{
			name: "numeric",
			arg:  magicNumberArg{value: 65, size: 1},
			want: map[byte]string{'b': "65", 'c': "A", 's': "65", 'd': "65", 'i': "65", 'o': "101", 'u': "65", 'x': "41", 'X': "41"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for conv, want := range tc.want {
				got, err := renderMagicMessage("%"+string(conv), tc.arg)
				if err != nil || got != want || strings.Contains(got, "%!") {
					t.Errorf("%%%c = (%q, %v), want %q", conv, got, err, want)
				}
			}
		})
	}
}

func TestMagicNumericConversionErrorsPreserveAccumulatedValues(t *testing.T) {
	for _, tc := range []struct {
		name, format string
		arg          any
		want         string
	}{
		{"uint-max-as-signed", "%d", "18446744073709551615", "9223372036854775807"},
		{"signed-underflow", "%i", "-9223372036854775809", "-9223372036854775808"},
		{"unsigned-overflow", "%u", "18446744073709551616", "18446744073709551615"},
		{"hex-overflow", "%x", "18446744073709551616", "ffffffffffffffff"},
		{"partial-number", "%d", "12x", "12"},
		{"unsigned-magic-as-signed", "%d", magicNumberArg{value: ^uint64(0), size: 8}, "9223372036854775807"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := renderMagicMessage("prefix="+tc.format+";suffix", tc.arg)
			if got != "prefix="+tc.want+";suffix" || err == nil || strings.Contains(got, "%!") {
				t.Fatalf("render %q as %q = (%q, %v), want accumulated value %q and error", tc.arg, tc.format, got, err, tc.want)
			}
		})
	}
}

func TestMagicNumericConversionSkipsLeadingWhitespace(t *testing.T) {
	for _, tc := range []struct {
		name, format, arg, want string
		wantErr                 bool
	}{
		{"signed", "%d", " \t\n\v\f\r-12", "-12", false},
		{"unsigned", "%u", " \t\n\v\f\r-1", "18446744073709551615", false},
		{"signed-whitespace-only", "%i", " \t\n\v\f\r", "0", true},
		{"unsigned-whitespace-only", "%x", " \t\n\v\f\r", "0", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := renderMagicMessage(tc.format, tc.arg)
			if got != tc.want || (err != nil) != tc.wantErr {
				t.Fatalf("render %q as %q = (%q, %v), want (%q, error=%v)", tc.arg, tc.format, got, err, tc.want, tc.wantErr)
			}
			if tc.wantErr && !strings.Contains(err.Error(), "is not a valid integer") {
				t.Fatalf("render %q diagnostic = %q, want invalid-integer diagnostic", tc.arg, err)
			}
		})
	}
}

func TestMagicNumericConversionErrorsSetStatusWithoutBecomingOpenErrors(t *testing.T) {
	dir := t.TempDir()
	put(t, dir, "huge", []byte("18446744073709551615"))
	put(t, dir, "partial", []byte("12x"))
	put(t, dir, "spaces", []byte(" \t"))
	put(t, dir, "magic", []byte(strings.Join([]string{
		"0\tstring\t18446744073709551615\tvalue=%d;done",
		"0\tstring\t12x\tvalue=%u;done",
		"0\tstring\t\\ \\t\tvalue=%u;done",
	}, "\n")+"\n"))

	out, errb, code := invoke(t, dir, "", "-M", "magic", "huge", "missing", "partial", "spaces")
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != 4 || lines[0] != "huge: value=9223372036854775807;done" ||
		!strings.Contains(lines[1], "missing: cannot open") || lines[2] != "partial: value=12;done" ||
		lines[3] != "spaces: value=0;done" || code != 1 || strings.Count(errb, "file: magic message") != 3 ||
		!strings.Contains(errb, "outside the signed integer range") ||
		!strings.Contains(errb, "was not completely converted") ||
		!strings.Contains(errb, "is not a valid integer") ||
		strings.Contains(out, "%!") || strings.Contains(out, "huge: cannot open") || strings.Contains(out, "partial: cannot open") {
		t.Fatalf("runtime conversion errors = (%q, %q, %d), want saturated/partial output and three diagnostics", out, errb, code)
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
