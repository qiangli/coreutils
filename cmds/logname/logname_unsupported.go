//go:build !darwin && !dragonfly && !freebsd && !linux

package lognamecmd

func platformLoginName(_ []string) string {
	return ""
}
