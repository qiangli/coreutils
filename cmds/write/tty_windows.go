//go:build windows

package writecmd

import "github.com/qiangli/coreutils/tool"

// Windows consoles have no device path of the utmp `ut_line` shape, so there
// is nothing truthful to name. run() refuses before this matters (see
// platform_windows.go); the function exists so the package compiles and so a
// future Windows path cannot silently inherit a Unix assumption.
func defaultSenderTTY(*tool.RunContext) string { return "" }
