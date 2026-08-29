//go:build unix

package rmcmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRmRecursiveBeyondPathMax is a clean-room reducer for a tree whose leaf
// cannot be named by one pathname syscall. The tree is built one component at
// a time so setup does not depend on the behavior under test.
func TestRmRecursiveBeyondPathMax(t *testing.T) {
	dir := t.TempDir()
	top := filepath.Join(dir, "tree")
	if err := os.Mkdir(top, 0o755); err != nil {
		t.Fatal(err)
	}

	current, err := os.OpenRoot(top)
	if err != nil {
		t.Fatal(err)
	}
	var components []string
	for i := 0; i < 650; i++ {
		name := fmt.Sprintf("d%06d", i)
		if err := current.Mkdir(name, 0o755); err != nil {
			current.Close()
			t.Fatalf("create component %d: %v", i, err)
		}
		next, err := current.OpenRoot(name)
		if err != nil {
			current.Close()
			t.Fatalf("open component %d: %v", i, err)
		}
		current.Close()
		current = next
		components = append(components, name)
	}
	if err := current.WriteFile("leaf", []byte("x"), 0o600); err != nil {
		current.Close()
		t.Fatal(err)
	}
	current.Close()

	deepPath := filepath.Join(top, filepath.FromSlash(strings.Join(components, "/")), "leaf")
	if len(deepPath) <= 4096 {
		t.Fatalf("reducer path length = %d, want over Linux PATH_MAX", len(deepPath))
	}
	_, errb, code := runTool(t, dir, "-r", "tree")
	if code != 0 || errb != "" {
		t.Fatalf("rm -r over-depth tree: code=%d err=%q", code, errb)
	}
	if _, err := os.Lstat(top); !os.IsNotExist(err) {
		t.Fatalf("over-depth tree remains: %v", err)
	}
}
