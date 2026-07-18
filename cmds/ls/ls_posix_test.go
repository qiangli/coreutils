package lscmd

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

	"github.com/qiangli/coreutils/tool"
)

// runToolEnv is runToolAt with an explicit environment.
func runToolEnv(t *testing.T, dir string, env []string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Env:   env,
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
	}
	code = cmd.Run(rc, args)
	return out.String(), errb.String(), code
}

// mkNames creates the named empty files in a fresh temp dir.
func mkNames(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, n := range names {
		write(t, dir, n, "")
	}
	return dir
}

// POSIX -m: entries are written as a comma-separated stream, with no
// trailing separator on the final entry.
func TestCommaFormatNoTrailingSeparator(t *testing.T) {
	dir := mkNames(t, "a.txt", "b.txt")
	out, _, code := runToolAt(t, dir, "-m")
	if want := "a.txt, b.txt\n"; code != 0 || out != want {
		t.Errorf("ls -m = (%q, %d), want (%q, 0)", out, code, want)
	}
}

// POSIX -m wraps the stream at the output line width.
func TestCommaFormatWrapsAtWidth(t *testing.T) {
	dir := mkNames(t, "aaaa", "bbbb", "cccc")
	out, _, code := runToolAt(t, dir, "-m", "-w", "12")
	if want := "aaaa, bbbb,\ncccc\n"; code != 0 || out != want {
		t.Errorf("ls -m -w 12 = (%q, %d), want (%q, 0)", out, code, want)
	}
}

// POSIX -C: multi-column output sorted down the columns.
func TestColumnsDown(t *testing.T) {
	dir := mkNames(t, "aa", "bb", "cc", "dd")
	out, _, code := runToolAt(t, dir, "-C", "-w", "12")
	if want := "aa  cc\nbb  dd\n"; code != 0 || out != want {
		t.Errorf("ls -C -w 12 = (%q, %d), want (%q, 0)", out, code, want)
	}
}

// POSIX -x: multi-column output sorted across the rows.
func TestColumnsAcross(t *testing.T) {
	dir := mkNames(t, "aa", "bb", "cc", "dd")
	out, _, code := runToolAt(t, dir, "-x", "-w", "12")
	if want := "aa  bb  cc\ndd\n"; code != 0 || out != want {
		t.Errorf("ls -x -w 12 = (%q, %d), want (%q, 0)", out, code, want)
	}
}

// A name too wide for the line falls back to one entry per line, and
// -w 0 means no limit at all.
func TestColumnsWidthEdges(t *testing.T) {
	dir := mkNames(t, "aaaaaaaaaaaaaaaa", "b")
	out, _, code := runToolAt(t, dir, "-C", "-w", "8")
	if want := "aaaaaaaaaaaaaaaa\nb\n"; code != 0 || out != want {
		t.Errorf("ls -C -w 8 = (%q, %d), want (%q, 0)", out, code, want)
	}
	out, _, code = runToolAt(t, dir, "-C", "-w", "0")
	if want := "aaaaaaaaaaaaaaaa  b\n"; code != 0 || out != want {
		t.Errorf("ls -C -w 0 = (%q, %d), want (%q, 0)", out, code, want)
	}
}

// The column width comes from COLUMNS when -w is absent.
func TestColumnsHonorsColumnsEnv(t *testing.T) {
	dir := mkNames(t, "aa", "bb", "cc", "dd")
	out, _, code := runToolEnv(t, dir, []string{"COLUMNS=12"}, "-C")
	if want := "aa  cc\nbb  dd\n"; code != 0 || out != want {
		t.Errorf("COLUMNS=12 ls -C = (%q, %d), want (%q, 0)", out, code, want)
	}
}

// POSIX: the format is chosen by the last format option on the command
// line, so -1 after -l selects one-entry-per-line.
func TestFormatLastOneWins(t *testing.T) {
	dir := mkNames(t, "a.txt")
	out, _, code := runToolAt(t, dir, "-l", "-1")
	if want := "a.txt\n"; code != 0 || out != want {
		t.Errorf("ls -l -1 = (%q, %d), want (%q, 0)", out, code, want)
	}
	out, _, code = runToolAt(t, dir, "-1", "-l")
	if code != 0 || !strings.HasPrefix(out, "total ") {
		t.Errorf("ls -1 -l = (%q, %d), want a long listing", out, code)
	}
}

// POSIX -q: write non-printable characters in file names as '?'.
func TestHideControlChars(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("control characters are not valid in Windows file names")
	}
	dir := t.TempDir()
	write(t, dir, "a\tb", "")
	out, _, code := runToolAt(t, dir, "-q")
	if want := "a?b\n"; code != 0 || out != want {
		t.Errorf("ls -q = (%q, %d), want (%q, 0)", out, code, want)
	}
	// --hide-control-chars is the long spelling of -q.
	out, _, code = runToolAt(t, dir, "--hide-control-chars")
	if want := "a?b\n"; code != 0 || out != want {
		t.Errorf("ls --hide-control-chars = (%q, %d), want (%q, 0)", out, code, want)
	}
	// Without -q the name is written literally (non-tty default).
	out, _, code = runToolAt(t, dir)
	if want := "a\tb\n"; code != 0 || out != want {
		t.Errorf("ls = (%q, %d), want (%q, 0)", out, code, want)
	}
}

