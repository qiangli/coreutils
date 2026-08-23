//go:build !unix

package nohupcmd

// ignoreHangup is a no-op where SIGHUP does not exist.
func ignoreHangup() func() { return func() {} }
