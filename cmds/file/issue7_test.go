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

// TestFileIssue7MagicOptionArgumentsAndPermutation pins the SYNOPSIS and
// OPTIONS clauses for the parser behavior this command actually supports:
// separate -m/-M operands, repeated ordered sources, and interspersed option
// parsing. The attached short-option form is a non-conflicting parser
// extension and is pinned separately from the required forms.
func TestFileIssue7MagicOptionArgumentsAndPermutation(t *testing.T) {
	dir := t.TempDir()
	put(t, dir, "payload", []byte("ABCD\n"))
	put(t, dir, "png", []byte("\x89PNG\r\n\x1a\nrest"))
	put(t, dir, "first", []byte("0\tstring\tAB\tfirst\n"))
	put(t, dir, "second", []byte("0\tstring\tAB\tsecond\n"))
	put(t, dir, "pngmagic", []byte("0\tstring\t\\211PNG\tcustom png\n"))
	put(t, dir, "miss", []byte("0\tstring\tZZ\tmiss\n"))

	tests := []struct {
		name string
		args []string
		want string
	}{
		{"attached-magic", []string{"-Mfirst", "payload"}, "payload: first\n"},
		{"separate-magic", []string{"-M", "first", "payload"}, "payload: first\n"},
		{"repeated-replacement-order", []string{"-M", "miss", "-M", "second", "payload"}, "payload: second\n"},
		{"interspersed-additional-before-default", []string{"payload", "-mfirst", "-d"}, "payload: first\n"},
		{"interspersed-default-before-replacement", []string{"png", "-d", "-Mpngmagic"}, "png: PNG image data\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, errb, code := invoke(t, dir, "", tc.args...)
			if code != 0 || errb != "" || out != tc.want {
				t.Fatalf("file %v = (%q, %q, %d), want %q", tc.args, out, errb, code, tc.want)
			}
		})
	}
}

// TestFileIssue7DoubleDashParsing documents -- only because the shared parser
// supports it. After --, a dash-leading path is an operand, and unsupported
// GNU-style long options still fail when they appear before --.
func TestFileIssue7DoubleDashParsing(t *testing.T) {
	dir := t.TempDir()
	put(t, dir, "-dash", []byte("hello\n"))
	out, errb, code := invoke(t, dir, "", "--", "-dash")
	if out != "-dash: ASCII text\n" || errb != "" || code != 0 {
		t.Fatalf("file -- -dash = (%q, %q, %d)", out, errb, code)
	}
	out, errb, code = invoke(t, dir, "", "--mime", "--", "-dash")
	if out != "" || code != 2 || !strings.Contains(errb, "mime") {
		t.Fatalf("file --mime -- -dash = (%q, %q, %d), want unsupported-option usage error", out, errb, code)
	}
}

// TestFileIssue7SymbolicLinkAlternativeFormat pins the STDOUT alternative
// format clause: when file is identified as a symbolic link (via -h, or a
// dangling link, which the -h option text says is identified "as if -h had
// been specified"), the output form is "%s: %s %s\n", <file>, <type>,
// <contents of link>, with <type> containing "symbolic link to". The -i
// option only restricts further classification of regular files, so it does
// not change the symbolic-link line.
func TestFileIssue7SymbolicLinkAlternativeFormat(t *testing.T) {
	dir := t.TempDir()
	put(t, dir, "target", []byte("payload\n"))
	if err := os.Symlink("target", filepath.Join(dir, "link")); err != nil {
		t.Skipf("symlink creation failed: %v", err)
	}
	if err := os.Symlink("absent", filepath.Join(dir, "dangling")); err != nil {
		t.Skipf("symlink creation failed: %v", err)
	}
	out, errb, code := invoke(t, dir, "", "-h", "link", "dangling")
	want := "link: symbolic link to target\ndangling: symbolic link to absent\n"
	if code != 0 || errb != "" || out != want {
		t.Fatalf("file -h = (%q, %q, %d), want %q", out, errb, code, want)
	}
	// Without -h the usable link is followed, so its line reports content
	// classification ("regular file" itself is only forced by -i); the
	// dangling link still uses the alternative format.
	out, errb, code = invoke(t, dir, "", "link", "dangling")
	want = "link: ASCII text\ndangling: symbolic link to absent\n"
	if code != 0 || errb != "" || out != want {
		t.Fatalf("file links = (%q, %q, %d), want %q", out, errb, code, want)
	}
	// -i leaves the symbolic-link clause untouched.
	out, errb, code = invoke(t, dir, "", "-i", "-h", "link")
	if code != 0 || errb != "" || out != "link: symbolic link to target\n" {
		t.Fatalf("file -i -h link = (%q, %q, %d)", out, errb, code)
	}
}

func mkdir(dir, name string) error { return os.Mkdir(filepath.Join(dir, name), 0o755) }
