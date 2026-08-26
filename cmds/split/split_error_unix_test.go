//go:build !windows

package splitcmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

// TestSplitOutputCreateErrorIsFailure pins the Issue 7 OUTPUT FILES /
// CONSEQUENCES OF ERRORS contract on a failed output open: split diagnoses the
// unwritable target naming it, exits 1, and does not silently succeed. A
// regular file in an intermediate pathname component makes os.Create fail
// deterministically, including when the test is run as root.
func TestSplitOutputCreateErrorIsFailure(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}

	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Stdio: tool.Stdio{In: strings.NewReader("a\nb\n"), Out: &out, Err: &errb},
	}
	code := run(rc, []string{"-l", "1", "-", "blocker/x"})
	if code != 1 {
		t.Fatalf("invalid output path: code=%d, want 1 (stderr %q)", code, errb.String())
	}
	if !strings.Contains(errb.String(), "split:") || !strings.Contains(errb.String(), "blocker/xaa") {
		t.Fatalf("output-create diagnostic missing/vague: %q", errb.String())
	}
}
