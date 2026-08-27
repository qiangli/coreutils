//go:build !unix

package makecmd

import "github.com/qiangli/coreutils/tool"

func installMakeSignalContext(_ *tool.RunContext) func() { return func() {} }
