package odcmd

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/coreutils/tool"
)

func runOD(t *testing.T, dir, stdin string, args ...string) (string, string, int) {
	return runODProfile(t, runtimeProfile(), dir, stdin, args...)
}

func runODProfile(t *testing.T, profile platformProfile, dir, stdin string, args ...string) (string, string, int) {
	return runODProfileEnv(t, profile, nil, dir, stdin, args...)
}

func runODProfileEnv(t *testing.T, profile platformProfile, env []string, dir, stdin string, args ...string) (string, string, int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Env:   env,
		Stdio: tool.Stdio{In: strings.NewReader(stdin), Out: &out, Err: &errb},
	}
	code := runWithProfile(rc, args, profile)
	return out.String(), errb.String(), code
}

func TestODCTypeLocaleRenderingAndPrecedence(t *testing.T) {
	profile := runtimeProfile()
	cases := []struct {
		name  string
		env   []string
		input string
		want  string
	}{
		{name: "default-posix", input: "\xc3\xa4", want: " 303 244\n"},
		{name: "c", env: []string{"LC_CTYPE=C"}, input: "\xc3\xa4", want: " 303 244\n"},
		{name: "posix", env: []string{"LC_CTYPE=POSIX"}, input: "\xc3\xa4", want: " 303 244\n"},
		{name: "c-utf8", env: []string{"LC_CTYPE=C.UTF-8"}, input: "\xc3\xa4", want: "   ä  **\n"},
		{name: "posix-utf8-alias", env: []string{"LC_CTYPE=POSIX.utf8"}, input: "\xc3\xa4", want: "   ä  **\n"},
		{name: "german-utf8", env: []string{"LC_CTYPE=de_DE.UTF-8"}, input: "\xc3\xa4", want: "   ä  **\n"},
		{name: "german-latin1", env: []string{"LC_CTYPE=de_DE.ISO-8859-1"}, input: "\xe4", want: "   \xe4\n"},
		{name: "lc-all-wins", env: []string{"LANG=de_DE.UTF-8", "LC_CTYPE=de_DE.UTF-8", "LC_ALL=C"}, input: "\xc3\xa4", want: " 303 244\n"},
		{name: "ctype-wins-lang", env: []string{"LANG=C", "LC_CTYPE=de_DE.UTF-8"}, input: "\xc3\xa4", want: "   ä  **\n"},
		{name: "empty-lc-all-falls-through", env: []string{"LANG=C", "LC_CTYPE=de_DE.UTF-8", "LC_ALL="}, input: "\xc3\xa4", want: "   ä  **\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, errb, code := runODProfileEnv(t, profile, tc.env, t.TempDir(), tc.input, "-A", "n", "-t", "c")
			if out != tc.want || errb != "" || code != 0 {
				t.Fatalf("od locale -t c = (%q, %q, %d), want (%q, empty, 0)", out, errb, code, tc.want)
			}
		})
	}
}

func TestODCTypeUTF8ContinuationAcrossOutputGroups(t *testing.T) {
	out, errb, code := runODProfileEnv(t, runtimeProfile(), []string{"LC_CTYPE=C.UTF-8"}, t.TempDir(), "XäY", "-A", "n", "-t", "c", "-w", "2")
	if want := "   X   ä\n  **   Y\n"; out != want || errb != "" || code != 0 {
		t.Fatalf("od split UTF-8 character = (%q, %q, %d), want (%q, empty, 0)", out, errb, code, want)
	}

	// A valid but non-printable multibyte character is one octal field per
	// encoded byte; malformed UTF-8 likewise never becomes U+FFFD.
	for _, tc := range []struct {
		name, input, want string
	}{
		{name: "non-printable", input: "\xc2\x85", want: " 302 205\n"},
		{name: "malformed", input: "\xc3X", want: " 303   X\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errb, code := runODProfileEnv(t, runtimeProfile(), []string{"LC_CTYPE=C.UTF-8"}, t.TempDir(), tc.input, "-A", "n", "-t", "c")
			if out != tc.want || errb != "" || code != 0 {
				t.Fatalf("od %s UTF-8 = (%q, %q, %d), want (%q, empty, 0)", tc.name, out, errb, code, tc.want)
			}
		})
	}
}

func TestODCTypeRequiredEscapesAndOctalFields(t *testing.T) {
	input := []byte{0, '\\', '\a', '\b', '\f', '\n', '\r', '\t', '\v', 0x1b}
	fields, continuation := renderCFields(input, nil, ctypeModel{encoding: ctypeASCII}, 0)
	want := []string{"\\0", "\\", "\\a", "\\b", "\\f", "\\n", "\\r", "\\t", "\\v", "033"}
	if !equalStrings(fields, want) || continuation != 0 {
		t.Fatalf("C/POSIX -t c fields = (%q, %d), want (%q, 0)", fields, continuation, want)
	}

	fields, continuation = renderCFields([]byte{0x9f, 0xa0, 0xe4}, nil, ctypeModel{encoding: ctypeLatin1}, 0)
	want = []string{"237", string([]byte{0xa0}), string([]byte{0xe4})}
	if !equalStrings(fields, want) || continuation != 0 {
		t.Fatalf("ISO-8859-1 -t c fields = (%q, %d), want (%q, 0)", fields, continuation, want)
	}
}

type channelODWriter struct{ writes chan []byte }

func (w channelODWriter) Write(p []byte) (int, error) {
	copyOfP := append([]byte(nil), p...)
	w.writes <- copyOfP
	return len(p), nil
}

func TestODUTF8FullASCIIGroupDoesNotWaitForLookahead(t *testing.T) {
	reader, writer := io.Pipe()
	writes := make(chan []byte, 32)
	done := make(chan error, 1)
	o := options{
		addrRadix: "n",
		formats:   []dumpFormat{{kind: "c", size: 1}},
		endian:    binary.LittleEndian,
		limit:     -1,
		width:     4,
		ctype:     ctypeModel{encoding: ctypeUTF8},
		radix:     '.',
	}
	go func() {
		// A one-byte buffer exposes writeLine's progress before dump waits
		// for the next input group. Production buffering is a separate
		// output policy; this test pins that the decoder itself does not
		// demand bytes beyond a complete ASCII suffix.
		out := bufio.NewWriterSize(channelODWriter{writes: writes}, 1)
		err := dump(reader, out, o)
		_ = out.Flush()
		done <- err
	}()
	if _, err := writer.Write([]byte("ABCD")); err != nil {
		t.Fatal(err)
	}

	var got []byte
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	for len(got) < len("   A   B   C   D") {
		select {
		case p := <-writes:
			got = append(got, p...)
		case err := <-done:
			t.Fatalf("dump returned before rendering the full group: %v", err)
		case <-timer.C:
			_ = writer.Close()
			t.Fatal("od waited for bytes beyond a complete ASCII output group")
		}
	}
	if string(got) != "   A   B   C   D" {
		t.Fatalf("first streamed group = %q, want ASCII group", got)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatalf("dump after pipe EOF: %v", err)
	}
}

func TestODUTF8BoundaryLookaheadIsExact(t *testing.T) {
	for _, tc := range []struct {
		name  string
		block []byte
		want  int
	}{
		{name: "empty", block: nil, want: 0},
		{name: "ascii", block: []byte("ABCD"), want: 0},
		{name: "complete-two-byte", block: []byte("Aä"), want: 0},
		{name: "two-byte-lead", block: []byte{'A', 0xc3}, want: 1},
		{name: "three-byte-lead", block: []byte{'A', 0xe2}, want: 2},
		{name: "three-byte-prefix", block: []byte{'A', 0xe2, 0x82}, want: 1},
		{name: "four-byte-lead", block: []byte{'A', 0xf0}, want: 3},
		{name: "stray-continuation", block: []byte{'A', 0x80}, want: 0},
		{name: "invalid-lead", block: []byte{'A', 0xff}, want: 0},
		{name: "invalid-prefix", block: []byte{'A', 0xe0, 0x80}, want: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := missingUTF8SuffixBytes(tc.block); got != tc.want {
				t.Fatalf("missingUTF8SuffixBytes(% x) = %d, want %d", tc.block, got, tc.want)
			}
		})
	}
}

