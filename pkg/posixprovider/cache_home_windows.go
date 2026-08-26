// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build windows

package posixprovider

import (
	"fmt"
	"strings"

	"golang.org/x/sys/windows"
)

// accountHome resolves the current process token's profile directory. Neither
// route reads USERPROFILE, HOME, or another mutable environment value.
func accountHome() (string, error) {
	var tok windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &tok); err != nil {
		return "", fmt.Errorf("cannot open current process token: %w", err)
	}
	defer tok.Close()

	home, profileErr := tok.GetUserProfileDirectory()
	if profileErr != nil || strings.TrimSpace(home) == "" {
		var knownErr error
		home, knownErr = tok.KnownFolderPath(windows.FOLDERID_Profile, windows.KF_FLAG_DEFAULT)
		if knownErr != nil {
			return "", fmt.Errorf("cannot resolve authenticated account profile: %v (known folder: %w)", profileErr, knownErr)
		}
	}
	if strings.TrimSpace(home) == "" {
		return "", fmt.Errorf("authenticated account profile is empty")
	}
	return home, nil
}
