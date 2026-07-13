// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build windows

package steward

import (
	"fmt"
	"strings"

	"golang.org/x/sys/windows"
)

// accountID is the OS account this process actually runs as: its SID.
//
// NOT %USERNAME%, which is an environment string the process can overwrite — the
// windows half of the same hole the unix side closes with the uid. The SID is what the
// kernel checks its own access decisions against, it is taken from THIS PROCESS's token
// rather than from any ambient string, and it is stable across a rename of the account.
func accountID() (string, error) {
	tok := windows.GetCurrentProcessToken()
	u, err := tok.GetTokenUser()
	if err != nil {
		return "", fmt.Errorf("cannot read this process's token user: %w", err)
	}
	sid := u.User.Sid.String()
	if strings.TrimSpace(sid) == "" {
		return "", fmt.Errorf("this process's token carries no user SID")
	}
	return "sid:" + sid, nil
}
