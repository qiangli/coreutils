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
	// A programming environment is a libc contract, not a property of Go's
	// word size.  In particular Darwin's current libc advertises V6 LP64 only;
	// Windows advertises none.  Ask the platform adapter rather than inferring.
	return platformSpecification(s)
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

// registerInventory is deliberately separate from values.  These are the
// POSIX.1-2016 mandatory spellings, retained from the independently verified
// name audit.  A known spelling is permitted to be undefined: manufacturing a
// number is worse than saying the platform cannot determine it without libc.
func init() {
	for _, name := range []string{
		"AIO_LISTIO_MAX", "AIO_MAX", "AIO_PRIO_DELTA_MAX", "ATEXIT_MAX", "BC_DIM_MAX", "BC_SCALE_MAX", "COLL_WEIGHTS_MAX", "DELAYTIMER_MAX", "EXPR_NEST_MAX", "HOST_NAME_MAX", "IOV_MAX", "LOGIN_NAME_MAX", "MQ_OPEN_MAX", "MQ_PRIO_MAX", "PTHREAD_DESTRUCTOR_ITERATIONS", "PTHREAD_KEYS_MAX", "PTHREAD_STACK_MIN", "PTHREAD_THREADS_MAX", "RTSIG_MAX", "SEM_NSEMS_MAX", "SEM_VALUE_MAX", "SIGQUEUE_MAX", "SS_REPL_MAX", "STREAM_MAX", "TIMER_MAX", "TRACE_EVENT_NAME_MAX", "TRACE_NAME_MAX", "TRACE_SYS_MAX", "TRACE_USER_EVENT_MAX", "TTY_NAME_MAX", "TZNAME_MAX",
		"_POSIX_ADVISORY_INFO", "_POSIX_ASYNCHRONOUS_IO", "_POSIX_BARRIERS", "_POSIX_CLOCK_SELECTION", "_POSIX_CPUTIME", "_POSIX_FILE_LOCKING", "_POSIX_FSYNC", "_POSIX_IPV6", "_POSIX_JOB_CONTROL", "_POSIX_MAPPED_FILES", "_POSIX_MEMLOCK", "_POSIX_MEMLOCK_RANGE", "_POSIX_MEMORY_PROTECTION", "_POSIX_MESSAGE_PASSING", "_POSIX_MONOTONIC_CLOCK", "_POSIX_PRIORITIZED_IO", "_POSIX_PRIORITY_SCHEDULING", "_POSIX_RAW_SOCKETS", "_POSIX_READER_WRITER_LOCKS", "_POSIX_REALTIME_SIGNALS", "_POSIX_REGEXP", "_POSIX_SAVED_IDS", "_POSIX_SEMAPHORES", "_POSIX_SHARED_MEMORY_OBJECTS", "_POSIX_SHELL", "_POSIX_SPAWN", "_POSIX_SPIN_LOCKS", "_POSIX_SPORADIC_SERVER", "_POSIX_SS_REPL_MAX", "_POSIX_SYNCHRONIZED_IO", "_POSIX_THREADS", "_POSIX_THREAD_ATTR_STACKADDR", "_POSIX_THREAD_ATTR_STACKSIZE", "_POSIX_THREAD_CPUTIME", "_POSIX_THREAD_PRIO_INHERIT", "_POSIX_THREAD_PRIO_PROTECT", "_POSIX_THREAD_PRIORITY_SCHEDULING", "_POSIX_THREAD_PROCESS_SHARED", "_POSIX_THREAD_SAFE_FUNCTIONS", "_POSIX_THREAD_SPORADIC_SERVER", "_POSIX_TIMEOUTS", "_POSIX_TIMERS", "_POSIX_TRACE", "_POSIX_TRACE_EVENT_FILTER", "_POSIX_TRACE_INHERIT", "_POSIX_TRACE_LOG", "_POSIX_TYPED_MEMORY_OBJECTS", "_POSIX_V6_ILP32_OFF32", "_POSIX_V6_ILP32_OFFBIG", "_POSIX_V6_LP64_OFF64", "_POSIX_V6_LPBIG_OFFBIG", "_POSIX_V7_ILP32_OFF32", "_POSIX_V7_ILP32_OFFBIG", "_POSIX_V7_LP64_OFF64", "_POSIX_V7_LPBIG_OFFBIG",
		"_POSIX2_C_BIND", "_POSIX2_C_DEV", "_POSIX2_CHAR_TERM", "_POSIX2_FORT_DEV", "_POSIX2_FORT_RUN", "_POSIX2_LOCALEDEF", "_POSIX2_PBS", "_POSIX2_PBS_ACCOUNTING", "_POSIX2_PBS_CHECKPOINT", "_POSIX2_PBS_LOCATE", "_POSIX2_PBS_MESSAGE", "_POSIX2_PBS_TRACK", "_POSIX2_SW_DEV", "_POSIX2_UPE", "_XOPEN_CRYPT", "_XOPEN_ENH_I18N", "_XOPEN_REALTIME", "_XOPEN_REALTIME_THREADS", "_XOPEN_SHM", "_XOPEN_STREAMS", "_XOPEN_UNIX", "_XOPEN_UUCP",
		"CHAR_BIT", "CHAR_MAX", "CHAR_MIN", "INT_MIN", "LLONG_MAX", "LLONG_MIN", "LONG_BIT", "LONG_MAX", "LONG_MIN", "MB_LEN_MAX", "NL_ARGMAX", "NL_LANGMAX", "NL_MSGMAX", "NL_NMAX", "NL_SETMAX", "NL_TEXTMAX", "NZERO", "SCHAR_MAX", "SCHAR_MIN", "SHRT_MAX", "SHRT_MIN", "SSIZE_MAX", "UCHAR_MAX", "UINT_MAX", "ULLONG_MAX", "ULONG_MAX", "USHRT_MAX", "WORD_BIT",
		"_POSIX_AIO_LISTIO_MAX", "_POSIX_AIO_MAX", "_POSIX_CLOCKRES_MIN", "_POSIX_DELAYTIMER_MAX", "_POSIX_HOST_NAME_MAX", "_POSIX_LOGIN_NAME_MAX", "_POSIX_MQ_OPEN_MAX", "_POSIX_MQ_PRIO_MAX", "_POSIX_RE_DUP_MAX", "_POSIX_RTSIG_MAX", "_POSIX_SEM_NSEMS_MAX", "_POSIX_SEM_VALUE_MAX", "_POSIX_SIGQUEUE_MAX", "_POSIX_SS_REPL_MAX", "_POSIX_SYMLINK_MAX", "_POSIX_SYMLOOP_MAX", "_POSIX_THREAD_DESTRUCTOR_ITERATIONS", "_POSIX_THREAD_KEYS_MAX", "_POSIX_THREAD_THREADS_MAX", "_POSIX_TIMER_MAX", "_POSIX_TRACE_EVENT_NAME_MAX", "_POSIX_TRACE_NAME_MAX", "_POSIX_TRACE_SYS_MAX", "_POSIX_TRACE_USER_EVENT_MAX", "_POSIX_TTY_NAME_MAX", "_POSIX2_CHARCLASS_NAME_MAX", "_XOPEN_IOV_MAX", "_XOPEN_NAME_MAX", "_XOPEN_PATH_MAX",
	} {
		if _, ok := sysVars[name]; !ok {
			sysVars[name] = undefinedVal
		}
	}
	pathVars["FILESIZEBITS"] = pcFilesizeBits
	pathVars["POSIX2_SYMLINKS"] = pc2Symlinks
	pathVars["POSIX_ALLOC_SIZE_MIN"] = pcAllocSizeMin
	pathVars["POSIX_REC_INCR_XFER_SIZE"] = pcRecIncrXferSize
	pathVars["POSIX_REC_MAX_XFER_SIZE"] = pcRecMaxXferSize
	pathVars["POSIX_REC_MIN_XFER_SIZE"] = pcRecMinXferSize
	pathVars["POSIX_REC_XFER_ALIGN"] = pcRecXferAlign
	pathVars["SYMLINK_MAX"] = pcSymlinkMax
	pathVars["_POSIX_ASYNC_IO"] = pcAsyncIO
	pathVars["_POSIX_PRIO_IO"] = pcPrioIO
	pathVars["_POSIX_SYNC_IO"] = pcSyncIO
	pathVars["_POSIX_TIMESTAMP_RESOLUTION"] = pcUndefined
}

