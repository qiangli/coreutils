//go:build unix

package chowncmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/cmds/internal/rootguard"
)

func TestChownPreserveRootUsesIdentity(t *testing.T) {
	u := currentUser(t)
	dir := t.TempDir()
	guarded := filepath.Join(dir, "guarded")
	child := filepath.Join(guarded, "child")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}

	old := isFilesystemRoot
	var followFinal []bool
	isFilesystemRoot = func(path string, follow bool) bool {
		followFinal = append(followFinal, follow)
		return rootguard.SameFile(path, guarded, follow)
	}
	t.Cleanup(func() { isFilesystemRoot = old })

	alias := filepath.Join("guarded", "child") + string(filepath.Separator) + ".."
	_, errb, code := runTool(t, dir, "-R", "--preserve-root", u.Uid, alias)
	if code != 1 || !strings.Contains(errb, "dangerous to operate recursively") {
		t.Fatalf("identity guard: code=%d err=%q", code, errb)
	}
	if want := "(same as '" + rootguard.RootPath(guarded) + "')"; !strings.Contains(errb, want) {
		t.Fatalf("identity guard diagnostic=%q, want %q", errb, want)
	}
	if len(followFinal) != 1 || followFinal[0] {
		t.Fatalf("default -R followFinal calls=%v, want [false]", followFinal)
	}
	_, errb, code = runTool(t, dir, "-R", "-H", "--preserve-root", u.Uid, alias)
	if code != 1 || !strings.Contains(errb, "dangerous to operate recursively") {
		t.Fatalf("-H identity guard: code=%d err=%q", code, errb)
	}
	if len(followFinal) != 2 || !followFinal[1] {
		t.Fatalf("-H followFinal calls=%v, want [false true]", followFinal)
	}
}
