package dfcmd

import (
	"bytes"
	"context"
	"regexp"
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

func TestDefaultListing(t *testing.T) {
	out, errb, code := runTool(t)
	if code != 0 {
		t.Fatalf("df code = %d, stderr = %q", code, errb)
	}
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("df output = %q, want header + at least one mount", out)
	}
	hdr := lines[0]
	// POSIX XSI default units are 512-byte blocks.
	for _, col := range []string{"Filesystem", "512-blocks", "Used", "Available", "Use%", "Mounted on"} {
		if !strings.Contains(hdr, col) {
			t.Errorf("header %q missing column %q", hdr, col)
		}
	}
}

func TestKFlagSelects1024(t *testing.T) {
	for _, argv := range [][]string{{"-k"}, {"--kibibytes"}} {
		out, _, code := runTool(t, argv...)
		if code != 0 || !strings.Contains(firstLine(out), "1024-blocks") {
			t.Errorf("df %v: code=%d header = %q, want 1024-blocks", argv, code, firstLine(out))
		}
	}
}

// TestDefaultUnitsAreHalfOfK pins the unit relationship on live data:
// the same file system reports twice as many 512-byte blocks as
// 1024-byte blocks (within rounding).
func TestDefaultUnitsAreHalfOfK(t *testing.T) {
	const total = uint64(1 << 20)
	if got := fmtValue(total, scaleMode{blockSize: 512}); got != "2048" {
		t.Errorf("512-byte units for 1MiB = %s, want 2048", got)
	}
	if got := fmtValue(total, scaleMode{blockSize: 1024}); got != "1024" {
		t.Errorf("1024-byte units for 1MiB = %s, want 1024", got)
	}
	// Partial blocks round up.
	if got := fmtValue(1025, scaleMode{blockSize: 512}); got != "3" {
		t.Errorf("512-byte units for 1025 bytes = %s, want 3", got)
	}
}

func TestHumanHeader(t *testing.T) {
	out, _, code := runTool(t, "-h")
	if code != 0 {
		t.Fatalf("df -h code = %d", code)
	}
	hdr := firstLine(out)
	for _, col := range []string{"Filesystem", "Size", "Used", "Avail", "Use%", "Mounted on"} {
		if !strings.Contains(hdr, col) {
			t.Errorf("df -h header %q missing %q", hdr, col)
		}
	}
	if strings.Contains(hdr, "1K-blocks") {
		t.Errorf("df -h header still shows 1K-blocks: %q", hdr)
	}
}

func TestSIHeader(t *testing.T) {
	out, _, code := runTool(t, "-H")
	if code != 0 {
		t.Fatalf("df -H code = %d", code)
	}
	hdr := firstLine(out)
	for _, col := range []string{"Filesystem", "Size", "Used", "Avail", "Use%", "Mounted on"} {
		if !strings.Contains(hdr, col) {
			t.Errorf("df -H header %q missing %q", hdr, col)
		}
	}
}

func TestBlockSizeHeader(t *testing.T) {
	out, _, code := runTool(t, "-B", "1M")
	if code != 0 {
		t.Fatalf("df -B 1M code = %d", code)
	}
	if hdr := firstLine(out); !strings.Contains(hdr, "1048576B-blocks") {
		t.Errorf("df -B 1M header = %q, want custom block header", hdr)
	}

	out, _, code = runTool(t, "-BM")
	if code != 0 {
		t.Fatalf("df -BM code = %d", code)
	}
	if hdr := firstLine(out); !strings.Contains(hdr, "1048576B-blocks") {
		t.Errorf("df -BM header = %q, want custom block header", hdr)
	}
}