// POSIX -s: the block count precedes each name, in every format, and a
// "total" line precedes a directory's entries.
func TestSizeBlocksShortFormat(t *testing.T) {
	dir := mkNames(t, "a.txt")
	out, _, code := runToolAt(t, dir, "-s")
	if code != 0 {
		t.Fatalf("ls -s exit = %d, out=%q", code, out)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("ls -s = %q, want a total line and one entry", out)
	}
	if !regexp.MustCompile(`^total \d+$`).MatchString(lines[0]) {
		t.Errorf("ls -s first line = %q, want a total line", lines[0])
	}
	if !regexp.MustCompile(`^ *\d+ a\.txt$`).MatchString(lines[1]) {
		t.Errorf("ls -s entry line = %q, want block count then name", lines[1])
	}
}

// --time-style selects the -l timestamp rendering; the ISO styles are
// the ones with a fixed, locale-independent shape.
func TestTimeStyles(t *testing.T) {
	dir := mkNames(t, "f")
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"-l", "--time-style=long-iso"}, `\d{4}-\d{2}-\d{2} \d{2}:\d{2}`},
		{[]string{"-l", "--time-style=full-iso"}, `\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\.\d{9} [+-]\d{4}`},
		{[]string{"-l", "--time-style=iso"}, `\d{2}-\d{2} \d{2}:\d{2}`},
		{[]string{"--full-time"}, `\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\.\d{9} [+-]\d{4}`},
	} {
		out, _, code := runToolAt(t, dir, tc.args...)
		if code != 0 {
			t.Fatalf("ls %v exit = %d, out = %q", tc.args, code, out)
		}
		if !regexp.MustCompile(tc.want + ` f\n$`).MatchString(out) {
			t.Errorf("ls %v = %q, want a timestamp matching %s", tc.args, out, tc.want)
		}
	}
	// An unknown style fails loudly rather than being ignored.
	if _, _, code := runToolAt(t, dir, "-l", "--time-style=bogus"); code != 2 {
		t.Errorf("ls --time-style=bogus exit = %d, want 2", code)
	}
}

// --si renders human-readable sizes in powers of 1000.
func TestSIHumanSizes(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "big", strings.Repeat("x", 2500))
	out, _, code := runToolAt(t, dir, "-l", "--si")
	if code != 0 || !strings.Contains(out, " 2.5k ") {
		t.Errorf("ls -l --si = (%q, %d), want a 2.5k size", out, code)
	}
	// -h keeps powers of 1024.
	out, _, code = runToolAt(t, dir, "-l", "-h")
	if code != 0 || !strings.Contains(out, " 2.5K ") {
		t.Errorf("ls -l -h = (%q, %d), want a 2.5K size", out, code)
	}
}

// --block-size scales the -s block counts and the -l size column.
func TestBlockSize(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "f", strings.Repeat("x", 3000))
	out, _, code := runToolAt(t, dir, "-l", "--block-size=K")
	if code != 0 || !strings.Contains(out, " 3 ") {
		t.Errorf("ls -l --block-size=K = (%q, %d), want a size of 3 (KiB, rounded up)", out, code)
	}
	out, _, code = runToolAt(t, dir, "-l", "--block-size=1000")
	if code != 0 || !strings.Contains(out, " 3 ") {
		t.Errorf("ls -l --block-size=1000 = (%q, %d), want a size of 3", out, code)
	}
	// -s block counts scale too: with a unit of 1 they are bytes, so a
	// strictly larger number than the default 1KiB units.
	out, _, code = runToolAt(t, dir, "-s", "--block-size=1")
	if code != 0 {
		t.Fatalf("ls -s --block-size=1 exit = %d, out = %q", code, out)
	}
	m := regexp.MustCompile(`^total (\d+)\n *(\d+) f\n$`).FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("ls -s --block-size=1 = %q, want a total and one entry", out)
	}
	if n, _ := strconv.Atoi(m[2]); n < 1024 {
		t.Errorf("ls -s --block-size=1 block count = %s, want a byte count", m[2])
	}
	if m[1] != m[2] {
		t.Errorf("ls -s --block-size=1 total = %s, want it to match the single entry %s", m[1], m[2])
	}
	// An unparsable size fails loudly.
	if _, _, code := runToolAt(t, dir, "--block-size=zzz"); code != 2 {
		t.Errorf("ls --block-size=zzz exit = %d, want 2", code)
	}
}

// -L makes ls report the referenced file, not the symlink, for entries
// found inside a listed directory.
func TestDereferenceDirectoryEntries(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs privileges on Windows")
	}
	dir := t.TempDir()
	write(t, dir, "f", "hello")
	if err := os.Symlink(filepath.Join(dir, "f"), filepath.Join(dir, "s")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	out, _, code := runToolAt(t, dir, "-l", "-L")
	if code != 0 {
		t.Fatalf("ls -lL exit = %d, out=%q", code, out)
	}
	if strings.Contains(out, " -> ") {
		t.Errorf("ls -lL = %q, want no symlink target", out)
	}
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if strings.HasPrefix(line, "l") {
			t.Errorf("ls -lL line = %q, want the referent's file type", line)
		}
	}
	if n := strings.Count(out, " 5 "); n != 2 {
		t.Errorf("ls -lL = %q, want both entries to report size 5", out)
	}
	// Without -L the link itself is reported.
	out, _, _ = runToolAt(t, dir, "-l")
	if !strings.Contains(out, " -> ") {
		t.Errorf("ls -l = %q, want a symlink target", out)
	}
}
