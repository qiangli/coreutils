package findcmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/coreutils/tool"
)

// runFind is the canonical test harness shape for cmds packages.
func runFind(t *testing.T, dir string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	if dir == "" {
		dir = t.TempDir()
	}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
	}
	code = cmd.Run(rc, args)
	return out.String(), errb.String(), code
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func setupTree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", "hello")      // 5 bytes
	writeFile(t, dir, "b.go", "package b\n") // 10 bytes
	writeFile(t, dir, "empty.txt", "")
	writeFile(t, dir, "skipme/e.txt", "x\n")
	writeFile(t, dir, "sub/c.txt", "0123456789") // 10 bytes
	writeFile(t, dir, "sub/deep/d.go", "package d\n")
	return dir
}

func TestFindDefaultPrintLexical(t *testing.T) {
	dir := setupTree(t)
	out, _, code := runFind(t, dir, ".")
	want := ".\n./a.txt\n./b.go\n./empty.txt\n./skipme\n./skipme/e.txt\n./sub\n./sub/c.txt\n./sub/deep\n./sub/deep/d.go\n"
	if out != want || code != 0 {
		t.Errorf("find . = (%q, %d), want (%q, 0)", out, code, want)
	}
}

func TestFindTests(t *testing.T) {
	dir := setupTree(t)
	cases := []struct {
		args []string
		want string
	}{
		{[]string{".", "-name", "*.go"}, "./b.go\n./sub/deep/d.go\n"},
		{[]string{".", "-iname", "A.TXT"}, "./a.txt\n"},
		{[]string{".", "-type", "d"}, ".\n./skipme\n./sub\n./sub/deep\n"},
		{[]string{".", "-type", "f", "-name", "c.txt"}, "./sub/c.txt\n"},
		{[]string{".", "-maxdepth", "1", "-type", "f"}, "./a.txt\n./b.go\n./empty.txt\n"},
		{[]string{".", "-mindepth", "2", "-name", "*.txt"}, "./skipme/e.txt\n./sub/c.txt\n"},
		{[]string{".", "-maxdepth", "0"}, ".\n"},
		{[]string{".", "-path", "*deep*"}, "./sub/deep\n./sub/deep/d.go\n"},
		{[]string{".", "-path", "./sub/*", "-type", "f"}, "./sub/c.txt\n./sub/deep/d.go\n"},
		{[]string{".", "-type", "f", "-size", "5c"}, "./a.txt\n"},
		{[]string{".", "-type", "f", "-size", "+5c"}, "./b.go\n./sub/c.txt\n./sub/deep/d.go\n"},
		{[]string{".", "-size", "-6c", "-type", "f", "-name", "*.txt", "!", "-empty"},
			"./a.txt\n./skipme/e.txt\n"},
		// GNU round-up gotcha: -size -1k matches only 0-byte files
		{[]string{".", "-type", "f", "-size", "-1k"}, "./empty.txt\n"},
		{[]string{".", "-empty"}, "./empty.txt\n"},
		// default block unit: every small file is 1 512-block
		{[]string{".", "-type", "f", "-size", "1", "-name", "a*"}, "./a.txt\n"},
		// operators
		{[]string{".", "-name", "skipme", "-prune", "-o", "-name", "*.txt", "-print"},
			"./a.txt\n./empty.txt\n./sub/c.txt\n"},
		{[]string{".", "-type", "f", "!", "-name", "*.txt"}, "./b.go\n./sub/deep/d.go\n"},
		{[]string{".", "-type", "f", "-not", "-name", "*.txt"}, "./b.go\n./sub/deep/d.go\n"},
		{[]string{".", "(", "-name", "a.txt", "-o", "-name", "b.go", ")"}, "./a.txt\n./b.go\n"},
		{[]string{".", "-name", "a.txt", "-a", "-type", "f"}, "./a.txt\n"},
		// explicit operand path prefixes output
		{[]string{"sub", "-name", "*.go"}, "sub/deep/d.go\n"},
		{[]string{"sub/", "-type", "d"}, "sub/\nsub/deep\n"},
		// multiple start points
		{[]string{"skipme", "sub", "-name", "*.txt"}, "skipme/e.txt\nsub/c.txt\n"},
	}
	for _, c := range cases {
		out, errb, code := runFind(t, dir, c.args...)
		if out != c.want || code != 0 {
			t.Errorf("find %v = (%q, %d, err=%q), want (%q, 0)", c.args, out, code, errb, c.want)
		}
	}
}

func TestFindDefaultPath(t *testing.T) {
	dir := setupTree(t)
	out, _, code := runFind(t, dir, "-name", "a.txt")
	if out != "./a.txt\n" || code != 0 {
		t.Errorf("default path: out=%q code=%d", out, code)
	}
}

func TestFindPrint0(t *testing.T) {
	dir := setupTree(t)
	out, _, code := runFind(t, dir, ".", "-name", "*.go", "-print0")
	if out != "./b.go\x00./sub/deep/d.go\x00" || code != 0 {
		t.Errorf("-print0: out=%q code=%d", out, code)
	}
}

