package shufcmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func runToolDir(t *testing.T, dir, stdin string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Stdio: tool.Stdio{In: strings.NewReader(stdin), Out: &out, Err: &errb},
	}
	code = cmd.Run(rc, args)
	return out.String(), errb.String(), code
}

func runTool(t *testing.T, stdin string, args ...string) (string, string, int) {
	t.Helper()
	return runToolDir(t, t.TempDir(), stdin, args...)
}

// outLines splits shuf output into lines (output is randomly ordered,
// so tests assert on the multiset, not the sequence).
func outLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}

func sorted(ls []string) []string {
	out := append([]string(nil), ls...)
	sort.Strings(out)
	return out
}

func TestShufPermutesStdin(t *testing.T) {
	in := "a\nb\nc\nd\ne\n"
	out, errb, code := runTool(t, in)
	if code != 0 {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	got := sorted(outLines(out))
	want := []string{"a", "b", "c", "d", "e"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("permutation lost lines: got %v want %v", got, want)
	}
}

func TestShufHeadCount(t *testing.T) {
	in := "a\nb\nc\nd\ne\n"
	out, _, code := runTool(t, in, "-n", "2")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	lines := outLines(out)
	if len(lines) != 2 {
		t.Fatalf("-n 2: got %d lines: %q", len(lines), out)
	}
	universe := map[string]bool{"a": true, "b": true, "c": true, "d": true, "e": true}
	if lines[0] == lines[1] || !universe[lines[0]] || !universe[lines[1]] {
		t.Errorf("-n 2: lines must be distinct input lines: %v", lines)
	}
	// -n larger than input emits everything
	out, _, _ = runTool(t, in, "-n", "99")
	if len(outLines(out)) != 5 {
		t.Errorf("-n 99: got %q", out)
	}
	// -n 0 emits nothing
	out, _, code = runTool(t, in, "-n", "0")
	if out != "" || code != 0 {
		t.Errorf("-n 0: out=%q code=%d", out, code)
	}
}

func TestShufEcho(t *testing.T) {
	out, _, code := runTool(t, "", "-e", "x", "y", "z")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	got := sorted(outLines(out))
	if strings.Join(got, ",") != "x,y,z" {
		t.Errorf("-e: got %v", got)
	}
	// -e with no ARGs is an empty input
	out, _, code = runTool(t, "", "-e")
	if out != "" || code != 0 {
		t.Errorf("-e empty: out=%q code=%d", out, code)
	}
}

func TestShufInputRange(t *testing.T) {
	out, _, code := runTool(t, "", "-i", "3-7")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	got := sorted(outLines(out))
	if strings.Join(got, ",") != "3,4,5,6,7" {
		t.Errorf("-i 3-7: got %v", got)
	}
	// empty range LO == HI+1 is allowed and emits nothing
	out, _, code = runTool(t, "", "-i", "1-0")
	if out != "" || code != 0 {
		t.Errorf("-i 1-0: out=%q code=%d", out, code)
	}
	// -n with a huge range must not materialize it
	out, _, code = runTool(t, "", "-i", "1-1000000000000", "-n", "3")
	if code != 0 || len(outLines(out)) != 3 {
		t.Errorf("huge range: out=%q code=%d", out, code)
	}
	seen := map[string]bool{}
	for _, l := range outLines(out) {
		if seen[l] {
			t.Errorf("huge range: duplicate %q in %q", l, out)
		}
		seen[l] = true
	}
}

func TestShufFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f"), []byte("1\n2\n3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// relative operand resolves against rc.Dir
	out, _, code := runToolDir(t, dir, "", "f")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if got := sorted(outLines(out)); strings.Join(got, ",") != "1,2,3" {
		t.Errorf("file: got %v", got)
	}
	// last line without trailing newline still counts as a line
	if err := os.WriteFile(filepath.Join(dir, "g"), []byte("1\n2"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, _ = runToolDir(t, dir, "", "g")
	if got := sorted(outLines(out)); strings.Join(got, ",") != "1,2" {
		t.Errorf("no-final-newline: got %v", got)
	}
	// "-" reads stdin
	out, _, _ = runToolDir(t, dir, "q\n", "-")
	if out != "q\n" {
		t.Errorf("dash: out=%q", out)
	}
	// missing file
	_, errb, code := runToolDir(t, dir, "", "nosuch")
	if code != 1 || !strings.Contains(errb, "shuf: nosuch:") {
		t.Errorf("missing file: code=%d err=%q", code, errb)
	}
}

func TestShufErrors(t *testing.T) {
	// extra operand without -e
	_, errb, code := runTool(t, "", "a", "b")
	if code != 2 || !strings.Contains(errb, "extra operand 'b'") {
		t.Errorf("extra operand: code=%d err=%q", code, errb)
	}
	// -i forbids operands
	_, errb, code = runTool(t, "", "-i", "1-5", "f")
	if code != 2 || !strings.Contains(errb, "extra operand 'f'") {
		t.Errorf("-i operand: code=%d err=%q", code, errb)
	}
	// -e and -i cannot combine
	_, errb, code = runTool(t, "", "-e", "-i", "1-5", "x")
	if code != 2 || !strings.Contains(errb, "cannot combine -e and -i options") {
		t.Errorf("-e -i: code=%d err=%q", code, errb)
	}
	// invalid ranges
	for _, r := range []string{"5-3", "x-3", "1-y", "7", "-1-3"} {
		_, errb, code = runTool(t, "", "-i", r)
		if code != 1 || !strings.Contains(errb, "invalid input range: '"+r+"'") {
			t.Errorf("-i %s: code=%d err=%q", r, code, errb)
		}
	}
	// invalid counts
	for _, n := range []string{"x", "-1", "1.5"} {
		_, errb, code = runTool(t, "", "-n", n)
		if code != 1 || !strings.Contains(errb, "invalid line count: '"+n+"'") {
			t.Errorf("-n %s: code=%d err=%q", n, code, errb)
		}
	}
}

func TestShufUnknownFlag(t *testing.T) {
	_, errb, code := runTool(t, "", "--frobnicate")
	if code != 2 || !strings.Contains(errb, "frobnicate") || !strings.Contains(errb, "pure-Go") {
		t.Errorf("unknown flag: code=%d err=%q", code, errb)
	}
}

func TestShufHelpAndVersion(t *testing.T) {
	out, _, code := runTool(t, "", "--help")
	if code != 0 || !strings.Contains(out, "Usage: shuf") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	out, _, code = runTool(t, "", "--version")
	if code != 0 || !strings.Contains(out, "shuf") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}
}

// An absurd range without -n must fail with GNU's clean diagnostic
// instead of attempting the allocation (host-OOM regression: this
// exact invocation is uutils' #12500 repro and took a host down when
// run before the guard existed).
func TestShufHugeRangeMemoryExhausted(t *testing.T) {
	_, errb, code := runTool(t, "", "-i", "1-18446744073709551615")
	if code != 1 || errb != "shuf: memory exhausted\n" {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	// A huge -n over a huge range is just as unsatisfiable.
	_, errb, code = runTool(t, "", "-n", "18446744073709551615", "-i", "1-18446744073709551615")
	if code != 1 || errb != "shuf: memory exhausted\n" {
		t.Fatalf("huge -n: code=%d err=%q", code, errb)
	}
}

// Sampling a few values from extreme ranges stays cheap, including the
// full uint64 range 0-MaxUint64 whose span overflows uint64.
func TestShufHugeRangeSmallSample(t *testing.T) {
	out, errb, code := runTool(t, "", "-n", "3", "-i", "1-100000000000")
	if code != 0 || errb != "" {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	if got := len(strings.Fields(out)); got != 3 {
		t.Fatalf("want 3 samples, got %d (%q)", got, out)
	}
	out, errb, code = runTool(t, "", "-n", "1", "-i", "0-18446744073709551615")
	if code != 0 || errb != "" {
		t.Fatalf("full u64: code=%d err=%q", code, errb)
	}
	if got := len(strings.Fields(out)); got != 1 {
		t.Fatalf("full u64: want 1 sample, got %d (%q)", got, out)
	}
}

// A count beyond int range means "everything available", as in GNU.
func TestShufHugeHeadCountLineMode(t *testing.T) {
	out, errb, code := runTool(t, "a\nb\n", "-n", "18446744073709551615")
	if code != 0 || errb != "" {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	if got := len(strings.Fields(out)); got != 2 {
		t.Fatalf("want both lines, got %d (%q)", got, out)
	}
}

func TestShufNewOptions(t *testing.T) {
	dir := t.TempDir()

	// 1. Output file (-o)
	out, _, code := runToolDir(t, dir, "a\nb\nc\n", "-o", "out.txt")
	if code != 0 || out != "" {
		t.Errorf("-o output option: out=%q code=%d", out, code)
	}
	data, err := os.ReadFile(filepath.Join(dir, "out.txt"))
	if err != nil {
		t.Fatal(err)
	}
	got := sorted(outLines(string(data)))
	if strings.Join(got, ",") != "a,b,c" {
		t.Errorf("shuf to file lost data: %q", string(data))
	}

	// 2. Repeat (-r)
	out, _, code = runTool(t, "a\n", "-r", "-n", "5")
	if code != 0 || out != "a\na\na\na\na\n" {
		t.Errorf("-r repeat option: out=%q code=%d", out, code)
	}

	// 3. Zero Terminated (-z)
	out, _, code = runTool(t, "a\x00b\x00", "-z")
	if code != 0 {
		t.Errorf("-z option code: %d", code)
	}
	parts := strings.Split(strings.TrimSuffix(out, "\x00"), "\x00")
	sort.Strings(parts)
	if strings.Join(parts, ",") != "a,b" {
		t.Errorf("-z output: %q", out)
	}

	// 4. Random Seed (--random-seed)
	// Output should be deterministic for the same seed!
	out1, _, _ := runTool(t, "", "-i", "1-100", "-n", "10", "--random-seed=12345")
	out2, _, _ := runTool(t, "", "-i", "1-100", "-n", "10", "--random-seed=12345")
	if out1 != out2 {
		t.Errorf("random seed not deterministic: out1=%q out2=%q", out1, out2)
	}

	// 5. Random Source (--random-source)
	srcFile := filepath.Join(dir, "rand_src")
	// 8 bytes for 1 Uint64: let's write 80 bytes for 10 samples
	var srcBytes []byte
	for i := 0; i < 80; i++ {
		srcBytes = append(srcBytes, byte(i))
	}
	if err := os.WriteFile(srcFile, srcBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	out, errb, code := runToolDir(t, dir, "", "-i", "1-100", "-n", "5", "--random-source=rand_src")
	if code != 0 || errb != "" {
		t.Errorf("random source failed: code=%d err=%q", code, errb)
	}
}
