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
	"time"

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

// GNU documents -1 as ineffective when long format is already active.
func TestFormatLongTakesPrecedenceOverOne(t *testing.T) {
	dir := mkNames(t, "a.txt")
	out, _, code := runToolAt(t, dir, "-l", "-1")
	if code != 0 || !strings.HasPrefix(out, "total ") {
		t.Errorf("ls -l -1 = (%q, %d), want a long listing", out, code)
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
	entries := 0
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 5 && (fields[len(fields)-1] == "f" || fields[len(fields)-1] == "s") {
			entries++
			if fields[4] != "5" {
				t.Errorf("ls -lL line = %q, size = %q, want 5", line, fields[4])
			}
		}
	}
	if entries != 2 {
		t.Errorf("ls -lL = %q, found %d file entries, want 2", out, entries)
	}
	// Without -L the link itself is reported.
	out, _, _ = runToolAt(t, dir, "-l")
	if !strings.Contains(out, " -> ") {
		t.Errorf("ls -l = %q, want a symlink target", out)
	}
}

// GNU documents -1 as the exception to ordinary format-option ordering: it
// has no effect while long format is active. --format=single-column remains
// an explicit format transition and does replace long format.
func TestOrderLongAndOneFormat(t *testing.T) {
	dir := mkNames(t, "a.txt")
	for _, tc := range []struct {
		args     []string
		wantLong bool
	}{
		{[]string{"-l", "-1"}, true},
		{[]string{"-1", "-l"}, true},
		{[]string{"-l1"}, true},
		{[]string{"-1l"}, true},
		{[]string{"--long", "-1"}, true},
		{[]string{"-1", "--long"}, true},
		{[]string{"--format=single-column", "-l"}, true},
		{[]string{"-l", "--format=single-column"}, false},
		{[]string{"--format=long", "-1"}, true},
		{[]string{"-1", "--format=long"}, true},
		{[]string{"--full-time", "-1"}, true},
		{[]string{"-1", "--full-time"}, true},
		{[]string{"-l", "--form=single-column"}, false},
		{[]string{"--form=single-column", "-l"}, true},
		{[]string{"-l", "-C", "-1"}, false},
		{[]string{"-C", "-1", "-l"}, true},
		{[]string{"--format=long", "-1", "--format=single-column"}, false},
		{[]string{"--format", "single-column", "-1", "--format", "long", "-1"}, true},
		{[]string{"-l", "--format=single-column", "--long=false"}, false},
		{[]string{"-l", "--format=single-column", "--full-time=false"}, false},
		{[]string{"-l", "--zero"}, true},
		{[]string{"--zero", "-l"}, true},
	} {
		out, _, code := runToolAt(t, dir, tc.args...)
		if code != 0 {
			t.Fatalf("ls %v exit = %d, out = %q", tc.args, code, out)
		}
		isLong := strings.HasPrefix(out, "total ")
		if isLong != tc.wantLong {
			t.Errorf("ls %v: isLong = %v, want %v (out = %q)", tc.args, isLong, tc.wantLong, out)
		}
		if !tc.wantLong && out != "a.txt\n" {
			t.Errorf("ls %v = %q, want \"a.txt\\n\"", tc.args, out)
		}
	}
}

func TestFormatScannerDoesNotReadOptionValuesAsOptions(t *testing.T) {
	dir := mkNames(t, "-1", "a.txt")
	out, errb, code := runToolAt(t, dir, "-l", "--ignore", "-1")
	if code != 0 || errb != "" || !strings.HasPrefix(out, "total ") || strings.Contains(out, " -1\n") {
		t.Fatalf("ls -l --ignore -1 = (%q, %q, %d), want long output without -1", out, errb, code)
	}
}

// Numeric IDs imply long format too, so the same -1 exception applies.
func TestOrderNumericAndOneFormat(t *testing.T) {
	dir := mkNames(t, "a.txt")
	for _, tc := range []struct {
		args     []string
		wantLong bool
	}{
		{[]string{"-n", "-1"}, true},
		{[]string{"-1", "-n"}, true},
		{[]string{"-n1"}, true},
		{[]string{"-1n"}, true},
		{[]string{"--numeric-uid-gid", "-1"}, true},
		{[]string{"-1", "--numeric-uid-gid"}, true},
		{[]string{"--format=single-column", "-n"}, true},
		{[]string{"-n", "--format=single-column"}, false},
		{[]string{"--format=long", "-n"}, true},
		{[]string{"-n", "--format=long"}, true},
		{[]string{"-n", "--format=single-column", "--numeric-uid-gid=false"}, false},
	} {
		out, _, code := runToolAt(t, dir, tc.args...)
		if code != 0 {
			t.Fatalf("ls %v exit = %d, out = %q", tc.args, code, out)
		}
		isLong := strings.HasPrefix(out, "total ")
		if isLong != tc.wantLong {
			t.Errorf("ls %v: isLong = %v, want %v (out = %q)", tc.args, isLong, tc.wantLong, out)
		}
		if !tc.wantLong && out != "a.txt\n" {
			t.Errorf("ls %v = %q, want \"a.txt\\n\"", tc.args, out)
		}
	}
}

