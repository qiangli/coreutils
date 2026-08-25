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

func TestPOSIXOneFileSystemExcludesCrossDeviceNonDirectory(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "root")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	rootInfo, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	foreignInfo, err := os.Stat("/dev/null")
	if err != nil {
		t.Skipf("/dev/null unavailable: %v", err)
	}
	rootDev, rootOK := fileDev(rootInfo)
	foreignDev, foreignOK := fileDev(foreignInfo)
	if !rootOK || !foreignOK || rootDev == foreignDev {
		t.Skip("test needs /dev/null on a different device")
	}
	if err := os.Symlink("/dev/null", filepath.Join(root, "foreign")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	out, errb, code := runToolAt(t, dir, "-aLx", "root")
	if code != 0 || errb != "" {
		t.Fatalf("du -aLx root = (%q, %q, %d)", out, errb, code)
	}
	if strings.Contains(out, "root/foreign") {
		t.Fatalf("cross-device non-directory was evaluated: %q", out)
	}
}

func TestPOSIXHardLinksDeduplicateWithinOneOperand(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "root")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	first := filepath.Join(root, "first")
	if err := os.WriteFile(first, []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(first, filepath.Join(root, "second")); err != nil {
		t.Skipf("hard links unavailable: %v", err)
	}
	out, errb, code := runToolAt(t, dir, "-ab", "root")
	if code != 0 || errb != "" {
		t.Fatalf("du -ab root = (%q, %q, %d)", out, errb, code)
	}
	if got := strings.Count(out, "root/first") + strings.Count(out, "root/second"); got != 1 {
		t.Fatalf("hard-linked file reported %d times, want exactly once: %q", got, out)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Fatalf("du -ab root lines=%v, want one file and root total", lines)
	}
	parts := strings.SplitN(lines[len(lines)-1], " ", 2)
	if len(parts) != 2 || parts[1] != "root" {
		t.Fatalf("root total line=%q", lines[len(lines)-1])
	}
	if size, err := strconv.ParseInt(parts[0], 10, 64); err != nil || size != int64(len("payload")) {
		t.Fatalf("root apparent total=%q, want one payload (%d bytes)", parts[0], len("payload"))
	}
}
