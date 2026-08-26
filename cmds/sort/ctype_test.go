package sortcmd

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

// fakeCtype is a deterministic de_DE.ISO-8859-1-shaped LC_CTYPE provider. It
// folds the two accented pairs used by the tests (Ä/ä 0xC4/0xE4, É/é
// 0xC9/0xE9) alongside ASCII a-z, classifies the high-byte letters as
// alphanumeric and printable, and keeps <blank> as space+tab only. No glibc is
// required, so the discriminating behavior is proven on every platform.
type fakeCtype struct {
	closed bool
}

func (f *fakeCtype) IsAlnum(c byte) (bool, error) {
	switch {
	case c >= '0' && c <= '9', c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
		return true, nil
	case c == 0xC4 || c == 0xE4 || c == 0xC9 || c == 0xE9:
		return true, nil
	}
	return false, nil
}

func (f *fakeCtype) IsBlank(c byte) (bool, error) { return c == ' ' || c == '\t', nil }

func (f *fakeCtype) IsPrint(c byte) (bool, error) {
	if c >= 0x20 && c <= 0x7e {
		return true, nil
	}
	// ISO-8859-1 printable high range (Latin-1 supplement) minus the C1
	// controls 0x80-0x9f.
	return c >= 0xa0, nil
}

func (f *fakeCtype) ToUpper(b []byte) ([]byte, error) {
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

func (f *fakeCtype) Close() error { f.closed = true; return nil }

func runSortCtype(t *testing.T, input string, env []string, openCtype ctypeOpener, args ...string) (string, string, int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(), Dir: t.TempDir(),
		Env:   env,
		Stdio: tool.Stdio{In: strings.NewReader(input), Out: &out, Err: &errb},
	}
	code := runWithProviders(rc, args, func(string) (stringCollator, error) {
		return nil, errors.New("collator must not open")
	}, openCtype)
	return out.String(), errb.String(), code
}

// TestSortLCCTypeFoldsHighByteLetters proves -f folds Ä/ä (and É/é) to equal
// under a non-C LC_CTYPE, so a case-insensitive sort groups them and -u
// collapses the pair — behavior the ASCII fold table cannot produce.
func TestSortLCCTypeFoldsHighByteLetters(t *testing.T) {
	f := &fakeCtype{}
	opened := 0
	open := func(string) (ctypeProvider, error) { opened++; return f, nil }
	// Two lines whose keys differ only in case of the umlaut must compare
	// equal under -f; -u then keeps a single representative.
	out, errb, code := runSortCtype(t, "\xe4x\n\xc4x\n", []string{"LC_CTYPE=de_DE.ISO-8859-1"}, open, "-f", "-u")
	if code != 0 || errb != "" {
		t.Fatalf("code %d, stderr %q", code, errb)
	}
	if out != "\xe4x\n" {
		t.Fatalf("-f -u output = %q, want the single folded representative %q", out, "\xe4x\n")
	}
	if opened != 1 {
		t.Fatalf("ctype provider opened %d times, want 1", opened)
	}
	if !f.closed {
		t.Fatal("ctype provider was not closed after snapshot")
	}
}

// TestSortLCCTypeFoldOrdersAcrossCase proves the folded key drives ORDER, not
// just equality: "Äz" and "äa" fold to the same first letter, so the second
// character decides, placing "äa" before "Äz" though byte order would put the
// 0xC4 line first.
func TestSortLCCTypeFoldOrdersAcrossCase(t *testing.T) {
	f := &fakeCtype{}
	out, _, code := runSortCtype(t, "\xc4z\n\xe4a\n", []string{"LC_CTYPE=de_DE.ISO-8859-1"},
		func(string) (ctypeProvider, error) { return f, nil }, "-f")
	if code != 0 {
		t.Fatalf("code %d", code)
	}
	if out != "\xe4a\n\xc4z\n" {
		t.Fatalf("-f order = %q, want folded order %q", out, "\xe4a\n\xc4z\n")
	}
}

// TestSortLCCTypeDictKeepsHighByteAlnum proves -d keeps the locale's
// alphanumeric high bytes (0xC4) while still discarding punctuation, so a line
// whose only distinguishing character is an umlaut is not flattened away.
func TestSortLCCTypeDictKeepsHighByteAlnum(t *testing.T) {
	f := &fakeCtype{}
	// After -d strips '!' and '@', "a\xc4" < "az" iff 0xC4 is retained and
	// ordered after 'z' in byte order (0xC4 > 0x7a); reverse of that pair
	// confirms 0xC4 survived the dictionary filter rather than being dropped.
	out, _, code := runSortCtype(t, "a@z\na!\xc4\n", []string{"LC_CTYPE=de_DE.ISO-8859-1"},
		func(string) (ctypeProvider, error) { return f, nil }, "-d")
	if code != 0 {
		t.Fatalf("code %d", code)
	}
	// Keys after -d: "az" and "a\xc4"; 'z' (0x7a) < 0xC4, so "az" sorts first.
	if out != "a@z\na!\xc4\n" {
		t.Fatalf("-d order = %q, want %q", out, "a@z\na!\xc4\n")
	}
}

