//go:build linux

package runconcmd

import (
	"os"
	"strings"
)

func currentContext() (string, error) {
	data, err := os.ReadFile("/proc/self/attr/current")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func setExecContext(ctx string) error {
	return os.WriteFile("/proc/self/attr/exec", []byte(ctx), 0)
}
