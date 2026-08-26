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
// read-only destination directory makes os.Create fail deterministically.
func TestSplitOutputCreateErrorIsFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory write permission bits")
	}
	dir := t.TempDir()
	ro := filepath.Join(dir, "ro")
	if err := os.Mkdir(ro, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(ro, 0o755) })

	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   ro,
		Stdio: tool.Stdio{In: strings.NewReader("a\nb\n"), Out: &out, Err: &errb},
	}
	code := run(rc, []string{"-l", "1"})
	if code != 1 {
		t.Fatalf("read-only output dir: code=%d, want 1 (stderr %q)", code, errb.String())
	}
	if !strings.Contains(errb.String(), "split:") || !strings.Contains(errb.String(), "xaa") {
		t.Fatalf("output-create diagnostic missing/vague: %q", errb.String())
	}
}
