package trcmd

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

// runTrEnv runs tr with an explicit invocation environment and an opener
// that must never be reached, so every assertion below is about the
// character universe LC_CTYPE selects rather than about installed data.
func runTrEnv(t *testing.T, env []string, stdin string, args ...string) (string, string, int) {
	t.Helper()
	return runWithOpener(t, env, stdin, func(name string) (ctypeProvider, error) {
		t.Fatalf("single-byte provider opened for %q", name)
		return nil, nil
	}, args...)
}

func runTrOpener(t *testing.T, env []string, stdin string, opener ctypeOpener, args ...string) (string, string, int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(), Dir: t.TempDir(), Env: env,
		Stdio: tool.Stdio{In: strings.NewReader(stdin), Out: &out, Err: &errb},
	}
	code := runWithCType(rc, args, opener)
	return out.String(), errb.String(), code
}

func posixUTF8() []string { return []string{"POSIXLY_CORRECT=1", "LC_ALL=en_US.UTF-8"} }

// errUnusableLocale stands in for a provider that cannot serve a locale.
var errUnusableLocale = errors.New("locale unavailable")

// TestTrUTF8LocaleIsUsable is the regression for the defect this issue
// closes: tr resolved LC_CTYPE unconditionally and handed any non-C name to
// the single-byte provider, so every invocation under a UTF-8 locale — the
// default on ordinary hosts — failed with status 2 and produced no output.
// XCU:tr:EXIT_STATUS allows greater than zero only "if an error occurs".
func TestTrUTF8LocaleIsUsable(t *testing.T) {
	for _, env := range [][]string{
		{"LC_ALL=en_US.UTF-8"},
		{"LC_CTYPE=C.UTF-8"},
		{"LANG=ja_JP.utf8"},
		{"POSIXLY_CORRECT=1", "LC_ALL=en_US.UTF-8"},
	} {
		out, errOut, code := runTrEnv(t, env, "abc\n", "a-c", "x-z")
		if code != 0 || errOut != "" || out != "xyz\n" {
			t.Fatalf("env=%v: code=%d stdout=%q stderr=%q", env, code, out, errOut)
		}
	}
}

// TestTrPOSIXMultibyteCharacterBoundaries covers XCU:tr:ENVIRONMENT_VARIABLES
// LC_CTYPE, "the interpretation of sequences of bytes of text data as
// characters": a multi-byte character is one operand character and one input
// character, and is never split into its bytes.
func TestTrPOSIXMultibyteCharacterBoundaries(t *testing.T) {
	env := posixUTF8()
	for _, tc := range []struct {
		name string
		args []string
		in   string
		want string
	}{
		{"translate one character", []string{"é", "e"}, "héllo\n", "hello\n"},
		{"delete one character", []string{"-d", "é"}, "héllo\n", "hllo\n"},
		{"squeeze one character", []string{"-s", "é"}, "hééllo\n", "héllo\n"},
		{"translate to a wider character", []string{"e", "界"}, "hello\n", "h界llo\n"},
		{"a shared lead byte does not match", []string{"-d", "é"}, "êé\n", "ê\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errOut, code := runTrEnv(t, env, tc.in, tc.args...)
			if code != 0 || errOut != "" || out != tc.want {
				t.Fatalf("code=%d stdout=%q stderr=%q, want %q", code, out, errOut, tc.want)
			}
		})
	}

	// The C locale is the byte universe, where the same operand matches the
	// second byte of every character sharing that byte.
	out, errOut, code := runTrEnv(t, []string{"POSIXLY_CORRECT=1", "LC_ALL=C"}, "héllo\n", "-d", "é")
	if code != 0 || errOut != "" || out != "hllo\n" {
		t.Fatalf("C locale: code=%d stdout=%q stderr=%q", code, out, errOut)
	}
}

