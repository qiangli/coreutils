package cpcmd

import (
	"os"
	"path/filepath"

	"github.com/qiangli/coreutils/tool"
)

// pathResolver anchors relative operands to the invocation directory without
// changing the process working directory. On Linux the platform hook keeps an
// open directory descriptor and uses /proc/self/fd for component-wise lookup;
// other platforms retain RunContext.Path's joined-path behavior.
type pathResolver struct {
	rc     *tool.RunContext
	dir    *os.File
	prefix string
	tried  bool
}

func newPathResolver(rc *tool.RunContext) *pathResolver {
	return &pathResolver{rc: rc}
}

func (p *pathResolver) path(operand string) string {
	if operand == "" || filepath.IsAbs(operand) || p.rc.Dir == "" || p.rc.DirIsProcessCwd {
		return p.rc.Path(operand)
	}
	p.ensure()
	if p.prefix != "" {
		// Do not filepath.Join or Clean here: a leading ../ must be resolved by
		// the kernel relative to the opened invocation directory, not lexically
		// against the /proc/self/fd directory.
		return p.prefix + "/" + operand
	}
	return p.rc.Path(operand)
}

func (p *pathResolver) close() {
	if p.dir != nil {
		_ = p.dir.Close()
		p.dir = nil
		p.prefix = ""
	}
}
