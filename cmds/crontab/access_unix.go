//go:build !windows

package crontabcmd

import "os"

func defaultCronAccessDirs() []string { return []string{"/usr/lib/cron", "/etc/cron.d", "/etc"} }
func effectiveUID() int               { return os.Geteuid() }
