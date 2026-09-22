//go:build !windows

package tool

// posixErrorText is the identity off Windows: the OS already returns the
// POSIX strerror(3) text, and rewriting it could only introduce a
// difference.
func posixErrorText(error) (string, bool) { return "", false }
