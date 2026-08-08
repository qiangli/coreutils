//go:build !windows

package nicecmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNiceRunsExecutableTextWithoutShebang(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plain-script")
	if err := os.WriteFile(path, []byte("printf '<%s><%s>\\n' \"$1\" \"$2\"\nexit 7\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	out, errOut, code := runNiceEnv(t, []string{"PATH=" + dir}, "plain-script", "left", "right")
	if code != 7 {
		t.Fatalf("code=%d stderr=%q, want child status 7", code, errOut)
	}
	if got := strings.TrimSpace(out); got != "<left><right>" {
		t.Fatalf("output=%q, want arguments propagated", got)
	}
}
