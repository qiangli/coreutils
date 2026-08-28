package lscmd

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

type fakeLsCollator struct {
	closed bool
}

func (f *fakeLsCollator) Compare(a, b string) (int, error) {
	// A deliberately visible non-C order makes the selected category
	// observable without depending on locale archives on the test host.
	return -strings.Compare(a, b), nil
}

func (f *fakeLsCollator) Close() error {
	f.closed = true
	return nil
}

type fakeLsCtype struct {
	closed bool
}

func (f *fakeLsCtype) IsPrint(c byte) (bool, error) {
	return c >= 0x20 && c < 0x7f || c == 0xc3 || c == 0xa4, nil
}

func (f *fakeLsCtype) Close() error {
	f.closed = true
	return nil
}

func runLsLocale(t *testing.T, dir string, env []string, collator collatorOpener, ctype ctypeOpener, args ...string) (string, string, int) {
	t.Helper()
	var out, errOut bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(), Dir: dir, Env: env,
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errOut},
	}
	code := runWithLocale(rc, args, collator, ctype)
	return out.String(), errOut.String(), code
}

func noLsCollator(name string) (stringCollator, error) {
	return nil, fmt.Errorf("unexpected LC_COLLATE provider for %q", name)
}

func noLsCtype(name string) (ctypeProvider, error) {
	return nil, fmt.Errorf("unexpected LC_CTYPE provider for %q", name)
}

// POSIX ls TP804/805/807 reducers: LANG is the fallback, category variables
// override LANG, and a nonempty LC_ALL overrides both. LC_COLLATE controls the
// order in which pathnames are written.
func TestLsLocaleCollationPrecedence(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "a", "")
	write(t, dir, "b", "")

	tests := []struct {
		name string
		env  []string
		want string
	}{
		{"LANG fallback", []string{"LANG=x-test"}, "b\na\n"},
		{"LC_COLLATE over LANG", []string{"LANG=C", "LC_COLLATE=x-test"}, "b\na\n"},
		{"LC_ALL over category", []string{"LANG=C", "LC_COLLATE=C", "LC_ALL=x-test"}, "b\na\n"},
		{"LC_ALL C disables non-C category", []string{"LANG=x-test", "LC_COLLATE=x-test", "LC_ALL=C"}, "a\nb\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var opened *fakeLsCollator
			open := func(name string) (stringCollator, error) {
				if name != "x-test" {
					t.Fatalf("opened locale %q, want x-test", name)
				}
				opened = &fakeLsCollator{}
				return opened, nil
			}
			out, errOut, code := runLsLocale(t, dir, tc.env, open, noLsCtype, "-1")
			if code != 0 || errOut != "" || out != tc.want {
				t.Fatalf("ls locale = (%q, %q, %d), want (%q, empty, 0)", out, errOut, code, tc.want)
			}
			if opened != nil && !opened.closed {
				t.Fatal("collation provider was not closed")
			}
		})
	}
}

// POSIX ls TP804/805/806 reducers: -q replaces characters that LC_CTYPE says
// are non-printable, not every byte outside ASCII. Printable non-ASCII bytes
// supplied by the selected character map must survive byte-for-byte.
func TestLsLocaleCTypePrecedenceForQuestionMark(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "ä", "")

	tests := []struct {
		name string
		env  []string
		want string
	}{
		{"LANG fallback", []string{"LANG=x-test", "LC_COLLATE=C"}, "ä\n"},
		{"LC_CTYPE over LANG", []string{"LANG=C", "LC_COLLATE=C", "LC_CTYPE=x-test"}, "ä\n"},
		{"LC_ALL over category", []string{"LANG=C", "LC_COLLATE=C", "LC_CTYPE=C", "LC_ALL=x-test"}, "ä\n"},
		{"LC_ALL C disables non-C category", []string{"LANG=x-test", "LC_COLLATE=C", "LC_CTYPE=x-test", "LC_ALL=C"}, "??\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var opened *fakeLsCtype
			open := func(name string) (ctypeProvider, error) {
				if name != "x-test" {
					t.Fatalf("opened locale %q, want x-test", name)
				}
				opened = &fakeLsCtype{}
				return opened, nil
			}
			out, errOut, code := runLsLocale(t, dir, tc.env, noLsCollator, open, "-1q", "--sort=none")
			if code != 0 || errOut != "" || out != tc.want {
				t.Fatalf("ls -q locale = (%q, %q, %d), want (%q, empty, 0)", out, errOut, code, tc.want)
			}
			if opened != nil && !opened.closed {
				t.Fatal("ctype provider was not closed")
			}
		})
	}
}
