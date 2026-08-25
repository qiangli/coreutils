//go:build !unix

package atcmd

func processUmask() (uint32, bool) { return 0, false }
