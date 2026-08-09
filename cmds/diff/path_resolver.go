package diffcmd

import (
	"os"
	"path/filepath"

	"github.com/qiangli/coreutils/tool"
)

// pathResolver anchors every relative operand to one diff invocation's
// working directory. Linux can resolve beneath an opened directory descriptor
// when materializing Dir+operand would exceed PATH_MAX; other platforms retain
// RunContext.Path's platform-specific behavior.
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
		// Preserve every operand component for kernel fd-relative resolution.
		// filepath.Join/Clean would resolve leading .. against /proc/self/fd.
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
