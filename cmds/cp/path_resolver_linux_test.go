//go:build linux

package cpcmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
	"golang.org/x/sys/unix"
)

func TestPathResolverClosesDirectoryDescriptor(t *testing.T) {
	rc := &tool.RunContext{Dir: t.TempDir()}
	paths := newPathResolver(rc)
	got := paths.path("source")
	if paths.dir == nil || !strings.HasPrefix(got, "/proc/self/fd/") {
		t.Fatalf("resolver=%q dir=%v, want fd-relative Linux path", got, paths.dir)
	}
	fd := paths.dir.Fd()
	paths.close()
	if _, err := unix.FcntlInt(fd, unix.F_GETFD, 0); !errors.Is(err, unix.EBADF) {
		t.Fatalf("directory fd %d remains open: %v", fd, err)
	}
}

func TestCpNearPathMaxWithVirtualWorkingDirectory(t *testing.T) {
	base := t.TempDir()
	original, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(original) }()
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
	if len(longDir) != targetDirLength || len(filepath.Join(longDir, "source")) < 4096 {
		t.Fatalf("bad PATH_MAX fixture: dir=%d source=%d", len(longDir), len(filepath.Join(longDir, "source")))
	}
	if err := os.WriteFile("source", []byte("near-path-max"), 0o644); err != nil {
		t.Fatal(err)
	}
	deepDir, err := os.Open(".")
	if err != nil {
		t.Fatal(err)
	}
	defer deepDir.Close()
	if err := os.Chdir(base); err != nil {
		t.Fatal(err)
	}

	_, errb, code := runTool(t, longDir, "source", "dest")
	if code != 0 {
		t.Fatalf("code=%d stderr=%q", code, errb)
	}
	data, err := os.ReadFile(fmt.Sprintf("/proc/self/fd/%d/dest", deepDir.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "near-path-max" {
		t.Fatalf("destination=%q", data)
	}
}
