package filecmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// POSIX.1-2016 file evidence (Issue 7, XCU:file). The suite already pins the
// -d/-m/-M ordering, -i/-h forms, empty/text/data classification, write
// failures, and magic-file diagnostics; these tests isolate the remaining
// normative axes: per-operand output order, the "-" stdin operand, and the
// exact STDOUT line form.

// TestFileIssue7OperandOrderPreserved pins the OPERANDS, STDOUT, and EXIT
// STATUS clauses. The Issue 7 file page carries an explicit command-specific
// exception overriding the Utility Description Defaults: nonexistent,
// unreadable, or undetermined operands SHALL NOT affect the exit status, and
// the standard-output "<file>: <type>" line carries "cannot open". So an
// inaccessible operand in the middle leaves the status 0, keeps its stdout
// line in operand order, and processing continues with later operands. The
// absence of a second stderr diagnostic and the exact "ASCII text" label
// below pin Bashy's deterministic choices; POSIX does not require that text
// label or forbid an additional diagnostic. The word "directory" is required.
func TestFileIssue7OperandOrderPreserved(t *testing.T) {
	dir := t.TempDir()
	put(t, dir, "good", []byte("hello\n"))
	if err := mkdir(dir, "sub"); err != nil {
		t.Fatal(err)
	}
	out, errb, code := invoke(t, dir, "", "good", "missing", "sub")
	if code != 0 || errb != "" {
		t.Fatalf("code=%d err=%q, want status 0 and Bashy's deterministic stderr silence", code, errb)
	}
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("out=%q, want exactly one line per operand", out)
	}
	if !strings.HasPrefix(lines[0], "good: ") || !strings.Contains(lines[0], "ASCII text") {
		t.Fatalf("first line %q lost operand order/type", lines[0])
	}
	if !strings.HasPrefix(lines[1], "missing: ") || !strings.Contains(lines[1], "cannot open") {
		t.Fatalf("unopenable operand line %q must contain \"cannot open\" on stdout", lines[1])
	}
	if !strings.HasPrefix(lines[2], "sub: ") || !strings.Contains(lines[2], "directory") {
		t.Fatalf("third line %q lost operand order/type (continuation required)", lines[2])
	}
	if !strings.Contains(out, ": ") {
		t.Fatalf("out=%q lost the colon-space separator", out)
	}
}

// TestFileIssue7StdinOperandUsedByName pins Bashy's documented implementation
// choice for the POSIX implementation-defined "-" operand: standard input is
// inspected, reported under that name, and interleaved in operand order with
// named operands. This is compatibility evidence, not a mandatory POSIX
// interpretation of "-".
func TestFileIssue7StdinOperandUsedByName(t *testing.T) {
	dir := t.TempDir()
	put(t, dir, "named", []byte("file text\n"))
	out, errb, code := invoke(t, dir, "stream text\n", "-", "named")
	want := "-: ASCII text\nnamed: ASCII text\n"
	if code != 0 || errb != "" || out != want {
		t.Fatalf("file - named = (%q, %q, %d), want %q", out, errb, code, want)
	}
	out, errb, code = invoke(t, dir, "never read\n", "named")
	if code != 0 || errb != "" || out != "named: ASCII text\n" {
		t.Fatalf("file named = (%q, %q, %d), stdin must stay unread", out, errb, code)
	}
}

// TestFileIssue7MissingOperandIsUsageError pins the required operand arity.
// Status 2 is Bashy's repository-wide usage convention; POSIX requires only a
// greater-than-zero error status for this invalid invocation.
func TestFileIssue7MissingOperandIsUsageError(t *testing.T) {
	dir := t.TempDir()
	out, errb, code := invoke(t, dir, "ignored\n")
	if code != 2 || out != "" || !strings.Contains(errb, "missing file operand") {
		t.Fatalf("file (no operands) = (%q, %q, %d), want usage error 2", out, errb, code)
	}
}

func mkdir(dir, name string) error { return os.Mkdir(filepath.Join(dir, name), 0o755) }
