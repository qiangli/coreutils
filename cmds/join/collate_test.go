package joincmd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

// germanWeight models the LC_COLLATE ordering the bounded de_DE.ISO-8859-1
// certification locale must reproduce for join's field comparison: the umlaut
// 0xE4 ('ä') sorts as a secondary of 'a' — after 'a' and before 'b' — whereas
// byte order would place 0xE4 after every ASCII letter. Unlisted bytes keep a
// stable order well past the listed weights so the comparator is total.
func germanWeight(b byte) int {
	switch b {
	case 'a':
		return 10
	case 0xe4: // ä
		return 11
	case 'b':
		return 20
	case 'c':
		return 30
	case 'o':
		return 40
	case 0xf6: // ö
		return 41
	default:
		return 1000 + int(b)
	}
}

func germanCompare(a, b string) int {
	n := min(len(a), len(b))
	for i := 0; i < n; i++ {
		wa, wb := germanWeight(a[i]), germanWeight(b[i])
		switch {
		case wa < wb:
			return -1
		case wa > wb:
			return 1
		}
	}
	switch {
	case len(a) < len(b):
		return -1
	case len(a) > len(b):
		return 1
	}
	return 0
}

type fakeJoinCollator struct {
	compares int
	closed   bool
	err      error
	closeErr error
}

func (f *fakeJoinCollator) Compare(a, b string) (int, error) {
	if f.err != nil {
		return 0, f.err
	}
	f.compares++
	return germanCompare(a, b), nil
}

func (f *fakeJoinCollator) Close() error { f.closed = true; return f.closeErr }

func writeJoinFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func runJoinWithFake(t *testing.T, dir string, env []string, open collatorOpener, args ...string) (string, string, int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Env:   env,
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
	}
	code := runWithCollator(rc, args, open)
	return out.String(), errb.String(), code
}

// TestJoinLCCollateOrdersFieldComparison pins POSIX join ENVIRONMENT
// VARIABLES: under a supported non-C LC_COLLATE, field comparison — join
// equality and the sorted-order check — uses the collating sequence, not byte
// order. File1 is ordered a < ä < b under de_DE collation, which is NOT byte
// order (0xE4 > 'b'); join must still pair the ä group and emit no disorder.
func TestJoinLCCollateOrdersFieldComparison(t *testing.T) {
	dir := t.TempDir()
	f1 := writeJoinFile(t, dir, "f1", "a A\n"+string([]byte{0xe4})+"pfel AE\nb B\n")
	f2 := writeJoinFile(t, dir, "f2", string([]byte{0xe4})+"pfel M\n")
	fake := &fakeJoinCollator{}
	opened := ""
	open := func(name string) (stringCollator, error) { opened = name; return fake, nil }
	out, errb, code := runJoinWithFake(t, dir, []string{"LC_COLLATE=de_DE.iso88591"}, open, f1, f2)
	want := string([]byte{0xe4}) + "pfel AE M\n"
	if code != 0 || errb != "" || out != want {
		t.Fatalf("run = (%q, %q, %d), want (%q, empty, 0)", out, errb, code, want)
	}
	if opened != "de_DE.iso88591" || !fake.closed {
		t.Fatalf("opened=%q closed=%v", opened, fake.closed)
	}
}

// TestJoinLCCollateFlagsCollationDisorder proves the sorted-order check runs
// under the collating sequence: input that is byte-ascending (a, b, ä) is
// collation-DISordered (ä sorts before b), so once an unpairable line has been
// seen join reports "is not sorted" and exits nonzero — a diagnosis byte order
// would never make.
func TestJoinLCCollateFlagsCollationDisorder(t *testing.T) {
	dir := t.TempDir()
	byteSorted := "a\nb\n" + string([]byte{0xe4}) + "\n"
	f1 := writeJoinFile(t, dir, "f1", byteSorted)
	f2 := writeJoinFile(t, dir, "f2", "c\n") // unpairable, arms the order check

	fake := &fakeJoinCollator{}
	out, errb, code := runJoinWithFake(t, dir, []string{"LC_COLLATE=de_DE.iso88591"},
		func(string) (stringCollator, error) { return fake, nil }, f1, f2)
	if code != 1 || !strings.Contains(errb, "is not sorted") || !strings.Contains(errb, "input is not in sorted order") {
		t.Fatalf("collation disorder = (out %q, err %q, code %d), want disorder exit 1", out, errb, code)
	}

	// The identical bytes under C collation are in order: no collator is opened
	// and no disorder is reported.
	out, errb, code = runJoinWithFake(t, dir, []string{"LC_COLLATE=C"},
		func(string) (stringCollator, error) { t.Fatal("C locale opened a collator"); return nil, nil }, f1, f2)
	if code != 0 || strings.Contains(errb, "not sorted") {
		t.Fatalf("C collation = (out %q, err %q, code %d), want ordered exit 0", out, errb, code)
	}
}

