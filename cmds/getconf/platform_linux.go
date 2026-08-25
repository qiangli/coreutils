//go:build linux

package getconfcmd

// Linux does not expose libc's sysconf/confstr table through a kernel API.
// Do not mistake Go's target ABI for a glibc (or musl) conformance statement.
func platformValue(name string) (string, bool) {
	switch name {
	case "_POSIX_VERSION", "_POSIX2_VERSION", "_XOPEN_VERSION", "RE_DUP_MAX":
		return undefined, true
	}
	return "", false
}

func platformSpecification(string) bool { return false }

func platformConfstrValue(name string) (string, bool) {
	if name == "PATH" {
		return "/bin:/usr/bin", true
	}
	return undefined, isConfstrName(name)
}

func platformDifferentialNames() []string { return nil }
