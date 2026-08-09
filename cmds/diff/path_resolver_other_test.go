//go:build !linux

package diffcmd

import (
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func TestPathResolverUsesRunContextPath(t *testing.T) {
	rc := &tool.RunContext{Dir: t.TempDir()}
	paths := newPathResolver(rc)
	defer paths.close()
	if got, want := paths.path("nested/file"), rc.Path("nested/file"); got != want {
		t.Fatalf("non-Linux resolver path = %q, want RunContext path %q", got, want)
	}
	if paths.dir != nil || paths.prefix != "" {
		t.Fatalf("non-Linux resolver acquired process-cwd state: dir=%v prefix=%q", paths.dir, paths.prefix)
	}
}
