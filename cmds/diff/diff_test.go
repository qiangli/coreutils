package diffcmd

import (
	"bytes"
	"context"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/coreutils/tool"
)

// runIn is the canonical runTool harness shape, parameterized by the
// invocation working directory so tests can stage files first.
func runIn(t *testing.T, dir, stdin string, args ...string) (stdout, stderr string, code int) {
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

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestNormalFormat(t *testing.T) {
	cases := []struct {
		name, a, b, want string
		code             int
	}{
		{"identical", "a\nb\n", "a\nb\n", "", 0},
		{"change", "apple\nbanana\ncherry\n", "apple\nberry\ncherry\n",
			"2c2\n< banana\n---\n> berry\n", 1},
		{"append", "1\n2\n", "1\n2\n3\n", "2a3\n> 3\n", 1},
		{"delete", "1\n2\n3\n", "1\n3\n", "2d1\n< 2\n", 1},
		{"prepend", "2\n", "1\n2\n", "0a1\n> 1\n", 1},
		{"empty-vs-lines", "", "a\nb\n", "0a1,2\n> a\n> b\n", 1},
		{"lines-vs-empty", "a\nb\n", "", "1,2d0\n< a\n< b\n", 1},
		{"multiline-change", "a\nx\ny\nd\n", "a\nP\nQ\nR\nd\n",
			"2,3c2,4\n< x\n< y\n---\n> P\n> Q\n> R\n", 1},
		// trailing-newline difference is a line difference in GNU
		{"noeol-new", "x\n", "x",
			"1c1\n< x\n---\n> x\n" + noNewline + "\n", 1},
		{"noeol-both", "a\nb", "a\nc",
			"2c2\n< b\n" + noNewline + "\n---\n> c\n" + noNewline + "\n", 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, "a.txt", c.a)
			writeFile(t, dir, "b.txt", c.b)
			out, errb, code := runIn(t, dir, "", "a.txt", "b.txt")
			if out != c.want || code != c.code {
				t.Errorf("diff a b = (%q, %d), want (%q, %d); stderr=%q", out, code, c.want, c.code, errb)
			}
		})
	}
}

func TestUnifiedGolden(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", "apple\nbanana\ncherry\n")
	writeFile(t, dir, "b.txt", "apple\nberry\ncherry\n")
	ta := time.Date(2026, 3, 4, 5, 6, 7, 0, time.Local)
	tb := time.Date(2026, 3, 4, 6, 7, 8, 0, time.Local)
	if err := os.Chtimes(filepath.Join(dir, "a.txt"), ta, ta); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filepath.Join(dir, "b.txt"), tb, tb); err != nil {
		t.Fatal(err)
	}
	want := "--- a.txt\t" + ta.Format(stampLayout) + "\n" +
		"+++ b.txt\t" + tb.Format(stampLayout) + "\n" +
		"@@ -1,3 +1,3 @@\n" +
		" apple\n" +
		"-banana\n" +
		"+berry\n" +
		" cherry\n"
	for _, args := range [][]string{
		{"-u", "a.txt", "b.txt"},
		{"-U3", "a.txt", "b.txt"},
		{"-U", "3", "a.txt", "b.txt"},
		{"--unified=3", "a.txt", "b.txt"},
		{"--unified", "a.txt", "b.txt"},
	} {
		out, _, code := runIn(t, dir, "", args...)
		if out != want || code != 1 {
			t.Errorf("diff %v:\n got (%q, %d)\nwant (%q, 1)", args, out, code, want)
		}
	}
}

// GNU stamp shape: "2026-03-04 05:06:07.000000000 -0700" (nanoseconds
// always 9 digits, numeric zone).
func TestStampShape(t *testing.T) {
	s := time.Date(2026, 3, 4, 5, 6, 7, 123456789, time.UTC).Format(stampLayout)
	if s != "2026-03-04 05:06:07.123456789 +0000" {
		t.Errorf("stamp = %q", s)
	}
}

