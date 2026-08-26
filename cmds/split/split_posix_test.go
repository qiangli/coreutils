package splitcmd

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

// TestSplitEmptyInputCreatesNoFiles pins the Issue 7 OUTPUT FILES behavior for
// an empty input: no output file is created and the exit status is success.
func TestSplitEmptyInputCreatesNoFiles(t *testing.T) {
	for _, mode := range [][]string{
		nil,         // default line mode
		{"-l", "1"}, // explicit lines
		{"-b", "4"}, // bytes
		{"-C", "4"}, // line-bytes
		{"-t", ":"}, // record separator
	} {
		t.Run(strings.Join(mode, " "), func(t *testing.T) {
			dir := t.TempDir()
			out, errb, code := runTool(t, dir, "", mode...)
			if code != 0 || out != "" || errb != "" {
				t.Fatalf("empty input %v = (%q, %q, %d), want ('', '', 0)", mode, out, errb, code)
			}
			if files := listFiles(t, dir); len(files) != 0 {
				t.Fatalf("empty input created files: %v", files)
			}
		})
	}
}

// TestSplitPartialFinalLine proves an input whose final line has no trailing
// newline is preserved byte-for-byte in the last output file (Issue 7: the
// output bytes are unchanged).
func TestSplitPartialFinalLine(t *testing.T) {
	dir := t.TempDir()
	if _, errb, code := runTool(t, dir, "a\nb\nc", "-l", "1"); code != 0 || errb != "" {
		t.Fatalf("split partial final line: code=%d stderr=%q", code, errb)
	}
	got := listFiles(t, dir)
	if !equal(got, []string{"xaa", "xab", "xac"}) {
		t.Fatalf("files: %v", got)
	}
	if readFile(t, dir, "xac") != "c" {
		t.Fatalf("partial final line changed: %q, want %q", readFile(t, dir, "xac"), "c")
	}
	// Round-trip: concatenation reproduces the input exactly.
	whole := readFile(t, dir, "xaa") + readFile(t, dir, "xab") + readFile(t, dir, "xac")
	if whole != "a\nb\nc" {
		t.Fatalf("round-trip changed bytes: %q", whole)
	}
}

// TestSplitDashStdinOperand covers the special token: an explicit '-' file
// operand selects standard input, exactly like an omitted operand, and the
// PREFIX operand still applies.
func TestSplitDashStdinOperand(t *testing.T) {
	dir := t.TempDir()
	if _, errb, code := runTool(t, dir, "1\n2\n", "-l", "1", "-", "part_"); code != 0 || errb != "" {
		t.Fatalf("split - part_: code=%d stderr=%q", code, errb)
	}
	if got := listFiles(t, dir); !equal(got, []string{"part_aa", "part_ab"}) {
		t.Fatalf("dash-stdin files: %v", got)
	}
}

// failAfterReader yields some bytes and then a non-EOF error so the input
// read-failure branch (Issue 7 CONSEQUENCES OF ERRORS) is exercised
// deterministically without a real device.
type failAfterReader struct {
	data []byte
	done bool
}

func (r *failAfterReader) Read(p []byte) (int, error) {
	if !r.done {
		r.done = true
		return copy(p, r.data), nil
	}
	return 0, fmt.Errorf("simulated read fault")
}

// TestSplitInputReadErrorIsFailure pins that a non-EOF read failure on the
// input is diagnosed and yields a >0 status. Bytes already read may have been
// written; the contract is the failing exit and the diagnostic, matching GNU.
func TestSplitInputReadErrorIsFailure(t *testing.T) {
	for _, args := range [][]string{
		{"-l", "1"},
		{"-b", "3"},
		{"-C", "3"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			dir := t.TempDir()
			var out, errb bytes.Buffer
			rc := &tool.RunContext{
				Ctx:   context.Background(),
				Dir:   dir,
				Stdio: tool.Stdio{In: &failAfterReader{data: []byte("ab\ncd\n")}, Out: &out, Err: &errb},
			}
			if code := run(rc, args); code != 1 {
				t.Fatalf("read fault %v: code=%d, want 1 (stderr %q)", args, code, errb.String())
			}
			if !strings.Contains(errb.String(), "split:") {
				t.Fatalf("read fault %v: missing diagnostic: %q", args, errb.String())
			}
		})
	}
}