func TestUnsupportedDiredAndAutomaticClassifyFailClosed(t *testing.T) {
	dir := mkNames(t, "a.txt")
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--dired"}, "--dired (-D) is not supported"},
		{[]string{"-D"}, "--dired (-D) is not supported"},
		{[]string{"--dired", "--zero"}, "options --dired and --zero are incompatible"},
		{[]string{"--classify=auto"}, "--classify=auto is not supported"},
		{[]string{"--classify=auto", "--file-type=false"}, "--classify=auto is not supported"},
	} {
		out, errb, code := runToolAt(t, dir, tc.args...)
		if code != 2 || out != "" || !strings.Contains(errb, tc.want) {
			t.Errorf("ls %v = (%q, %q, %d), want (empty, diagnostic containing %q, 2)", tc.args, out, errb, code, tc.want)
		}
	}

	// A later implemented indicator mode supersedes auto, so no terminal
	// capability is needed and the invocation remains supported.
	out, errb, code := runToolAt(t, dir, "--classify=auto", "--indicator-style=none")
	if code != 0 || errb != "" || out != "a.txt\n" {
		t.Errorf("ls --classify=auto --indicator-style=none = (%q, %q, %d)", out, errb, code)
	}
}

func TestZeroPreservesLongFormatAndTerminatesLongRecords(t *testing.T) {
	dir := mkNames(t, "a.txt")
	out, errb, code := runToolAt(t, dir, "-l", "--zero")
	if code != 0 || errb != "" {
		t.Fatalf("ls -l --zero = (%q, %q, %d)", out, errb, code)
	}
	if !strings.HasPrefix(out, "total ") || strings.Contains(out, "\n") || strings.Count(out, "\x00") != 2 {
		t.Errorf("ls -l --zero = %q, want long total and entry records terminated by NUL", out)
	}
}

// Table-driven tests for indicator option ordering between -F, -p, --file-type,
// --indicator-style, and --classify (last-one-wins).
func TestOrderIndicatorClassifyAndSlash(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "file.txt", "content")
	runSh := filepath.Join(dir, "run.sh")
	if err := os.WriteFile(runSh, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	hasSymlink := true
	if err := os.Symlink("file.txt", filepath.Join(dir, "link")); err != nil {
		hasSymlink = false
	}

	for _, tc := range []struct {
		args    []string
		wantDir string
		wantRun string
		wantLnk string
	}{
		// -F vs -p separate and clustered
		{[]string{"-F", "-p"}, "subdir/", "run.sh", "link"},
		{[]string{"-p", "-F"}, "subdir/", "run.sh*", "link@"},
		{[]string{"-Fp"}, "subdir/", "run.sh", "link"},
		{[]string{"-pF"}, "subdir/", "run.sh*", "link@"},
		// --file-type vs -F / -p
		{[]string{"--file-type", "-F"}, "subdir/", "run.sh*", "link@"},
		{[]string{"-F", "--file-type"}, "subdir/", "run.sh", "link@"},
		{[]string{"-p", "--file-type"}, "subdir/", "run.sh", "link@"},
		{[]string{"--file-type", "-p"}, "subdir/", "run.sh", "link"},
		// --indicator-style
		{[]string{"--indicator-style=slash", "-F"}, "subdir/", "run.sh*", "link@"},
		{[]string{"-F", "--indicator-style=slash"}, "subdir/", "run.sh", "link"},
		{[]string{"--indicator-style=file-type", "-F"}, "subdir/", "run.sh*", "link@"},
		{[]string{"-F", "--indicator-style=file-type"}, "subdir/", "run.sh", "link@"},
		{[]string{"--indicator-style=none", "-F"}, "subdir/", "run.sh*", "link@"},
		{[]string{"-F", "--indicator-style=none"}, "subdir", "run.sh", "link"},
		{[]string{"-F", "--indicator-style=none", "--file-type=false"}, "subdir", "run.sh", "link"},
		{[]string{"-p", "--indicator-style=none"}, "subdir", "run.sh", "link"},
		// --classify
		{[]string{"--classify=never", "-F"}, "subdir/", "run.sh*", "link@"},
		{[]string{"-F", "--classify=never"}, "subdir", "run.sh", "link"},
		{[]string{"-p", "--classify"}, "subdir/", "run.sh*", "link@"},
		{[]string{"--classify", "-p"}, "subdir/", "run.sh", "link"},
	} {
		out, _, code := runToolAt(t, dir, tc.args...)
		if code != 0 {
			t.Fatalf("ls %v exit = %d, out = %q", tc.args, code, out)
		}
		lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
		// Verify directory indicator
		if !containsLine(lines, tc.wantDir) {
			t.Errorf("ls %v: missing dir entry %q in %v", tc.args, tc.wantDir, lines)
		}
		// Verify executable indicator (on Unix; Windows doesn't have 0111 exec permission bits)
		if runtime.GOOS != "windows" {
			if !containsLine(lines, tc.wantRun) {
				t.Errorf("ls %v: missing exec entry %q in %v", tc.args, tc.wantRun, lines)
			}
		}
		// Verify symlink indicator
		if hasSymlink {
			if !containsLine(lines, tc.wantLnk) {
				t.Errorf("ls %v: missing link entry %q in %v", tc.args, tc.wantLnk, lines)
			}
		}
		// Verify regular file indicator is always without suffix
		if !containsLine(lines, "file.txt") {
			t.Errorf("ls %v: missing regular file entry %q in %v", tc.args, "file.txt", lines)
		}
	}
}

