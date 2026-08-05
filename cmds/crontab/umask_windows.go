//go:build windows

package crontabcmd

func processUmask() (uint32, bool) { return 0, false }
