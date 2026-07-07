package ducmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/coreutils/tool"
)

// runToolAt is the canonical test harness shape for cmds packages,
// with an explicit working directory.
func runToolAt(t *testing.T, dir string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	return runToolAtWithInput(t, dir, "", args...)
}

func runToolAtWithInput(t *testing.T, dir, input string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Stdio: tool.Stdio{In: strings.NewReader(input), Out: &out, Err: &errb},
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

func TestApparentSizeDoesNotForceBytes(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "f", strings.Repeat("x", 1536))
	out, _, code := runToolAt(t, dir, "-A", "f")
	if code != 0 || out != "2\tf\n" {
		t.Errorf("du -A f = (%q, %d), want apparent size in default 1K units", out, code)
	}
	out, _, code = runToolAt(t, dir, "-b", "f")
	if code != 0 || out != "1536\tf\n" {
		t.Errorf("du -b f = (%q, %d), want apparent bytes", out, code)
	}
}

func TestBlockSizeCluster(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "f", "x")
	out, errb, code := runToolAt(t, dir, "-A", "-BM", "f")
	if code != 0 || errb != "" || out != "1\tf\n" {
		t.Fatalf("du -A -BM f = (%q, %q, %d), want 1 MiB block", out, errb, code)
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

func TestVerboseShowsAllFiles(t *testing.T) {
	dir := mkTree(t)
	out, _, code := runToolAt(t, dir, "-vb", "tree")
	if code != 0 {
		t.Fatalf("du -vb code = %d", code)
	}
	_, paths := parseLines(t, out)
	want := []string{"tree/f1", "tree/f2", "tree/sub/f3", "tree/sub", "tree"}
	if strings.Join(paths, " ") != strings.Join(want, " ") {
		t.Errorf("du -vb paths = %v, want %v", paths, want)
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

func TestSeparateDirs(t *testing.T) {
	dir := mkTree(t)
	vals, paths := parseDu(t, dir, "-bS", "tree")
	if strings.Join(paths, " ") != "tree/sub tree" {
		t.Fatalf("du -bS paths = %v", paths)
	}
	if vals[0] != 30 || vals[1] != 30 {
		t.Errorf("du -bS values = %v, want sub=30 root direct files=30", vals)
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

func TestSymlinkDereferenceModes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires privileges on many Windows setups")
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "target"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, "target"), "f", "1234567")
	if err := os.Symlink("target", filepath.Join(dir, "link")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	out, _, code := runToolAt(t, dir, "-sb", "link")
	if code != 0 {
		t.Fatalf("du -sb link code = %d", code)
	}
	if out == "7\tlink\n" {
		t.Fatalf("du -sb link followed symlink by default: %q", out)
	}

	for _, flag := range []string{"-D", "-H", "--dereference-args"} {
		out, _, code = runToolAt(t, dir, "-sb", flag, "link")
		if code != 0 || out != "7\tlink\n" {
			t.Errorf("du -sb %s link = (%q, %d), want dereferenced target", flag, out, code)
		}
	}

	if err := os.MkdirAll(filepath.Join(dir, "root"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../target", filepath.Join(dir, "root", "alias")); err != nil {
		t.Fatal(err)
	}
	_, paths := parseDu(t, dir, "-abL", "root")
	if strings.Join(paths, " ") != "root/alias/f root/alias root" {
		t.Errorf("du -abL root paths = %v, want traversal through symlinked directory", paths)
	}
	_, paths = parseDu(t, dir, "-abP", "root")
	if strings.Join(paths, " ") != "root/alias root" {
		t.Errorf("du -abP root paths = %v, want symlink itself only", paths)
	}
}

func TestCountLinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("os.FileInfo link counts are not available on Windows")
	}
	dir := t.TempDir()
	write(t, dir, "f", "data")
	if err := os.Link(filepath.Join(dir, "f"), filepath.Join(dir, "g")); err != nil {
		t.Skipf("hard links unavailable: %v", err)
	}
	vals, paths := parseDu(t, dir, "-cb", "f", "g")
	if strings.Join(paths, " ") != "f total" || vals[1] != 4 {
		t.Errorf("du -cb hardlinks = (%v, %v), want second link deduped in total", vals, paths)
	}
	vals, paths = parseDu(t, dir, "-clb", "f", "g")
	if strings.Join(paths, " ") != "f g total" || vals[2] != 8 {
		t.Errorf("du -clb hardlinks = (%v, %v), want both links counted", vals, paths)
	}
}

func TestInodes(t *testing.T) {
	dir := mkTree(t)
	out, _, code := runToolAt(t, dir, "-s", "--inodes", "tree")
	if code != 0 || out != "5\ttree\n" {
		t.Errorf("du -s --inodes tree = (%q, %d), want five filesystem entries", out, code)
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

func TestFiles0FromFileAndStdin(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "a", "abc")
	write(t, dir, "b", "12345")
	write(t, dir, "list", "a\x00b\x00")

	out, _, code := runToolAt(t, dir, "-b", "--files0-from=list")
	if code != 0 {
		t.Fatalf("--files0-from file code = %d", code)
	}
	vals, paths := parseLines(t, out)
	if strings.Join(paths, " ") != "a b" || vals[0] != 3 || vals[1] != 5 {
		t.Errorf("--files0-from file = (%v, %v)", vals, paths)
	}

	out, _, code = runToolAtWithInput(t, dir, "b\x00", "-b", "--files0-from=-")
	if code != 0 || out != "5\tb\n" {
		t.Errorf("--files0-from stdin = (%q, %d)", out, code)
	}

	_, errb, code := runToolAt(t, dir, "--files0-from=list", "a")
	if code != 2 || !strings.Contains(errb, "file operands cannot be combined") {
		t.Errorf("--files0-from operands: code=%d err=%q", code, errb)
	}
}

func TestNullOutputTerminator(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "f", "hello")
	out, _, code := runToolAt(t, dir, "-0b", "f")
	if code != 0 || out != "5\tf\x00" {
		t.Errorf("du -0b f = (%q, %d), want NUL-terminated line", out, code)
	}
	out, _, code = runToolAt(t, dir, "--null", "-b", "f")
	if code != 0 || out != "5\tf\x00" {
		t.Errorf("du --null -b f = (%q, %d), want NUL-terminated line", out, code)
	}
}

func TestExcludeAndExcludeFrom(t *testing.T) {
	dir := mkTree(t)
	out, _, code := runToolAt(t, dir, "-ab", "--exclude=f2", "tree")
	if code != 0 {
		t.Fatalf("--exclude code = %d", code)
	}
	_, paths := parseLines(t, out)
	if strings.Contains(strings.Join(paths, " "), "tree/f2") || !strings.Contains(strings.Join(paths, " "), "tree/f1") {
		t.Errorf("--exclude paths = %v", paths)
	}

	write(t, dir, "patterns", "sub\n")
	out, _, code = runToolAt(t, dir, "-ab", "--exclude-from=patterns", "tree")
	if code != 0 {
		t.Fatalf("--exclude-from code = %d", code)
	}
	_, paths = parseLines(t, out)
	got := strings.Join(paths, " ")
	if strings.Contains(got, "tree/sub") || !strings.Contains(got, "tree/f1") || !strings.Contains(got, "tree") {
		t.Errorf("--exclude-from paths = %v", paths)
	}

	write(t, dir, "patterns-x", "f1\n")
	out, _, code = runToolAt(t, dir, "-ab", "-X", "patterns-x", "tree")
	if code != 0 {
		t.Fatalf("-X code = %d", code)
	}
	_, paths = parseLines(t, out)
	got = strings.Join(paths, " ")
	if strings.Contains(got, "tree/f1") || !strings.Contains(got, "tree/f2") {
		t.Errorf("-X paths = %v", paths)
	}
}

func TestThreshold(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "small", strings.Repeat("s", 10))
	write(t, dir, "big", strings.Repeat("b", 30))
	out, _, code := runToolAt(t, dir, "-b", "-t", "20", "small", "big")
	if code != 0 || out != "30\tbig\n" {
		t.Errorf("du -b -t 20 = (%q, %d), want only big file", out, code)
	}
	out, _, code = runToolAt(t, dir, "-b", "--threshold=-20", "small", "big")
	if code != 0 || out != "10\tsmall\n" {
		t.Errorf("du -b --threshold=-20 = (%q, %d), want only small file", out, code)
	}
}

func TestTimeStyle(t *testing.T) {
	dir := t.TempDir()
	p := write(t, dir, "f", "hello")
	ts := time.Date(2020, 3, 4, 5, 6, 7, 0, time.Local)
	if err := os.Chtimes(p, ts, ts); err != nil {
		t.Fatal(err)
	}
	out, _, code := runToolAt(t, dir, "-b", "--time=mtime", "--time-style=+%Y-%m-%d", "f")
	if code != 0 || out != "5\t2020-03-04\tf\n" {
		t.Errorf("du --time --time-style = (%q, %d)", out, code)
	}
	out, _, code = runToolAt(t, dir, "-b", "--time-style=iso", "f")
	if code != 0 || out != "5\t2020-03-04\tf\n" {
		t.Errorf("du --time-style=iso = (%q, %d)", out, code)
	}
}

func TestBlockSizeModes(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "f", strings.Repeat("x", 1536))
	out, _, code := runToolAt(t, dir, "-b", "-B", "512", "f")
	if code != 0 || out != "3\tf\n" {
		t.Errorf("-b -B 512 = (%q, %d), want 3 blocks", out, code)
	}
	out, _, code = runToolAt(t, dir, "-b", "-k", "f")
	if code != 0 || out != "2\tf\n" {
		t.Errorf("-b -k = (%q, %d), want 2 KiB blocks", out, code)
	}
	out, _, code = runToolAt(t, dir, "-b", "-m", "f")
	if code != 0 || out != "1\tf\n" {
		t.Errorf("-b -m = (%q, %d), want 1 MiB block", out, code)
	}
	out, _, code = runToolAt(t, dir, "-b", "--block-size=1K", "f")
	if code != 0 || out != "2\tf\n" {
		t.Errorf("--block-size=1K = (%q, %d), want 2 blocks", out, code)
	}
}

func TestSIHumanReadable(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "f", strings.Repeat("x", 1001))
	out, _, code := runToolAt(t, dir, "-b", "--si", "f")
	if code != 0 || out != "1.1K\tf\n" {
		t.Errorf("du -b --si f = (%q, %d), want SI human output", out, code)
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
	if !strings.Contains(out, "-h, --human-readable") {
		t.Errorf("--help output should preserve semantic -h:\n%s", out)
	}
	if strings.Contains(out, "-h, --help") {
		t.Errorf("--help output should not advertise -h as help when semantic:\n%s", out)
	}
	if !strings.Contains(out, "-V, --version") {
		t.Errorf("--help output missing -V version alias:\n%s", out)
	}
	out, _, code = runTool(t, "--version")
	if code != 0 || !strings.Contains(out, "du") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}
	out, _, code = runTool(t, "-V")
	if code != 0 || !strings.Contains(out, "du") {
		t.Errorf("-V: code=%d out=%q", code, out)
	}
}