func TestUnifiedEdgeRanges(t *testing.T) {
	cases := []struct {
		name, a, b string
		args       []string
		wantHunks  string // output after the two header lines
	}{
		{"U0-delete", "1\n2\n3\n", "1\n3\n", []string{"-U0"},
			"@@ -2 +1,0 @@\n-2\n"},
		{"U0-insert", "1\n3\n", "1\n2\n3\n", []string{"-U0"},
			"@@ -1,0 +2 @@\n+2\n"},
		{"empty-old", "", "a\nb\n", []string{"-u"},
			"@@ -0,0 +1,2 @@\n+a\n+b\n"},
		{"empty-new", "a\nb\n", "", []string{"-u"},
			"@@ -1,2 +0,0 @@\n-a\n-b\n"},
		{"append", "1\n2\n", "1\n2\n3\n", []string{"-u"},
			"@@ -1,2 +1,3 @@\n 1\n 2\n+3\n"},
		{"noeol-marker", "x\n", "x", []string{"-u"},
			"@@ -1 +1 @@\n-x\n+x\n" + noNewline + "\n"},
		{"noeol-context", "1\n2", "one\n2", []string{"-u"},
			"@@ -1,2 +1,2 @@\n-1\n+one\n 2\n" + noNewline + "\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, "a", c.a)
			writeFile(t, dir, "b", c.b)
			out, _, code := runIn(t, dir, "", append(c.args, "a", "b")...)
			lines := strings.SplitN(out, "\n", 3)
			if len(lines) < 3 {
				t.Fatalf("short output %q", out)
			}
			if !strings.HasPrefix(lines[0], "--- a\t") || !strings.HasPrefix(lines[1], "+++ b\t") {
				t.Errorf("headers wrong: %q / %q", lines[0], lines[1])
			}
			if lines[2] != c.wantHunks || code != 1 {
				t.Errorf("hunks = (%q, %d), want (%q, 1)", lines[2], code, c.wantHunks)
			}
		})
	}
}

// GNU merges hunks separated by at most 2*context unchanged lines.
func TestUnifiedHunkMerge(t *testing.T) {
	mk := func(n int, repl map[int]string) string {
		var sb strings.Builder
		for i := 1; i <= n; i++ {
			if r, ok := repl[i]; ok {
				sb.WriteString(r + "\n")
			} else {
				sb.WriteString(strings.Repeat("l", 1) + itoa(i) + "\n")
			}
		}
		return sb.String()
	}
	dir := t.TempDir()
	writeFile(t, dir, "a", mk(20, nil))
	// gap of 6 (== 2*3): one hunk
	writeFile(t, dir, "b", mk(20, map[int]string{5: "five", 12: "twelve"}))
	out, _, _ := runIn(t, dir, "", "-u", "a", "b")
	if strings.Count(out, "@@") != 2 { // one hunk header == two "@@"
		t.Errorf("gap=2C: want 1 hunk, got:\n%s", out)
	}
	// gap of 7 (> 2*3): two hunks
	writeFile(t, dir, "c", mk(20, map[int]string{5: "five", 13: "thirteen"}))
	out, _, _ = runIn(t, dir, "", "-u", "a", "c")
	if strings.Count(out, "@@") != 4 {
		t.Errorf("gap=2C+1: want 2 hunks, got:\n%s", out)
	}
}

func itoa(i int) string {
	return string(rune('0'+i/10)) + string(rune('0'+i%10))
}

func TestBrief(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a", "x\n")
	writeFile(t, dir, "b", "y\n")
	writeFile(t, dir, "c", "x\n")
	out, _, code := runIn(t, dir, "", "-q", "a", "b")
	if out != "Files a and b differ\n" || code != 1 {
		t.Errorf("-q differ: (%q, %d)", out, code)
	}
	out, _, code = runIn(t, dir, "", "-q", "a", "c")
	if out != "" || code != 0 {
		t.Errorf("-q same: (%q, %d)", out, code)
	}
}