func TestODLocaleCategoriesFailClosedOnlyWhenUsed(t *testing.T) {
	profile := runtimeProfile()
	opened := false
	profile.openInput = func(string) (io.ReadCloser, error) {
		opened = true
		return io.NopCloser(strings.NewReader("data")), nil
	}

	_, errb, code := runODProfileEnv(t, profile, []string{"LC_CTYPE=fr_FR.UTF-8"}, t.TempDir(), "", "-A", "n", "-t", "c", "input")
	if code != 1 || !strings.Contains(errb, `LC_CTYPE "fr_FR.UTF-8" is unavailable`) || opened {
		t.Fatalf("unsupported LC_CTYPE = (stderr %q, code %d, opened %v), want fail before open", errb, code, opened)
	}

	opened = false
	_, errb, code = runODProfileEnv(t, profile, []string{"LC_NUMERIC=fr_FR.UTF-8"}, t.TempDir(), "", "-A", "n", "-t", "fD", "input")
	if code != 1 || !strings.Contains(errb, `LC_NUMERIC "fr_FR.UTF-8" is unavailable`) || opened {
		t.Fatalf("unsupported LC_NUMERIC = (stderr %q, code %d, opened %v), want fail before open", errb, code, opened)
	}

	// An irrelevant category cannot reject an invocation which never uses it.
	out, errb, code := runODProfileEnv(t, runtimeProfile(), []string{"LC_CTYPE=fr_FR.UTF-8", "LC_NUMERIC=fr_FR.UTF-8"}, t.TempDir(), "A", "-A", "n", "-t", "x1")
	if out != " 41\n" || errb != "" || code != 0 {
		t.Fatalf("irrelevant locale categories = (%q, %q, %d), want hex success", out, errb, code)
	}
}

func TestODNumericLocaleAllFloatingTypesAndABIs(t *testing.T) {
	float32OneAndHalf := []byte{0, 0, 0xc0, 0x3f}
	float64OneAndHalf := []byte{0, 0, 0, 0, 0, 0, 0xf8, 0x3f}
	x87OneAndHalf16 := []byte{0, 0, 0, 0, 0, 0, 0, 0xc0, 0xff, 0x3f, 0, 0, 0, 0, 0, 0}
	x87OneAndHalf12 := x87OneAndHalf16[:12]
	ieee128OneAndHalf := make([]byte, 16)
	binary.LittleEndian.PutUint64(ieee128OneAndHalf[8:], uint64(0x3fff)<<48|uint64(1)<<47)
	ibmOneAndHalf := make([]byte, 16)
	binary.BigEndian.PutUint64(ibmOneAndHalf[:8], math.Float64bits(1.5))

	tests := []struct {
		name    string
		profile platformProfile
		format  string
		data    []byte
	}{
		{name: "fF", profile: platformProfile{abi: abiFor("linux", "amd64"), endian: binary.LittleEndian}, format: "fF", data: float32OneAndHalf},
		{name: "f4", profile: platformProfile{abi: abiFor("linux", "amd64"), endian: binary.LittleEndian}, format: "f4", data: float32OneAndHalf},
		{name: "fD", profile: platformProfile{abi: abiFor("linux", "amd64"), endian: binary.LittleEndian}, format: "fD", data: float64OneAndHalf},
		{name: "f8", profile: platformProfile{abi: abiFor("linux", "amd64"), endian: binary.LittleEndian}, format: "f8", data: float64OneAndHalf},
		{name: "bare-f", profile: platformProfile{abi: abiFor("linux", "amd64"), endian: binary.LittleEndian}, format: "f", data: float64OneAndHalf},
		{name: "x87-16", profile: platformProfile{abi: abiFor("linux", "amd64"), endian: binary.LittleEndian}, format: "fL", data: x87OneAndHalf16},
		{name: "x87-explicit-16", profile: platformProfile{abi: abiFor("linux", "amd64"), endian: binary.LittleEndian}, format: "f16", data: x87OneAndHalf16},
		{name: "x87-12", profile: platformProfile{abi: abiFor("linux", "386"), endian: binary.LittleEndian}, format: "fL", data: x87OneAndHalf12},
		{name: "x87-explicit-12", profile: platformProfile{abi: abiFor("linux", "386"), endian: binary.LittleEndian}, format: "f12", data: x87OneAndHalf12},
		{name: "ieee64-long-double", profile: platformProfile{abi: abiFor("windows", "amd64"), endian: binary.LittleEndian}, format: "fL", data: float64OneAndHalf},
		{name: "ieee128", profile: platformProfile{abi: abiFor("linux", "arm64"), endian: binary.LittleEndian}, format: "fL", data: ieee128OneAndHalf},
		{name: "ieee128-explicit", profile: platformProfile{abi: abiFor("linux", "arm64"), endian: binary.LittleEndian}, format: "f16", data: ieee128OneAndHalf},
		{name: "ibm-double-double", profile: platformProfile{abi: abiFor("linux", "ppc64"), endian: binary.BigEndian}, format: "fL", data: ibmOneAndHalf},
		{name: "ibm-double-double-explicit", profile: platformProfile{abi: abiFor("linux", "ppc64"), endian: binary.BigEndian}, format: "f16", data: ibmOneAndHalf},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.profile.openInput = runtimeProfile().openInput
			out, errb, code := runODProfileEnv(t, tc.profile, []string{"LC_NUMERIC=de_DE.UTF-8"}, t.TempDir(), string(tc.data), "-A", "n", "-t", tc.format)
			if out != " 1,5\n" || errb != "" || code != 0 {
				t.Fatalf("od %s German radix = (%q, %q, %d), want (1,5, empty, 0)", tc.format, out, errb, code)
			}
		})
	}
}

func TestODNumericLocalePrecedenceAndScientificRadix(t *testing.T) {
	data := make([]byte, 8)
	binary.LittleEndian.PutUint64(data, math.Float64bits(1.25e20))
	for _, tc := range []struct {
		name string
		env  []string
		want string
	}{
		{name: "numeric", env: []string{"LC_NUMERIC=de_DE.ISO-8859-1"}, want: " 1,25e+20\n"},
		{name: "lang", env: []string{"LANG=de_DE.UTF-8"}, want: " 1,25e+20\n"},
		{name: "lc-all-comma", env: []string{"LANG=C", "LC_NUMERIC=C", "LC_ALL=de_DE.UTF-8"}, want: " 1,25e+20\n"},
		{name: "lc-all-posix", env: []string{"LANG=de_DE.UTF-8", "LC_NUMERIC=de_DE.UTF-8", "LC_ALL=POSIX"}, want: " 1.25e+20\n"},
		{name: "empty-lc-all", env: []string{"LANG=C", "LC_NUMERIC=de_DE.UTF-8", "LC_ALL="}, want: " 1,25e+20\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			profile := runtimeProfile()
			profile.abi, profile.endian = abiFor("linux", "amd64"), binary.LittleEndian
			out, errb, code := runODProfileEnv(t, profile, tc.env, t.TempDir(), string(data), "-A", "n", "-t", "fD")
			if out != tc.want || errb != "" || code != 0 {
				t.Fatalf("od numeric precedence = (%q, %q, %d), want (%q, empty, 0)", out, errb, code, tc.want)
			}
		})
	}
}

func TestODDefaultOctalWords(t *testing.T) {
	out, errb, code := runOD(t, t.TempDir(), "ABCD")
	want := "0000000 041101 042103\n0000004\n"
	if out != want || errb != "" || code != 0 {
		t.Fatalf("od default = (%q, %q, %d), want (%q, \"\", 0)", out, errb, code, want)
	}
}

