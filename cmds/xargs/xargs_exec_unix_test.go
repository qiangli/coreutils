//go:build !windows

package xargscmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestXargsRunsExecutableTextWithoutShebang(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plain-script")
	if err := os.WriteFile(path, []byte("printf '<%s><%s>\\n' \"$1\" \"$2\"\nexit 7\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	out, errOut, code := runXargsEnv(t, dir, []string{"PATH=."}, "input-value\n", "plain-script", "fixed-value")
	if code != 123 {
		t.Fatalf("code=%d stderr=%q, want xargs child-failure mapping 123", code, errOut)
	}
	if got := strings.TrimSpace(out); got != "<fixed-value><input-value>" {
		t.Fatalf("output=%q, want command and input arguments propagated", got)
	}
}