// TestSortLCCTypeIgnoreNPDropsNonPrint proves -i drops a C1 control (0x85) that
// the locale classifies as non-printing while keeping the printable high byte.
func TestSortLCCTypeIgnoreNPDropsNonPrint(t *testing.T) {
	f := &fakeCtype{}
	// "a\x85z" and "az" must compare equal once the non-printing 0x85 is
	// dropped by -i; -u then collapses them.
	out, _, code := runSortCtype(t, "a\x85z\naz\n", []string{"LC_CTYPE=de_DE.ISO-8859-1"},
		func(string) (ctypeProvider, error) { return f, nil }, "-i", "-u")
	if code != 0 {
		t.Fatalf("code %d", code)
	}
	if out != "a\x85z\n" && out != "az\n" {
		t.Fatalf("-i -u output = %q, want a single representative", out)
	}
	if strings.Count(out, "\n") != 1 {
		t.Fatalf("-i did not treat lines as equal: %q", out)
	}
}

// TestSortLCCTypeUnsupportedFailsClosed proves a provider open failure under a
// non-C LC_CTYPE with a -f key exits 2 before any input is consumed.
func TestSortLCCTypeUnsupportedFailsClosed(t *testing.T) {
	out, errb, code := runSortCtype(t, "b\na\n", []string{"LC_CTYPE=de_DE.ISO-8859-1"},
		func(string) (ctypeProvider, error) { return nil, errors.New("provider unavailable") }, "-f")
	if code != 2 || out != "" || !strings.Contains(errb, "provider unavailable") {
		t.Fatalf("code/out/err = %d/%q/%q, want fail-closed exit 2", code, out, errb)
	}
}

// TestSortLCCTypeSnapshotFailurePrecedesOutput proves a mid-snapshot provider
// error exits 2, closes the provider, and produces no output.
func TestSortLCCTypeSnapshotFailurePrecedesOutput(t *testing.T) {
	fe := &failingCtype{}
	out, errb, code := runSortCtype(t, "b\na\n", []string{"LC_CTYPE=de_DE.ISO-8859-1"},
		func(string) (ctypeProvider, error) { return fe, nil }, "-d")
	if code != 2 || out != "" || !strings.Contains(errb, "classify broke") {
		t.Fatalf("code/out/err = %d/%q/%q, want snapshot failure exit 2", code, out, errb)
	}
	if !fe.closed {
		t.Fatal("provider was not closed after snapshot failure")
	}
}

type failingCtype struct{ closed bool }

func (failingCtype) IsAlnum(byte) (bool, error)       { return false, errors.New("classify broke") }
func (failingCtype) IsBlank(byte) (bool, error)       { return false, nil }
func (failingCtype) IsPrint(byte) (bool, error)       { return false, nil }
func (failingCtype) ToUpper(b []byte) ([]byte, error) { return b, nil }
func (f *failingCtype) Close() error                  { f.closed = true; return nil }

// TestSortCPOSIXNeverOpensCtype proves that C/POSIX LC_CTYPE keeps the ASCII
// tables and never touches the provider, and that a -f key without any locale
// still folds ASCII case.
func TestSortCPOSIXNeverOpensCtype(t *testing.T) {
	for _, name := range []string{"C", "POSIX", ""} {
		t.Run("ctype="+name, func(t *testing.T) {
			env := []string{}
			if name != "" {
				env = []string{"LC_CTYPE=" + name}
			}
			out, errb, code := runSortCtype(t, "B\na\n", env, func(string) (ctypeProvider, error) {
				return nil, errors.New("must not open")
			}, "-f")
			// ASCII fold: 'a'->'A' (0x41) < 'B' (0x42), so "a" sorts before "B".
			if code != 0 || errb != "" || out != "a\nB\n" {
				t.Fatalf("run = (%q, %q, %d), want ASCII fold order", out, errb, code)
			}
		})
	}
}

// TestSortNoTextKeyNeverOpensCtype proves the provider is left untouched when no
// active key uses -f/-d/-i, even under a non-C LC_CTYPE.
func TestSortNoTextKeyNeverOpensCtype(t *testing.T) {
	out, errb, code := runSortCtype(t, "b\na\n", []string{"LC_CTYPE=de_DE.ISO-8859-1"},
		func(string) (ctypeProvider, error) { return nil, errors.New("must not open") })
	if code != 0 || errb != "" || out != "a\nb\n" {
		t.Fatalf("run = (%q, %q, %d), want plain byte sort with no ctype open", out, errb, code)
	}
}
