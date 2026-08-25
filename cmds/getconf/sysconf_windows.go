//go:build windows

package getconfcmd

import "github.com/qiangli/coreutils/tool"

// Windows has no sysconf/pathconf. Rather than invent numbers, the selectors
// are inert and every probed value reports "undefined"; the standard's
// compile-time minimums in vars.go remain correct and are still answered, since
// those are constants from the specification rather than host measurements.
const (
	scArgMax = iota
	scChildMax
	scClkTck
	scNgroupsMax
	scOpenMax
	scPagesize
	scNprocessorsConf
	scNprocessorsOnln

	pcLinkMax
	pcMaxCanon
	pcMaxInput
	pcNameMax
	pcPathMax
	pcPipeBuf
	pcChownRestricted
	pcNoTrunc
	pcVdisable
)

func sysconfStr(int) (string, bool) { return undefined, true }

func reDupMaxStr() (string, bool)   { return "255", true }
func symloopMaxStr() (string, bool) { return undefined, true }

func pathconfStr(*tool.RunContext, int, string) (string, bool) { return undefined, true }

// Kept only so the shared inventory compiles; platformValue intercepts these
// names before a value can be emitted on Windows.
const (
	posixVersion  = 0
	posix2Version = 0
)
