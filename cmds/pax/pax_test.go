package paxcmd

import (
	"archive/tar"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/coreutils/tool"
)

func exec(t *testing.T, dir string, stdin string, args ...string) (string, string, int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Dir: dir, Stdio: tool.Stdio{In: strings.NewReader(stdin), Out: &out, Err: &errb}}
	// Run FIRST and bind the code, then read the buffers. Returning
	// `out.String(), errb.String(), run(...)` reads the buffers before run
	// executes, so every assertion silently sees empty output.
	code := run(rc, args)
	return out.String(), errb.String(), code
}

func makeTree(t *testing.T) string {
	t.Helper()
	d := t.TempDir()
	if err := os.MkdirAll(filepath.Join(d, "src", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	for p, c := range map[string]string{"src/a.txt": "alpha", "src/sub/b.txt": "beta"} {
		if err := os.WriteFile(filepath.Join(d, p), []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return d
}

func TestWriteThenListThenExtractRoundTrips(t *testing.T) {
	d := makeTree(t)
	arc := filepath.Join(d, "out.tar")
	if _, e, code := exec(t, d, "", "-w", "-f", arc, "src"); code != 0 {
		t.Fatalf("write failed: %d %s", code, e)
	}
	out, _, code := exec(t, d, "", "-f", arc)
	if code != 0 {
		t.Fatalf("list failed: %d", code)
	}
	for _, want := range []string{"src/a.txt", "src/sub/b.txt"} {
		if !strings.Contains(out, want) {
			t.Errorf("listing missing %s; got:\n%s", want, out)
		}
	}
	dest := t.TempDir()
	if _, e, code := exec(t, dest, "", "-r", "-f", arc); code != 0 {
		t.Fatalf("extract failed: %d %s", code, e)
	}
	got, err := os.ReadFile(filepath.Join(dest, "src", "sub", "b.txt"))
	if err != nil || string(got) != "beta" {
		t.Fatalf("extracted content wrong: %q %v", got, err)
	}
}

func TestExplicitDashArchiveIsAFile(t *testing.T) {
	d := makeTree(t)
	if _, e, code := exec(t, d, "", "-w", "-f", "-", "src/a.txt"); code != 0 {
		t.Fatalf("write -f - failed: %d %s", code, e)
	}
	archive := filepath.Join(d, "-")
	if info, err := os.Stat(archive); err != nil || info.Size() == 0 {
		t.Fatalf("explicit dash archive = (%v, %v), want non-empty regular file", info, err)
	}
	out, e, code := exec(t, d, "", "-f", "-")
	if code != 0 || e != "" || !strings.Contains(out, "src/a.txt") {
		t.Fatalf("list -f - = (%q, %q, %d), want src/a.txt", out, e, code)
	}
}

// overLongDestinationName builds a member pathname that reaches the
// destination {PATH_MAX} while every component stays within {NAME_MAX}, so
// the total-pathname limit is what rejects it.
func overLongDestinationName() string {
	component := strings.Repeat("d", 200)
	name := component
	for len(name) < destinationPathMax {
		name += "/" + component
	}
	return name + "/f.txt"
}

// POSIX classes a member pathname longer than the destination hierarchy
// allows as an invalid value: with the default invalid=bypass action the
// member is skipped with a diagnostic and a nonzero exit while every other
// member is still extracted. It must not condemn the whole archive.
func TestOverLongMemberPathnameIsBypassedAndProcessingContinues(t *testing.T) {
	d := t.TempDir()
	long := overLongDestinationName()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, m := range []struct{ name, body string }{
		{"good.txt", "ok"},
		{long, "deep"},
	} {
		h := &tar.Header{Name: m.name, Mode: 0o644, Size: int64(len(m.body)), ModTime: time.Unix(1700000000, 0), Format: tar.FormatPAX}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(m.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, "deep.tar"), buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	_, errs, code := exec(t, d, "", "-r", "-f", "deep.tar")
	if code != 1 || errs == "" {
		t.Fatalf("over-long member extract = (%d, %q), want status 1 with a diagnostic", code, errs)
	}
	if got, err := os.ReadFile(filepath.Join(d, "good.txt")); err != nil || string(got) != "ok" {
		t.Fatalf("valid sibling member = (%q, %v), want extracted \"ok\"", got, err)
	}
	if _, err := os.Stat(filepath.Join(d, strings.Repeat("d", 200))); !os.IsNotExist(err) {
		t.Fatalf("bypassed member left destination changes: %v", err)
	}
	// The same archive lists cleanly: length limits belong to the destination
	// hierarchy, and list mode has none.
	out, errs, code := exec(t, d, "", "-f", "deep.tar")
	if code != 0 || errs != "" || !strings.Contains(out, "good.txt") || !strings.Contains(out, long) {
		t.Fatalf("list of over-long member = (%q, %q, %d), want both members and status 0", out, errs, code)
	}
}

// A -s substitution can rewrite a valid archived name into one the
// destination cannot hold. The member is bypassed with a diagnostic and a
// nonzero exit instead of failing mid-extraction.
func TestSubstitutionToOverLongPathnameIsBypassed(t *testing.T) {
	d := makeTree(t)
	if _, e, code := exec(t, d, "", "-w", "-f", "a.tar", "src/a.txt", "src/sub/b.txt"); code != 0 {
		t.Fatalf("write failed: %d %s", code, e)
	}
	dest := t.TempDir()
	long := overLongDestinationName()
	_, errs, code := exec(t, dest, "", "-r", "-f", filepath.Join(d, "a.tar"), "-s", ",^src/a\\.txt$,"+long+",")
	if code != 1 || !strings.Contains(errs, "cannot be created in the destination hierarchy") {
		t.Fatalf("over-long substitution = (%d, %q), want status 1 with a bypass diagnostic", code, errs)
	}
	if got, err := os.ReadFile(filepath.Join(dest, "src", "sub", "b.txt")); err != nil || string(got) != "beta" {
		t.Fatalf("unrelated member = (%q, %v), want extracted \"beta\"", got, err)
	}
	if _, err := os.Stat(filepath.Join(dest, strings.Repeat("d", 200))); !os.IsNotExist(err) {
		t.Fatalf("bypassed member left destination changes: %v", err)
	}
}

func TestEscapingMemberIsDiagnosedAndSkipped(t *testing.T) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, m := range []struct{ name, body string }{
		{"safe.txt", "ok"},
		{"../escape.txt", "bad"},
	} {
		h := &tar.Header{Name: m.name, Mode: 0o644, Size: int64(len(m.body)), Typeflag: tar.TypeReg}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		tw.Write([]byte(m.body))
	}
	tw.Close()

	dest := t.TempDir()
	_, errs, code := exec(t, dest, buf.String(), "-r")
	if code == 0 {
		t.Fatal("an escaping member must fail the run")
	}
	if !strings.Contains(errs, "refusing") {
		t.Errorf("expected a refusal on stderr, got %q", errs)
	}
	if got, err := os.ReadFile(filepath.Join(dest, "safe.txt")); err != nil || string(got) != "ok" {
		t.Fatalf("safe member = (%q, %v), want extracted", got, err)
	}
	if _, err := os.Lstat(filepath.Join(filepath.Dir(dest), "escape.txt")); err == nil {
		t.Error("a member escaped the extraction root")
	}
}

func TestCopyModeUsesTheSameSafetyPathAsExtract(t *testing.T) {
	d := makeTree(t)
	dest := filepath.Join(d, "dest")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, e, code := exec(t, d, "", "-r", "-w", "src", "dest"); code != 0 {
		t.Fatalf("copy failed: %d %s", code, e)
	}
	got, err := os.ReadFile(filepath.Join(dest, "src", "a.txt"))
	if err != nil || string(got) != "alpha" {
		t.Fatalf("copy did not reproduce content: %q %v", got, err)
	}
}

func TestCopyModeReadsSourceOperandsFromStandardInput(t *testing.T) {
	d := makeTree(t)
	dest := filepath.Join(d, "dest")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, e, code := exec(t, d, "src/a.txt\nsrc/sub/b.txt\n", "-r", "-w", "dest"); code != 0 {
		t.Fatalf("copy from standard input failed: %d %s", code, e)
	}
	for path, want := range map[string]string{
		"src/a.txt":     "alpha",
		"src/sub/b.txt": "beta",
	} {
		got, err := os.ReadFile(filepath.Join(dest, path))
		if err != nil || string(got) != want {
			t.Fatalf("copied %s = %q, %v; want %q", path, got, err, want)
		}
	}
}

func TestFirstOperandEndsOptionParsing(t *testing.T) {
	d := t.TempDir()
	if err := os.WriteFile(filepath.Join(d, "source"), []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, destination := range []string{"-d", "--"} {
		if err := os.Mkdir(filepath.Join(d, destination), 0o755); err != nil {
			t.Fatal(err)
		}
		if _, e, code := exec(t, d, "", "-r", "-w", "source", destination); code != 0 {
			t.Fatalf("copy to operand %q failed: %d %s", destination, code, e)
		}
		got, err := os.ReadFile(filepath.Join(d, destination, "source"))
		if err != nil || string(got) != "content" {
			t.Fatalf("copy to %q = %q, %v; want content", destination, got, err)
		}
	}
}

// -s takes ANY delimiter, not just '/'. Assuming '/' breaks the common case of
// rewriting paths, which contain it.
func TestSubstitutionAcceptsAlternateDelimiters(t *testing.T) {
	for _, spec := range []string{"/src/dst/", "#src#dst#", "|src|dst|"} {
		s, err := parseSubstitution(spec)
		if err != nil {
			t.Fatalf("%s: %v", spec, err)
		}
		if got := applySubstitutions([]substitution{s}, "src/a.txt", nil); got != "dst/a.txt" {
			t.Errorf("%s: got %q", spec, got)
		}
	}
}

func TestSubstitutionRegexpAtAdvertisedLimit(t *testing.T) {
	s, err := parseSubstitution(`/^a\{255\}$/x/`)
	if err != nil {
		t.Fatal(err)
	}
	if got := applySubstitutions([]substitution{s}, strings.Repeat("a", 255), nil); got != "x" {
		t.Fatalf("large BRE substitution = %q, want x", got)
	}
	if _, err := parseSubstitution(`/a\{256\}/x/`); err == nil {
		t.Fatal("above-limit BRE compiled")
	}
}

// An empty replacement means "drop this member" — that is how -s is used to
// exclude, so it must not be confused with "leave the name unchanged".
func TestSubstitutionToEmptyDropsTheMember(t *testing.T) {
	s, err := parseSubstitution("/^src\\/sub\\/.*//")
	if err != nil {
		t.Fatal(err)
	}
	if got := applySubstitutions([]substitution{s}, "src/sub/b.txt", nil); got != "" {
		t.Errorf("expected the member to be dropped, got %q", got)
	}
	if got := applySubstitutions([]substitution{s}, "src/a.txt", nil); got != "src/a.txt" {
		t.Errorf("unrelated member must be untouched, got %q", got)
	}
}

func TestSubstitutionAmpersandAndGroups(t *testing.T) {
	s, _ := parseSubstitution("/a\\(.*\\)z/[&]/")
	_ = s
	g, err := parseSubstitution(`/\(xx*\)/<\1>/`)
	if err != nil {
		t.Fatal(err)
	}
	if got := applySubstitutions([]substitution{g}, "axxb", nil); got != "a<xx>b" {
		t.Errorf("group backreference: got %q", got)
	}
	amp, err := parseSubstitution(`/xx*/[&]/`)
	if err != nil {
		t.Fatal(err)
	}
	if got := applySubstitutions([]substitution{amp}, "axxb", nil); got != "a[xx]b" {
		t.Errorf("ampersand whole-match: got %q", got)
	}
}

func TestKeepDoesNotOverwrite(t *testing.T) {
	d := makeTree(t)
	arc := filepath.Join(d, "o.tar")
	exec(t, d, "", "-w", "-f", arc, "src")
	dest := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dest, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(dest, "src", "a.txt")
	os.WriteFile(keep, []byte("ORIGINAL"), 0o644)
	if _, e, code := exec(t, dest, "", "-r", "-k", "-f", arc); code != 0 {
		t.Fatalf("extract -k failed: %d %s", code, e)
	}
	got, _ := os.ReadFile(keep)
	if string(got) != "ORIGINAL" {
		t.Errorf("-k overwrote an existing file: %q", got)
	}
}

func TestPatternSelectionAndComplement(t *testing.T) {
	d := makeTree(t)
	arc := filepath.Join(d, "o.tar")
	exec(t, d, "", "-w", "-f", arc, "src")
	out, _, _ := exec(t, d, "", "-f", arc, "src/sub")
	if !strings.Contains(out, "src/sub/b.txt") || strings.Contains(out, "src/a.txt") {
		t.Errorf("directory operand should select only its subtree; got:\n%s", out)
	}
	out, _, _ = exec(t, d, "", "-f", arc, "-c", "src/sub")
	if strings.Contains(out, "src/sub/b.txt") || !strings.Contains(out, "src/a.txt") {
		t.Errorf("-c should invert the selection; got:\n%s", out)
	}
}

func TestCPIOFormatWritesPOSIXArchive(t *testing.T) {
	d := t.TempDir()
	if err := os.WriteFile(filepath.Join(d, "example"), []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(d, "o.cpio")
	if _, errs, code := exec(t, d, "", "-w", "-x", "cpio", "-f", archive, "example"); code != 0 {
		t.Fatalf("cpio write failed: %d %s", code, errs)
	}
	data, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(data, []byte("070707")) || !bytes.Contains(data, []byte("example\x00")) || !bytes.Contains(data, []byte("TRAILER!!!\x00")) {
		t.Fatalf("invalid POSIX cpio archive: prefix=%q size=%d", data[:6], len(data))
	}
	if len(data)%512 != 0 {
		t.Fatalf("cpio archive size=%d, want 512-byte padding", len(data))
	}
}

func TestUstarFormatDropsUnrepresentableMetadata(t *testing.T) {
	d := t.TempDir()
	if err := os.WriteFile(filepath.Join(d, "example"), []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(d, "o.tar")
	if _, errs, code := exec(t, d, "", "-w", "-x", "ustar", "-f", archive, "example"); code != 0 {
		t.Fatalf("ustar write failed: %d %s", code, errs)
	}
	f, err := os.Open(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	header, err := tar.NewReader(f).Next()
	if err != nil {
		t.Fatal(err)
	}
	if header.Name != "example" || header.Format != tar.FormatUSTAR {
		t.Fatalf("ustar header name=%q format=%v", header.Name, header.Format)
	}
}

// Overwriting an existing destination is pax's DEFAULT. The planner refuses it
// as a policy matter; the command must override that, while still treating an
// escaping member as fatal. Both halves are asserted here because getting
// either backwards is silent: too strict makes pax useless, too loose makes it
// dangerous.
func TestOverwriteIsDefaultButEscapeStaysFatal(t *testing.T) {
	d := makeTree(t)
	arc := filepath.Join(d, "o.tar")
	exec(t, d, "", "-w", "-f", arc, "src")

	dest := t.TempDir()
	os.MkdirAll(filepath.Join(dest, "src"), 0o755)
	victim := filepath.Join(dest, "src", "a.txt")
	os.WriteFile(victim, []byte("STALE"), 0o644)
	if _, e, code := exec(t, dest, "", "-r", "-f", arc); code != 0 {
		t.Fatalf("default extract over an existing file must succeed: %d %s", code, e)
	}
	got, _ := os.ReadFile(victim)
	if string(got) != "alpha" {
		t.Errorf("default extract must overwrite; got %q", got)
	}

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	h := &tar.Header{Name: "../escape.txt", Mode: 0o644, Size: 3, Typeflag: tar.TypeReg}
	tw.WriteHeader(h)
	tw.Write([]byte("bad"))
	tw.Close()
	if _, _, code := exec(t, dest, buf.String(), "-r"); code == 0 {
		t.Error("an escaping member must stay fatal even though overwrite is allowed")
	}
}
