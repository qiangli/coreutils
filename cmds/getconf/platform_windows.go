//go:build windows

package getconfcmd

// Windows has no POSIX sysconf/pathconf/confstr ABI.  Fail closed: even values
// that happen to resemble POSIX, LP64, or a Unix default are not claims this
// platform is entitled to make.
func platformValue(string) (string, bool)             { return undefined, true }
func platformSpecification(string) bool               { return false }
func platformConfstrValue(name string) (string, bool) { return undefined, isConfstrName(name) }
func platformDifferentialNames() []string             { return nil }