func TestFindMtimeAndNewer(t *testing.T) {
	dir := setupTree(t)
	old := filepath.Join(dir, "old.txt")
	writeFile(t, dir, "old.txt", "old\n")
	threeDays := time.Now().Add(-72*time.Hour - time.Hour)
	if err := os.Chtimes(old, threeDays, threeDays); err != nil {
		t.Fatal(err)
	}

	out, _, code := runFind(t, dir, ".", "-type", "f", "-mtime", "+2")
	if out != "./old.txt\n" || code != 0 {
		t.Errorf("-mtime +2: out=%q code=%d", out, code)
	}
	out, _, _ = runFind(t, dir, ".", "-name", "old.txt", "-mtime", "0")
	if out != "" {
		t.Errorf("-mtime 0 matched old file: out=%q", out)
	}
	out, _, _ = runFind(t, dir, ".", "-name", "old.txt", "-mtime", "3")
	if out != "./old.txt\n" {
		t.Errorf("-mtime 3: out=%q", out)
	}

	out, _, code = runFind(t, dir, ".", "-type", "f", "!", "-newer", "old.txt")
	if out != "./old.txt\n" || code != 0 {
		t.Errorf("! -newer: out=%q code=%d", out, code)
	}
	out, _, _ = runFind(t, dir, ".", "-newer", "old.txt", "-name", "*.go")
	if out != "./b.go\n./sub/deep/d.go\n" {
		t.Errorf("-newer: out=%q", out)
	}
}

func TestFindTypeSymlink(t *testing.T) {
	dir := setupTree(t)
	if err := os.Symlink(filepath.Join(dir, "a.txt"), filepath.Join(dir, "link")); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}
	out, _, code := runFind(t, dir, ".", "-type", "l")
	if out != "./link\n" || code != 0 {
		t.Errorf("-type l: out=%q code=%d", out, code)
	}
	// -type f must not match the symlink (lstat semantics, -P default)
	out, _, _ = runFind(t, dir, ".", "-name", "link", "-type", "f")
	if out != "" {
		t.Errorf("-type f matched symlink: out=%q", out)
	}
}

func TestFindErrors(t *testing.T) {
	dir := setupTree(t)

	_, errb, code := runFind(t, dir, "missing")
	if code != 1 || !strings.Contains(errb, "missing") {
		t.Errorf("missing start point: code=%d err=%q", code, errb)
	}
	_, errb, code = runFind(t, dir, ".", "-frobnicate")
	if code != 2 || !strings.Contains(errb, "unknown predicate '-frobnicate'") {
		t.Errorf("unknown predicate: code=%d err=%q", code, errb)
	}
	_, errb, code = runFind(t, dir, ".", "-name")
	if code != 2 || !strings.Contains(errb, "missing argument to '-name'") {
		t.Errorf("missing argument: code=%d err=%q", code, errb)
	}
	_, errb, code = runFind(t, dir, ".", "-name", "x", "stray")
	if code != 2 || !strings.Contains(errb, "paths must precede expression") {
		t.Errorf("stray operand: code=%d err=%q", code, errb)
	}
	_, errb, code = runFind(t, dir, ".", "(", "-name", "x")
	if code != 2 || !strings.Contains(errb, "expected ')'") {
		t.Errorf("unmatched paren: code=%d err=%q", code, errb)
	}
	_, errb, code = runFind(t, dir, ".", "-type", "q")
	if code != 2 || !strings.Contains(errb, "unknown argument to -type") {
		t.Errorf("bad type: code=%d err=%q", code, errb)
	}
	_, errb, code = runFind(t, dir, ".", "-mtime", "x")
	if code != 2 || !strings.Contains(errb, "-mtime") {
		t.Errorf("bad mtime: code=%d err=%q", code, errb)
	}
	for _, action := range []string{"-exec", "-delete", "-ok"} {
		_, errb, code = runFind(t, dir, ".", action)
		if code != 2 || !strings.Contains(errb, "not supported") {
			t.Errorf("%s: code=%d err=%q", action, code, errb)
		}
	}
}

func TestFindHelpAndVersion(t *testing.T) {
	out, _, code := runFind(t, "", "--help")
	if code != 0 || !strings.Contains(out, "Usage: find") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	out, _, code = runFind(t, "", "--version")
	if code != 0 || !strings.Contains(out, "find") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}
}

func TestFnmatch(t *testing.T) {
	cases := []struct {
		pat, s string
		fold   bool
		want   bool
	}{
		{"*.go", "main.go", false, true},
		{"*.go", "main.gox", false, false},
		{"a?c", "abc", false, true},
		{"[a-c]x", "bx", false, true},
		{"[!a-c]x", "bx", false, false},
		{"[!a-c]x", "dx", false, true},
		{"[]x]", "]", false, true},
		{"*deep*", "./sub/deep/d.go", false, true}, // '*' crosses '/'
		{"a/?/b", "a/x/b", false, true},            // '?' crosses '/' too
		{`a\*b`, "a*b", false, true},
		{`a\*b`, "axb", false, false},
		{"ABC", "abc", true, true},
		{"[A-Z]x", "bx", true, true},
		{"[[:digit:]]*", "7z", false, true},
		{"[[:digit:]]*", "z7", false, false},
	}
	for _, c := range cases {
		if got := fnmatch(c.pat, c.s, c.fold); got != c.want {
			t.Errorf("fnmatch(%q, %q, fold=%v) = %v, want %v", c.pat, c.s, c.fold, got, c.want)
		}
	}
}
