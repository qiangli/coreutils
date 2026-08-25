//go:build windows

package crontabcmd

import "errors"

func cronDefaultPATH() string { return "" }
func cronShellPath() (string, error) {
	return "", errors.New("POSIX crontab installation is unsupported on Windows: shell and umask semantics cannot be guaranteed")
}
func validateCronShell(string) error {
	return errors.New("POSIX crontab shells are unsupported on Windows")
}