func TestIgnoreFlags(t *testing.T) {
	cases := []struct {
		name, a, b string
		args       []string
		code       int
	}{
		{"i-equal", "Hello World\n", "hello world\n", []string{"-i"}, 0},
		{"i-off", "Hello\n", "hello\n", nil, 1},
		{"b-runs-equal", "a b\nx\n", "a  b \nx\n", []string{"-b"}, 0},
		{"b-leading-runs-equal", " a\nx\n", "  a\nx\n", []string{"-b"}, 0},
		{"b-presence-differs", " a\nx\n", "a\nx\n", []string{"-b"}, 1},
		{"b-not-w", "a b\nx\n", "ab\nx\n", []string{"-b"}, 1},
		{"w-equal", "a b\nx\n", "ab\nx\n", []string{"-w"}, 0},
		{"w-tabs-equal", "a\tb c\n", "abc\n", []string{"-w"}, 0},
		{"iw-combined", "A B\n", "ab\n", []string{"-i", "-w"}, 0},
		{"q-respects-w", "a b\n", "ab\n", []string{"-q", "-w"}, 0},
		// GNU treats a missing newline at EOF as an ignorable
		// white-space difference under -b / -w, but not under -i
		{"w-noeol-equal", "x\n", "x", []string{"-w"}, 0},
		{"b-noeol-equal", "x\n", "x", []string{"-b"}, 0},
		{"i-noeol-differs", "x\n", "x", []string{"-i"}, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, "a", c.a)
			writeFile(t, dir, "b", c.b)
			out, errb, code := runIn(t, dir, "", append(c.args, "a", "b")...)
			if code != c.code {
				t.Errorf("code = %d, want %d (out=%q err=%q)", code, c.code, out, errb)
			}
		})
	}
}

func TestBinary(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "b1", "bin\x00ary\n")
	writeFile(t, dir, "b2", "bin\x00aryX\n")
	writeFile(t, dir, "b3", "bin\x00ary\n")
	out, _, code := runIn(t, dir, "", "b1", "b2")
	if out != "Binary files b1 and b2 differ\n" || code != 1 {
		t.Errorf("binary differ: (%q, %d)", out, code)
	}
	out, _, code = runIn(t, dir, "", "b1", "b3")
	if out != "" || code != 0 {
		t.Errorf("binary same: (%q, %d)", out, code)
	}
	out, _, code = runIn(t, dir, "", "-q", "b1", "b2")
	if out != "Files b1 and b2 differ\n" || code != 1 {
		t.Errorf("binary -q: (%q, %d)", out, code)
	}
}

func TestNewFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "b.txt", "hi\nthere\n")
	// without -N: trouble
	out, errb, code := runIn(t, dir, "", "absent.txt", "b.txt")
	if code != 2 || !strings.Contains(errb, "diff: absent.txt: No such file or directory") {
		t.Errorf("missing without -N: code=%d err=%q out=%q", code, errb, out)
	}
	// with -N: treated as empty
	out, _, code = runIn(t, dir, "", "-N", "absent.txt", "b.txt")
	if out != "0a1,2\n> hi\n> there\n" || code != 1 {
		t.Errorf("-N normal: (%q, %d)", out, code)
	}
	// unified: absent side stamped with the epoch
	out, _, code = runIn(t, dir, "", "-uN", "absent.txt", "b.txt")
	wantHead := "--- absent.txt\t" + time.Unix(0, 0).Format(stampLayout) + "\n"
	if !strings.HasPrefix(out, wantHead) || code != 1 {
		t.Errorf("-uN header: (%q, %d), want prefix %q", out, code, wantHead)
	}
	if !strings.Contains(out, "@@ -0,0 +1,2 @@\n+hi\n+there\n") {
		t.Errorf("-uN hunk: %q", out)
	}
}

func makeTree(t *testing.T, dir string) {
	writeFile(t, dir, "old/same.txt", "same\n")
	writeFile(t, dir, "new/same.txt", "same\n")
	writeFile(t, dir, "old/diff.txt", "old\n")
	writeFile(t, dir, "new/diff.txt", "new\n")
	writeFile(t, dir, "old/only_old.txt", "a\n")
	writeFile(t, dir, "new/only_new.txt", "b\n")
	writeFile(t, dir, "old/sub/inner.txt", "i1\n")
	writeFile(t, dir, "new/sub/inner.txt", "i2\n")
}

func TestRecursiveGolden(t *testing.T) {
	dir := t.TempDir()
	makeTree(t, dir)
	want := "diff -r old/diff.txt new/diff.txt\n" +
		"1c1\n< old\n---\n> new\n" +
		"Only in new: only_new.txt\n" +
		"Only in old: only_old.txt\n" +
		"diff -r old/sub/inner.txt new/sub/inner.txt\n" +
		"1c1\n< i1\n---\n> i2\n"
	out, errb, code := runIn(t, dir, "", "-r", "old", "new")
	if out != want || code != 1 {
		t.Errorf("diff -r:\n got (%q, %d)\nwant (%q, 1); stderr=%q", out, code, want, errb)
	}
}

