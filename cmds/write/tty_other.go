//go:build !unix && !windows

package writecmd

import "github.com/qiangli/coreutils/tool"

// Platforms outside the unix and windows families (js/wasm, plan9) have no
// terminal device this tool can name. write already refuses on them
// (platform_other.go); this keeps the package building.
func defaultSenderTTY(*tool.RunContext) string { return "" }
