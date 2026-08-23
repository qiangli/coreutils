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

func confstrValue(name string) (string, bool) {
	switch name {
	case "PATH", "CS_PATH":
		return "/bin:/usr/bin", true
	}
	return "", false
}

// Windows makes no POSIX conformance claim of its own; report the revision this
// implementation targets rather than inventing a platform claim.
const (
	posixVersion  = 200809
	posix2Version = 200809
)
