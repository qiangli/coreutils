//go:build linux

package findcmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

// POSIX requires find to descend to arbitrary depths without failing merely
// because a pathname assembled during traversal exceeds PATH_MAX. Build the
// hierarchy component-by-component so the fixture itself does not rely on a
// long absolute pathname.
func TestFindDescendsBeyondPathMax(t *testing.T) {
	root := t.TempDir()
	parent, err := os.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()

	const component = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ01234567"
	const depth = 72 // 72*60 plus separators exceeds Linux PATH_MAX.
	fd := int(parent.Fd())
	for i := 0; i < depth; i++ {
		if err := unix.Mkdirat(fd, component, 0o755); err != nil {
			t.Fatalf("mkdirat depth %d: %v", i, err)
		}
		next, err := unix.Openat(fd, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
		if err != nil {
			t.Fatalf("openat depth %d: %v", i, err)
		}
		if i > 0 {
			_ = unix.Close(fd)
		}
		fd = next
	}
	defer unix.Close(fd)
	leaf, err := unix.Openat(fd, "needle", unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	_ = unix.Close(leaf)

	// The traversal must not retain one descriptor per recursion level.
	// Keep the process limit low enough that the former implementation failed
	// around depth 27, then restore it before TempDir cleanup runs.
	var oldLimit unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_NOFILE, &oldLimit); err != nil {
		t.Fatal(err)
	}
	lowLimit := oldLimit
	if lowLimit.Cur > 32 {
		lowLimit.Cur = 32
		if err := unix.Setrlimit(unix.RLIMIT_NOFILE, &lowLimit); err != nil {
			t.Fatalf("lower RLIMIT_NOFILE: %v", err)
		}
		defer func() {
			if err := unix.Setrlimit(unix.RLIMIT_NOFILE, &oldLimit); err != nil {
				t.Errorf("restore RLIMIT_NOFILE: %v", err)
			}
		}()
	}

	out, errOut, code := runFindEnv(t, filepath.Dir(root), []string{"POSIXLY_CORRECT=1"}, filepath.Base(root), "-name", "needle", "-print")
	if code != 0 {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
	if strings.Contains(errOut, "File name too long") {
		t.Fatalf("find exposed PATH_MAX during traversal: %q", errOut)
	}
	wantSuffix := strings.Repeat("/"+component, depth) + "/needle\n"
	if !strings.HasSuffix(out, wantSuffix) {
		t.Fatalf("output does not contain deep leaf; len=%d suffix=%q", len(out), out[max(0, len(out)-120):])
	}
}

func TestTraversalStateStatsEmptyRegularFile(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "empty"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	rootInfo, err := os.Lstat(root)
	if err != nil {
		t.Fatal(err)
	}
	s := newTraversalState(root, 'P', rootInfo)
	defer s.close()
	info, err := s.lstatChild(nil, root, "empty")
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() || info.Size() != 0 {
		t.Fatalf("empty entry = mode %v size %d", info.Mode(), info.Size())
	}
}

func TestTraversalStateKeepsRootIdentityAfterPathReplacement(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "root")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "original"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	rootInfo, err := os.Lstat(root)
	if err != nil {
		t.Fatal(err)
	}
	s := newTraversalState(root, 'P', rootInfo)
	defer s.close()

	moved := filepath.Join(parent, "moved")
	if err := os.Rename(root, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "spoof"), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	ents, err := s.readDir(nil, root)
	if err != nil {
		t.Fatal(err)
	}
	if len(ents) != 1 || ents[0].Name() != "original" {
		t.Fatalf("descriptor root followed replacement path: entries=%v", entryNames(ents))
	}
}

func TestTraversalStateRejectsMismatchedRootIdentity(t *testing.T) {
	root := t.TempDir()
	other := t.TempDir()
	otherInfo, err := os.Lstat(other)
	if err != nil {
		t.Fatal(err)
	}
	s := newTraversalState(root, 'P', otherInfo)
	defer s.close()
	if _, err := s.readDir(nil, root); err == nil || !strings.Contains(err.Error(), "changed during traversal") {
		t.Fatalf("mismatched root identity error = %v", err)
	}
}

func TestTraversalStateRejectsDescendantSymlinkInPhysicalMode(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "real"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real", filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}

	rootInfo, err := os.Lstat(root)
	if err != nil {
		t.Fatal(err)
	}
	physical := newTraversalState(root, 'P', rootInfo)
	defer physical.close()
	if _, err := physical.readDir([]string{"link"}, filepath.Join(root, "link")); err == nil {
		t.Fatal("physical traversal followed a descendant symlink")
	}

	logical := newTraversalState(root, 'L', rootInfo)
	defer logical.close()
	if _, err := logical.readDir([]string{"link"}, filepath.Join(root, "link")); err != nil {
		t.Fatalf("logical traversal did not follow descendant symlink: %v", err)
	}
}

func entryNames(ents []os.DirEntry) []string {
	names := make([]string, len(ents))
	for i, ent := range ents {
		names[i] = ent.Name()
	}
	return names
}

func TestFindTraversalClosesDirectoryDescriptors(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "a", "b", "c"), 0o755); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Skipf("proc descriptor inventory unavailable: %v", err)
	}
	for i := 0; i < 10; i++ {
		_, errOut, code := runFindEnv(t, filepath.Dir(root), []string{"POSIXLY_CORRECT=1"}, filepath.Base(root), "-print")
		if code != 0 {
			t.Fatalf("iteration %d: code=%d stderr=%q", i, code, errOut)
		}
	}
	after, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Fatal(err)
	}
	if len(after) > len(before)+1 {
		t.Fatalf("directory descriptors leaked: before=%d after=%d", len(before), len(after))
	}
}
