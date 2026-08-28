//go:build windows

package getconfcmd

// Windows has no POSIX sysconf/pathconf/confstr ABI.  Fail closed: even values
// that happen to resemble POSIX, LP64, or a Unix default are not claims this
// platform is entitled to make.
func platformValue(name string) (string, bool) {
	// BC_BASE_MAX describes the bc bundled in this multicall, not a host ABI.
	// Let the shared product-owned value answer it on every supported target.
	if name == "BC_BASE_MAX" {
		return "", false
	}
	// The *_MIN names are specification constants, not Windows capability
	// claims. Everything else would be a made-up POSIX, X/Open, or ABI value.
	if compileTimeMinimum(name) {
		return "", false
	}
	return undefined, true
}
func platformSpecification(string) bool               { return false }
func platformConfstrValue(name string) (string, bool) { return undefined, isConfstrName(name) }
func platformDifferentialNames() []string             { return nil }