func TestPortablePrintTypeAndInodesHeaders(t *testing.T) {
	// POSIX -P without -k: 512-blocks and Capacity.
	out, _, code := runTool(t, "-P")
	if code != 0 {
		t.Fatalf("df -P code = %d", code)
	}
	hdr0 := firstLine(out)
	for _, col := range []string{"Filesystem", "512-blocks", "Used", "Available", "Capacity", "Mounted on"} {
		if !strings.Contains(hdr0, col) {
			t.Errorf("df -P header = %q, missing %q", hdr0, col)
		}
	}
	if strings.Contains(hdr0, "Use%") {
		t.Errorf("df -P header = %q, must say Capacity, not Use%%", hdr0)
	}

	// POSIX -P with -k: 1024-blocks, in either order and clustered.
	for _, argv := range [][]string{{"-P", "-k"}, {"-k", "-P"}, {"-kP"}, {"-Pk"}} {
		out, _, code = runTool(t, argv...)
		if code != 0 {
			t.Fatalf("df %v code = %d", argv, code)
		}
		hdr := firstLine(out)
		if !strings.Contains(hdr, "1024-blocks") || !strings.Contains(hdr, "Capacity") {
			t.Errorf("df %v header = %q, want 1024-blocks + Capacity", argv, hdr)
		}
	}

	out, _, code = runTool(t, "-T")
	if code != 0 {
		t.Fatalf("df -T code = %d", code)
	}
	if hdr := firstLine(out); !strings.Contains(hdr, "Type") {
		t.Errorf("df -T header = %q, want Type", hdr)
	}

	out, _, code = runTool(t, "-i")
	if code != 0 {
		t.Fatalf("df -i code = %d", code)
	}
	hdr := firstLine(out)
	for _, col := range []string{"Inodes", "IUsed", "IFree", "IUse%"} {
		if !strings.Contains(hdr, col) {
			t.Errorf("df -i header %q missing %q", hdr, col)
		}
	}
}

// TestPortableExactOutput pins the POSIX -P STDOUT format on a
// synthetic file system: "%s %d %d %d %d%% %s\n" fields under the
// exact "Filesystem 512-blocks Used Available Capacity Mounted on"
// header (1024-blocks with -k), with the percentage rounded up.
func TestPortableExactOutput(t *testing.T) {
	rows := []mountEntry{{device: "/dev/x", point: "/", total: 1 << 20, used: 1 << 19, avail: 1 << 19}}

	var buf bytes.Buffer
	printTable(&buf, rows, tableOptions{
		scale:    scaleMode{blockSize: 512, header: "512-blocks"},
		portable: true,
	})
	want := "Filesystem 512-blocks Used Available Capacity Mounted on\n" +
		"/dev/x           2048 1024      1024      50% /\n"
	if got := buf.String(); got != want {
		t.Errorf("df -P output:\n%q\nwant:\n%q", got, want)
	}

	buf.Reset()
	printTable(&buf, rows, tableOptions{
		scale:    scaleMode{blockSize: 1024, header: "1024-blocks"},
		portable: true,
	})
	want = "Filesystem 1024-blocks Used Available Capacity Mounted on\n" +
		"/dev/x            1024  512       512      50% /\n"
	if got := buf.String(); got != want {
		t.Errorf("df -P -k output:\n%q\nwant:\n%q", got, want)
	}
}

// TestPortableRoundsPercentUp pins the POSIX -P rule that a fractional
// <percentage used> is rounded to the next highest integer.
func TestPortableRoundsPercentUp(t *testing.T) {
	rows := []mountEntry{{device: "/dev/x", point: "/", total: 200 * 512, used: 1 * 512, avail: 199 * 512}}
	var buf bytes.Buffer
	printTable(&buf, rows, tableOptions{
		scale:    scaleMode{blockSize: 512, header: "512-blocks"},
		portable: true,
	})
	lines := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
	if len(lines) != 2 || !strings.Contains(lines[1], " 1% ") {
		t.Errorf("df -P 0.5%%-used output = %q, want 1%% (rounded up)", buf.String())
	}
}

// TestPortableOneLinePerFilesystem: with -P each mounted file system is
// exactly one line of six ordered fields (live data).
func TestPortableOneLinePerFilesystem(t *testing.T) {
	out, _, code := runTool(t, "-P")
	if code != 0 {
		t.Fatalf("df -P code = %d", code)
	}
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	lineRE := regexp.MustCompile(`^\S.*\s+\d+\s+\d+\s+\d+\s+(\d+%|-)\s+\S.*$`)
	for _, l := range lines[1:] {
		if !lineRE.MatchString(l) {
			t.Errorf("df -P line %q does not match the portable format", l)
		}
	}
}

