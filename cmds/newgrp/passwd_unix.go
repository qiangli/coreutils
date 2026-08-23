//go:build unix

package newgrpcmd

import (
	"bufio"
	"os"
	"strings"
)

// passwdFile is a variable so a test can point it at a fixture.
var passwdFile = "/etc/passwd"

// passwdShell returns the login shell recorded for name, or "" when there is no
// readable password file with such an entry. os/user deliberately does not
// expose this field, and newgrp needs it as the fallback behind $SHELL.
//
// A directory-service account has no line here; that is not an error, it just
// means the /bin/sh default applies.
func passwdShell(name string) string {
	f, err := os.Open(passwdFile)
	if err != nil {
		return ""
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Split(sc.Text(), ":")
		if len(fields) < 7 || fields[0] != name {
			continue
		}
		return fields[6]
	}
	return ""
}