// TestTrPOSIXMultibytePreservesUninterpretableBytes proves a byte that
// begins no character is neither split, dropped, nor rewritten: XCU:tr:STDOUT
// requires the input to be written "identically except for the translations,
// deletions, and repeated-character squeezing requested".
func TestTrPOSIXMultibytePreservesUninterpretableBytes(t *testing.T) {
	env := posixUTF8()
	out, errOut, code := runTrEnv(t, env, "h\xffllo\n", "-d", "l")
	if code != 0 || errOut != "" || out != "h\xffo\n" {
		t.Fatalf("delete: code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	// \377 names that exact byte, which is a single-byte binary value per
	// XCU:tr:OPERANDS, so it can still be selected explicitly.
	out, errOut, code = runTrEnv(t, env, "h\xffllo\n", "-d", "\\377")
	if code != 0 || errOut != "" || out != "hllo\n" {
		t.Fatalf("octal: code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	// A valid encoding of U+FFFD is a real character and is not confused
	// with an uninterpretable byte.
	out, errOut, code = runTrEnv(t, env, "�\xff\n", "-d", "�")
	if code != 0 || errOut != "" || out != "\xff\n" {
		t.Fatalf("replacement: code=%d stdout=%q stderr=%q", code, out, errOut)
	}
}

// TestTrPOSIXMultibyteOctalEscapesAreBytes covers the XCU:tr:OPERANDS
// sentence "Multi-byte characters require multiple, concatenated escape
// sequences of this type, including the leading <backslash> for each byte",
// together with the APPLICATION USAGE note that octal sequences always refer
// to single byte binary values.
func TestTrPOSIXMultibyteOctalEscapesAreBytes(t *testing.T) {
	env := posixUTF8()
	// \303\251 is the UTF-8 encoding of U+00E9: concatenated, the two
	// escapes name one character.
	out, errOut, code := runTrEnv(t, env, "héllo\n", "\\303\\251", "e")
	if code != 0 || errOut != "" || out != "hello\n" {
		t.Fatalf("concatenated: code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	// A single lead byte on its own names no character, so it matches only
	// the raw byte — which a well-formed input never contains alone.
	out, errOut, code = runTrEnv(t, env, "héllo\n", "-d", "\\303")
	if code != 0 || errOut != "" || out != "héllo\n" {
		t.Fatalf("lone lead byte: code=%d stdout=%q stderr=%q", code, out, errOut)
	}
}

// TestTrPOSIXMultibyteClasses covers XCU:tr:OPERANDS "[:class:] ... Represents
// all characters belonging to the defined character class, as defined by the
// current setting of the LC_CTYPE locale category" for the multi-byte
// universe, including the XBD 7.3.1 rule that digit holds only the ten
// decimal digits.
func TestTrPOSIXMultibyteClasses(t *testing.T) {
	env := posixUTF8()
	for _, tc := range []struct {
		name string
		args []string
		in   string
		want string
	}{
		{"alpha spans non-ASCII letters", []string{"-d", "[:alpha:]"}, "aé界1!\n", "1!\n"},
		{"digit is the ten decimal digits", []string{"-d", "[:digit:]"}, "1٢3\n", "٢\n"},
		{"alnum is alpha plus digit", []string{"-d", "[:alnum:]"}, "aé1٢!\n", "٢!\n"},
		{"blank spans non-ASCII spaces", []string{"-d", "[:blank:]"}, "a　 b\n", "ab\n"},
		{"space spans non-ASCII white space", []string{"-d", "[:space:]"}, "a  b\n", "ab"},
		{"upper and lower are Unicode cases", []string{"-d", "[:upper:]"}, "aÉé\n", "aé\n"},
		{"punct spans non-ASCII punctuation", []string{"-d", "[:punct:]"}, "a«b»\n", "ab\n"},
		{"cntrl excludes graphic characters", []string{"-d", "[:cntrl:]"}, "a\x01é\n", "aé"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errOut, code := runTrEnv(t, env, tc.in, tc.args...)
			if code != 0 || errOut != "" || out != tc.want {
				t.Fatalf("code=%d stdout=%q stderr=%q, want %q", code, out, errOut, tc.want)
			}
		})
	}

	// An unrecognised class name is still a diagnosed error.
	_, errOut, code := runTrEnv(t, env, "", "-d", "[:bogus:]")
	if code != 1 || !strings.Contains(errOut, "invalid character class") {
		t.Fatalf("bogus class: code=%d stderr=%q", code, errOut)
	}
}

// TestTrPOSIXMultibyteCaseTranslation covers the XCU:tr:OPERANDS rule that
// [:upper:] and [:lower:] appearing in the same relative positions of string1
// and string2 request case conversion, using the LC_CTYPE case mapping.
func TestTrPOSIXMultibyteCaseTranslation(t *testing.T) {
	env := posixUTF8()
	out, errOut, code := runTrEnv(t, env, "héllo wörld\n", "[:lower:]", "[:upper:]")
	if code != 0 || errOut != "" || out != "HÉLLO WÖRLD\n" {
		t.Fatalf("to upper: code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	out, errOut, code = runTrEnv(t, env, "HÉLLO WÖRLD\n", "[:upper:]", "[:lower:]")
	if code != 0 || errOut != "" || out != "héllo wörld\n" {
		t.Fatalf("to lower: code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	// Both directions in one invocation swap the cases of each character.
	out, errOut, code = runTrEnv(t, env, "Héllo\n", "[:upper:][:lower:]", "[:lower:][:upper:]")
	if code != 0 || errOut != "" || out != "hÉLLO\n" {
		t.Fatalf("swap: code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	// The C locale maps only the ASCII letters, which is what makes the
	// non-ASCII half of the assertions above locale-dependent.
	out, errOut, code = runTrEnv(t, []string{"POSIXLY_CORRECT=1", "LC_ALL=C"}, "héllo\n", "[:lower:]", "[:upper:]")
	if code != 0 || errOut != "" || out != "HéLLO\n" {
		t.Fatalf("C locale: code=%d stdout=%q stderr=%q", code, out, errOut)
	}
}

// TestTrPOSIXMultibyteRanges covers XCU:tr:OPERANDS "c-c", which represents
// the range of collating elements between the endpoints, and the sentence
// that a second endpoint preceding the first makes the construct invalid or
// empty. Ranges here are ordered by character value, which is the collating
// sequence of the POSIX locale.
func TestTrPOSIXMultibyteRanges(t *testing.T) {
	env := posixUTF8()
	out, errOut, code := runTrEnv(t, env, "αβγδ\n", "α-γ", "A-C")
	if code != 0 || errOut != "" || out != "ABCδ\n" {
		t.Fatalf("greek range: code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	// A range whose endpoints are one-byte and multi-byte still covers the
	// characters between them, not the bytes.
	out, errOut, code = runTrEnv(t, env, "a~é\n", "-d", "a-é")
	if code != 0 || errOut != "" || out != "\n" {
		t.Fatalf("mixed-width range: code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	_, errOut, code = runTrEnv(t, env, "", "-d", "γ-α")
	if code != 1 || !strings.Contains(errOut, "reverse collating sequence order") {
		t.Fatalf("reverse range: code=%d stderr=%q", code, errOut)
	}
}

// TestTrPOSIXMultibyteComplement covers XCU:tr:OPTIONS -c/-C over the
// multi-byte universe, which holds more characters than can be enumerated.
func TestTrPOSIXMultibyteComplement(t *testing.T) {
	env := posixUTF8()
	for _, tc := range []struct {
		name string
		args []string
		in   string
		want string
	}{
		{"delete complement", []string{"-cd", "é\n"}, "héllo\n", "é\n"},
		{"squeeze complement", []string{"-cs", "é"}, "haaébb\n", "haéb\n"},
		{"translate complement to one character", []string{"-c", "é\n", "X"}, "héllo\n", "XéXXX\n"},
		// The complemented domain is unbounded, so -t consumes only its
		// first character: U+0080, the first character outside ASCII.
		{"truncate complement", []string{"-ct", "\\000-\\177", "界"}, "a\u0080é\n", "a界é\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errOut, code := runTrEnv(t, env, tc.in, tc.args...)
			if code != 0 || errOut != "" || out != tc.want {
				t.Fatalf("code=%d stdout=%q stderr=%q, want %q", code, out, errOut, tc.want)
			}
		})
	}

	// A [c*] fill has no computable repeat count against an unbounded
	// complemented domain, so it is refused by name instead of guessed.
	out, errOut, code := runTrEnv(t, env, "abc\n", "-c", "abc", "[x*]")
	if code != 2 || out != "" || !strings.Contains(errOut, "is not supported") {
		t.Fatalf("[c*] with -c: code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	// Complemented case classes stay refused with the existing diagnostic.
	_, errOut, code = runTrEnv(t, env, "abc\n", "-c", "abc", "[:upper:]")
	if code != 1 || !strings.Contains(errOut, "must map all characters in the domain to one") {
		t.Fatalf("[:upper:] with -c: code=%d stderr=%q", code, errOut)
	}
}

// TestTrPOSIXMultibyteRepeatAndTruncate covers the [c*n] convention and the
// -t option in the multi-byte universe.
func TestTrPOSIXMultibyteRepeatAndTruncate(t *testing.T) {
	env := posixUTF8()
	for _, tc := range []struct {
		name string
		args []string
		in   string
		want string
	}{
		{"fill pads string2", []string{"éab", "[界*]"}, "éab\n", "界界界\n"},
		{"explicit repeat count", []string{"abc", "[界*2]x"}, "abc\n", "界界x\n"},
		{"truncate string1", []string{"-t", "éab", "界"}, "éab\n", "界ab\n"},
		{"pad with the last character", []string{"éab", "界"}, "éab\n", "界界界\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errOut, code := runTrEnv(t, env, tc.in, tc.args...)
			if code != 0 || errOut != "" || out != tc.want {
				t.Fatalf("code=%d stdout=%q stderr=%q, want %q", code, out, errOut, tc.want)
			}
		})
	}
}

// TestTrPOSIXLocalePrecedenceSelectsCTypeCategory proves the XBD
// Internationalization Variables precedence for the category that selects
// the character universe.
func TestTrPOSIXLocalePrecedenceSelectsCTypeCategory(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  []string
	}{
		{"LC_ALL wins", []string{"POSIXLY_CORRECT=1", "LANG=C", "LC_CTYPE=C", "LC_ALL=en_US.UTF-8"}},
		{"LC_CTYPE over LANG", []string{"POSIXLY_CORRECT=1", "LANG=C", "LC_CTYPE=en_US.UTF-8"}},
		{"LANG last", []string{"POSIXLY_CORRECT=1", "LANG=en_US.UTF-8"}},
		{"empty values fall through", []string{"POSIXLY_CORRECT=1", "LC_ALL=", "LC_CTYPE=", "LANG=en_US.UTF-8"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errOut, code := runTrEnv(t, tc.env, "héllo\n", "-d", "é")
			if code != 0 || errOut != "" || out != "hllo\n" {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errOut)
			}
		})
	}

	// A single-byte locale name still reaches the provider, and LC_ALL still
	// overrides the category variable when it does.
	opened := ""
	opener := func(name string) (ctypeProvider, error) {
		opened = name
		return newFakeProvider(nil), nil
	}
	env := []string{"POSIXLY_CORRECT=1", "LANG=lang", "LC_CTYPE=ctype", "LC_ALL=chosen_8bit"}
	if _, _, code := runTrOpener(t, env, "", opener, "-d", "a"); code != 0 {
		t.Fatalf("provider locale: code=%d", code)
	}
	if opened != "chosen_8bit" {
		t.Fatalf("resolved LC_CTYPE = %q, want chosen_8bit", opened)
	}
}

// TestTrOutsidePOSIXModeKeepsByteCharacters pins the extension tier: GNU
// Coreutils' tr documents no multi-byte support, so without the POSIX switch
// a UTF-8 locale keeps byte characters. It must still never fail for want of
// a single-byte provider carrying that codeset, which is the defect this
// issue closes.
func TestTrOutsidePOSIXModeKeepsByteCharacters(t *testing.T) {
	out, errOut, code := runTrEnv(t, []string{"LC_ALL=en_US.UTF-8"}, "héllo\n", "-d", "é")
	if code != 0 || errOut != "" || out != "hllo\n" {
		t.Fatalf("delete: code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	out, errOut, code = runTrEnv(t, []string{"LC_ALL=en_US.UTF-8"}, "héllo\n", "[:lower:]", "[:upper:]")
	if code != 0 || errOut != "" || out != "HéLLO\n" {
		t.Fatalf("case: code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	// A non-UTF-8 locale name still reaches the provider outside POSIX mode,
	// so an unusable one is still diagnosed rather than silently approximated.
	_, errOut, code = runTrOpener(t, []string{"LC_ALL=unsupported_8bit"}, "", func(string) (ctypeProvider, error) {
		return nil, errUnusableLocale
	}, "-d", "a")
	if code != 2 || !strings.Contains(errOut, "failed to open LC_CTYPE") {
		t.Fatalf("unsupported: code=%d stderr=%q", code, errOut)
	}
}

// TestTrCollationResidualIsRecorded is the standing evidence for the one
// clause this issue does not close. XCU:tr:ENVIRONMENT_VARIABLES gives
// LC_COLLATE "the behavior of range expressions and equivalence classes",
// and XCU:tr:OPERANDS scopes its definition of c-c to the POSIX locale.
// This implementation answers both from character values only: a non-C
// LC_COLLATE changes nothing, and [=c=] holds exactly its own character.
// Closing that needs a multi-byte collation-sequence provider, which does
// not exist in this repository — pkg/collate offers strcoll comparison for
// two ISO-8859-1 aliases and enumerates no equivalence class.
func TestTrCollationResidualIsRecorded(t *testing.T) {
	base := []string{"POSIXLY_CORRECT=1", "LC_CTYPE=en_US.UTF-8"}
	for _, collate := range []string{"C", "en_US.UTF-8", "de_DE.UTF-8"} {
		env := append(append([]string{}, base...), "LC_COLLATE="+collate)
		// An equivalence class expands to its own character only. Under a
		// non-C LC_COLLATE glibc would also admit the accented forms.
		out, errOut, code := runTrEnv(t, env, "hello héllo\n", "[=e=]", "X")
		if code != 0 || errOut != "" || out != "hXllo héllo\n" {
			t.Fatalf("LC_COLLATE=%s equivalence: code=%d stdout=%q stderr=%q", collate, code, out, errOut)
		}
		// A range covers the characters between the endpoints by value,
		// independent of the collating sequence LC_COLLATE names.
		out, errOut, code = runTrEnv(t, env, "aéz\n", "-d", "a-z")
		if code != 0 || errOut != "" || out != "é\n" {
			t.Fatalf("LC_COLLATE=%s range: code=%d stdout=%q stderr=%q", collate, code, out, errOut)
		}
	}
}