func TestODFormatsAndOffsets(t *testing.T) {
	// GNU prints hexadecimal addresses 6 digits wide.
	out, _, code := runOD(t, t.TempDir(), "abc\n", "-A", "x", "-t", "x1", "-N", "3")
	if want := "000000 61 62 63\n000003\n"; out != want || code != 0 {
		t.Fatalf("od x1 = (%q, %d), want (%q, 0)", out, code, want)
	}
	out, _, code = runOD(t, t.TempDir(), "a\n", "-A", "n", "-t", "c")
	if want := "   a  \\n\n"; out != want || code != 0 {
		t.Fatalf("od c no addresses = (%q, %d), want (%q, 0)", out, code, want)
	}
}

func TestODTypeAliases(t *testing.T) {
	out, _, code := runOD(t, t.TempDir(), "AB", "-A", "n", "-t", "xC")
	if want := " 41 42\n"; out != want || code != 0 {
		t.Fatalf("od -t xC = (%q, %d), want (%q, 0)", out, code, want)
	}
	out, _, code = runOD(t, t.TempDir(), "AB", "-A", "n", "-t", "xS")
	if want := " 4241\n"; out != want || code != 0 {
		t.Fatalf("od -t xS = (%q, %d), want (%q, 0)", out, code, want)
	}
	out, _, code = runOD(t, t.TempDir(), "a\n", "-A", "n", "-t", "char")
	if want := "   a  \\n\n"; out != want || code != 0 {
		t.Fatalf("od -t char = (%q, %d), want (%q, 0)", out, code, want)
	}
}

func TestODShortAliasesAndWidth(t *testing.T) {
	out, _, code := runOD(t, t.TempDir(), "abcd", "-b", "-w", "2")
	if want := "0000000 141 142\n0000002 143 144\n0000004\n"; out != want || code != 0 {
		t.Fatalf("od -b -w = (%q, %d), want (%q, 0)", out, code, want)
	}

	out, _, code = runOD(t, t.TempDir(), "AB", "-x")
	if want := "0000000 4241\n0000002\n"; out != want || code != 0 {
		t.Fatalf("od -x = (%q, %d), want (%q, 0)", out, code, want)
	}

	out, _, code = runOD(t, t.TempDir(), "AB", "-d")
	if want := "0000000 16961\n0000002\n"; out != want || code != 0 {
		t.Fatalf("od -d = (%q, %d), want (%q, 0)", out, code, want)
	}
}

func TestODSkipAndFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "in"), []byte("abcd"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, code := runOD(t, dir, "", "-t", "o1", "-j", "2", "in")
	if want := "0000002 143 144\n0000004\n"; out != want || code != 0 {
		t.Fatalf("od skip file = (%q, %d), want (%q, 0)", out, code, want)
	}
}

func TestODMultiFormatEndianStringsAndTraditionalSkip(t *testing.T) {
	out, _, code := runOD(t, t.TempDir(), "ABCD", "-A", "x", "-t", "x2", "-t", "u1", "--endian", "big", "-w", "4")
	want := "000000 4142 4344\n        65  66  67  68\n000004\n"
	if out != want || code != 0 {
		t.Fatalf("od multi/endian = (%q, %d), want (%q, 0)", out, code, want)
	}

	// -S prints NUL-terminated runs only, with no trailing offset line.
	out, _, code = runOD(t, t.TempDir(), "\x00abc\x00de\x00", "-A", "d", "-S", "3")
	want = "0000001 abc\n"
	if out != want || code != 0 {
		t.Fatalf("od strings = (%q, %d), want (%q, 0)", out, code, want)
	}

	out, _, code = runOD(t, t.TempDir(), "abcd", "+2")
	want = "0000002 062143\n0000004\n"
	if out != want || code != 0 {
		t.Fatalf("od traditional skip = (%q, %d), want (%q, 0)", out, code, want)
	}
}

func TestODRejectsBadFormat(t *testing.T) {
	_, errb, code := runOD(t, t.TempDir(), "", "-t", "x4")
	if code != 0 || errb != "" {
		t.Fatalf("od x4 should now be supported code=%d err=%q", code, errb)
	}

	_, errb, code = runOD(t, t.TempDir(), "", "-t", "z9")
	if code != 2 || !strings.Contains(errb, "unsupported output format") {
		t.Fatalf("od bad format code=%d err=%q", code, errb)
	}
}

func TestODRejectsBadWidth(t *testing.T) {
	_, errb, code := runOD(t, t.TempDir(), "", "-w", "0")
	if code != 2 || !strings.Contains(errb, "invalid output width") {
		t.Fatalf("od bad width code=%d err=%q", code, errb)
	}
}

// -t a is named characters (GNU: distinct from -t c, high bit ignored).
func TestODNamedCharsVsC(t *testing.T) {
	out, _, code := runOD(t, t.TempDir(), "a b\n", "-a", "-A", "n")
	if want := "   a  sp   b  nl\n"; out != want || code != 0 {
		t.Fatalf("od -a = (%q, %d), want (%q, 0)", out, code, want)
	}
	out, _, code = runOD(t, t.TempDir(), "a b\n\a\v", "-t", "c", "-A", "n")
	if want := "   a       b  \\n  \\a  \\v\n"; out != want || code != 0 {
		t.Fatalf("od -t c = (%q, %d), want (%q, 0)", out, code, want)
	}
	out, _, code = runOD(t, t.TempDir(), "\xe1", "-a", "-A", "n")
	if want := "   a\n"; out != want || code != 0 {
		t.Fatalf("od -a high bit = (%q, %d), want (%q, 0)", out, code, want)
	}
}

// GNU elides consecutive identical lines with a single *; -v outputs all.
func TestODDuplicateSuppression(t *testing.T) {
	data := strings.Repeat("\x00", 48)
	out, _, code := runOD(t, t.TempDir(), data)
	want := "0000000 000000 000000 000000 000000 000000 000000 000000 000000\n*\n0000060\n"
	if out != want || code != 0 {
		t.Fatalf("od dup = (%q, %d), want (%q, 0)", out, code, want)
	}
	out, _, code = runOD(t, t.TempDir(), data, "-v")
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != 4 || strings.Contains(out, "*") || code != 0 {
		t.Fatalf("od -v = (%q, %d), want 3 data lines + final offset and no *", out, code)
	}
}

// The XSI +offset operand is octal by default; '.' means decimal and a
// trailing 'b' multiplies by 512. -b (an XSI type alias, unlike -t)
// keeps the offset operand recognized.
func TestODTraditionalOffsetRadix(t *testing.T) {
	data := strings.Repeat("x", 20)
	out, _, code := runOD(t, t.TempDir(), data, "-b", "+20")
	if code != 0 || !strings.HasPrefix(out, "0000020 170 170 170 170\n") {
		t.Fatalf("od +20 (octal) = (%q, %d)", out, code)
	}
	out, _, code = runOD(t, t.TempDir(), data, "-b", "+16.")
	if code != 0 || !strings.HasPrefix(out, "0000020 170 170 170 170\n") {
		t.Fatalf("od +16. (decimal) = (%q, %d)", out, code)
	}
	// --traditional keeps the trailing +offset form even when -t closes
	// the POSIX gate.
	out, _, code = runOD(t, t.TempDir(), data, "--traditional", "-t", "o1", "+20")
	if code != 0 || !strings.HasPrefix(out, "0000020 170 170 170 170\n") {
		t.Fatalf("od --traditional -t o1 +20 = (%q, %d)", out, code)
	}
}