func TestOutputFields(t *testing.T) {
	out, _, code := runTool(t, "--output=source,fstype,target")
	if code != 0 {
		t.Fatalf("df --output code = %d", code)
	}
	hdr := firstLine(out)
	for _, col := range []string{"Filesystem", "Type", "Mounted on"} {
		if !strings.Contains(hdr, col) {
			t.Errorf("--output header %q missing %q", hdr, col)
		}
	}
	if strings.Contains(hdr, "Used") {
		t.Errorf("--output header %q includes unrequested Used column", hdr)
	}
}

func TestOutputWithoutFieldList(t *testing.T) {
	out, _, code := runTool(t, "--output")
	if code != 0 {
		t.Fatalf("df --output code = %d", code)
	}
	hdr := firstLine(out)
	for _, col := range []string{"Filesystem", "Type", "Inodes", "Mounted on"} {
		if !strings.Contains(hdr, col) {
			t.Errorf("--output header %q missing %q", hdr, col)
		}
	}
}

func TestTypeFilterIsLongOnly(t *testing.T) {
	// GNU type filtering survives under its long spelling only.
	out, _, code := runTool(t, "--type", "__definitely_missing_fs_type__")
	if code != 0 {
		t.Fatalf("df --type missing-type code = %d", code)
	}
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != 1 {
		t.Errorf("df --type missing-type output = %q, want header only", out)
	}
}

// TestXSITotalsOption pins POSIX XSI -t: a no-argument totals option,
// NOT GNU's -t TYPE filter. A word after -t is an operand, never a
// file-system type.
func TestXSITotalsOption(t *testing.T) {
	for _, argv := range [][]string{{"-t"}, {"--total"}, {"-kt"}, {"-tk"}} {
		out, errb, code := runTool(t, argv...)
		if code != 0 {
			t.Fatalf("df %v code = %d, stderr = %q", argv, code, errb)
		}
		lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
		last := lines[len(lines)-1]
		if !strings.HasPrefix(last, "total") {
			t.Errorf("df %v last line = %q, want totals row", argv, last)
		}
	}

	// -t must not consume the next word: it is an operand, and a
	// missing operand is a loud per-operand error, not a filter.
	out, errb, code := runTool(t, "-t", "__definitely_missing_fs_type__")
	if code != 1 {
		t.Errorf("df -t missing-operand code = %d, want 1", code)
	}
	if !strings.Contains(errb, "__definitely_missing_fs_type__") {
		t.Errorf("df -t missing-operand stderr = %q, want operand named", errb)
	}
	if out != "" {
		t.Errorf("df -t missing-operand stdout = %q, want empty", out)
	}
}

// TestXSITotalsWithOperand covers -t combined with a file operand:
// the containing file system plus a totals row.
func TestXSITotalsWithOperand(t *testing.T) {
	out, errb, code := runTool(t, "-t", ".")
	if code != 0 {
		t.Fatalf("df -t . code = %d, stderr = %q", code, errb)
	}
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("df -t . = %q, want header + mount + totals", out)
	}
	if !strings.HasPrefix(lines[2], "total") {
		t.Errorf("df -t . last line = %q, want totals row", lines[2])
	}
}

// TestClusterArgumentNotMistakenForFlags: letters inside an
// argument-taking shorthand's in-word argument (-xTYPE) must not be
// consumed as flags — here 'k' must not select 1024-byte units and
// 't' must not add a totals row.
func TestClusterArgumentNotMistakenForFlags(t *testing.T) {
	for _, argv := range [][]string{{"-xk9fs"}, {"-xtmpfs_missing"}} {
		out, errb, code := runTool(t, argv...)
		if code != 0 {
			t.Fatalf("df %v code = %d, stderr = %q", argv, code, errb)
		}
		if hdr := firstLine(out); !strings.Contains(hdr, "512-blocks") {
			t.Errorf("df %v header = %q, want default 512-blocks", argv, hdr)
		}
		for _, l := range strings.Split(out, "\n") {
			if strings.HasPrefix(l, "total") {
				t.Errorf("df %v grew a totals row: %q", argv, l)
			}
		}
	}
}

