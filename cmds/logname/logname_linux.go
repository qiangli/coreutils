//go:build linux

package lognamecmd

func platformLoginName() string {
	return loginNameFromLoginUID()
}
