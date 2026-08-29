//go:build unix

package rmdircmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRmdirBeyondPathMax is a clean-room reducer for an empty directory whose
// complete pathname cannot be passed through one pathname syscall.
func TestRmdirBeyondPathMax(t *testing.T) {
	dir := t.TempDir()
	base, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer base.Close()
	components := make([]string, 0, 650)
	current, err := base.OpenRoot(".")
	if err != nil {
		t.Fatal(err)
	}
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
	current.Close()

	// Keep TempDir cleanup independent of pathname limits if an assertion
	// fails before the command removes the leaf.
	t.Cleanup(func() {
		for i := len(components); i > 0; i-- {
			_ = base.Remove(strings.Join(components[:i], "/"))
		}
	})
	operand := filepath.FromSlash(strings.Join(components, "/"))
	if len(filepath.Join(dir, operand)) <= 4096 {
		t.Fatalf("reducer path length = %d, want over Linux PATH_MAX", len(filepath.Join(dir, operand)))
	}
	_, errb, code := runTool(t, dir, operand)
	if code != 0 || errb != "" {
		t.Fatalf("rmdir over-PATH_MAX operand: code=%d err=%q", code, errb)
	}
	parent, err := base.OpenRoot(strings.Join(components[:len(components)-1], "/"))
	if err != nil {
		t.Fatalf("open leaf parent: %v", err)
	}
	defer parent.Close()
	if _, err := parent.Lstat(components[len(components)-1]); !os.IsNotExist(err) {
		t.Fatalf("over-PATH_MAX directory remains: %v", err)
	}
}
