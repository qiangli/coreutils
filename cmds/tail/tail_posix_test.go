package tailcmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

// errAfterReader yields data once, then fails with a non-EOF error, so the
// input read-failure branch (POSIX CONSEQUENCES_OF_ERRORS for a FILE/STDIN
// read) is exercised without a real device.
type errAfterReader struct {
	data []byte
	done bool
}

func (r *errAfterReader) Read(p []byte) (int, error) {
	if !r.done {
		r.done = true
		n := copy(p, r.data)
		return n, nil
	}
	return 0, fmt.Errorf("simulated read fault")
}

// TestTailInputReadErrorIsFailure pins that a non-EOF read failure on the
// input is diagnosed on stderr and yields exit status 1, distinct from the
// stdout write-failure branch already covered by TestTailWriteFailure.
func TestTailInputReadErrorIsFailure(t *testing.T) {
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   t.TempDir(),
		Stdio: tool.Stdio{In: &errAfterReader{data: []byte("keep\n")}, Out: &out, Err: &errb},
	}
	code := cmd.Run(rc, []string{"-n", "5"})
	if code != 1 {
		t.Fatalf("read fault: code=%d, want 1 (stderr %q)", code, errb.String())
	}
	// The stdin operand is named by its literal token '-' in the read-error
	// diagnostic (the format is unspecified by Issue 7; status 1 is the
	// load-bearing contract).
	if !strings.Contains(errb.String(), "tail: error reading '-'") {
		t.Fatalf("read fault diagnostic missing: %q", errb.String())
	}
}

// TestTailMissingOperandContinues proves the POSIX/extension multi-operand
// rule: a failing operand is diagnosed and given exit status 1, while every
// remaining operand is still processed to stdout with its header.
func TestTailMissingOperandContinues(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "present", "a\nb\nc\n")

	out, errb, code := runTool(t, dir, "", "missing", "present")
	if code != 1 {
		t.Fatalf("mixed operands: code=%d, want 1", code)
	}
	if !strings.Contains(errb, "cannot open 'missing' for reading") {
		t.Errorf("missing-operand diagnostic absent: %q", errb)
	}
	// The surviving operand is still tailed, with a header because there is
	// more than one file operand.
	if !strings.Contains(out, "==> present <==\n") || !strings.Contains(out, "a\nb\nc\n") {
		t.Errorf("surviving operand not processed after a failed one: %q", out)
	}
}

// TestTailClusteredModeLastWins covers the clustered-argument edge that the
// scanOrder fix closes: when -c and -n are both present and the last mode
// letter is clustered behind the boolean -f (as in -fn2), the extension
// "last of -c/-n wins" must still select lines mode, not bytes.
func TestTailClusteredModeLastWins(t *testing.T) {
	// -c5 first, then -f followed by -n2 in one cluster. Last mode is -n, so
	// the last two LINES are emitted, not the last five bytes.
	//
	// -f would normally block waiting for growth, so drive it against a
	// non-follow input by cancelling immediately is unnecessary here: with no
	// file operand and a pipe/closed stdin, -f is ignored per Issue 7 and the
	// single pass exits. Use a plain string stdin (a non-pipe reader) — the
	// ignore path applies because stdin is not an *os.File regular file.
	out, errb, code := runTool(t, "", "l1\nl2\nl3\nl4\n", "-c5", "-fn2")
	if code != 0 {
		t.Fatalf("clustered -c5 -fn2: code=%d stderr=%q", code, errb)
	}
	if out != "l3\nl4\n" {
		t.Fatalf("clustered mode resolution: got %q, want last two lines %q", out, "l3\nl4\n")
	}
}

// TestTailBytesFromStartNonSeekable pins the -c +N read-driven skip on a
// non-seekable stream (a pipe): the N-1 leading bytes are discarded by
// reading, so the origin-1 byte offset is honored without a Seek.
func TestTailBytesFromStartNonSeekable(t *testing.T) {
	pr, pw := io.Pipe()
	go func() {
		_, _ = io.WriteString(pw, "abcdefgh")
		_ = pw.Close()
	}()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   t.TempDir(),
		Stdio: tool.Stdio{In: pr, Out: &out, Err: &errb},
	}
	if code := cmd.Run(rc, []string{"-c", "+4"}); code != 0 {
		t.Fatalf("-c +4 on pipe: code=%d stderr=%q", code, errb.String())
	}
	if out.String() != "defgh" {
		t.Fatalf("-c +4 on pipe: got %q, want %q", out.String(), "defgh")
	}
}
