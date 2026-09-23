//go:build windows

package getconfcmd

import (
	"strconv"

	"github.com/qiangli/coreutils/tool"
	"mvdan.cc/sh/v3/execbudget"
)

// Windows has no sysconf/pathconf. Rather than invent host numbers, the
// selectors are inert except for Bashy's enforced ARG_MAX. Specification
// minimums and product constants pass through platformValue; other host
// capabilities still report "undefined".
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
	pc2Symlinks           = pcUndefined
	pcAllocSizeMin        = pcUndefined
	pcAsyncIO             = pcUndefined
	pcFilesizeBits        = pcUndefined
	pcPrioIO              = pcUndefined
	pcRecIncrXferSize     = pcUndefined
	pcRecMaxXferSize      = pcUndefined
	pcRecMinXferSize      = pcUndefined
	pcRecXferAlign        = pcUndefined
	pcSymlinkMax          = pcUndefined
	pcSyncIO              = pcUndefined
	pcTimestampResolution = pcUndefined
)

func sysconfStr(which int) (string, bool) {
	if which == scArgMax {
		return strconv.Itoa(execbudget.BashyArgMax), true
	}
	return undefined, true
}

func symloopMaxStr() (string, bool) { return undefined, true }
func clockTicksStr() (string, bool) { return undefined, true }

func pathconfStr(*tool.RunContext, int, string) (string, bool, error) {
	return undefined, true, nil
}

// Kept only so the shared inventory compiles; platformValue intercepts these
// names before a value can be emitted on Windows.
const (
	posixVersion  = 0
	posix2Version = 0
)
