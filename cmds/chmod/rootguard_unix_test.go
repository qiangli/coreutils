//go:build unix

package chmodcmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/cmds/internal/rootguard"
)

func TestChmodPreserveRootUsesIdentity(t *testing.T) {
	dir := t.TempDir()
	guarded := filepath.Join(dir, "guarded")
	deep := filepath.Join(guarded, "child", "deep")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}

	old := isFilesystemRoot
	var followFinal []bool
	isFilesystemRoot = func(path string, follow bool) bool {
		followFinal = append(followFinal, follow)
		return rootguard.SameFile(path, guarded, follow)
	}
	t.Cleanup(func() { isFilesystemRoot = old })

	alias := filepath.Join("guarded", "child", "deep") + string(filepath.Separator) + ".." + string(filepath.Separator) + ".."
	_, errb, code := runTool(t, dir, "-R", "--preserve-root", "600", alias)
	if code != 1 || !strings.Contains(errb, "dangerous to operate recursively") {
		t.Fatalf("identity guard: code=%d err=%q", code, errb)
	}
	if want := "(same as '" + rootguard.RootPath(guarded) + "')"; !strings.Contains(errb, want) {
		t.Fatalf("identity guard diagnostic=%q, want %q", errb, want)
	}
	if len(followFinal) != 1 || followFinal[0] {
		t.Fatalf("default -R followFinal calls=%v, want [false]", followFinal)
	}
	link := filepath.Join(dir, "root-link")
	if err := os.Symlink(guarded, link); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}
	_, errb, code = runTool(t, dir, "-R", "-H", "--preserve-root", "600", "root-link")
	if code != 1 || !strings.Contains(errb, "dangerous to operate recursively") {
		t.Fatalf("-H identity guard: code=%d err=%q", code, errb)
	}
	if len(followFinal) != 2 || !followFinal[1] {
		t.Fatalf("-H followFinal calls=%v, want [false true]", followFinal)
	}
	fi, err := os.Stat(guarded)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o755 {
		t.Fatalf("guarded directory was modified: mode=%#o", fi.Mode().Perm())
	}
}
