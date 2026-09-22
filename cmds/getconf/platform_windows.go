//go:build windows

package getconfcmd

// Windows has no POSIX sysconf/pathconf/confstr ABI.  Fail closed: even values
// that happen to resemble POSIX, LP64, or a Unix default are not claims this
// platform is entitled to make.
func platformValue(name string) (string, bool) {
	// A compile-time constant of this multicall or of the data model every
	// target shares (BC_BASE_MAX describes the bundled bc; INT_MAX and
	// LINE_MAX are the same number wherever bashy builds) is not a host ABI
	// claim. Let the shared product-owned value answer it. bash's own
	// printf7.sub reads `getconf INT_MAX` to build an overflow case.
	if productConstant(name) {
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
