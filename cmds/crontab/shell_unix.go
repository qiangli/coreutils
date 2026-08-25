//go:build !windows

package crontabcmd

import (
	"fmt"
	"os"
	"path/filepath"
)

func cronDefaultPATH() string { return "/usr/bin:/bin" }
func cronShellPath() (string, error) {
	const shell = "/bin/sh"
	if err := validateCronShell(shell); err != nil {
		return "", fmt.Errorf("default shell is unavailable: %w", err)
	}
	return shell, nil
}
func validateCronShell(shell string) error {
	if !filepath.IsAbs(shell) {
		return fmt.Errorf("%q is not an absolute executable pathname", shell)
	}
	info, err := os.Stat(shell)
	if err != nil {
		return fmt.Errorf("cannot use %q: %w", shell, err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return fmt.Errorf("%q is not executable", shell)
	}
	return nil
}
