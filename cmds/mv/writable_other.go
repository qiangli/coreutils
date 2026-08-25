//go:build !unix && !windows

package mvcmd

// isWritable fails closed where the platform has no reliable access check.
// This keeps implicit prompting conservative and makes js/wasip builds whole.
func isWritable(path string) bool { return false }
