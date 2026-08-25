//go:build linux || darwin || freebsd || netbsd || openbsd

package ducmd

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

// POSIX du reports allocated space in 512-byte units by default. Unix
// st_blocks is already expressed in those units, so the default result must
// equal it exactly; -k must round the same allocation up to 1024-byte units.
func TestPOSIXDefaultUsesAllocated512ByteBlocks(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f")
	if err := os.WriteFile(p, []byte(strings.Repeat("x", 5000)), 0o644); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok || st.Blocks <= 0 {
		t.Skip("filesystem does not expose a positive st_blocks value")
	}
	blocks := int64(st.Blocks)
	out, errb, code := runToolAt(t, dir, "f")
	if code != 0 || errb != "" || out != strconv.FormatInt(blocks, 10)+" f\n" {
		t.Fatalf("du f = (%q, %q, %d), want %d allocated 512-byte blocks", out, errb, code, blocks)
	}
	wantK := (blocks + 1) / 2
	out, errb, code = runToolAt(t, dir, "-k", "f")
	if code != 0 || errb != "" || out != strconv.FormatInt(wantK, 10)+" f\n" {
		t.Fatalf("du -k f = (%q, %q, %d), want %d allocated 1024-byte blocks", out, errb, code, wantK)
	}
}