func TestRecursiveUnifiedHeaderEcho(t *testing.T) {
	dir := t.TempDir()
	makeTree(t, dir)
	// the per-pair header echoes option tokens as typed ("-ru")
	out, _, code := runIn(t, dir, "", "-ru", "old", "new")
	if code != 1 {
		t.Fatalf("code = %d", code)
	}
	for _, want := range []string{
		"diff -ru old/diff.txt new/diff.txt\n--- old/diff.txt\t",
		"diff -ru old/sub/inner.txt new/sub/inner.txt\n--- old/sub/inner.txt\t",
		"\n-old\n+new\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("-ru output missing %q:\n%s", want, out)
		}
	}
}

func TestRecursiveBrief(t *testing.T) {
	dir := t.TempDir()
	makeTree(t, dir)
	want := "Files old/diff.txt and new/diff.txt differ\n" +
		"Only in new: only_new.txt\n" +
		"Only in old: only_old.txt\n" +
		"Files old/sub/inner.txt and new/sub/inner.txt differ\n"
	out, _, code := runIn(t, dir, "", "-qr", "old", "new")
	if out != want || code != 1 {
		t.Errorf("diff -qr:\n got (%q, %d)\nwant (%q, 1)", out, code, want)
	}
}

func TestRecursiveNewFile(t *testing.T) {
	dir := t.TempDir()
	makeTree(t, dir)
	out, _, code := runIn(t, dir, "", "-r", "-N", "old", "new")
	if code != 1 {
		t.Fatalf("code = %d", code)
	}
	for _, want := range []string{
		"diff -r -N old/only_new.txt new/only_new.txt\n0a1\n> b\n",
		"diff -r -N old/only_old.txt new/only_old.txt\n1d0\n< a\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("-rN output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Only in") {
		t.Errorf("-rN should not print Only in:\n%s", out)
	}
}

func TestNonRecursiveDirs(t *testing.T) {
	dir := t.TempDir()
	makeTree(t, dir)
	out, _, code := runIn(t, dir, "", "old", "new")
	if !strings.Contains(out, "Common subdirectories: old/sub and new/sub\n") || code != 1 {
		t.Errorf("no -r: (%q, %d)", out, code)
	}
	if strings.Contains(out, "inner.txt") {
		t.Errorf("no -r must not descend:\n%s", out)
	}
}

func TestMixedTypes(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "old/typemix/keep", "x\n")
	writeFile(t, dir, "new/typemix", "f\n")
	out, _, code := runIn(t, dir, "", "-r", "old", "new")
	want := "File old/typemix is a directory while file new/typemix is a regular file\n"
	if out != want || code != 1 {
		t.Errorf("mixed types: (%q, %d), want (%q, 1)", out, code, want)
	}
}

func TestFileVsDir(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "same.txt", "same\n")
	writeFile(t, dir, "old/same.txt", "same\n")
	_, _, code := runIn(t, dir, "", "same.txt", "old")
	if code != 0 {
		t.Errorf("file vs dir same: code = %d", code)
	}
	writeFile(t, dir, "old/same.txt", "other\n")
	out, _, code := runIn(t, dir, "", "same.txt", "old")
	if out != "1c1\n< same\n---\n> other\n" || code != 1 {
		t.Errorf("file vs dir differ: (%q, %d)", out, code)
	}
}

func TestStdin(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "b", "x\n")
	out, _, code := runIn(t, dir, "y\n", "-q", "-", "b")
	if out != "Files - and b differ\n" || code != 1 {
		t.Errorf("stdin differ: (%q, %d)", out, code)
	}
	out, _, code = runIn(t, dir, "x\n", "-", "b")
	if out != "" || code != 0 {
		t.Errorf("stdin same: (%q, %d)", out, code)
	}
}