// The -c/-u/--time pre-scan must not read option arguments as selectors:
// an attached -I value or a separate --ignore value is data, never flags
// (GNU getopt: -I and --ignore take a required argument).
func TestTimeSelectorIgnoresOptionValues(t *testing.T) {
	dir := t.TempDir()
	p := write(t, dir, "a.txt", "")
	atime := time.Date(2002, 3, 4, 5, 6, 7, 0, time.Local)
	mtime := time.Date(2001, 2, 3, 4, 5, 6, 0, time.Local)
	if err := os.Chtimes(p, atime, mtime); err != nil {
		t.Fatal(err)
	}
	mtimeOut, _, code := runToolAt(t, dir, "-l", "--time-style=long-iso")
	if code != 0 || !strings.Contains(mtimeOut, "2001-02-03 04:05") {
		t.Fatalf("ls -l --time-style=long-iso = (%q, %d), want the mtime", mtimeOut, code)
	}
	for _, args := range [][]string{
		{"-l", "--time-style=long-iso", "-Ic"},            // attached value "c"
		{"-l", "--time-style=long-iso", "-Iu"},            // attached value "u"
		{"-l", "--time-style=long-iso", "--ignore", "-c"}, // separate value
		{"-l", "--time-style=long-iso", "--ignore", "-u"},
	} {
		out, errb, code := runToolAt(t, dir, args...)
		if code != 0 || errb != "" || out != mtimeOut {
			t.Errorf("ls %v = (%q, %q, %d), want the mtime listing %q", args, out, errb, code, mtimeOut)
		}
	}
	// Control: a real -u after the pattern still selects the access time.
	if out, _, _ := runToolAt(t, dir, "-l", "--time-style=long-iso", "-Ic", "-u"); !strings.Contains(out, "2002-03-04 05:06") {
		t.Errorf("ls -l -Ic -u = %q, want the 2002-03-04 access time", out)
	}
}

// The -a/-A last-one-wins pre-scan must not read an -I value as a flag.
func TestAllAlmostAllIgnoresIgnorePatternValues(t *testing.T) {
	dir := mkNames(t, "a.txt")
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"-a", "-A", "-Ia"}, "a.txt\n"},        // -A is the last real flag
		{[]string{"-A", "-a", "-IA"}, ".\n..\na.txt\n"}, // -a is the last real flag
	} {
		out, errb, code := runToolAt(t, dir, tc.args...)
		if code != 0 || errb != "" || out != tc.want {
			t.Errorf("ls %v = (%q, %q, %d), want (%q, empty, 0)", tc.args, out, errb, code, tc.want)
		}
	}
}

func TestOrderIndicatorLongAbbreviationsAndOptionValues(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, dir, "-Fp", "ignored")
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"-F", "--indicator=none", "-I-Fp"}, "subdir\n"},
		{[]string{"--indicator=none", "-F", "-I-Fp"}, "subdir/\n"},
		{[]string{"-F", "--class=never", "-I-Fp"}, "subdir\n"},
		// An attached -I value is data even when it contains an F or p.
		{[]string{"-F", "-I-Fp"}, "subdir/\n"},
	} {
		out, errb, code := runToolAt(t, dir, tc.args...)
		if code != 0 || errb != "" || out != tc.want {
			t.Errorf("ls %v = (%q, %q, %d), want (%q, empty, 0)", tc.args, out, errb, code, tc.want)
		}
	}
}

func containsLine(lines []string, target string) bool {
	for _, l := range lines {
		if l == target {
			return true
		}
	}
	return false
}
