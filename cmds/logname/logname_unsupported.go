//go:build !darwin && !dragonfly && !freebsd && !linux

package lognamecmd

func platformLoginName() string {
	return ""
}
