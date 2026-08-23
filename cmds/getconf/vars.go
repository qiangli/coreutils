package getconfcmd

import (
	"strconv"

	"github.com/qiangli/coreutils/tool"
)

// knownSpecification reports whether -v names the programming environment this
// build targets. POSIX defines several (POSIX_V7_ILP32_OFF32 and friends); a
// 64-bit build honestly supports only the LP64 ones, and claiming otherwise
// would report this environment's numbers under a different name.
func knownSpecification(s string) bool {
	switch s {
	case "POSIX_V7_LP64_OFF64", "POSIX_V6_LP64_OFF64", "XBS5_LP64_OFF64",
		"POSIX_V7_LPBIG_OFFBIG", "POSIX_V6_LPBIG_OFFBIG":
		return strconv.IntSize == 64
	}
	return false
}

// sysVars are the system configuration variables. A value of -1 with no errno
// means "no limit", which getconf must report as "undefined" rather than as a
// number — callers distinguish an absent limit from a small one.
//
// confstr-style string variables (PATH, CS_PATH) are handled separately because
// their result is text rather than a number.
var sysVars = map[string]func() (string, bool){
	// Runtime invariant values obtainable from the OS.
	"ARG_MAX":           func() (string, bool) { return sysconfStr(scArgMax) },
	"BC_BASE_MAX":       constVal(99),
	"BC_STRING_MAX":     constVal(1000),
	"CHILD_MAX":         func() (string, bool) { return sysconfStr(scChildMax) },
	"CLK_TCK":           func() (string, bool) { return sysconfStr(scClkTck) },
	"INT_MAX":           constVal(2147483647),
	"LINE_MAX":          constVal(2048),
	"NGROUPS_MAX":       func() (string, bool) { return sysconfStr(scNgroupsMax) },
	"OPEN_MAX":          func() (string, bool) { return sysconfStr(scOpenMax) },
	"PAGESIZE":          func() (string, bool) { return sysconfStr(scPagesize) },
	"PAGE_SIZE":         func() (string, bool) { return sysconfStr(scPagesize) },
	"RE_DUP_MAX":        reDupMaxStr,
	"SYMLOOP_MAX":       symloopMaxStr,
	"_NPROCESSORS_CONF": func() (string, bool) { return sysconfStr(scNprocessorsConf) },
	"_NPROCESSORS_ONLN": func() (string, bool) { return sysconfStr(scNprocessorsOnln) },

	// Compile-time minimums fixed by the standard. These are constants in the
	// specification, not host measurements, so they are answered from the
	// standard rather than probed.
	"_POSIX_ARG_MAX":           constVal(4096),
	"_POSIX_CHILD_MAX":         constVal(25),
	"_POSIX_LINK_MAX":          constVal(8),
	"_POSIX_MAX_CANON":         constVal(255),
	"_POSIX_MAX_INPUT":         constVal(255),
	"_POSIX_NAME_MAX":          constVal(14),
	"_POSIX_NGROUPS_MAX":       constVal(8),
	"_POSIX_OPEN_MAX":          constVal(20),
	"_POSIX_PATH_MAX":          constVal(256),
	"_POSIX_PIPE_BUF":          constVal(512),
	"_POSIX_SSIZE_MAX":         constVal(32767),
	"_POSIX_STREAM_MAX":        constVal(8),
	"_POSIX_TZNAME_MAX":        constVal(6),
	"_POSIX_VERSION":           constVal(posixVersion),
	"_POSIX2_VERSION":          constVal(posix2Version),
	"_POSIX2_BC_BASE_MAX":      constVal(99),
	"_POSIX2_BC_DIM_MAX":       constVal(2048),
	"_POSIX2_BC_SCALE_MAX":     constVal(99),
	"_POSIX2_BC_STRING_MAX":    constVal(1000),
	"_POSIX2_COLL_WEIGHTS_MAX": constVal(2),
	"_POSIX2_EXPR_NEST_MAX":    constVal(32),
	"_POSIX2_LINE_MAX":         constVal(2048),
	"_POSIX2_RE_DUP_MAX":       constVal(255),
	"_XOPEN_VERSION":           constVal(700),
}

// pathVars are the pathname configuration variables, resolved with pathconf(2)
// against the operand path.
var pathVars = map[string]int{
	"LINK_MAX":                pcLinkMax,
	"MAX_CANON":               pcMaxCanon,
	"MAX_INPUT":               pcMaxInput,
	"NAME_MAX":                pcNameMax,
	"PATH_MAX":                pcPathMax,
	"PIPE_BUF":                pcPipeBuf,
	"_POSIX_CHOWN_RESTRICTED": pcChownRestricted,
	"_POSIX_NO_TRUNC":         pcNoTrunc,
	"_POSIX_VDISABLE":         pcVdisable,
}

func constVal(v int64) func() (string, bool) {
	return func() (string, bool) { return strconv.FormatInt(v, 10), true }
}

// systemValue resolves a system variable. The second result reports whether the
// NAME is known at all — distinct from a known name whose value is undefined.
func systemValue(name string) (string, bool) {
	if s, ok := confstrValue(name); ok {
		return s, true
	}
	f, ok := sysVars[name]
	if !ok {
		return "", false
	}
	return f()
}

// pathValue resolves a pathname variable against path. An unknown NAME is
// reported as unknown; a known NAME with no limit prints "undefined".
func pathValue(rc *tool.RunContext, name, path string) (string, bool) {
	key, ok := pathVars[name]
	if !ok {
		return "", false
	}
	return pathconfStr(rc, key, path)
}