const pcUndefined = -1

var confstrVars = []string{
	"PATH", "POSIX_V6_ILP32_OFF32_CFLAGS", "POSIX_V6_ILP32_OFF32_LDFLAGS", "POSIX_V6_ILP32_OFF32_LIBS", "POSIX_V6_ILP32_OFFBIG_CFLAGS", "POSIX_V6_ILP32_OFFBIG_LDFLAGS", "POSIX_V6_ILP32_OFFBIG_LIBS", "POSIX_V6_LP64_OFF64_CFLAGS", "POSIX_V6_LP64_OFF64_LDFLAGS", "POSIX_V6_LP64_OFF64_LIBS", "POSIX_V6_LPBIG_OFFBIG_CFLAGS", "POSIX_V6_LPBIG_OFFBIG_LDFLAGS", "POSIX_V6_LPBIG_OFFBIG_LIBS", "POSIX_V6_WIDTH_RESTRICTED_ENVS", "V6_ENV", "POSIX_V7_ILP32_OFF32_CFLAGS", "POSIX_V7_ILP32_OFF32_LDFLAGS", "POSIX_V7_ILP32_OFF32_LIBS", "POSIX_V7_ILP32_OFFBIG_CFLAGS", "POSIX_V7_ILP32_OFFBIG_LDFLAGS", "POSIX_V7_ILP32_OFFBIG_LIBS", "POSIX_V7_LP64_OFF64_CFLAGS", "POSIX_V7_LP64_OFF64_LDFLAGS", "POSIX_V7_LP64_OFF64_LIBS", "POSIX_V7_LPBIG_OFFBIG_CFLAGS", "POSIX_V7_LPBIG_OFFBIG_LDFLAGS", "POSIX_V7_LPBIG_OFFBIG_LIBS", "POSIX_V7_THREADS_CFLAGS", "POSIX_V7_THREADS_LDFLAGS", "POSIX_V7_WIDTH_RESTRICTED_ENVS", "V7_ENV",
}