// GNU errors when -j skips past the end of the combined input.
func TestODSkipPastEOF(t *testing.T) {
	_, errb, code := runOD(t, t.TempDir(), "hi", "-j", "100")
	if code != 1 || !strings.Contains(errb, "cannot skip past end of combined input") {
		t.Fatalf("od skip past eof: code=%d err=%q", code, errb)
	}
}

func TestODNewTypeAliases(t *testing.T) {
	// octal format alias
	out, _, code := runOD(t, t.TempDir(), "AB", "-A", "n", "-t", "octal1")
	if want := " 101 102\n"; out != want || code != 0 {
		t.Fatalf("od -t octal1 = (%q, %d), want (%q, 0)", out, code, want)
	}
	// hex format alias
	out, _, code = runOD(t, t.TempDir(), "AB", "-A", "n", "-t", "hex1")
	if want := " 41 42\n"; out != want || code != 0 {
		t.Fatalf("od -t hex1 = (%q, %d), want (%q, 0)", out, code, want)
	}
	// signed format alias (maps to d=decimal)
	out, _, code = runOD(t, t.TempDir(), "AB", "-A", "n", "-t", "signed1")
	if want := "   65   66\n"; out != want || code != 0 {
		t.Fatalf("od -t signed1 = (%q, %d), want (%q, 0)", out, code, want)
	}
	// unsigned decimal format alias
	out, _, code = runOD(t, t.TempDir(), "AB", "-A", "n", "-t", "unsigned1")
	if want := "  65  66\n"; out != want || code != 0 {
		t.Fatalf("od -t unsigned1 = (%q, %d), want (%q, 0)", out, code, want)
	}
	// Size aliases: char, short, int, long
	out, _, code = runOD(t, t.TempDir(), "ABCD", "-A", "n", "-t", "xchar")
	if want := " 41 42 43 44\n"; out != want || code != 0 {
		t.Fatalf("od -t xchar = (%q, %d), want (%q, 0)", out, code, want)
	}
	out, _, code = runOD(t, t.TempDir(), "ABCD", "-A", "n", "-t", "xshort")
	if want := " 4241 4443\n"; out != want || code != 0 {
		t.Fatalf("od -t xshort = (%q, %d), want (%q, 0)", out, code, want)
	}
	out, _, code = runOD(t, t.TempDir(), "ABCD", "-A", "n", "-t", "xint")
	if want := " 44434241\n"; out != want || code != 0 {
		t.Fatalf("od -t xint = (%q, %d), want (%q, 0)", out, code, want)
	}
}

func TestODTraditionalOffsetBeforeFile(t *testing.T) {
	// --traditional allows +offset before file name
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "in"), []byte("abcd"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, code := runOD(t, dir, "", "--traditional", "+2", "-t", "o1", "in")
	if want := "0000002 143 144\n0000004\n"; out != want || code != 0 {
		t.Fatalf("od --traditional +2 file = (%q, %d), want (%q, 0)", out, code, want)
	}
}

// POSIX: a bare -t type letter with no explicit size takes the type's
// natural C size — d/o/u/x default to "int" (4 bytes), f defaults to
// "double" (8 bytes) — never the 2-byte "short" default.
func TestODBareTypeDefaultsToIntWidth(t *testing.T) {
	cases := []struct {
		format string
		data   string
		want   string
	}{
		{"d", "ABCD", " 1145258561\n"},
		{"o", "ABCD", " 010420641101\n"},
		{"u", "ABCD", " 1145258561\n"},
		{"x", "ABCD", " 44434241\n"},
		{"f", "\x00\x00\x00\x00\x00\x00\xf0\x3f", " 1\n"},
		// Bare word-form aliases go through the same default-size path.
		{"octal", "ABCD", " 010420641101\n"},
		{"hex", "ABCD", " 44434241\n"},
		{"signed", "ABCD", " 1145258561\n"},
		{"unsigned", "ABCD", " 1145258561\n"},
	}
	for _, tc := range cases {
		out, errb, code := runOD(t, t.TempDir(), tc.data, "-A", "n", "-t", tc.format)
		if out != tc.want || errb != "" || code != 0 {
			t.Fatalf("od -t %s = (%q, %q, %d), want (%q, \"\", 0)", tc.format, out, errb, code, tc.want)
		}
	}
}

func TestODByteCountLowercaseSuffixes(t *testing.T) {
	cases := []struct {
		text string
		want int64
	}{
		{"1", 1},
		{"010", 8},
		{"0x10", 16},
		{"0Xff", 255},
		{"1b", 512},
		{"2b", 1024},
		{"1k", 1024},
		{"3k", 3072},
		{"1m", 1048576},
		{"2m", 2097152},
	}
	for _, tc := range cases {
		got, err := parseBytes(tc.text)
		if err != nil || got != tc.want {
			t.Fatalf("parseBytes(%q) = (%d, %v), want (%d, nil)", tc.text, got, err, tc.want)
		}
	}
}

func TestODByteCountOverflowIsUsageError(t *testing.T) {
	// "G" (1024^3) is already a recognized multiplier; a count this large
	// overflows int64 on multiplication and must fail cleanly rather than
	// silently wrapping.
	if _, err := parseBytes("9223372036854775807G"); err == nil {
		t.Fatalf("parseBytes(overflow) = nil error, want an error")
	}

	_, errb, code := runOD(t, t.TempDir(), "hi", "-N", "9223372036854775807G")
	if code != 2 || !strings.Contains(errb, "invalid byte count") {
		t.Fatalf("od -N overflow: code=%d err=%q, want code 2 and invalid byte count", code, errb)
	}

	_, errb, code = runOD(t, t.TempDir(), "hi", "-j", "9223372036854775807G")
	if code != 2 || !strings.Contains(errb, "invalid skip count") {
		t.Fatalf("od -j overflow: code=%d err=%q, want code 2 and invalid skip count", code, errb)
	}

	// Also confirm overflow via the newly accepted lowercase "m" suffix.
	if _, err := parseBytes("9223372036854775807m"); err == nil {
		t.Fatalf("parseBytes(overflow via m) = nil error, want an error")
	}
}

func TestODJSkipWithLowercaseSuffix(t *testing.T) {
	data := strings.Repeat("a", 1024) + "bc"
	out, errb, code := runOD(t, t.TempDir(), data, "-A", "n", "-t", "c", "-j", "1k", "-w", "2")
	want := "   b   c\n"
	if out != want || errb != "" || code != 0 {
		t.Fatalf("od -j 1k = (%q, %q, %d), want (%q, \"\", 0)", out, errb, code, want)
	}
}

func TestODByteCountRadixPrefixes(t *testing.T) {
	data := "0123456789abcdefXYZ"
	for _, tc := range []struct {
		name string
		arg  string
		want string
	}{
		{name: "decimal", arg: "10", want: "   a   b   c\n"},
		{name: "octal", arg: "010", want: "   8   9   a\n"},
		{name: "hex-lower", arg: "0xa", want: "   a   b   c\n"},
		{name: "hex-upper", arg: "0X10", want: "   X   Y   Z\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errb, code := runOD(t, t.TempDir(), data, "-A", "n", "-t", "c", "-j", tc.arg, "-N", "3")
			if out != tc.want || errb != "" || code != 0 {
				t.Fatalf("od -j %s = (%q, %q, %d), want (%q, empty, 0)", tc.arg, out, errb, code, tc.want)
			}
		})
	}

	for _, arg := range []string{"0x", "018", "0xg", "0X-1"} {
		_, errb, code := runOD(t, t.TempDir(), data, "-j", arg)
		if code != 2 || !strings.Contains(errb, "invalid skip count") {
			t.Errorf("od -j %s = (stderr %q, code %d), want invalid skip count/code 2", arg, errb, code)
		}
	}
}

