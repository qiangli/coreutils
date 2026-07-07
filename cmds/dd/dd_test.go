package ddcmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func runTool(t *testing.T, dir string, stdin string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Stdio: tool.Stdio{In: strings.NewReader(stdin), Out: &out, Err: &errb},
	}
	code = cmd.Run(rc, args)
	return out.String(), errb.String(), code
}

func TestDdCopiesFileWithStatusNone(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "in"), []byte("abcdef"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, errb, code := runTool(t, dir, "", "if=in", "of=out", "bs=2", "count=2", "status=none")
	if code != 0 || out != "" || errb != "" {
		t.Fatalf("dd: code=%d out=%q err=%q", code, out, errb)
	}
	got, err := os.ReadFile(filepath.Join(dir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "abcd" {
		t.Fatalf("out=%q want abcd", got)
	}
}

func TestDdStdinStdout(t *testing.T) {
	out, errb, code := runTool(t, t.TempDir(), "hello", "bs=2", "count=2", "status=none")
	if code != 0 || errb != "" {
		t.Fatalf("dd stdin: code=%d err=%q", code, errb)
	}
	if out != "hell" {
		t.Fatalf("stdout=%q want hell", out)
	}
}

func TestDdSkipSeekNotrunc(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "in"), []byte("0123456789"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "out"), []byte("abcdefghij"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "", "if=in", "of=out", "bs=2", "skip=2", "seek=1", "count=2", "conv=notrunc", "status=none")
	if code != 0 {
		t.Fatalf("dd skip seek: code=%d err=%q", code, errb)
	}
	got, err := os.ReadFile(filepath.Join(dir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "ab4567ghij" {
		t.Fatalf("out=%q want ab4567ghij", got)
	}
}

func TestDdErrors(t *testing.T) {
	dir := t.TempDir()
	_, errb, code := runTool(t, dir, "", "conv=sync")
	if code != 2 || !strings.Contains(errb, "not supported") {
		t.Fatalf("conv unsupported: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, dir, "", "bad")
	if code != 2 || !strings.Contains(errb, "unrecognized operand") {
		t.Fatalf("bad operand: code=%d err=%q", code, errb)
	}
}

// POSIX: seek= preserves the skipped-over output blocks; without
// conv=notrunc the file is truncated at the seek offset, not to zero.
func TestDdSeekPreservesExistingPrefix(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "in"), []byte("BB"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "out"), []byte("AAAAAAAA"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "", "if=in", "of=out", "bs=4", "seek=1", "status=none")
	if code != 0 {
		t.Fatalf("dd seek: code=%d err=%q", code, errb)
	}
	got, err := os.ReadFile(filepath.Join(dir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "AAAABB" {
		t.Fatalf("out=%q want AAAABB (prefix preserved, truncated at seek)", got)
	}
}

// Without seek=, the default truncation still empties an existing file.
func TestDdDefaultTruncatesExistingOutput(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "in"), []byte("BB"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "out"), []byte("AAAAAAAA"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "", "if=in", "of=out", "status=none")
	if code != 0 {
		t.Fatalf("dd: code=%d err=%q", code, errb)
	}
	got, err := os.ReadFile(filepath.Join(dir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "BB" {
		t.Fatalf("out=%q want BB", got)
	}
}

// With ibs=/obs=, output is re-blocked into obs-sized records and
// "records out" counts those, not the input records.
func TestDdReblocksOutputRecords(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "in"), []byte("abcdefgh"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "", "if=in", "of=out", "ibs=1", "obs=4")
	if code != 0 {
		t.Fatalf("dd reblock: code=%d err=%q", code, errb)
	}
	want := "8+0 records in\n2+0 records out\n8 bytes copied\n"
	if errb != want {
		t.Fatalf("status=%q want %q", errb, want)
	}
	got, err := os.ReadFile(filepath.Join(dir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "abcdefgh" {
		t.Fatalf("out=%q want abcdefgh", got)
	}
}

// bs= disables re-blocking: each input block is written as read, so
// records out mirrors records in.
func TestDdBsWritesRecordsAsRead(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "in"), []byte("abcdef"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "", "if=in", "of=out", "bs=4")
	if code != 0 {
		t.Fatalf("dd bs: code=%d err=%q", code, errb)
	}
	want := "1+1 records in\n1+1 records out\n6 bytes copied\n"
	if errb != want {
		t.Fatalf("status=%q want %q", errb, want)
	}
}
