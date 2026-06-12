package gzipcmd

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/coreutils/tool"
)

// runTool is the canonical test harness shape for cmds packages
// (output captured AFTER Run).
func runTool(t *testing.T, tl *tool.Tool, dir string, stdin io.Reader, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	if stdin == nil {
		stdin = strings.NewReader("")
	}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Stdio: tool.Stdio{In: stdin, Out: &out, Err: &errb},
	}
	code = tl.Run(rc, args)
	return out.String(), errb.String(), code
}

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCompressDecompressRoundtrip(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "f.txt", "hello gzip world\n")
	if err := os.Chmod(p, 0o640); err != nil {
		t.Fatal(err)
	}
	mtime := time.Date(2024, 3, 14, 15, 9, 26, 0, time.UTC)
	if err := os.Chtimes(p, mtime, mtime); err != nil {
		t.Fatal(err)
	}

	_, errb, code := runTool(t, gzipTool, dir, nil, "f.txt")
	if code != 0 {
		t.Fatalf("gzip: code=%d err=%q", code, errb)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatalf("original should be removed after compression")
	}
	gz := p + ".gz"
	fi, err := os.Stat(gz)
	if err != nil {
		t.Fatalf("f.txt.gz missing: %v", err)
	}
	if !fi.ModTime().Equal(mtime) {
		t.Errorf("mtime not preserved on .gz: got %v want %v", fi.ModTime(), mtime)
	}
	if runtime.GOOS != "windows" && fi.Mode().Perm() != 0o640 {
		t.Errorf("mode not preserved on .gz: got %o", fi.Mode().Perm())
	}

	_, errb, code = runTool(t, gzipTool, dir, nil, "-d", "f.txt.gz")
	if code != 0 {
		t.Fatalf("gzip -d: code=%d err=%q", code, errb)
	}
	if _, err := os.Stat(gz); !os.IsNotExist(err) {
		t.Fatalf(".gz should be removed after decompression")
	}
	got, err := os.ReadFile(p)
	if err != nil || string(got) != "hello gzip world\n" {
		t.Fatalf("roundtrip content = %q, %v", got, err)
	}
	fi, _ = os.Stat(p)
	if !fi.ModTime().Equal(mtime) {
		t.Errorf("mtime not preserved on decompress: got %v", fi.ModTime())
	}
}

