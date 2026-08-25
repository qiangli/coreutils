package paxcmd

import (
	"archive/tar"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeArchive builds an archive from the given operands in dir and returns its
// path, so selection tests can run against a known member set.
func writeArchive(t *testing.T, dir string, operands ...string) string {
	t.Helper()
	arc := filepath.Join(dir, "a.tar")
	args := append([]string{"-w", "-f", arc}, operands...)
	if _, e, code := exec(t, dir, "", args...); code != 0 {
		t.Fatalf("write failed: %d %s", code, e)
	}
	return arc
}

// An operand pattern that selects no member must be diagnosed on stderr and
// force a non-zero exit — a silent exit 0 lets a mistyped pattern look like an
// empty archive.
func TestUnmatchedPatternIsDiagnosed(t *testing.T) {
	d := makeTree(t)
	arc := writeArchive(t, d, "src")
	out, errs, code := exec(t, d, "", "-f", arc, "src/a.txt", "nope*")
	if code == 0 {
		t.Fatalf("an unmatched pattern must exit non-zero; out=%q", out)
	}
	if !strings.Contains(errs, "nope*") || !strings.Contains(errs, "not matched") {
		t.Errorf("expected an unmatched diagnostic naming the pattern, got %q", errs)
	}
	// The pattern that DID match is still listed.
	if !strings.Contains(out, "src/a.txt") {
		t.Errorf("the matched pattern should still select its member; out=%q", out)
	}
}

func TestUnmatchedPatternInReadMode(t *testing.T) {
	d := makeTree(t)
	arc := writeArchive(t, d, "src")

	dest := t.TempDir()
	_, errs, code := exec(t, dest, "", "-r", "-f", arc, "nope*")
	if code == 0 {
		t.Fatalf("read with an unmatched pattern must exit non-zero")
	}
	if !strings.Contains(errs, "not matched") {
		t.Errorf("expected an unmatched diagnostic, got %q", errs)
	}
	if _, err := os.Lstat(filepath.Join(dest, "src", "a.txt")); err == nil {
		t.Error("an unmatched pattern must not extract anything")
	}

	// A matching pattern extracts and exits 0.
	dest2 := t.TempDir()
	if _, e, code := exec(t, dest2, "", "-r", "-f", arc, "src/a.txt"); code != 0 {
		t.Fatalf("a matching pattern must succeed: %d %s", code, e)
	}
	if _, err := os.Lstat(filepath.Join(dest2, "src", "a.txt")); err != nil {
		t.Errorf("the selected member was not extracted: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(dest2, "src", "sub", "b.txt")); err == nil {
		t.Error("only the selected member should be extracted")
	}
}

// -n selects only the FIRST archive member matching each pattern.
func TestSelectorDashNFirstMatchOnly(t *testing.T) {
	s := newSelector(&options{selectNoPattern: true}, []string{"a*.txt", "b*.txt"})
	if !s.keep("a1.txt", false) {
		t.Error("the first match of a pattern must be kept")
	}
	if s.keep("a2.txt", false) {
		t.Error("-n must drop later matches of the same pattern")
	}
	// A different pattern is independent: its own first match is still kept.
	if !s.keep("b1.txt", false) {
		t.Error("-n tracks first-match per pattern independently")
	}
	if s.keep("b2.txt", false) {
		t.Error("-n must drop later matches of the second pattern too")
	}
}

func TestDashNSelectsFirstMatchEndToEnd(t *testing.T) {
	d := t.TempDir()
	for _, n := range []string{"a1.txt", "a2.txt"} {
		if err := os.WriteFile(filepath.Join(d, n), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	arc := writeArchive(t, d, "a1.txt", "a2.txt")
	out, _, code := exec(t, d, "", "-f", arc, "-n", "a*.txt")
	if code != 0 {
		t.Fatalf("list -n failed: %d", code)
	}
	lines := strings.Fields(strings.TrimSpace(out))
	if len(lines) != 1 || lines[0] != "a1.txt" {
		t.Errorf("-n should list only the first match; got %q", out)
	}
}

// -d makes a directory pattern match only the directory itself, in every mode.
func TestSelectorDashDStopsHierarchy(t *testing.T) {
	// Default: a directory pattern selects everything beneath it.
	s := newSelector(&options{}, []string{"d1"})
	s.prime([]selectorMember{{name: "d1", isDir: true}, {name: "d1/f.txt"}})
	if !s.keep("d1/f.txt", false) {
		t.Error("without -d, a directory pattern selects the subtree")
	}

	sd := newSelector(&options{dirsNoDescend: true}, []string{"d1"})
	sd.prime([]selectorMember{{name: "d1", isDir: true}, {name: "d1/f.txt"}})
	if sd.keep("d1/f.txt", false) {
		t.Error("-d must stop the pattern at the directory itself")
	}
	if !sd.keep("d1", true) {
		t.Error("-d must still select the named directory itself")
	}
}

func TestDashDTrailingSlashSelectsRealArchiveDirectory(t *testing.T) {
	d := makeTree(t)
	arc := writeArchive(t, d, "src")
	out, errs, code := exec(t, d, "", "-d", "-f", arc, "src/")
	if code != 0 {
		t.Fatalf("trailing-slash directory operand failed: %d %s", code, errs)
	}
	if strings.TrimSpace(out) != "src/" {
		t.Fatalf("-d src/ should select only the directory member; got %q", out)
	}
}

func TestTrailingSlashPatternDoesNotSelectSameNamedRegularFile(t *testing.T) {
	s := newSelector(&options{}, []string{"plain/"})
	if s.keep("plain", false) {
		t.Fatal("a trailing-slash directory pattern must not select a regular file")
	}
}

func TestTrailingSlashPatternRequiresDirectoryMember(t *testing.T) {
	d := t.TempDir()
	arc := filepath.Join(d, "a.tar")
	f, err := os.Create(arc)
	if err != nil {
		t.Fatal(err)
	}
	tw := tar.NewWriter(f)
	if err := tw.WriteHeader(&tar.Header{Name: "src/file", Mode: 0o644, Size: 1, Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	out, errs, code := exec(t, d, "", "-f", arc, "src/")
	if code == 0 || out != "" || !strings.Contains(errs, "not matched") {
		t.Fatalf("directory pattern without a directory member: code=%d out=%q err=%q", code, out, errs)
	}
}

func TestComplementExcludesMatchedDirectoryHierarchy(t *testing.T) {
	d := makeTree(t)
	arc := writeArchive(t, d, "src")
	out, errs, code := exec(t, d, "", "-c", "-f", arc, "src/*")
	if code != 0 {
		t.Fatalf("complement failed: %d %s", code, errs)
	}
	if strings.Contains(out, "src/sub/") || strings.Contains(out, "src/sub/b.txt") {
		t.Fatalf("complement retained a matched directory hierarchy: %q", out)
	}
}

func TestHierarchySelectionIsIndependentOfDirectoryHeaderOrder(t *testing.T) {
	d := t.TempDir()
	arc := filepath.Join(d, "reverse.tar")
	f, err := os.Create(arc)
	if err != nil {
		t.Fatal(err)
	}
	tw := tar.NewWriter(f)
	if err := tw.WriteHeader(&tar.Header{Name: "src/file", Mode: 0o644, Size: 1, Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := tw.WriteHeader(&tar.Header{Name: "src/", Mode: 0o755, Typeflag: tar.TypeDir}); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	out, errs, code := exec(t, d, "", "-f", arc, "s*")
	if code != 0 || !strings.Contains(out, "src/file") || !strings.Contains(out, "src/") {
		t.Fatalf("normal selection: code=%d out=%q err=%q", code, out, errs)
	}
	out, errs, code = exec(t, d, "", "-c", "-f", arc, "s*")
	if code != 0 || out != "" {
		t.Fatalf("complement selection: code=%d out=%q err=%q", code, out, errs)
	}
}

func TestDashNPrefixChildCannotConsumeLaterDirectRegularMatch(t *testing.T) {
	s := newSelector(&options{selectNoPattern: true}, []string{"a"})
	s.prime([]selectorMember{{name: "a/x"}, {name: "a"}})
	if s.keep("a/x", false) {
		t.Fatal("a prefix-only child must not consume -n")
	}
	if !s.keep("a", false) {
		t.Fatal("the first direct match must be selected by -n")
	}
}

func TestDashNDirectoryHierarchyExcludesLaterDuplicateRoot(t *testing.T) {
	s := newSelector(&options{selectNoPattern: true}, []string{"a"})
	s.prime([]selectorMember{{name: "a/", isDir: true}, {name: "a/x"}, {name: "a"}})
	if !s.keep("a/", true) {
		t.Fatal("first matching directory must be selected")
	}
	if !s.keep("a/x", false) {
		t.Fatal("selected directory hierarchy must be retained")
	}
	if s.keep("a", false) {
		t.Fatal("later duplicate root is not a hierarchy descendant and must not bypass -n")
	}
}

// The g flag rewrites every occurrence; without it only the first is rewritten.
func TestSubstitutionGlobalFlag(t *testing.T) {
	g, err := parseSubstitution("/x/Y/g")
	if err != nil {
		t.Fatal(err)
	}
	if got := applySubstitutions([]substitution{g}, "xxx", nil); got != "YYY" {
		t.Errorf("global flag: got %q, want YYY", got)
	}
	first, err := parseSubstitution("/x/Y/")
	if err != nil {
		t.Fatal(err)
	}
	if got := applySubstitutions([]substitution{first}, "xxx", nil); got != "Yxx" {
		t.Errorf("without g only the first match rewrites: got %q, want Yxx", got)
	}
}

// The p flag reports "old >> new" to stderr for each renamed member.
func TestSubstitutionPrintReportsRename(t *testing.T) {
	d := makeTree(t)
	arc := writeArchive(t, d, "src")
	out, errs, code := exec(t, d, "", "-f", arc, "-s", "/a/AA/p")
	if code != 0 {
		t.Fatalf("list with -s p failed: %d %s", code, errs)
	}
	if !strings.Contains(errs, "src/a.txt >> src/AA.txt") {
		t.Errorf("expected a rename report on stderr, got %q", errs)
	}
	// The rewritten name is what the listing shows.
	if !strings.Contains(out, "src/AA.txt") {
		t.Errorf("listing should show the rewritten name; got %q", out)
	}
}

func TestSubstitutionPrintReportsEmptyResult(t *testing.T) {
	s, err := parseSubstitution(",src/a,,p")
	if err != nil {
		t.Fatal(err)
	}
	var report bytes.Buffer
	if got := applySubstitutions([]substitution{s}, "src/a", &report); got != "" {
		t.Fatalf("empty substitution result=%q", got)
	}
	if got := report.String(); got != "src/a >> \n" {
		t.Fatalf("empty substitution report=%q", got)
	}
}

func TestReadExtractsMatchesDespiteUnmatchedOperand(t *testing.T) {
	d := makeTree(t)
	arc := writeArchive(t, d, "src")
	dest := t.TempDir()
	_, errs, code := exec(t, dest, "", "-r", "-f", arc, "src/a.txt", "missing*")
	if code == 0 || !strings.Contains(errs, "missing*") {
		t.Fatalf("mixed selection should diagnose and fail: code=%d err=%q", code, errs)
	}
	if got, err := os.ReadFile(filepath.Join(dest, "src", "a.txt")); err != nil || string(got) != "alpha" {
		t.Fatalf("matched member was not extracted: data=%q err=%v", got, err)
	}
}

func TestReadSubstitutionPreservesHardLinkTarget(t *testing.T) {
	d := t.TempDir()
	arc := filepath.Join(d, "links.tar")
	f, err := os.Create(arc)
	if err != nil {
		t.Fatal(err)
	}
	tw := tar.NewWriter(f)
	if err := tw.WriteHeader(&tar.Header{Name: "src/a", Mode: 0o644, Size: 1, Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := tw.WriteHeader(&tar.Header{Name: "src/b", Linkname: "src/a", Mode: 0o644, Typeflag: tar.TypeLink}); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	dest := t.TempDir()
	_, errs, code := exec(t, dest, "", "-r", "-f", arc, "-s", ",^src/,dst/,")
	if code != 0 {
		t.Fatalf("hard-link extraction failed: %d %s", code, errs)
	}
	a, err := os.Stat(filepath.Join(dest, "dst", "a"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.Stat(filepath.Join(dest, "dst", "b"))
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(a, b) {
		t.Fatal("rewritten hard-link members do not share an inode")
	}
}

func TestMalformedBREPatternIsRejected(t *testing.T) {
	d := makeTree(t)
	arc := writeArchive(t, d, "src")
	for _, bad := range []string{`/a\(/x/`, `/[z-a]/x/`} {
		_, errs, code := exec(t, d, "", "-f", arc, "-s", bad)
		if code != 2 {
			t.Errorf("%s: expected a usage exit 2, got %d", bad, code)
		}
		if !strings.Contains(errs, "invalid -s pattern") {
			t.Errorf("%s: expected a pattern diagnostic, got %q", bad, errs)
		}
	}
}

func TestSubstitutionWithExtraDelimiterIsRejected(t *testing.T) {
	d := makeTree(t)
	arc := writeArchive(t, d, "src")
	_, errs, code := exec(t, d, "", "-f", arc, "-s", "/a/b//")
	if code != 2 {
		t.Fatalf("expected usage exit 2, got %d (stderr %q)", code, errs)
	}
	if !strings.Contains(errs, "invalid -s expression") {
		t.Fatalf("expected malformed substitution diagnostic, got %q", errs)
	}
}

// -b/-o/-t/-X/-l are recognized so their mode legality is enforced, and
// unsupported options are refused loudly rather than silently accepted.
// (-H and -L are implemented; their behavior is covered in follow_test.go.)
func TestModeOptionLegality(t *testing.T) {
	d := makeTree(t)
	arc := writeArchive(t, d, "src")

	cases := []struct {
		name    string
		args    []string
		wantSub string // substring expected on stderr
	}{
		{"b-in-list-illegal", []string{"-f", arc, "-b", "10k"}, "-b is valid only in write mode"},
		{"b-in-write-invalid", []string{"-w", "-b", "513", "-f", arc, "src"}, "multiple of 512"},
		{"b-in-copy-illegal", []string{"-r", "-w", "-b", "10k", "src", d}, "-b is valid only in write mode"},
		{"o-in-list-unimpl", []string{"-f", arc, "-o", "listopt=x"}, "-o is not supported"},
		{"t-in-list-illegal", []string{"-t", "-f", arc}, "-t is valid only in write or copy mode"},
		{"t-in-read-illegal", []string{"-r", "-t", "-f", arc}, "-t is valid only in write or copy mode"},
		{"t-in-write-unimpl", []string{"-w", "-t", "-f", arc, "src"}, "-t is not supported"},
		{"X-in-list-illegal", []string{"-f", arc, "-X"}, "-X is valid only in write or copy mode"},
		{"X-in-read-illegal", []string{"-r", "-X", "-f", arc}, "-X is valid only in write or copy mode"},
		{"l-in-list-illegal", []string{"-l", "-f", arc}, "-l is valid only in copy mode"},
		{"l-in-write-illegal", []string{"-w", "-l", "-f", arc, "src"}, "-l is valid only in copy mode"},
		{"l-in-copy-unimpl", []string{"-r", "-w", "-l", "src", d}, "-l is not supported"},
		{"c-and-n-together", []string{"-f", arc, "-c", "-n", "x"}, "-c and -n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, errs, code := exec(t, d, "", tc.args...)
			if code != 2 {
				t.Errorf("expected exit 2, got %d (stderr %q)", code, errs)
			}
			if !strings.Contains(errs, tc.wantSub) {
				t.Errorf("expected stderr to contain %q, got %q", tc.wantSub, errs)
			}
		})
	}
}