func TestODShortTypeAliases(t *testing.T) {
	cases := []struct {
		flag string
		data string
		want string
	}{
		{"-D", "ABCD", " 1145258561\n"},
		{"-F", "\x00\x00\x00\x00\x00\x00\xf0\x3f", " 1\n"},
		{"-H", "ABCD", " 44434241\n"},
		{"-I", "ABCD", " 1145258561\n"},
		{"-O", "ABCD", " 010420641101\n"},
		{"-X", "ABCD", " 44434241\n"},
		{"-e", "\x00\x00\x00\x00\x00\x00\xf0\x3f", " 1\n"},
		{"-f", "\x00\x00\x80\x3f", " 1\n"},
		{"-i", "ABCD", " 1145258561\n"},
		{"-s", "ABCD", " 16961 17475\n"},
	}
	for _, tc := range cases {
		out, _, code := runOD(t, t.TempDir(), tc.data, tc.flag, "-A", "n")
		if out != tc.want || code != 0 {
			t.Fatalf("od %s = (%q, %d), want (%q, 0)", tc.flag, out, code, tc.want)
		}
	}
	// POSIX: output lines appear in the order the type options were given.
	out, _, code := runOD(t, t.TempDir(), "ABCD", "-x", "-d", "-A", "n")
	if want := " 4241 4443\n 16961 17475\n"; out != want || code != 0 {
		t.Fatalf("od -x -d = (%q, %d), want (%q, 0)", out, code, want)
	}
}

// POSIX: concatenated format strings like "x1x2" should parse as two formats (x1 and x2)
func TestODConcatenatedFormatStrings(t *testing.T) {
	cases := []struct {
		format string
		data   string
		want   string
	}{
		// Two hex formats
		{"x1x2", "ABCD", " 41 42 43 44\n 4241 4443\n"},
		// Three formats (test smaller data)
		{"x1o1", "AB", " 41 42\n 101 102\n"},
		// Octal and decimal
		{"o1d1", "AB", " 101 102\n   65   66\n"},
	}
	for _, tc := range cases {
		out, errb, code := runOD(t, t.TempDir(), tc.data, "-A", "n", "-t", tc.format)
		if out != tc.want || errb != "" || code != 0 {
			t.Fatalf("od -t %s = (%q, %q, %d), want (%q, \"\", 0)", tc.format, out, errb, code, tc.want)
		}
	}
}

// POSIX: C type codes for floats (fF, fD) and sizes (C, S, I, L with d/o/u/x)
func TestODCTypeCodeFormats(t *testing.T) {
	cases := []struct {
		format string
		data   string
		want   string
	}{
		// Float C type codes
		{"fF", "\x00\x00\x80\x3f", " 1\n"},                 // float (4 bytes)
		{"fD", "\x00\x00\x00\x00\x00\x00\xf0\x3f", " 1\n"}, // double (8 bytes)
		// Integer C type codes
		{"dC", "A", "   65\n"},                       // char (1 byte signed)
		{"dS", "AB", " 16961\n"},                     // short (2 bytes signed, little-endian)
		{"dI", "ABCD", " 1145258561\n"},              // int (4 bytes signed, little-endian)
		{"dL", "ABCDEFGH", " 5208208757389214273\n"}, // long (8 bytes signed, little-endian)
		// Hex with C type codes
		{"xC", "AB", " 41 42\n"},       // byte hex
		{"xS", "ABCD", " 4241 4443\n"}, // short hex
	}
	for _, tc := range cases {
		out, errb, code := runOD(t, t.TempDir(), tc.data, "-A", "n", "-t", tc.format)
		if out != tc.want || errb != "" || code != 0 {
			t.Fatalf("od -t %s = (%q, %q, %d), want (%q, \"\", 0)", tc.format, out, errb, code, tc.want)
		}
	}
}

// POSIX: multiple operand files concatenation
func TestODMultipleFileConcatenation(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f1"), []byte("A"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "f2"), []byte("B"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "f3"), []byte("C"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, code := runOD(t, dir, "", "-A", "n", "-t", "c", "f1", "f2", "f3")
	if want := "   A   B   C\n"; out != want || code != 0 {
		t.Fatalf("od multi-file = (%q, %d), want (%q, 0)", out, code, want)
	}
}

// POSIX: stdin dash operand
func TestODStdinDashOperand(t *testing.T) {
	out, _, code := runOD(t, t.TempDir(), "hello", "-A", "n", "-t", "c", "-")
	if want := "   h   e   l   l   o\n"; out != want || code != 0 {
		t.Fatalf("od stdin dash = (%q, %d), want (%q, 0)", out, code, want)
	}
}

// POSIX: combined format aliases (e.g., octal1hex1)
func TestODCombinedNamedFormats(t *testing.T) {
	cases := []struct {
		format string
		data   string
		want   string
	}{
		{"octal1hex1", "AB", " 101 102\n 41 42\n"},
		{"octal", "ABCD", " 010420641101\n"},
		{"hex", "ABCD", " 44434241\n"},
		{"signed", "ABCD", " 1145258561\n"},
		{"unsigned", "ABCD", " 1145258561\n"},
	}
	for _, tc := range cases {
		out, errb, code := runOD(t, t.TempDir(), tc.data, "-A", "n", "-t", tc.format)
		if out != tc.want || errb != "" || code != 0 {
			t.Fatalf("od -t %s = (%q, %q, %d), want (%q, \"\", 0)", tc.format, out, errb, code, tc.want)
		}
	}
}

// Edge case: byte-exact offsets and final line with -N
func TestODByteExactOffsetAndLimit(t *testing.T) {
	data := "0123456789"
	out, _, code := runOD(t, t.TempDir(), data, "-A", "d", "-t", "c", "-j", "2", "-N", "3")
	if !strings.Contains(out, "2") && !strings.Contains(out, "234") {
		// Should start at offset 2 (decimal) and read 3 bytes
		t.Fatalf("od -j 2 -N 3 = (%q, %d), format not as expected", out, code)
	}
}

// POSIX: output lines are written in the order the type options appear
// on the command line, across repeated -t and the XSI -bcdosx aliases,
// including bundled short flags.
func TestODFormatOrderMatchesCommandLine(t *testing.T) {
	cases := []struct {
		name string
		args []string
		data string
		want string
	}{
		{"b then c", []string{"-A", "n", "-b", "-c"}, "A", " 101\n   A\n"},
		{"c then b", []string{"-A", "n", "-c", "-b"}, "A", "   A\n 101\n"},
		{"bundled bc", []string{"-A", "n", "-bc"}, "A", " 101\n   A\n"},
		{"bundled cb", []string{"-A", "n", "-cb"}, "A", "   A\n 101\n"},
		{"t then alias", []string{"-A", "n", "-t", "x1", "-c"}, "A", " 41\n   A\n"},
		{"alias then t", []string{"-A", "n", "-c", "-t", "x1"}, "A", "   A\n 41\n"},
		{"d t o1 x", []string{"-A", "n", "-d", "-t", "o1", "-x"}, "AB", " 16961\n 101 102\n 4241\n"},
	}
	for _, tc := range cases {
		out, errb, code := runOD(t, t.TempDir(), tc.data, tc.args...)
		if out != tc.want || errb != "" || code != 0 {
			t.Errorf("od %s = (%q, %q, %d), want (%q, \"\", 0)", tc.name, out, errb, code, tc.want)
		}
	}
	// With addresses: only the first line of a block carries the offset;
	// continuation lines are blank-prefixed to the same width.
	out, errb, code := runOD(t, t.TempDir(), "A", "-b", "-c")
	want := "0000000 101\n          A\n0000001\n"
	if out != want || errb != "" || code != 0 {
		t.Fatalf("od -b -c = (%q, %q, %d), want (%q, \"\", 0)", out, errb, code, want)
	}
}

