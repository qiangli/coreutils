package joincmd

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

// fakeJoinCtype is a deterministic de_DE.ISO-8859-1-shaped LC_CTYPE provider:
// it folds ASCII a-z and the two accented pairs Ä/ä (0xC4/0xE4) and É/é
// (0xC9/0xE9) to uppercase. No glibc is needed, so -i locale folding is proven
// on every platform.
type fakeJoinCtype struct{ closed bool }

func (f *fakeJoinCtype) ToUpper(b []byte) ([]byte, error) {
	out := make([]byte, len(b))
	for i, c := range b {
		switch {
		case c >= 'a' && c <= 'z':
			out[i] = c - ('a' - 'A')
		case c == 0xE4: // ä -> Ä
			out[i] = 0xC4
		case c == 0xE9: // é -> É
			out[i] = 0xC9
		default:
			out[i] = c
		}
	}
	return out, nil
}

func (f *fakeJoinCtype) Close() error { f.closed = true; return nil }

func runJoinWithProviders(t *testing.T, dir string, env []string, open collatorOpener, openCtype ctypeOpener, args ...string) (string, string, int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Env:   env,
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
	}
	code := runWithProviders(rc, args, open, openCtype)
	return out.String(), errb.String(), code
}

func mustNotOpenCollator(t *testing.T) collatorOpener {
	return func(string) (stringCollator, error) {
		t.Helper()
		t.Fatal("collator opened on the -i byte path")
		return nil, nil
	}
}

// TestJoinLCCTypeFoldsHighByteForIgnoreCase pins POSIX join -i under a non-C
// LC_CTYPE: 'Ä' (0xC4) and 'ä' (0xE4) are case variants of one letter, so the
// two lines must pair — a match the ASCII-only fold cannot make. The provider
// is opened once and closed after its uppercase table is snapshotted.
func TestJoinLCCTypeFoldsHighByteForIgnoreCase(t *testing.T) {
	dir := t.TempDir()
	writeJoinFile(t, dir, "f1", string([]byte{0xC4})+" A\n")
	writeJoinFile(t, dir, "f2", string([]byte{0xE4})+" B\n")
	fake := &fakeJoinCtype{}
	opened := ""
	openCtype := func(name string) (ctypeProvider, error) { opened = name; return fake, nil }
	out, errb, code := runJoinWithProviders(t, dir, []string{"LC_CTYPE=de_DE.ISO-8859-1"},
		mustNotOpenCollator(t), openCtype, "-i", "f1", "f2")
	want := string([]byte{0xC4}) + " A B\n"
	if code != 0 || errb != "" || out != want {
		t.Fatalf("run = (%q, %q, %d), want (%q, empty, 0)", out, errb, code, want)
	}
	if opened != "de_DE.ISO-8859-1" || !fake.closed {
		t.Fatalf("opened=%q closed=%v, want opened+closed once", opened, fake.closed)
	}
}

// TestJoinLCCTypeCKeepsASCIIFold proves that under a C/POSIX LC_CTYPE the -i
// path stays on ASCII folding: 'Ä'/'ä' do NOT pair (no provider is opened),
// while ASCII 'A'/'a' still do.
func TestJoinLCCTypeCKeepsASCIIFold(t *testing.T) {
	dir := t.TempDir()
	writeJoinFile(t, dir, "f1", "A x\n"+string([]byte{0xC4})+" y\n")
	writeJoinFile(t, dir, "f2", "a z\n")
	mustNotOpenCtype := func(string) (ctypeProvider, error) {
		t.Fatal("ctype provider opened under C LC_CTYPE")
		return nil, nil
	}
	out, errb, code := runJoinWithProviders(t, dir, []string{"LC_CTYPE=C"},
		mustNotOpenCollator(t), mustNotOpenCtype, "-i", "f1", "f2")
	// 'A' pairs with 'a' via ASCII fold; the 0xC4 line finds no 0xE4 partner.
	if code != 0 || errb != "" || out != "A x z\n" {
		t.Fatalf("run = (%q, %q, %d), want ASCII-only fold pairing", out, errb, code)
	}
}

// TestJoinLCCTypeOpenFailureFailsClosed proves an unopenable LC_CTYPE provider
// under -i fails the invocation (exit 2) before any operand is read.
func TestJoinLCCTypeOpenFailureFailsClosed(t *testing.T) {
	dir := t.TempDir()
	writeJoinFile(t, dir, "f1", string([]byte{0xC4})+" A\n")
	writeJoinFile(t, dir, "f2", string([]byte{0xE4})+" B\n")
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(), Dir: dir,
		Env:   []string{"LC_CTYPE=de_DE.ISO-8859-1"},
		Stdio: tool.Stdio{In: panicReader{}, Out: &out, Err: &errb},
	}
	code := runWithProviders(rc, []string{"-i", "f1", "f2"},
		mustNotOpenCollator(t), func(string) (ctypeProvider, error) {
			return nil, errors.New("provider unavailable")
		})
	if code != 2 || out.Len() != 0 ||
		!strings.Contains(errb.String(), "LC_CTYPE=de_DE.ISO-8859-1") ||
		!strings.Contains(errb.String(), "provider unavailable") {
		t.Fatalf("open failure = (out %q, err %q, code %d), want fail-closed exit 2", out.String(), errb.String(), code)
	}
}

// TestJoinLCCTypeSnapshotFailure proves a provider whose ToUpper errors during
// the snapshot exits 2 and is still closed.
func TestJoinLCCTypeSnapshotFailure(t *testing.T) {
	dir := t.TempDir()
	writeJoinFile(t, dir, "f1", "a A\n")
	writeJoinFile(t, dir, "f2", "a B\n")
	fe := &failingJoinCtype{}
	_, errb, code := runJoinWithProviders(t, dir, []string{"LC_CTYPE=de_DE.ISO-8859-1"},
		mustNotOpenCollator(t), func(string) (ctypeProvider, error) { return fe, nil }, "-i", "f1", "f2")
	if code != 2 || !strings.Contains(errb, "toupper broke") {
		t.Fatalf("snapshot failure = (err %q, code %d), want exit 2", errb, code)
	}
	if !fe.closed {
		t.Fatal("provider was not closed after snapshot failure")
	}
}

type failingJoinCtype struct{ closed bool }

func (failingJoinCtype) ToUpper([]byte) ([]byte, error) { return nil, errors.New("toupper broke") }
func (f *failingJoinCtype) Close() error                { f.closed = true; return nil }

// TestJoinWithoutIgnoreCaseNeverOpensCtype proves the LC_CTYPE provider is only
// consulted for -i: a plain byte-order run under a non-C LC_CTYPE leaves it
// untouched.
func TestJoinWithoutIgnoreCaseNeverOpensCtype(t *testing.T) {
	dir := t.TempDir()
	writeJoinFile(t, dir, "f1", "a A\n")
	writeJoinFile(t, dir, "f2", "a B\n")
	out, errb, code := runJoinWithProviders(t, dir,
		[]string{"LC_CTYPE=de_DE.ISO-8859-1", "LC_COLLATE=C"},
		mustNotOpenCollator(t), func(string) (ctypeProvider, error) {
			t.Fatal("ctype provider opened without -i")
			return nil, nil
		}, "f1", "f2")
	if code != 0 || errb != "" || out != "a A B\n" {
		t.Fatalf("run = (%q, %q, %d), want plain byte join, no ctype open", out, errb, code)
	}
}
