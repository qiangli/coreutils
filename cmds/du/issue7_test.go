package ducmd

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// POSIX.1-2016 du evidence (Issue 7, XCU:du). The tests in this file pin the
// normative surface that the broader GNU-mode tests do not already isolate:
// the implicit "." operand, the exact STDOUT format with block rounding, and
// -x traversal on a single device.

// TestDuIssue7DefaultOperandIsWorkingDirectory pins the OPERANDS clause: with
// no file operand, du processes the file hierarchy rooted in the current
// working directory and reports every directory in that hierarchy. POSIX does
// not prescribe record order, so the assertion compares the path set. A
// regression to "error on missing operand" or an incomplete hierarchy fails.
//
// Test-control note: -b (apparent bytes) is a non-POSIX GNU extension used
// only as a deterministic size control; the POSIX surface under test is the
// implicit "." operand and the post-order report.
func TestDuIssue7DefaultOperandIsWorkingDirectory(t *testing.T) {
	dir := mkTree(t)
	out, _, code := runToolAt(t, dir, "-b")
	if code != 0 {
		t.Fatalf("du (no operands) code = %d, want 0", code)
	}
	var paths []string
	for _, ln := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
		parts := strings.SplitN(ln, " ", 2)
		if len(parts) != 2 {
			t.Fatalf("malformed du line %q", ln)
		}
		paths = append(paths, parts[1])
	}
	// mkTree builds dir/tree/{...}, so "." expands to the full hierarchy.
	sort.Strings(paths)
	if strings.Join(paths, " ") != ". ./tree ./tree/sub" {
		t.Errorf("du (no operands) paths = %v, want the complete hierarchy", paths)
	}
}

// TestDuIssue7StdoutFormatRoundsUpToBlocks pins the STDOUT format
// "<blocks> <path>" with exactly one <space>, the default 512-byte unit, the
// -k 1024-byte unit, and round-up-to-next-block arithmetic. Apparent size
// (-A) makes the byte count deterministic across platforms, so 5000 bytes
// must report ceil(5000/512)=10 units by default and ceil(5000/1024)=5 with
// -k; floor arithmetic or a two-space separator fails the exact strings.
//
// Test-control note: -A and -b (apparent size) are non-POSIX GNU extensions
// used only as deterministic size controls; the POSIX surfaces under test
// here are the STDOUT format, the default and -k units, and the round-up
// arithmetic, not the -A/-b interfaces.
func TestDuIssue7StdoutFormatRoundsUpToBlocks(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "f", strings.Repeat("x", 5000))
	out, errb, code := runToolAt(t, dir, "-A", "f")
	if code != 0 || errb != "" || out != "10 f\n" {
		t.Errorf("du -A f = (%q, %q, %d), want exactly \"10 f\\n\" in 512-byte units", out, errb, code)
	}
	out, errb, code = runToolAt(t, dir, "-A", "-k", "f")
	if code != 0 || errb != "" || out != "5 f\n" {
		t.Errorf("du -A -k f = (%q, %q, %d), want exactly \"5 f\\n\" in 1024-byte units", out, errb, code)
	}
}

// TestDuIssue7SummarizeFileOperand pins the -s rule that each file operand
// gets exactly one total line, and that a non-directory operand reports its
// own blocks.
//
// Test-control note: -A is a non-POSIX GNU extension used only as a
// deterministic size control; the POSIX surface under test is the -s
// per-operand single-line rule.
func TestDuIssue7SummarizeFileOperand(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "f", strings.Repeat("x", 3000))
	out, errb, code := runToolAt(t, dir, "-A", "-s", "f")
	if code != 0 || errb != "" || out != "6 f\n" {
		t.Errorf("du -A -s f = (%q, %q, %d), want \"6 f\\n\" (ceil(3000/512))", out, errb, code)
	}
}

// TestDuIssue7OneFileSystemKeepsSameDeviceEntries pins the -x clause on the
// only device boundary a hermetic test can control: a hierarchy that lives on
// one device must be fully traversed and summed, so -x must not drop the
// subdirectory. (A cross-device branch cannot be constructed without mounting
// a second filesystem, which no hermetic test may do; that boundary is
// recorded as an audit residual rather than asserted.)
//
// Test-control note: -b is a non-POSIX GNU extension used only as a
// deterministic size control; the POSIX surface under test is the -x
// same-device traversal rule.
func TestDuIssue7OneFileSystemKeepsSameDeviceEntries(t *testing.T) {
	dir := mkTree(t)
	plain, _, code := runToolAt(t, dir, "-b", "tree")
	if code != 0 {
		t.Fatalf("du -b tree code = %d", code)
	}
	crossed, errb, code := runToolAt(t, dir, "-b", "-x", "tree")
	if code != 0 || errb != "" {
		t.Fatalf("du -b -x tree = (%q, %q, %d), want clean run", crossed, errb, code)
	}
	if plain != crossed {
		t.Errorf("du -x changed same-device traversal:\nplain=%q\n-x=%q", plain, crossed)
	}
	if !strings.Contains(crossed, "tree/sub") {
		t.Errorf("du -x dropped same-device subdirectory: %q", crossed)
	}
}

// TestDuIssue7SymlinkNotFollowedByDefault pins the default (no -H/-L)
// symlink semantics: a symlink operand is measured as the link itself, not
// the referenced directory. The link target is a short RELATIVE name so the
// link's own size is deterministic (its pathname length), and the assertion
// contrasts the link's one-block report against the tree's multi-block
// total: a followed link would report the tree's total instead.
//
// Test-control note: -A (apparent size) is a non-POSIX GNU extension used
// here only as a deterministic size control; the POSIX surface under test is
// the operand/link measurement rule itself, not the -A interface.
func TestDuIssue7SymlinkNotFollowedByDefault(t *testing.T) {
	dir := t.TempDir()
	// A target hierarchy whose apparent total spans several 512-byte units
	// (with -A, directories contribute zero, so the file must exceed 512
	// bytes by itself for the control to discriminate).
	root := filepath.Join(dir, "tree")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, root, "f", strings.Repeat("x", 2000))
	// Relative target keeps the link's apparent size deterministic and tiny.
	if err := os.Symlink("tree", filepath.Join(dir, "link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	out, _, code := runToolAt(t, dir, "-A", "-s", "link")
	if code != 0 {
		t.Fatalf("du -A -s link code = %d", code)
	}
	line := strings.TrimSuffix(out, "\n")
	if n := parseFirstValue(t, line); n != 1 {
		t.Errorf("du followed or mis-measured the symlink operand: %q (want the link's own single 512-byte unit)", out)
	}
	// Control: the tree it points at spans several units, so a
	// dereferencing regression cannot hide behind the same value.
	treeOut, _, _ := runToolAt(t, dir, "-A", "-s", "tree")
	if treeN := parseFirstValue(t, strings.TrimSuffix(treeOut, "\n")); treeN <= 1 {
		t.Fatalf("control tree total %d too small to discriminate; fixture is broken", treeN)
	}
}

func parseFirstValue(t *testing.T, line string) int64 {
	t.Helper()
	fields := strings.Fields(line)
	if len(fields) == 0 {
		t.Fatalf("empty du line")
	}
	var n int64
	for _, c := range fields[0] {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int64(c-'0')
	}
	return n
}
