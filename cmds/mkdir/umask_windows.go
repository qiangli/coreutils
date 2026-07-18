//go:build windows

package mkdircmd

func umask() uint32 { return 0 }