// POSIX (XSI): the [+]offset operand is recognized only when there are
// no more than two operands and none of -A, -j, -N, -t, or -v is
// specified. Otherwise it is a plain filename.
func TestODXSIOffsetGating(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f1"), []byte("abcd"), 0o644); err != nil {
		t.Fatal(err)
	}
	// -t present: +2 is a filename; f1 still dumps, exit 1.
	out, errb, code := runOD(t, dir, "", "-t", "o1", "f1", "+2")
	if code != 1 || !strings.Contains(errb, "+2") {
		t.Fatalf("od -t o1 f1 +2 = (stderr %q, code %d), want +2 open error and exit 1", errb, code)
	}
	if want := "0000000 141 142 143 144\n0000004\n"; out != want {
		t.Fatalf("od -t o1 f1 +2 stdout = %q, want %q", out, want)
	}
	// -A, -j, -N, and -v close the gate too.
	for _, extra := range [][]string{{"-A", "o"}, {"-j", "1"}, {"-N", "2"}, {"-v"}} {
		args := append(append([]string{}, extra...), "f1", "+2")
		_, errb, code := runOD(t, dir, "", args...)
		if code != 1 || !strings.Contains(errb, "+2") {
			t.Errorf("od %v f1 +2 = (stderr %q, code %d), want +2 open error and exit 1", extra, errb, code)
		}
	}
	// More than two operands: the trailing +2 is a filename.
	_, errb, code = runOD(t, dir, "", "f1", "f1", "+2")
	if code != 1 || !strings.Contains(errb, "+2") {
		t.Fatalf("od f1 f1 +2 = (stderr %q, code %d), want +2 open error and exit 1", errb, code)
	}
	// The XSI type aliases do NOT close the gate.
	out, errb, code = runOD(t, dir, "", "-b", "f1", "+2")
	if want := "0000002 143 144\n0000004\n"; out != want || errb != "" || code != 0 {
		t.Fatalf("od -b f1 +2 = (%q, %q, %d), want (%q, \"\", 0)", out, errb, code, want)
	}
}

// POSIX (XSI): with exactly two operands and the last beginning with a
// digit, the last operand is an offset — octal by default, decimal
// with a trailing '.', units of 512 bytes with a trailing 'b'.
func TestODXSITwoOperandNumericOffset(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f"), []byte("0123456789abcdef"), 0o644); err != nil {
		t.Fatal(err)
	}
	want := "0000010   8   9   a   b   c   d   e   f\n0000020\n"
	for _, offset := range []string{"10", "8."} {
		out, errb, code := runOD(t, dir, "", "-c", "f", offset)
		if out != want || errb != "" || code != 0 {
			t.Errorf("od -c f %s = (%q, %q, %d), want (%q, \"\", 0)", offset, out, errb, code, want)
		}
	}
	// 'b' multiplies by 512.
	if err := os.WriteFile(filepath.Join(dir, "big"), []byte(strings.Repeat("x", 514)), 0o644); err != nil {
		t.Fatal(err)
	}
	out, errb, code := runOD(t, dir, "", "-c", "big", "1b")
	if want := "0001000   x   x\n0001002\n"; out != want || errb != "" || code != 0 {
		t.Fatalf("od -c big 1b = (%q, %q, %d), want (%q, \"\", 0)", out, errb, code, want)
	}
	// A single numeric operand is a filename (offset without a file
	// needs a leading '+').
	_, errb, code = runOD(t, dir, "", "10")
	if code != 1 || !strings.Contains(errb, "10") {
		t.Fatalf("od 10 = (stderr %q, code %d), want open error for file \"10\" and exit 1", errb, code)
	}
	// A digit-leading last operand that is not a valid offset is a
	// usage error, not a silent filename fallback.
	_, errb, code = runOD(t, dir, "", "f", "12x4")
	if code != 2 || !strings.Contains(errb, "invalid offset") {
		t.Fatalf("od f 12x4 = (stderr %q, code %d), want invalid offset and exit 2", errb, code)
	}
}

// POSIX: f takes the size letters F (float), D (double), and L (long
// double); d/o/u/x take C, S, I, and L. Cross-class letters are errors.
func TestODFloatSizeLetters(t *testing.T) {
	float1 := "\x00\x00\x80\x3f"                  // float32 1.0
	double1 := "\x00\x00\x00\x00\x00\x00\xf0\x3f" // float64 1.0
	cases := []struct {
		format string
		data   string
		want   string
	}{
		{"fF", float1, " 1\n"},
		{"fD", double1, " 1\n"},
		{"fL", double1, " 1\n"},
	}
	for _, tc := range cases {
		out, errb, code := runOD(t, t.TempDir(), tc.data, "-A", "n", "-t", tc.format)
		if out != tc.want || errb != "" || code != 0 {
			t.Errorf("od -t %s = (%q, %q, %d), want (%q, \"\", 0)", tc.format, out, errb, code, tc.want)
		}
	}
	for _, format := range []string{"fC", "fS", "fI", "dF", "xD", "uF", "oD"} {
		_, errb, code := runOD(t, t.TempDir(), "ABCDEFGH", "-t", format)
		if code != 2 || !strings.Contains(errb, "unsupported output format") {
			t.Errorf("od -t %s = (stderr %q, code %d), want unsupported output format and exit 2", format, errb, code)
		}
	}
}

// POSIX: -N larger than the available input is not an error.
func TestODReadBytesBeyondEOFNotError(t *testing.T) {
	out, errb, code := runOD(t, t.TempDir(), "hi", "-N", "100")
	if want := "0000000 064550\n0000002\n"; out != want || errb != "" || code != 0 {
		t.Fatalf("od -N 100 = (%q, %q, %d), want (%q, \"\", 0)", out, errb, code, want)
	}
}

// POSIX (XSI): -o is equivalent to -t o2.
func TestODOFlagOctal2(t *testing.T) {
	out, errb, code := runOD(t, t.TempDir(), "ab", "-o")
	if want := "0000000 061141\n0000002\n"; out != want || errb != "" || code != 0 {
		t.Fatalf("od -o = (%q, %q, %d), want (%q, \"\", 0)", out, errb, code, want)
	}
}

// POSIX: the size letter L after x selects 8-byte words.
func TestODHexLongSizeLetter(t *testing.T) {
	out, errb, code := runOD(t, t.TempDir(), "ABCDEFGH", "-A", "n", "-t", "xL")
	if want := " 4847464544434241\n"; out != want || errb != "" || code != 0 {
		t.Fatalf("od -t xL = (%q, %q, %d), want (%q, \"\", 0)", out, errb, code, want)
	}
}

// POSIX: skip and count apply to the concatenated input, with the
// cumulative offset carried across file boundaries.
func TestODSkipAndCountAcrossFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f1"), []byte("abcd"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "f2"), []byte("efgh"), 0o644); err != nil {
		t.Fatal(err)
	}
	// -j spans past the first file into the second.
	out, errb, code := runOD(t, dir, "", "-t", "c", "-j", "6", "f1", "f2")
	if want := "0000006   g   h\n0000010\n"; out != want || errb != "" || code != 0 {
		t.Fatalf("od -j 6 f1 f2 = (%q, %q, %d), want (%q, \"\", 0)", out, errb, code, want)
	}
	// -N crosses the file boundary.
	out, errb, code = runOD(t, dir, "", "-A", "n", "-t", "c", "-N", "6", "f1", "f2")
	if want := "   a   b   c   d   e   f\n"; out != want || errb != "" || code != 0 {
		t.Fatalf("od -N 6 f1 f2 = (%q, %q, %d), want (%q, \"\", 0)", out, errb, code, want)
	}
	// -j and -N combined straddle the boundary.
	out, errb, code = runOD(t, dir, "", "-t", "c", "-j", "2", "-N", "4", "f1", "f2")
	if want := "0000002   c   d   e   f\n0000006\n"; out != want || errb != "" || code != 0 {
		t.Fatalf("od -j 2 -N 4 f1 f2 = (%q, %q, %d), want (%q, \"\", 0)", out, errb, code, want)
	}
}

