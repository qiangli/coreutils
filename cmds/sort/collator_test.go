package sortcmd

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

type fakeCollator struct {
	compare func(string, string) (int, error)
	closed  bool
}

func (f *fakeCollator) Compare(a, b string) (int, error) { return f.compare(a, b) }
func (f *fakeCollator) Close() error                     { f.closed = true; return nil }

func reverseText(a, b string) (int, error) { return -strings.Compare(a, b), nil }

func runWithFake(t *testing.T, input, localeName string, open collatorOpener, args ...string) (string, string, int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(), Dir: t.TempDir(),
		Env:   []string{"LC_COLLATE=" + localeName},
		Stdio: tool.Stdio{In: strings.NewReader(input), Out: &out, Err: &errb},
	}
	code := runWithCollator(rc, args, open)
	return out.String(), errb.String(), code
}

func TestRunWithCollatorOrdersTextualComparisons(t *testing.T) {
	cases := []struct {
		name, input, want string
		args              []string
	}{
		{"normal", "a\nb\n", "b\na\n", nil},
		{"reverse", "a\nb\n", "a\nb\n", []string{"-r"}},
		{"key", "x a\nx b\n", "x b\nx a\n", []string{"-k2"}},
		{"stable", "a\nA\n", "a\nA\n", []string{"-s"}},
		{"unique", "a\na\nb\n", "b\na\n", []string{"-u"}},
		{"check", "b\na\n", "", []string{"-c"}},
		{"whole-line-fallback", "2 a\n2 b\n", "2 b\n2 a\n", []string{"-n"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeCollator{compare: reverseText}
			out, errb, code := runWithFake(t, tc.input, "de_DE.ISO-8859-1", func(string) (stringCollator, error) { return f, nil }, tc.args...)
			if code != 0 || out != tc.want || errb != "" {
				t.Fatalf("run = (out %q, err %q, code %d), want (%q, empty, 0)", out, errb, code, tc.want)
			}
			if !f.closed {
				t.Fatal("collator was not closed")
			}
		})
	}
}

func TestRunWithCollatorMerge(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "one"), []byte("b\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "two"), []byte("a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f := &fakeCollator{compare: reverseText}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Env: []string{"LC_COLLATE=de_DE.iso88591"}, Stdio: tool.Stdio{Out: &out, Err: &errb}}
	if code := runWithCollator(rc, []string{"-m", "one", "two"}, func(string) (stringCollator, error) { return f, nil }); code != 0 {
		t.Fatalf("code %d, stderr %s", code, errb.String())
	}
	if got := out.String(); got != "b\na\n" {
		t.Fatalf("output %q, want reverse-collated merge", got)
	}
	if !f.closed {
		t.Fatal("collator was not closed")
	}
}

func TestRunWithCollatorCompareFailureDiscardsOutput(t *testing.T) {
	compares := 0
	f := &fakeCollator{compare: func(string, string) (int, error) {
		compares++
		return 0, errors.New("compare broke")
	}}
	dir := t.TempDir()
	output := filepath.Join(dir, "output")
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Env: []string{"LC_COLLATE=de_DE.ISO-8859-1"}, Stdio: tool.Stdio{In: strings.NewReader("a\nb\n"), Out: &out, Err: &errb}}
	if code := runWithCollator(rc, []string{"-o", output}, func(string) (stringCollator, error) { return f, nil }); code != 2 {
		t.Fatalf("code %d, want 2; stderr %s", code, errb.String())
	}
	if out.Len() != 0 || !strings.Contains(errb.String(), "compare broke") {
		t.Fatalf("output/stderr = %q / %q", out.String(), errb.String())
	}
	if _, err := os.Stat(output); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("output file was touched: %v", err)
	}
	if !f.closed {
		t.Fatal("collator was not closed")
	}
	if compares != 1 {
		t.Fatalf("provider Compare called %d times, want once after its failure", compares)
	}
}

type panicReader struct{}

func (panicReader) Read([]byte) (int, error) { panic("input read before collation initialization") }

func TestRunWithCollatorInitFailureHasNoInputRandomFiles0OrOutputSideEffects(t *testing.T) {
	dir := t.TempDir()
	output := filepath.Join(dir, "output")
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Env: []string{"LC_COLLATE=fr_FR.UTF-8"}, Stdio: tool.Stdio{In: panicReader{}, Out: &out, Err: &errb}}
	called := false
	code := runWithCollator(rc, []string{"--random-source", "missing-random", "--files0-from", "missing-files0", "-o", output}, func(string) (stringCollator, error) {
		called = true
		return nil, errors.New("provider unavailable")
	})
	if code != 2 || !called || !strings.Contains(errb.String(), "provider unavailable") {
		t.Fatalf("code/called/stderr = %d/%v/%q", code, called, errb.String())
	}
	if _, err := os.Stat(output); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("output file was touched: %v", err)
	}
}

func TestRunWithCollatorKeepsCAndPOSIXByteOrdered(t *testing.T) {
	for _, name := range []string{"C", "POSIX"} {
		t.Run(name, func(t *testing.T) {
			opened := false
			out, errb, code := runWithFake(t, "b\na\n", name, func(string) (stringCollator, error) {
				opened = true
				return nil, errors.New("must not open")
			})
			if code != 0 || out != "a\nb\n" || errb != "" || opened {
				t.Fatalf("run = (%q, %q, %d, opened=%v)", out, errb, code, opened)
			}
		})
	}
}
