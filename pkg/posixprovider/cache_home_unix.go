// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build !windows

package posixprovider

import (
	"fmt"
	"os"
	"os/user"
	"strconv"
	"strings"
)

// accountHome returns the home directory recorded by the OS for the process's
// real UID. LookupId is intentional: HOME and os.UserHomeDir are ambient input,
// and user.Current may consult ambient identity on platforms without an
// account database.
func accountHome() (string, error) {
	uid := os.Getuid()
	if uid < 0 {
		return "", fmt.Errorf("this platform has no authenticated OS account identity")
	}
	u, err := user.LookupId(strconv.Itoa(uid))
	if err != nil {
		return "", fmt.Errorf("no OS account record for uid %d: %w", uid, err)
	}
	home := strings.TrimSpace(u.HomeDir)
	if home == "" {
		return "", fmt.Errorf("the OS account record for uid %d has no home directory", uid)
	}
	return home, nil
}
