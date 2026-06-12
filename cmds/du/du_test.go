package ducmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

// runToolAt is the canonical test harness shape for cmds packages,
// with an explicit working directory.
func runToolAt(t *testing.T, dir string, args ...string) (stdout, stderr string, code int) {
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

func runTool(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	return runToolAt(t, t.TempDir(), args...)
}

func write(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// mkTree builds dir/{f1(10B), f2(20B), sub/f3(30B)}.
func mkTree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	root := filepath.Join(dir, "tree")
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, root, "f1", strings.Repeat("a", 10))
	write(t, root, "f2", strings.Repeat("b", 20))
	write(t, filepath.Join(root, "sub"), "f3", strings.Repeat("c", 30))
	return dir
}

// parseLines splits du output into (value, path) pairs.
func parseLines(t *testing.T, out string) (vals []int64, paths []string) {
	t.Helper()
	for _, ln := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
		parts := strings.SplitN(ln, "\t", 2)
		if len(parts) != 2 {
			t.Fatalf("malformed du line %q", ln)
		}
		n, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			t.Fatalf("non-numeric du value in %q", ln)
		}
		vals = append(vals, n)
		paths = append(paths, parts[1])
	}
	return
}

func TestFileOperand(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "f", "hello")
	out, _, code := runToolAt(t, dir, "-b", "f")
	if code != 0 || out != "5\tf\n" {
		t.Errorf("du -b f = (%q, %d), want (\"5\\tf\\n\", 0)", out, code)
	}
}

func TestDirPostOrder(t *testing.T) {
	dir := mkTree(t)
	out, _, code := runToolAt(t, dir, "-b", "tree")
	if code != 0 {
		t.Fatalf("du -b tree code = %d", code)
	}
	vals, paths := parseLines(t, out)
	if len(paths) != 2 || paths[0] != "tree/sub" || paths[1] != "tree" {
		t.Fatalf("du order = %v, want [tree/sub tree]", paths)
	}
	if vals[0] < 30 || vals[1] < 60 || vals[1] < vals[0] {
		t.Errorf("du values = %v, want sub >= 30 and total >= 60", vals)
	}
}

func TestAll(t *testing.T) {
	dir := mkTree(t)
	out, _, code := runToolAt(t, dir, "-ab", "tree")
	if code != 0 {
		t.Fatalf("du -ab code = %d", code)
	}
	_, paths := parseLines(t, out)
	want := []string{"tree/f1", "tree/f2", "tree/sub/f3", "tree/sub", "tree"}
	if strings.Join(paths, " ") != strings.Join(want, " ") {
		t.Errorf("du -ab paths = %v, want %v", paths, want)
	}
}

func TestSummarize(t *testing.T) {
	dir := mkTree(t)
	out, _, code := runToolAt(t, dir, "-sb", "tree")
	if code != 0 {
		t.Fatalf("du -sb code = %d", code)
	}
	vals, paths := parseLines(t, out)
	if len(paths) != 1 || paths[0] != "tree" || vals[0] < 60 {
		t.Errorf("du -sb = (%v, %v)", vals, paths)
	}
}

func TestMaxDepth(t *testing.T) {
	dir := mkTree(t)
	// -d 0 behaves like -s.
	_, paths0 := parseDu(t, dir, "-b", "-d", "0", "tree")
	if strings.Join(paths0, " ") != "tree" {
		t.Errorf("du -d 0 paths = %v", paths0)
	}
	_, paths1 := parseDu(t, dir, "-b", "-d", "1", "tree")
	if strings.Join(paths1, " ") != "tree/sub tree" {
		t.Errorf("du -d 1 paths = %v", paths1)
	}
	// --max-depth long form.
	_, pathsL := parseDu(t, dir, "-b", "--max-depth=1", "tree")
	if strings.Join(pathsL, " ") != "tree/sub tree" {
		t.Errorf("du --max-depth=1 paths = %v", pathsL)
	}
}

func parseDu(t *testing.T, dir string, args ...string) ([]int64, []string) {
	t.Helper()
	out, _, code := runToolAt(t, dir, args...)
	if code != 0 {
		t.Fatalf("du %v code = %d", args, code)
	}
	vals, paths := parseLines(t, out)
	return vals, paths
}

func TestGrandTotal(t *testing.T) {
	dir := mkTree(t)
	write(t, dir, "single", "12345")
	vals, paths := parseDu(t, dir, "-cb", "single", "tree")
	n := len(paths)
	if n < 3 || paths[n-1] != "total" {
		t.Fatalf("du -cb paths = %v, want trailing total", paths)
	}
	if vals[n-1] != vals[0]+vals[n-2] {
		t.Errorf("du -cb total = %d, want %d", vals[n-1], vals[0]+vals[n-2])
	}
}

func TestHuman(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "f", strings.Repeat("x", 2048))
	out, _, code := runToolAt(t, dir, "-h", "f")
	if code != 0 || !regexp.MustCompile(`^\d+(\.\d)?[KMGTPE]?\tf\n$`).MatchString(out) {
		t.Errorf("du -h f = (%q, %d)", out, code)
	}
}

func TestDefaultUnitIs1K(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "f", strings.Repeat("x", 5000))
	out, _, code := runToolAt(t, dir, "f")
	if code != 0 {
		t.Fatalf("du f code = %d", code)
	}
	vals, _ := parseLines(t, out)
	// 5000 bytes is at least 5 1K-blocks but far fewer than 5000:
	// proves the value is in 1024-byte units, not bytes.
	if vals[0] < 5 || vals[0] > 64 {
		t.Errorf("du f = %d 1K-blocks, want a small block count", vals[0])
	}
}

func TestConflictingFlags(t *testing.T) {
	_, errb, code := runTool(t, "-s", "-a", ".")
	if code != 2 || !strings.Contains(errb, "cannot both summarize and show all entries") {
		t.Errorf("-s -a: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, "-s", "-d", "1", ".")
	if code != 2 || !strings.Contains(errb, "summarizing conflicts with --max-depth") {
		t.Errorf("-s -d: code=%d err=%q", code, errb)
	}
}

func TestErrors(t *testing.T) {
	_, errb, code := runTool(t, "nope")
	if code != 1 || !strings.Contains(errb, "cannot access 'nope'") {
		t.Errorf("missing operand file: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, "--frobnicate")
	if code != 2 || !strings.Contains(errb, "frobnicate") || !strings.Contains(errb, "pure-Go") {
		t.Errorf("unknown flag: code=%d err=%q", code, errb)
	}
}

func TestHelpAndVersion(t *testing.T) {
	out, _, code := runTool(t, "--help")
	if code != 0 || !strings.Contains(out, "Usage: du") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	out, _, code = runTool(t, "--version")
	if code != 0 || !strings.Contains(out, "du") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}
}
