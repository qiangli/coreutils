//go:build !windows

package timecmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTimeRunsExecutableTextWithoutShebang(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plain-script")
	if err := os.WriteFile(path, []byte("printf '<%s><%s>\\n' \"$1\" \"$2\"\nexit 7\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	out, errOut, code := runTimeEnv(t, dir, []string{"PATH=."}, "plain-script", "left", "right")
	if code != 7 {
		t.Fatalf("code=%d stderr=%q, want child status 7", code, errOut)
	}
	if got := strings.TrimSpace(out); got != "<left><right>" {
		t.Fatalf("output=%q, want arguments propagated", got)
	}
}