// POSIX: '-' means stdin and may be mixed with named files.
func TestODDashMixedWithFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f1"), []byte("A"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "f2"), []byte("C"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, errb, code := runOD(t, dir, "B", "-A", "n", "-t", "c", "f1", "-", "f2")
	if want := "   A   B   C\n"; out != want || errb != "" || code != 0 {
		t.Fatalf("od f1 - f2 = (%q, %q, %d), want (%q, \"\", 0)", out, errb, code, want)
	}
}

func TestODDefaultUsesNativeByteOrder(t *testing.T) {
	profile := runtimeProfile()
	profile.abi, profile.endian = abiFor("linux", "s390x"), binary.BigEndian
	out, errb, code := runODProfile(t, profile, t.TempDir(), "AB", "-A", "n", "-t", "x2")
	if want := " 4142\n"; out != want || errb != "" || code != 0 {
		t.Fatalf("big-endian native od = (%q, %q, %d), want (%q, empty, 0)", out, errb, code, want)
	}
	out, errb, code = runODProfile(t, profile, t.TempDir(), "AB", "-A", "n", "-t", "x2", "--endian=little")
	if want := " 4241\n"; out != want || errb != "" || code != 0 {
		t.Fatalf("explicit little od = (%q, %q, %d), want (%q, empty, 0)", out, errb, code, want)
	}
	if nativeEndianFor("s390x") != binary.BigEndian || nativeEndianFor("amd64") != binary.LittleEndian {
		t.Fatal("target byte-order table does not distinguish big- and little-endian architectures")
	}
}

func TestODCTypeSizesFollowTargetABI(t *testing.T) {
	linux := abiFor("linux", "amd64")
	windows := abiFor("windows", "amd64")
	if linux.longSize != 8 || windows.longSize != 4 {
		t.Fatalf("C long sizes: linux/amd64=%d windows/amd64=%d, want 8 and 4", linux.longSize, windows.longSize)
	}
	f, err := formatForType('x', "L", "xL", windows)
	if err != nil || f.size != 4 {
		t.Fatalf("windows xL = (%+v, %v), want size 4", f, err)
	}
	f, err = formatForType('d', "I", "dI", windows)
	if err != nil || f.size != 4 {
		t.Fatalf("windows dI = (%+v, %v), want size 4", f, err)
	}
	for _, goos := range []string{"linux", "darwin", "freebsd", "solaris", "openbsd", "netbsd", "dragonfly"} {
		abi := abiFor(goos, "amd64")
		if abi.longDoubleSize != 16 || abi.longDoubleEncoding != floatX87 {
			t.Errorf("%s/amd64 long double = (%d, %d), want 16-byte x87", goos, abi.longDoubleSize, abi.longDoubleEncoding)
		}
	}
	for _, tc := range []struct {
		goos, goarch string
		size         int
		encoding     floatEncoding
	}{
		{goos: "illumos", goarch: "amd64", size: 16, encoding: floatX87},
		{goos: "ios", goarch: "arm64", size: 8, encoding: floatIEEE64},
		{goos: "linux", goarch: "mips", size: 8, encoding: floatIEEE64},
		{goos: "linux", goarch: "mipsle", size: 8, encoding: floatIEEE64},
		{goos: "aix", goarch: "ppc64", size: 8, encoding: floatIEEE64},
		{goos: "linux", goarch: "ppc64", size: 16, encoding: floatIBMDoubleDouble},
		{goos: "linux", goarch: "ppc64le", size: 16, encoding: floatIBMDoubleDouble},
		{goos: "plan9", goarch: "arm", size: 0, encoding: floatNone},
	} {
		abi := abiFor(tc.goos, tc.goarch)
		if abi.longDoubleSize != tc.size || abi.longDoubleEncoding != tc.encoding {
			t.Errorf("%s/%s long double = (%d, %d), want (%d, %d)", tc.goos, tc.goarch,
				abi.longDoubleSize, abi.longDoubleEncoding, tc.size, tc.encoding)
		}
	}
}

func TestODAIXPPC64LongDoubleIsIEEE64(t *testing.T) {
	profile := runtimeProfile()
	profile.abi, profile.endian = abiFor("aix", "ppc64"), binary.BigEndian
	one := string([]byte{0x3f, 0xf0, 0, 0, 0, 0, 0, 0})
	for _, format := range []string{"fL", "f8"} {
		out, errb, code := runODProfile(t, profile, t.TempDir(), one, "-A", "n", "-t", format)
		if want := " 1\n"; out != want || errb != "" || code != 0 {
			t.Errorf("AIX ppc64 %s = (%q, %q, %d), want (%q, empty, 0)", format, out, errb, code, want)
		}
	}
}

// GCC 15.2's powerpc64 target assembly supplies the component bytes for 1/3,
// maximum, and minimum subnormal below, for both -mbig-endian and
// -mlittle-endian. The expected fields are GNU od 9.4 outputs from Ubuntu
// 24.04 ppc64le; the big-endian cases encode the same exact component values.
func TestODLinuxPPC64IBMDoubleDoubleOracles(t *testing.T) {
	profiles := []struct {
		name    string
		profile platformProfile
		encode  func(uint64) []byte
	}{
		{name: "ppc64", profile: platformProfile{abi: abiFor("linux", "ppc64"), endian: binary.BigEndian}, encode: func(v uint64) []byte {
			b := make([]byte, 8)
			binary.BigEndian.PutUint64(b, v)
			return b
		}},
		{name: "ppc64le", profile: platformProfile{abi: abiFor("linux", "ppc64le"), endian: binary.LittleEndian}, encode: func(v uint64) []byte {
			b := make([]byte, 8)
			binary.LittleEndian.PutUint64(b, v)
			return b
		}},
	}
	for _, target := range profiles {
		t.Run(target.name, func(t *testing.T) {
			target.profile.openInput = runtimeProfile().openInput
			for _, tc := range []struct {
				name      string
				hi, lo    uint64
				want      string
				wantClass floatClass
				wantNeg   bool
			}{
				{name: "third", hi: 0x3fd5555555555555, lo: 0x3c75555555555556, want: " 0.333333333333333333333333333333335\n"},
				{name: "maximum", hi: 0x7fefffffffffffff, lo: 0x7c8ffffffffffffe, want: " 1.79769313486231580793728971405301e+308\n"},
				{name: "infinity", hi: 0x7ff0000000000000, want: " inf\n", wantClass: floatInfinity},
				{name: "nan", hi: 0x7ff8000000000000, want: " nan\n", wantClass: floatNaN},
				{name: "minimum subnormal", hi: 1, want: " 5e-324\n"},
			} {
				data := append(target.encode(tc.hi), target.encode(tc.lo)...)
				decoded := decodeIBMDoubleDouble(append([]byte(nil), data...), target.profile.endian)
				if decoded.class != tc.wantClass || decoded.negative != tc.wantNeg {
					t.Errorf("%s decode class/sign = (%d, %v), want (%d, %v)", tc.name, decoded.class, decoded.negative, tc.wantClass, tc.wantNeg)
				}
				out, errb, code := runODProfile(t, target.profile, t.TempDir(), string(data), "-A", "n", "-t", "fL")
				if out != tc.want || errb != "" || code != 0 {
					t.Errorf("%s = (%q, %q, %d), want (%q, empty, 0)", tc.name, out, errb, code, tc.want)
				}
			}
		})
	}
}

