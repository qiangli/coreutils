// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build !windows

package steward

import (
	"fmt"
	"os"
	"strconv"
)

// accountID is the OS account this process actually runs as: the numeric UID.
//
// NOT $USER, $LOGNAME, or $USERNAME. Those are strings a process inherits and can
// overwrite, and a seat keyed on them was a seat an agent could change by exporting a
// variable — `USER=someone-else bashy steward claim` was a different seat, so the
// singleton could be sidestepped without touching a single file. The UID is what the
// kernel says, and no amount of exporting changes it.
//
// The uid, not the username, for the same reason at one remove: a username is a lookup
// through a name service that can be renamed, aliased, or unavailable; the uid is the
// identity the OS enforces its own permissions with.
func accountID() (string, error) {
	uid := os.Getuid()
	if uid < 0 {
		// Getuid returns -1 where the concept does not exist (js/wasm, plan9).
		return "", fmt.Errorf("this platform has no OS account identity")
	}
	return "uid:" + strconv.Itoa(uid), nil
}