func TestErrors(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a", "x\n")
	_, errb, code := runIn(t, dir, "")
	if code != 2 || !strings.Contains(errb, "missing operand after 'diff'") {
		t.Errorf("no operands: code=%d err=%q", code, errb)
	}
	_, errb, code = runIn(t, dir, "", "a")
	if code != 2 || !strings.Contains(errb, "missing operand after 'a'") {
		t.Errorf("one operand: code=%d err=%q", code, errb)
	}
	_, errb, code = runIn(t, dir, "", "a", "a", "a")
	if code != 2 || !strings.Contains(errb, "extra operand 'a'") {
		t.Errorf("three operands: code=%d err=%q", code, errb)
	}
	_, errb, code = runIn(t, dir, "", "nope", "a")
	if code != 2 || !strings.Contains(errb, "diff: nope: No such file or directory") {
		t.Errorf("missing file: code=%d err=%q", code, errb)
	}
	_, errb, code = runIn(t, dir, "", "-U", "x", "a", "a")
	if code != 2 || !strings.Contains(errb, "invalid context length 'x'") {
		t.Errorf("bad -U: code=%d err=%q", code, errb)
	}
	_, errb, code = runIn(t, dir, "", "a", "a", "-U")
	if code != 2 || !strings.Contains(errb, "option requires an argument -- 'U'") {
		t.Errorf("-U no arg: code=%d err=%q", code, errb)
	}
	// Unknown flag: contract error, exit 2, names the flag.
	_, errb, code = runIn(t, dir, "", "--frobnicate", "a", "a")
	if code != 2 || !strings.Contains(errb, "frobnicate") || !strings.Contains(errb, "pure-Go") {
		t.Errorf("unknown flag: code=%d err=%q", code, errb)
	}
	_, errb, code = runIn(t, dir, "", "-y", "a", "a")
	if code != 2 || !strings.Contains(errb, "y") {
		t.Errorf("unknown short flag: code=%d err=%q", code, errb)
	}
}

func TestHelpAndVersion(t *testing.T) {
	dir := t.TempDir()
	out, _, code := runIn(t, dir, "", "--help")
	if code != 0 || !strings.Contains(out, "Usage: diff") || !strings.Contains(out, "-U NUM") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	out, _, code = runIn(t, dir, "", "--version")
	if code != 0 || !strings.Contains(out, "diff") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}
}

// ---------------------------------------------------------------------------
// Engine: validity + minimality cross-check against an LCS DP oracle.

func lcsLen(a, b []int) int {
	dp := make([]int, len(b)+1)
	for i := 1; i <= len(a); i++ {
		prev := 0 // dp[i-1][j-1]
		for j := 1; j <= len(b); j++ {
			cur := dp[j]
			if a[i-1] == b[j-1] {
				dp[j] = prev + 1
			} else if dp[j-1] > dp[j] {
				dp[j] = dp[j-1]
			}
			prev = cur
		}
	}
	return dp[len(b)]
}

func TestMyersRandomized(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	for it := 0; it < 500; it++ {
		a := make([]int, rng.Intn(15))
		b := make([]int, rng.Intn(15))
		for i := range a {
			a[i] = rng.Intn(4)
		}
		for i := range b {
			b[i] = rng.Intn(4)
		}
		ops := myersOps(a, b)
		ai, bi, edits := 0, 0, 0
		for _, k := range ops {
			switch k {
			case opEq:
				if ai >= len(a) || bi >= len(b) || a[ai] != b[bi] {
					t.Fatalf("invalid Eq at a[%d] b[%d] for %v %v: %v", ai, bi, a, b, ops)
				}
				ai++
				bi++
			case opDel:
				ai++
				edits++
			case opIns:
				bi++
				edits++
			}
		}
		if ai != len(a) || bi != len(b) {
			t.Fatalf("script does not cover inputs: %v %v -> %v", a, b, ops)
		}
		if want := len(a) + len(b) - 2*lcsLen(a, b); edits != want {
			t.Fatalf("non-minimal script for %v %v: %d edits, want %d", a, b, edits, want)
		}
	}
}

func TestMyersLargeSmoke(t *testing.T) {
	if testing.Short() {
		t.Skip("large smoke test")
	}
	n := 5000
	a := make([]int, n)
	b := make([]int, 0, n+100)
	for i := range a {
		a[i] = i % 97
	}
	for i := range a {
		if i%50 == 7 {
			b = append(b, 1000+i) // replacement
			continue
		}
		b = append(b, a[i])
		if i%73 == 11 {
			b = append(b, 2000+i) // insertion
		}
	}
	ops := myersOps(a, b)
	ai, bi := 0, 0
	for _, k := range ops {
		switch k {
		case opEq:
			if a[ai] != b[bi] {
				t.Fatal("invalid Eq")
			}
			ai++
			bi++
		case opDel:
			ai++
		case opIns:
			bi++
		}
	}
	if ai != len(a) || bi != len(b) {
		t.Fatal("script does not cover inputs")
	}
}