func TestKeepAndStdout(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "data\n")

	_, _, code := runTool(t, gzipTool, dir, nil, "-k", "f.txt")
	if code != 0 {
		t.Fatal("gzip -k failed")
	}
	if _, err := os.Stat(filepath.Join(dir, "f.txt")); err != nil {
		t.Errorf("-k should keep the input: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "f.txt.gz")); err != nil {
		t.Errorf(".gz should exist: %v", err)
	}

	// -c writes to stdout and keeps the input
	dir2 := t.TempDir()
	writeFile(t, dir2, "g.txt", "stream me\n")
	out, _, code := runTool(t, gzipTool, dir2, nil, "-c", "g.txt")
	if code != 0 {
		t.Fatal("gzip -c failed")
	}
	if _, err := os.Stat(filepath.Join(dir2, "g.txt")); err != nil {
		t.Errorf("-c should keep the input: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir2, "g.txt.gz")); !os.IsNotExist(err) {
		t.Errorf("-c should not create g.txt.gz")
	}
	zr, err := gzip.NewReader(strings.NewReader(out))
	if err != nil {
		t.Fatalf("stdout is not gzip: %v", err)
	}
	plain, _ := io.ReadAll(zr)
	if string(plain) != "stream me\n" {
		t.Errorf("decompressed stdout = %q", plain)
	}
	if zr.Name != "g.txt" {
		t.Errorf("gzip header name = %q, want g.txt", zr.Name)
	}
}

func TestLevelsAndCluster(t *testing.T) {
	dir := t.TempDir()
	content := strings.Repeat("abcdefghij", 500)
	writeFile(t, dir, "f.txt", content)

	// combined cluster with digit: -9c
	out, _, code := runTool(t, gzipTool, dir, nil, "-9c", "f.txt")
	if code != 0 {
		t.Fatalf("gzip -9c: code=%d", code)
	}
	zr, err := gzip.NewReader(strings.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := io.ReadAll(zr); string(got) != content {
		t.Error("-9c roundtrip mismatch")
	}

	// standalone -1, and the documented long spellings
	for _, flag := range []string{"-1", "--fast", "--best"} {
		out, _, code := runTool(t, gzipTool, dir, nil, flag, "-c", "f.txt")
		if code != 0 {
			t.Fatalf("gzip %s -c: code=%d", flag, code)
		}
		zr, err := gzip.NewReader(strings.NewReader(out))
		if err != nil {
			t.Fatalf("%s output not gzip: %v", flag, err)
		}
		if got, _ := io.ReadAll(zr); string(got) != content {
			t.Errorf("%s roundtrip mismatch", flag)
		}
	}
}

func TestStdinStdout(t *testing.T) {
	dir := t.TempDir()
	// no operands: compress stdin → stdout
	out, _, code := runTool(t, gzipTool, dir, strings.NewReader("pipe data\n"))
	if code != 0 {
		t.Fatalf("stdin compress: code=%d", code)
	}
	// "-" operand: decompress stdin → stdout
	plain, _, code := runTool(t, gzipTool, dir, strings.NewReader(out), "-d", "-")
	if code != 0 || plain != "pipe data\n" {
		t.Fatalf("stdin decompress: code=%d out=%q", code, plain)
	}
}

func TestWarningsAndErrors(t *testing.T) {
	dir := t.TempDir()

	// compressing a file that already has the suffix: warning, exit 2
	writeFile(t, dir, "a.gz", "whatever")
	_, errb, code := runTool(t, gzipTool, dir, nil, "a.gz")
	if code != 2 || !strings.Contains(errb, "already has .gz suffix") {
		t.Errorf("suffix warning: code=%d err=%q", code, errb)
	}

	// decompressing unknown suffix: warning, exit 2
	writeFile(t, dir, "b.txt", "plain")
	_, errb, code = runTool(t, gzipTool, dir, nil, "-d", "b.txt")
	if code != 2 || !strings.Contains(errb, "unknown suffix") {
		t.Errorf("unknown suffix: code=%d err=%q", code, errb)
	}

	// not in gzip format: error, exit 1
	writeFile(t, dir, "c.gz", "this is not gzip data")
	_, errb, code = runTool(t, gzipTool, dir, nil, "-d", "c.gz")
	if code != 1 || !strings.Contains(errb, "not in gzip format") {
		t.Errorf("bad data: code=%d err=%q", code, errb)
	}
	if _, err := os.Stat(filepath.Join(dir, "c.gz")); err != nil {
		t.Errorf("failed decompress must keep the input: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "c")); !os.IsNotExist(err) {
		t.Errorf("failed decompress must not leave partial output")
	}

	// missing input: error, exit 1
	_, errb, code = runTool(t, gzipTool, dir, nil, "nope.txt")
	if code != 1 || !strings.Contains(errb, "No such file or directory") {
		t.Errorf("missing input: code=%d err=%q", code, errb)
	}

	// existing output without -f: warning; -f overwrites
	writeFile(t, dir, "d.txt", "fresh")
	writeFile(t, dir, "d.txt.gz", "stale")
	_, errb, code = runTool(t, gzipTool, dir, nil, "d.txt")
	if code != 2 || !strings.Contains(errb, "already exists; not overwritten") {
		t.Errorf("no-clobber: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, gzipTool, dir, nil, "-f", "d.txt")
	if code != 0 {
		t.Errorf("-f overwrite: code=%d err=%q", code, errb)
	}
	out, _, code := runTool(t, zcatTool, dir, nil, "d.txt.gz")
	if code != 0 || out != "fresh" {
		t.Errorf("-f result: code=%d out=%q", code, out)
	}

	// error in one operand + success in another: error wins (exit 1)
	writeFile(t, dir, "e.txt", "ok")
	_, _, code = runTool(t, gzipTool, dir, nil, "nope2.txt", "e.txt")
	if code != 1 {
		t.Errorf("mixed operands: code=%d", code)
	}
	if _, err := os.Stat(filepath.Join(dir, "e.txt.gz")); err != nil {
		t.Errorf("good operand should still be processed: %v", err)
	}

	// directory operand: warning
	_, errb, code = runTool(t, gzipTool, dir, nil, ".")
	if code != 2 || !strings.Contains(errb, "is a directory -- ignored") {
		t.Errorf("dir operand: code=%d err=%q", code, errb)
	}
}

func TestGunzipAndZcatAliases(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "alias test\n")
	if _, _, code := runTool(t, gzipTool, dir, nil, "-k", "f.txt"); code != 0 {
		t.Fatal("setup compress failed")
	}

	// zcat: decompress to stdout, suffix irrelevant, input kept
	out, _, code := runTool(t, zcatTool, dir, nil, "f.txt.gz")
	if code != 0 || out != "alias test\n" {
		t.Fatalf("zcat: code=%d out=%q", code, out)
	}
	if _, err := os.Stat(filepath.Join(dir, "f.txt.gz")); err != nil {
		t.Errorf("zcat must not remove input: %v", err)
	}
	// zcat works on gzip data without the .gz suffix
	raw, _ := os.ReadFile(filepath.Join(dir, "f.txt.gz"))
	writeFile(t, dir, "nosuffix", string(raw))
	out, _, code = runTool(t, zcatTool, dir, nil, "nosuffix")
	if code != 0 || out != "alias test\n" {
		t.Errorf("zcat without suffix: code=%d out=%q", code, out)
	}

	// gunzip: in-place decompress
	os.Remove(filepath.Join(dir, "f.txt"))
	_, errb, code := runTool(t, gunzipTool, dir, nil, "f.txt.gz")
	if code != 0 {
		t.Fatalf("gunzip: code=%d err=%q", code, errb)
	}
	got, err := os.ReadFile(filepath.Join(dir, "f.txt"))
	if err != nil || string(got) != "alias test\n" {
		t.Errorf("gunzip output = %q, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "f.txt.gz")); !os.IsNotExist(err) {
		t.Errorf("gunzip should remove the .gz input")
	}
}

func TestTgzSuffix(t *testing.T) {
	dir := t.TempDir()
	// build x.tgz: gzip data under a .tgz name
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	zw.Write([]byte("tar bytes pretend\n"))
	zw.Close()
	if err := os.WriteFile(filepath.Join(dir, "x.tgz"), buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, gunzipTool, dir, nil, "x.tgz")
	if code != 0 {
		t.Fatalf("gunzip x.tgz: code=%d err=%q", code, errb)
	}
	got, err := os.ReadFile(filepath.Join(dir, "x.tar"))
	if err != nil || string(got) != "tar bytes pretend\n" {
		t.Errorf("x.tgz should become x.tar: %q, %v", got, err)
	}
}

func TestMultipleOperands(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", "one")
	writeFile(t, dir, "b.txt", "two")
	_, _, code := runTool(t, gzipTool, dir, nil, "a.txt", "b.txt")
	if code != 0 {
		t.Fatalf("multi compress: code=%d", code)
	}
	for _, n := range []string{"a.txt.gz", "b.txt.gz"} {
		if _, err := os.Stat(filepath.Join(dir, n)); err != nil {
			t.Errorf("%s missing: %v", n, err)
		}
	}
	// -c concatenates members; gzip.Reader multistream reads them all
	out, _, code := runTool(t, zcatTool, dir, nil, "a.txt.gz", "b.txt.gz")
	if code != 0 || out != "onetwo" {
		t.Errorf("zcat concat: code=%d out=%q", code, out)
	}
}

func TestHelpVersionAndUnknownFlag(t *testing.T) {
	for _, tl := range []*tool.Tool{gzipTool, gunzipTool, zcatTool} {
		out, _, code := runTool(t, tl, t.TempDir(), nil, "--help")
		if code != 0 || !strings.Contains(out, "Usage: "+tl.Name) {
			t.Errorf("%s --help: code=%d out=%q", tl.Name, code, out)
		}
		out, _, code = runTool(t, tl, t.TempDir(), nil, "--version")
		if code != 0 || !strings.Contains(out, tl.Name) {
			t.Errorf("%s --version: code=%d out=%q", tl.Name, code, out)
		}
		_, errb, code := runTool(t, tl, t.TempDir(), nil, "--frobnicate")
		if code != 2 || !strings.Contains(errb, "frobnicate") || !strings.Contains(errb, "pure-Go") {
			t.Errorf("%s unknown flag: code=%d err=%q", tl.Name, code, errb)
		}
	}
}
