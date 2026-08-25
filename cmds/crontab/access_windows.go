//go:build windows

package crontabcmd

func defaultCronAccessDirs() []string { return nil }
func effectiveUID() int               { return 0 }
