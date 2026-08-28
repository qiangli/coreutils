//go:build linux

package lognamecmd

import (
	"os"
	"path/filepath"

	"github.com/qiangli/coreutils/cmds/internal/session"
)

func platformLoginName(env []string) string {
	// getlogin(3) is defined by the session recorded for the process's
	// terminal. The certification arm deliberately creates that utmp record;
	// a systemd-launched process has no audit login UID, so /proc/loginuid alone
	// is not an equivalent implementation.
	if tty, err := os.Readlink("/proc/self/fd/0"); err == nil {
		if name := loginNameFromSession("", tty, env); name != "" {
			return name
		}
	}
	// Keep the audit login UID as the cgo-free fallback for sessions whose
	// terminal database is absent. It persists across su/sudo and never falls
	// back to the effective account.
	return loginNameFromLoginUID()
}

func loginNameFromSession(path, tty string, env []string) string {
	records, err := session.ReadEnv(path, env)
	if err != nil {
		return ""
	}
	want := filepath.Base(tty)
	if want == "" || want == "." {
		return ""
	}
	for _, record := range records {
		if session.IsUser(record) && filepath.Base(record.TTY) == want {
			return bareUser(record.User)
		}
	}
	return ""
}