func TestFileOperand(t *testing.T) {
	out, errb, code := runTool(t, ".")
	if code != 0 {
		t.Fatalf("df . code = %d, stderr = %q", code, errb)
	}
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != 2 {
		t.Errorf("df . = %q, want header + exactly one mount line", out)
	}
}

func TestNonexistentOperand(t *testing.T) {
	out, errb, code := runTool(t, "definitely-not-here")
	if code != 1 || !strings.Contains(errb, "definitely-not-here") {
		t.Errorf("df missing file: code=%d err=%q", code, errb)
	}
	if strings.Contains(errb, "no file systems processed") {
		t.Errorf("df missing file stderr = %q, must contain only the operand diagnostic", errb)
	}
	if out != "" {
		t.Errorf("df missing file stdout = %q, want empty", out)
	}
}

func TestMixedOperandsPreserveSuccessfulOutput(t *testing.T) {
	out, errb, code := runTool(t, ".", "definitely-not-here")
	if code != 1 {
		t.Errorf("df mixed operands code = %d, want 1", code)
	}
	if !strings.Contains(errb, "definitely-not-here") {
		t.Errorf("df mixed operands stderr = %q, want failed operand", errb)
	}
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != 2 {
		t.Errorf("df mixed operands stdout = %q, want header + successful operand", out)
	}
}

func TestUsePct(t *testing.T) {
	cases := []struct {
		used, avail uint64
		want        string
	}{
		{0, 0, "-"},
		{0, 100, "0%"},
		{50, 50, "50%"},
		{1, 99, "1%"},  // 1.0 -> 1, exact
		{1, 199, "1%"}, // 0.5 rounds up
		{99, 1, "99%"},
		{100, 0, "100%"},
		{^uint64(0), 1, "100%"},
		{^uint64(0), ^uint64(0), "50%"},
	}
	for _, c := range cases {
		if got := usePct(c.used, c.avail); got != c.want {
			t.Errorf("usePct(%d, %d) = %q, want %q", c.used, c.avail, got, c.want)
		}
	}
}

func TestDivCeilDoesNotOverflow(t *testing.T) {
	for _, tc := range []struct {
		n, d, want uint64
	}{
		{0, 1, 0},
		{1, 1, 1},
		{^uint64(0), 2, 1 << 63},
		{^uint64(0), ^uint64(0), 1},
	} {
		if got := divCeil(tc.n, tc.d); got != tc.want {
			t.Errorf("divCeil(%d, %d) = %d, want %d", tc.n, tc.d, got, tc.want)
		}
	}
}

func TestHumanSizeMaximumValue(t *testing.T) {
	if got := humanSize(^uint64(0), 1024); got != "16E" {
		t.Errorf("humanSize(max uint64, 1024) = %q, want 16E", got)
	}
	if got := humanSize(^uint64(0), 1000); got != "19E" {
		t.Errorf("humanSize(max uint64, 1000) = %q, want 19E", got)
	}
}

func TestUnknownFlag(t *testing.T) {
	_, errb, code := runTool(t, "--frobnicate")
	if code != 2 || !strings.Contains(errb, "frobnicate") || !strings.Contains(errb, "pure-Go") {
		t.Errorf("unknown flag: code=%d err=%q", code, errb)
	}
}

func TestHelpAndVersion(t *testing.T) {
	out, _, code := runTool(t, "--help")
	if code != 0 || !strings.Contains(out, "Usage: df") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	for _, flag := range []string{"-V", "-k", "--block-size", "--si", "--portability", "--print-type", "--all", "--inodes", "--local", "--no-sync", "--sync", "--output", "--type", "--exclude-type", "--total"} {
		if !strings.Contains(out, flag) {
			t.Errorf("--help output missing %s", flag)
		}
	}
	out, errb, code := runTool(t, "-M")
	if code != 0 || errb != "" || !strings.Contains(firstLine(out), "1048576B-blocks") {
		t.Errorf("-M: code=%d err=%q first line=%q", code, errb, firstLine(out))
	}
	out, _, code = runTool(t, "--version")
	if code != 0 || !strings.Contains(out, "df") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}
	out, _, code = runTool(t, "-V")
	if code != 0 || !strings.Contains(out, "df") {
		t.Errorf("-V: code=%d out=%q", code, out)
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
