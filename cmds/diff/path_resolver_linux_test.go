//go:build linux

package diffcmd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
	"golang.org/x/sys/unix"
)

func TestPathResolverClosesDirectoryDescriptor(t *testing.T) {
	paths := newPathResolver(&tool.RunContext{Dir: t.TempDir()})
	got := paths.path("old")
	if paths.dir == nil || !strings.HasPrefix(got, "/proc/self/fd/") {
		t.Fatalf("resolver=%q dir=%v, want fd-relative Linux path", got, paths.dir)
	}
	fd := paths.dir.Fd()
	paths.close()
	paths.close() // invocation cleanup is idempotent and never closes another fd
	if _, err := unix.FcntlInt(fd, unix.F_GETFD, 0); !errors.Is(err, unix.EBADF) {
		t.Fatalf("directory fd %d remains open: %v", fd, err)
	}
}

func TestDiffNearPathMaxWithVirtualWorkingDirectory(t *testing.T) {
	base := t.TempDir()
	original, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(original); err != nil {
			t.Errorf("restore process cwd: %v", err)
		}
	}()
	if err := os.Chdir(base); err != nil {
		t.Fatal(err)
	}

	const targetDirLength = 4093
	longDir := base
	for len(longDir) < targetDirLength {
		nameLen := targetDirLength - len(longDir) - 1
		if nameLen > 200 {
			nameLen = 200
		}
		if nameLen <= 0 {
			break
		}
		name := strings.Repeat("x", nameLen)
		if err := os.Mkdir(name, 0o755); err != nil {
			t.Fatalf("mkdir component: %v", err)
		}
		if err := os.Chdir(name); err != nil {
			t.Fatalf("chdir component: %v", err)
		}
		longDir = filepath.Join(longDir, name)
	}
	if len(longDir) != targetDirLength || len(filepath.Join(longDir, "old")) < 4096 {
		t.Fatalf("bad PATH_MAX fixture: dir=%d operand=%d", len(longDir), len(filepath.Join(longDir, "old")))
	}
	if err := os.WriteFile("old", []byte("deep-old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("new", []byte("deep-new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(base); err != nil {
		t.Fatal(err)
	}
	// Conflicting process-cwd files make a raw-relative fallback observable.
	writeFile(t, base, "old", "process\n")
	writeFile(t, base, "new", "process\n")

	out, errb, code := runIn(t, longDir, "", "old", "new")
	want := "1c1\n< deep-old\n---\n> deep-new\n"
	if out != want || errb != "" || code != 1 {
		t.Fatalf("near-PATH_MAX diff = (%q, %q, %d), want (%q, empty stderr, 1)", out, errb, code, want)
	}
}
