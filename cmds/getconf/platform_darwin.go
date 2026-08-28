//go:build darwin

package getconfcmd

// Values in this table are Darwin libc ABI values (Darwin 25 SDK), not POSIX
// minima.  Values that can move at runtime remain in sysconfStr/pathconfStr.
// Keeping this separate makes an OS upgrade reviewable and prevents Linux or
// Windows from accidentally inheriting a Darwin conformance claim.
func platformValue(name string) (string, bool) {
	if v, ok := darwinValues[name]; ok {
		return v, true
	}
	return "", false
}

var darwinValues = map[string]string{
	"AIO_LISTIO_MAX": "90", "AIO_MAX": "90", "ATEXIT_MAX": "2147483647", "BC_DIM_MAX": "2048", "BC_SCALE_MAX": "99", "BC_STRING_MAX": "1000", "CLK_TCK": "100", "COLL_WEIGHTS_MAX": "2", "EXPR_NEST_MAX": "32", "HOST_NAME_MAX": "255", "IOV_MAX": "1024", "LINE_MAX": "2048", "LOGIN_NAME_MAX": "255", "PTHREAD_DESTRUCTOR_ITERATIONS": "4", "PTHREAD_KEYS_MAX": "512", "PTHREAD_STACK_MIN": "16384", "SEM_NSEMS_MAX": "87381", "SEM_VALUE_MAX": "32767", "SYMLOOP_MAX": "32", "TTY_NAME_MAX": "255", "TZNAME_MAX": "255",
	"_POSIX_VERSION": "200112", "_POSIX2_VERSION": "200112", "_XOPEN_VERSION": "600", "_POSIX_SAVED_IDS": "1", "_POSIX_V6_LP64_OFF64": "1", "_POSIX_V6_LPBIG_OFFBIG": "1", "_POSIX2_C_BIND": "200112", "_POSIX2_C_DEV": "200112", "_POSIX2_CHAR_TERM": "200112", "_POSIX2_FORT_RUN": "200112", "_POSIX2_LOCALEDEF": "200112", "_POSIX2_SW_DEV": "200112", "_POSIX2_UPE": "200112", "_POSIX_FILE_LOCKING": "4096", "_POSIX_FSYNC": "200112", "_POSIX_IPV6": "200112", "_POSIX_JOB_CONTROL": "200112", "_POSIX_MAPPED_FILES": "200112", "_POSIX_MEMORY_PROTECTION": "200112", "_POSIX_READER_WRITER_LOCKS": "200112", "_POSIX_REGEXP": "200112", "_POSIX_SHELL": "200112", "_POSIX_SPAWN": "200112", "_POSIX_THREADS": "200112", "_POSIX_THREAD_ATTR_STACKADDR": "200112", "_POSIX_THREAD_ATTR_STACKSIZE": "200112", "_POSIX_THREAD_PROCESS_SHARED": "200112", "_POSIX_THREAD_SAFE_FUNCTIONS": "200112", "_POSIX_SS_REPL_MAX": "4", "_POSIX_TRACE_EVENT_NAME_MAX": "30", "_POSIX_TRACE_NAME_MAX": "8", "_POSIX_TRACE_SYS_MAX": "8", "_POSIX_TRACE_USER_EVENT_MAX": "32", "_XOPEN_CRYPT": "1", "_XOPEN_ENH_I18N": "1", "_XOPEN_SHM": "1", "_XOPEN_UNIX": "1",
	"CHAR_BIT": "8", "CHAR_MAX": "127", "CHAR_MIN": "-128", "INT_MAX": "2147483647", "INT_MIN": "-2147483648", "LLONG_MAX": "9223372036854775807", "LLONG_MIN": "-9223372036854775808", "LONG_BIT": "64", "LONG_MAX": "9223372036854775807", "LONG_MIN": "-9223372036854775808", "MB_LEN_MAX": "6", "SCHAR_MAX": "127", "SCHAR_MIN": "-128", "SHRT_MAX": "32767", "SHRT_MIN": "-32768", "SSIZE_MAX": "9223372036854775807", "UCHAR_MAX": "255", "UINT_MAX": "4294967295", "ULLONG_MAX": "18446744073709551615", "ULONG_MAX": "18446744073709551615", "USHRT_MAX": "65535", "WORD_BIT": "32",
}

func platformDifferentialNames() []string {
	names := make([]string, 0, len(darwinValues))
	for name := range darwinValues {
		names = append(names, name)
	}
	return names
}

func platformSpecification(s string) bool {
	return s == "POSIX_V6_LP64_OFF64" || s == "POSIX_V6_LPBIG_OFFBIG"
}

func platformConfstrValue(name string) (string, bool) {
	switch name {
	case "PATH":
		return "/usr/bin:/bin:/usr/sbin:/sbin", true
	case "POSIX_V6_ILP32_OFF32_CFLAGS", "POSIX_V6_ILP32_OFF32_LDFLAGS", "POSIX_V6_ILP32_OFF32_LIBS":
		return "", true
	case "POSIX_V6_ILP32_OFFBIG_CFLAGS", "POSIX_V6_ILP32_OFFBIG_LDFLAGS":
		return "-W 32", true
	case "POSIX_V6_ILP32_OFFBIG_LIBS", "POSIX_V6_LP64_OFF64_LIBS", "POSIX_V6_LPBIG_OFFBIG_LIBS":
		return "", true
	case "POSIX_V6_LP64_OFF64_CFLAGS", "POSIX_V6_LP64_OFF64_LDFLAGS", "POSIX_V6_LPBIG_OFFBIG_CFLAGS", "POSIX_V6_LPBIG_OFFBIG_LDFLAGS":
		return "-W 64", true
	case "POSIX_V6_WIDTH_RESTRICTED_ENVS":
		return "_POSIX_V6_LP64_OFF64", true
	}
	return undefined, isConfstrName(name)
}
