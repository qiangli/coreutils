package splitcmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func runTool(t *testing.T, dir, stdin string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	if dir == "" {
		dir = t.TempDir()
	}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Stdio: tool.Stdio{In: strings.NewReader(stdin), Out: &out, Err: &errb},
	}
	code = cmd.Run(rc, args)
	return out.String(), errb.String(), code
}

func listFiles(t *testing.T, dir string) []string {
	t.Helper()
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range ents {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names
}

func readFile(t *testing.T, dir, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestSplitLines(t *testing.T) {
	dir := t.TempDir()
	_, _, code := runTool(t, dir, "1\n2\n3\n", "-l", "1")
	if code != 0 {
		t.Fatalf("split -l 1: code=%d", code)
	}
	if got := listFiles(t, dir); !equal(got, []string{"xaa", "xab", "xac"}) {
		t.Fatalf("files: %v", got)
	}
	if readFile(t, dir, "xaa") != "1\n" || readFile(t, dir, "xab") != "2\n" || readFile(t, dir, "xac") != "3\n" {
		t.Error("wrong contents")
	}
}

func TestSplitObsoleteShorthand(t *testing.T) {
	dir := t.TempDir()
	_, _, code := runTool(t, dir, "1\n2\n3\n", "-2")
	if code != 0 {
		t.Fatalf("split -2: code=%d", code)
	}
	if got := listFiles(t, dir); !equal(got, []string{"xaa", "xab"}) {
		t.Fatalf("files: %v", got)
	}
	if readFile(t, dir, "xaa") != "1\n2\n" || readFile(t, dir, "xab") != "3\n" {
		t.Error("wrong contents")
	}
}

func TestSplitBytes(t *testing.T) {
	dir := t.TempDir()
	_, _, code := runTool(t, dir, "abcdefghij", "-b", "4")
	if code != 0 {
		t.Fatalf("split -b 4: code=%d", code)
	}
	if got := listFiles(t, dir); !equal(got, []string{"xaa", "xab", "xac"}) {
		t.Fatalf("files: %v", got)
	}
	if readFile(t, dir, "xaa") != "abcd" || readFile(t, dir, "xab") != "efgh" || readFile(t, dir, "xac") != "ij" {
		t.Error("wrong contents")
	}

	// SIZE suffixes are accepted.
	dir2 := t.TempDir()
	if _, _, code := runTool(t, dir2, "small", "-b", "1K"); code != 0 {
		t.Fatalf("split -b 1K: code=%d", code)
	}
	if got := listFiles(t, dir2); !equal(got, []string{"xaa"}) {
		t.Fatalf("files: %v", got)
	}
}

func TestSplitNumericAndSuffixLen(t *testing.T) {
	dir := t.TempDir()
	if _, _, code := runTool(t, dir, "1\n2\n", "-d", "-l", "1"); code != 0 {
		t.Fatal("split -d failed")
	}
	if got := listFiles(t, dir); !equal(got, []string{"x00", "x01"}) {
		t.Fatalf("numeric suffixes: %v", got)
	}

	dir2 := t.TempDir()
	if _, _, code := runTool(t, dir2, "1\n2\n", "-a", "1", "-l", "1"); code != 0 {
		t.Fatal("split -a 1 failed")
	}
	if got := listFiles(t, dir2); !equal(got, []string{"xa", "xb"}) {
		t.Fatalf("-a 1 suffixes: %v", got)
	}
}

func TestSplitSuffixExhaustion(t *testing.T) {
	dir := t.TempDir()
	// 11 pieces with fixed numeric width 1 -> only 10 names exist.
	input := strings.Repeat("l\n", 11)
	_, errb, code := runTool(t, dir, input, "-d", "-a", "1", "-l", "1")
	if code != 1 || !strings.Contains(errb, "output file suffixes exhausted") {
		t.Errorf("exhaustion: err=%q code=%d", errb, code)
	}
}

func TestSplitAutoWiden(t *testing.T) {
	dir := t.TempDir()
	// 91 pieces, numeric, auto width: x00..x89 then x9000 (GNU
	// reserved-last-symbol scheme).
	var b strings.Builder
	for i := 0; i < 91; i++ {
		fmt.Fprintf(&b, "%d\n", i)
	}
	if _, _, code := runTool(t, dir, b.String(), "-d", "-l", "1"); code != 0 {
		t.Fatal("split auto-widen failed")
	}
	got := listFiles(t, dir)
	if len(got) != 91 || got[0] != "x00" || got[89] != "x89" || got[90] != "x9000" {
		t.Errorf("auto-widen names: n=%d first=%s last=%s", len(got), got[0], got[len(got)-1])
	}
}

func TestSplitChunks(t *testing.T) {
	dir := t.TempDir()
	if _, _, code := runTool(t, dir, "abcdefghij", "-n", "3"); code != 0 {
		t.Fatal("split -n 3 failed")
	}
	if got := listFiles(t, dir); !equal(got, []string{"xaa", "xab", "xac"}) {
		t.Fatalf("files: %v", got)
	}
	if readFile(t, dir, "xaa") != "abc" || readFile(t, dir, "xab") != "def" || readFile(t, dir, "xac") != "ghij" {
		t.Error("wrong chunk contents")
	}

	// l/N keeps lines intact.
	dir2 := t.TempDir()
	if _, _, code := runTool(t, dir2, "aaaa\nbb\ncc\n", "-n", "l/2"); code != 0 {
		t.Fatal("split -n l/2 failed")
	}
	whole := ""
	for _, name := range listFiles(t, dir2) {
		content := readFile(t, dir2, name)
		whole += content
		if content != "" && !strings.HasSuffix(content, "\n") {
			t.Errorf("chunk %s splits a line: %q", name, content)
		}
	}
	if whole != "aaaa\nbb\ncc\n" {
		t.Errorf("l/2 lost data: %q", whole)
	}

	// Unsupported chunk forms fail loudly.
	_, errb, code := runTool(t, "", "x", "-n", "2/4")
	if code != 2 || !strings.Contains(errb, "not supported") {
		t.Errorf("-n K/N: err=%q code=%d", errb, code)
	}
}

func TestSplitOperandsAndPrefix(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "in.txt"), []byte("1\n2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, code := runTool(t, dir, "", "-l", "1", "in.txt", "part_"); code != 0 {
		t.Fatal("split with prefix failed")
	}
	got := listFiles(t, dir)
	if !equal(got, []string{"in.txt", "part_aa", "part_ab"}) {
		t.Fatalf("files: %v", got)
	}

	_, errb, code := runTool(t, "", "", "a", "b", "c")
	if code != 2 || !strings.Contains(errb, "extra operand") {
		t.Errorf("extra operand: err=%q code=%d", errb, code)
	}
}

func TestSplitErrors(t *testing.T) {
	_, errb, code := runTool(t, "", "", "-l", "0")
	if code != 2 || !strings.Contains(errb, "invalid number of lines") {
		t.Errorf("-l 0: err=%q code=%d", errb, code)
	}

	_, errb, code = runTool(t, "", "", "-b", "x")
	if code != 2 || !strings.Contains(errb, "invalid number of bytes") {
		t.Errorf("-b x: err=%q code=%d", errb, code)
	}

	_, errb, code = runTool(t, "", "", "-l", "1", "-b", "2")
	if code != 2 || !strings.Contains(errb, "cannot split in more than one way") {
		t.Errorf("two modes: err=%q code=%d", errb, code)
	}

	_, errb, code = runTool(t, "", "", "missing")
	if code != 1 || !strings.Contains(errb, "cannot open 'missing' for reading") {
		t.Errorf("missing file: err=%q code=%d", errb, code)
	}

	_, errb, code = runTool(t, "", "", "--numeric-suffixes=5")
	if code != 2 || !strings.Contains(errb, "not supported") {
		t.Errorf("numeric-suffixes=5: err=%q code=%d", errb, code)
	}

	_, errb, code = runTool(t, "", "", "--frobnicate")
	if code != 2 || !strings.Contains(errb, "frobnicate") || !strings.Contains(errb, "pure-Go") {
		t.Errorf("unknown flag: err=%q code=%d", errb, code)
	}
}

func TestSplitHelpVersion(t *testing.T) {
	out, _, code := runTool(t, "", "", "--help")
	if code != 0 || !strings.Contains(out, "Usage: split") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	out, _, code = runTool(t, "", "", "--version")
	if code != 0 || !strings.Contains(out, "split") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
