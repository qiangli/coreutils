//go:build !linux

package runconcmd

import "fmt"

func currentContext() (string, error) {
	return "", fmt.Errorf("SELinux security contexts are not available on this platform")
}

func setExecContext(ctx string) error {
	return fmt.Errorf("SELinux security contexts are not available on this platform")
}