// TestJoinCPOSIXAndIgnoreCaseBypassCollator holds join on byte order for the
// C/POSIX locales and — as GNU join does — for -i, which selects a
// case-insensitive comparison rather than a collating one. The opener must not
// be called in any of these invocations.
func TestJoinCPOSIXAndIgnoreCaseBypassCollator(t *testing.T) {
	dir := t.TempDir()
	f1 := writeJoinFile(t, dir, "f1", "A x\na y\n")
	f2 := writeJoinFile(t, dir, "f2", "a z\n")
	mustNotOpen := func(string) (stringCollator, error) {
		t.Fatal("collator opened when byte order was required")
		return nil, nil
	}
	for _, tc := range []struct {
		name string
		env  []string
		args []string
		want string
	}{
		{"c", []string{"LC_COLLATE=C"}, []string{f1, f2}, "a y z\n"},
		{"posix", []string{"LC_COLLATE=POSIX"}, []string{f1, f2}, "a y z\n"},
		{"ignore-case-under-locale", []string{"LC_COLLATE=de_DE.iso88591"}, []string{"-i", f1, f2}, "A x z\na y z\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errb, code := runJoinWithFake(t, dir, tc.env, mustNotOpen, tc.args...)
			if code != 0 || errb != "" || out != tc.want {
				t.Fatalf("run = (%q, %q, %d), want (%q, empty, 0)", out, errb, code, tc.want)
			}
		})
	}
}

// TestJoinLCCollateOpenFailurePrecedesInput proves an unopenable provider or
// unsupported locale fails the invocation (exit 2) before any operand is read:
// the stdin operand's reader panics if touched.
func TestJoinLCCollateOpenFailurePrecedesInput(t *testing.T) {
	dir := t.TempDir()
	f2 := writeJoinFile(t, dir, "f2", "a z\n")
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Env:   []string{"LC_COLLATE=de_DE.iso88591"},
		Stdio: tool.Stdio{In: panicReader{}, Out: &out, Err: &errb},
	}
	code := runWithCollator(rc, []string{"-", f2}, func(string) (stringCollator, error) {
		return nil, errors.New("provider unavailable")
	})
	if code != 2 || !strings.Contains(errb.String(), "LC_COLLATE=de_DE.iso88591") || !strings.Contains(errb.String(), "provider unavailable") {
		t.Fatalf("open failure = (err %q, code %d), want exit 2 diagnostic", errb.String(), code)
	}
	if out.Len() != 0 {
		t.Fatalf("output written despite open failure: %q", out.String())
	}
}

// TestJoinLCCollateCompareFailureIsDiagnosed proves a mid-run collation failure
// is surfaced as a nonzero exit with a diagnostic rather than a silent
// misordered result.
func TestJoinLCCollateCompareFailureIsDiagnosed(t *testing.T) {
	dir := t.TempDir()
	f1 := writeJoinFile(t, dir, "f1", "a x\nb y\n")
	f2 := writeJoinFile(t, dir, "f2", "a z\n")
	fake := &fakeJoinCollator{err: errors.New("compare broke")}
	out, errb, code := runJoinWithFake(t, dir, []string{"LC_COLLATE=de_DE.iso88591"},
		func(string) (stringCollator, error) { return fake, nil }, f1, f2)
	if code != 1 || out != "" || !strings.Contains(errb, "compare broke") {
		t.Fatalf("compare failure = (out %q, err %q, code %d), want no output and exit 1 diagnostic", out, errb, code)
	}
	if !fake.closed {
		t.Fatal("collator was not closed")
	}
}

func TestJoinLCCollateCloseFailureDiscardsOutput(t *testing.T) {
	dir := t.TempDir()
	f1 := writeJoinFile(t, dir, "f1", "a A\n")
	f2 := writeJoinFile(t, dir, "f2", "a B\n")
	fake := &fakeJoinCollator{closeErr: errors.New("close broke")}
	out, errb, code := runJoinWithFake(t, dir, []string{"LC_COLLATE=de_DE.iso88591"},
		func(string) (stringCollator, error) { return fake, nil }, f1, f2)
	if code != 1 || out != "" || !strings.Contains(errb, "close broke") {
		t.Fatalf("close failure = (out %q, err %q, code %d), want no output and exit 1 diagnostic", out, errb, code)
	}
	if !fake.closed {
		t.Fatal("collator close was not attempted")
	}
}

type panicReader struct{}

func (panicReader) Read([]byte) (int, error) { panic("input read before LC_COLLATE validation") }
