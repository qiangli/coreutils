//go:build unix

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

const pathMaxHelper = "COREUTILS_PATHMAX_HELPER"

// TestCoreutilsStandaloneNearPathMaxRelativeOperand exercises the real
// process boundary, not a RunContext facsimile. POSIX GA67 supplies test(1)
// with a valid relative pathname near PATH_MAX; joining the inherited cwd to
// that operand would make an invalid, overlong absolute string. Main must mark
// its RunContext as native-cwd so the kernel receives the original relative
// operand, just as it does for an execve'd standalone utility.
func TestCoreutilsStandaloneNearPathMaxRelativeOperand(t *testing.T) {
	if os.Getenv(pathMaxHelper) == "1" {
		args := argsAfterDoubleDash(os.Args)
		if len(args) != 1 {
			os.Exit(2)
		}
		os.Args = []string{"coreutils", "test", "-f", args[0]}
		main()
		os.Exit(2)
	}

	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })
	if err := os.Chdir(base); err != nil {
		t.Fatal(err)
	}

	var parts []string
	relLen := 0
	addDir := func(name string) {
		t.Helper()
		if err := os.Mkdir(name, 0o755); err != nil {
			t.Fatalf("mkdir component: %v", err)
		}
		if err := os.Chdir(name); err != nil {
			t.Fatalf("chdir component: %v", err)
		}
		parts = append(parts, name)
		relLen += len(name)
		if len(parts) > 1 {
			relLen++
		}
	}
	const leaf = "ff"
	component := strings.Repeat("x", 40)
	for relLen+1+len(component)+1+len(leaf) < unix.PathMax-1 {
		addDir(component)
	}
	if extra := unix.PathMax - 2 - relLen - 1 - len(leaf) - 1; extra > 0 {
		addDir(strings.Repeat("x", extra))
	}
	if err := os.WriteFile(leaf, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	rel := filepath.Join(strings.Join(parts, string(filepath.Separator)), leaf)
	if len(rel)+1 > unix.PathMax {
		t.Fatalf("setup: relative operand len %d exceeds PATH_MAX %d", len(rel), unix.PathMax)
	}
	if len(filepath.Join(base, rel))+1 <= unix.PathMax {
		t.Fatalf("setup: joined path does not exceed PATH_MAX")
	}
	if err := os.Chdir(base); err != nil {
		t.Fatal(err)
	}

	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	child := exec.Command(exe,
		"-test.run=^TestCoreutilsStandaloneNearPathMaxRelativeOperand$",
		"--", rel)
	child.Dir = base
	child.Env = append(os.Environ(), pathMaxHelper+"=1")
	if output, err := child.CombinedOutput(); err != nil {
		t.Fatalf("standalone coreutils test -f <%d-byte path>: %v\n%s", len(rel), err, output)
	}
}
