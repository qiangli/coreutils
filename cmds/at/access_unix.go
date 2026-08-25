//go:build unix

package atcmd

import (
	"bytes"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

var atAccessDirs = []string{"/usr/lib/cron", "/etc/cron.d", "/etc"}

// checkAtAccess enforces the POSIX at.allow/at.deny policy: at.allow, when
// present, is the complete list of permitted users; otherwise at.deny lists
// the rejected users (an empty at.deny permits everyone); with neither file,
// only a privileged user may use at. The policy format is one user name per
// line, so an unreadable file, a stat failure, or a malformed line is a
// security-relevant ambiguity and fails closed rather than being skipped.
func checkAtAccess(rc *tool.RunContext) int {
	name := ""
	if current, err := user.Current(); err == nil {
		name = current.Username
	}
	if name == "" {
		name = rc.Getenv("LOGNAME")
	}
	deny := func() int {
		fmt.Fprintf(rc.Err, "at: user %s is not authorized\n", name)
		return 1
	}
	for _, dir := range atAccessDirs {
		allowPath := filepath.Join(dir, "at.allow")
		denyPath := filepath.Join(dir, "at.deny")
		if _, err := os.Stat(allowPath); err == nil {
			permitted, policyErr := accessFileContains(allowPath, name)
			if policyErr != nil || !permitted {
				return deny()
			}
			return 0
		} else if !os.IsNotExist(err) {
			return deny()
		}
		if _, err := os.Stat(denyPath); err == nil {
			blocked, policyErr := accessFileContains(denyPath, name)
			if policyErr != nil || blocked {
				return deny()
			}
			return 0
		} else if !os.IsNotExist(err) {
			return deny()
		}
	}
	if os.Geteuid() == 0 {
		return 0
	}
	return deny()
}

// accessFileContains reports whether the policy file lists username. The
// format is exactly one user name per line; blank lines and lines containing
// whitespace are malformed and surface as errors so the caller fails closed.
func accessFileContains(path, username string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	found := false
	lines := bytes.Split(data, []byte{'\n'})
	if len(lines) > 0 && len(lines[len(lines)-1]) == 0 {
		lines = lines[:len(lines)-1]
	}
	for index, raw := range lines {
		line := string(raw)
		if line == "" || strings.ContainsAny(line, " \t\r\v\f") {
			return false, fmt.Errorf("malformed policy line %d", index+1)
		}
		if line == username {
			found = true
		}
	}
	return found, nil
}