// The 16 bytes for 1.0L below are the oracle produced by
//
//	long double v = 1.0L; fwrite(&v, sizeof v, 1, stdout);
//
// on Ubuntu amd64 (sizeof(long double)==16, x87 80-bit payload), whose
// value GNU od -An -t fL renders as 1. This pins the representation and
// size, not merely an fD-compatible first eight bytes.
func TestODUbuntuAMD64LongDoubleGNUOracleAndPortableTargets(t *testing.T) {
	profile := runtimeProfile()
	profile.abi, profile.endian = abiFor("linux", "amd64"), binary.LittleEndian
	one := []byte{0, 0, 0, 0, 0, 0, 0, 0x80, 0xff, 0x3f, 0, 0, 0, 0, 0, 0}
	out, errb, code := runODProfile(t, profile, t.TempDir(), string(one), "-A", "n", "-t", "fL")
	if want := " 1\n"; out != want || errb != "" || code != 0 {
		t.Fatalf("linux/amd64 fL = (%q, %q, %d), want (%q, empty, 0)", out, errb, code, want)
	}
	big := append([]byte(nil), one...)
	reverseBytes(big)
	out, errb, code = runODProfile(t, profile, t.TempDir(), string(big), "-A", "n", "-t", "fL", "--endian=big")
	if want := " 1\n"; out != want || errb != "" || code != 0 {
		t.Fatalf("big-endian x87 fL = (%q, %q, %d), want (%q, empty, 0)", out, errb, code, want)
	}

	profile.abi, profile.endian = abiFor("linux", "arm64"), binary.LittleEndian
	quadOne := make([]byte, 16)
	binary.LittleEndian.PutUint64(quadOne[8:], uint64(0x3fff)<<48)
	out, errb, code = runODProfile(t, profile, t.TempDir(), string(quadOne), "-A", "n", "-t", "fL")
	if want := " 1\n"; out != want || errb != "" || code != 0 {
		t.Fatalf("linux/arm64 fL = (%q, %q, %d), want (%q, empty, 0)", out, errb, code, want)
	}

	profile.abi, profile.endian = abiFor("linux", "ppc64"), binary.BigEndian
	ppcOne := []byte{0x3f, 0xf0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	out, errb, code = runODProfile(t, profile, t.TempDir(), string(ppcOne), "-A", "n", "-t", "fL")
	if want := " 1\n"; out != want || errb != "" || code != 0 {
		t.Fatalf("linux/ppc64 fL = (%q, %q, %d), want (%q, empty, 0)", out, errb, code, want)
	}
}

// These byte strings are compiler oracles for 1/3 in the target formats. The
// expected text is the numeric field emitted by GNU od 9.4 on Ubuntu 24.04
// amd64 and arm64 (without GNU's implementation-specific field padding).
// Keeping the exact significand through rendering produces digits that are
// observably lost if either value is first converted to float64.
func TestODLongDoublePrecisionOracles(t *testing.T) {
	profile := runtimeProfile()
	profile.abi, profile.endian = abiFor("linux", "amd64"), binary.LittleEndian
	for _, tc := range []struct {
		name string
		data []byte
		want string
	}{
		{name: "third", data: []byte{0xab, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xfd, 0x3f, 0, 0, 0, 0, 0, 0}, want: " 0.33333333333333333334\n"},
		{name: "max finite", data: []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xfe, 0x7f, 0, 0, 0, 0, 0, 0}, want: " 1.189731495357231765e+4932\n"},
	} {
		out, errb, code := runODProfile(t, profile, t.TempDir(), string(tc.data), "-A", "n", "-t", "fL")
		if out != tc.want || errb != "" || code != 0 {
			t.Errorf("x87 %s = (%q, %q, %d), want (%q, empty, 0)", tc.name, out, errb, code, tc.want)
		}
	}

	// IEEE binary128 1/3 is 0x3ffd5555555555555555555555555555.
	profile.abi, profile.endian = abiFor("linux", "arm64"), binary.LittleEndian
	for _, tc := range []struct {
		name string
		data []byte
		want string
	}{
		{name: "third", data: []byte{0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0xfd, 0x3f}, want: " 0.3333333333333333333333333333333333\n"},
		{name: "max finite", data: []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xfe, 0x7f}, want: " 1.189731495357231765085759326628007e+4932\n"},
	} {
		out, errb, code := runODProfile(t, profile, t.TempDir(), string(tc.data), "-A", "n", "-t", "fL")
		if out != tc.want || errb != "" || code != 0 {
			t.Errorf("IEEE128 %s = (%q, %q, %d), want (%q, empty, 0)", tc.name, out, errb, code, tc.want)
		}
	}
}

func TestODPartialItemAppendsNullBytes(t *testing.T) {
	out, errb, code := runOD(t, t.TempDir(), "A", "-A", "n", "-t", "x2", "--endian=big")
	if want := " 4100\n"; out != want || errb != "" || code != 0 {
		t.Fatalf("big-endian partial x2 = (%q, %q, %d), want (%q, empty, 0)", out, errb, code, want)
	}
}

func TestODExactTypeGrammar(t *testing.T) {
	profile := runtimeProfile()
	profile.abi, profile.endian = abiFor("linux", "amd64"), binary.LittleEndian
	for _, format := range []string{"", "a1", "cC", "fI", "fLD", "dD", "xF", "x0", "f3", "x1,x2", "x1 x2", "x1\tx2"} {
		_, errb, code := runODProfile(t, profile, t.TempDir(), "0123456789abcdef", "-t", format)
		if code != 2 || !strings.Contains(errb, "unsupported output format") {
			t.Errorf("od -t %s = (stderr %q, code %d), want grammar error/code 2", format, errb, code)
		}
	}
	if _, err := parseFormats([]string{"f16dI"}, profile.abi); err != nil {
		t.Fatalf("valid concatenated f16dI rejected: %v", err)
	}
}

func TestODTraditionalOffsetOverflow(t *testing.T) {
	for _, offset := range []string{"+1000000000000000000b", "+2000000000000000000b"} {
		_, errb, code := runOD(t, t.TempDir(), "A", offset)
		if code != 2 || !strings.Contains(errb, "invalid offset") {
			t.Errorf("od %s = (stderr %q, code %d), want invalid offset/code 2", offset, errb, code)
		}
	}
}

type failingODWriter struct{ err error }

func (w failingODWriter) Write([]byte) (int, error) { return 0, w.err }

type errorReadCloser struct {
	io.Reader
	err error
}

func (r errorReadCloser) Close() error { return r.err }

func TestODOutputAndCloseErrorsSetStatus(t *testing.T) {
	var errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: t.TempDir(), Stdio: tool.Stdio{
		In: strings.NewReader("A"), Out: failingODWriter{err: errors.New("broken output")}, Err: &errb,
	}}
	if code := cmd.Run(rc, []string{"-A", "n", "-t", "x1"}); code != 1 || !strings.Contains(errb.String(), "write error: broken output") {
		t.Fatalf("output failure = (stderr %q, code %d), want write diagnostic/code 1", errb.String(), code)
	}

	profile := runtimeProfile()
	profile.openInput = func(string) (io.ReadCloser, error) {
		return errorReadCloser{Reader: strings.NewReader("A"), err: errors.New("broken close")}, nil
	}
	out, errText, code := runODProfile(t, profile, t.TempDir(), "", "-A", "n", "-t", "x1", "input")
	if out != " 41\n" || code != 1 || !strings.Contains(errText, "input: close error:") || !strings.Contains(errText, "roken close") {
		t.Fatalf("close failure = (%q, %q, %d), want data plus close diagnostic/code 1", out, errText, code)
	}
}

// An unreadable operand yields a diagnostic and exit 1, but the
// remaining files are still dumped.
func TestODMissingFileContinues(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f2"), []byte("efgh"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, errb, code := runOD(t, dir, "", "-A", "n", "-t", "c", "missing", "f2")
	if code != 1 || !strings.Contains(errb, "missing") {
		t.Fatalf("od missing f2 = (stderr %q, code %d), want diagnostic and exit 1", errb, code)
	}
	if want := "   e   f   g   h\n"; out != want {
		t.Fatalf("od missing f2 stdout = %q, want %q", out, want)
	}
}
