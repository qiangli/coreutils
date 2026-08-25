//go:build linux

package getconfcmd

// Linux does not expose libc's sysconf/confstr table through a kernel API.
// Do not mistake Go's target ABI for a glibc (or musl) conformance statement.
func platformValue(name string) (string, bool) {
	switch name {
	case "BC_BASE_MAX", "BC_STRING_MAX", "INT_MAX", "LINE_MAX", "RE_DUP_MAX", "SYMLOOP_MAX", "_POSIX_VERSION", "_POSIX2_VERSION", "_XOPEN_VERSION":
		return undefined, true
	}
	return "", false
}

func platformSpecification(string) bool { return false }

func platformConfstrValue(name string) (string, bool) {
	return undefined, isConfstrName(name)
}

func platformDifferentialNames() []string { return nil }