func isConfstrName(name string) bool {
	for _, n := range confstrVars {
		if name == n {
			return true
		}
	}
	return false
}

func constVal(v int64) func() (string, bool) {
	return func() (string, bool) { return strconv.FormatInt(v, 10), true }
}

func compileTimeMinimum(name string) bool {
	switch name {
	case "_POSIX_ARG_MAX", "_POSIX_CHILD_MAX", "_POSIX_LINK_MAX", "_POSIX_MAX_CANON", "_POSIX_MAX_INPUT", "_POSIX_NAME_MAX", "_POSIX_NGROUPS_MAX", "_POSIX_OPEN_MAX", "_POSIX_PATH_MAX", "_POSIX_PIPE_BUF", "_POSIX_SSIZE_MAX", "_POSIX_STREAM_MAX", "_POSIX_TZNAME_MAX", "_POSIX2_BC_BASE_MAX", "_POSIX2_BC_DIM_MAX", "_POSIX2_BC_SCALE_MAX", "_POSIX2_BC_STRING_MAX", "_POSIX2_COLL_WEIGHTS_MAX", "_POSIX2_EXPR_NEST_MAX", "_POSIX2_LINE_MAX", "_POSIX2_RE_DUP_MAX":
		return true
	}
	return false
}

func undefinedVal() (string, bool) { return undefined, true }

// systemValue resolves a system variable. The second result reports whether the
// NAME is known at all — distinct from a known name whose value is undefined.
func systemValue(name string) (string, bool) {
	if s, ok := platformConfstrValue(name); ok {
		return s, true
	}
	f, ok := sysVars[name]
	if !ok {
		return "", false
	}
	if s, handled := platformValue(name); handled {
		return s, true
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
	if key == pcUndefined {
		return undefined, true
	}
	return pathconfStr(rc, key, path)
}
