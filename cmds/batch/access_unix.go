//go:build unix

package batchcmd

import (
	"bufio"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

var batchAccessDirs = []string{"/usr/lib/cron", "/etc/cron.d", "/etc"}

func checkBatchAccess(rc *tool.RunContext) int {
	name := ""
	if current, err := user.Current(); err == nil {
		name = current.Username
	}
	if name == "" {
		name = rc.Getenv("LOGNAME")
	}
	for _, dir := range batchAccessDirs {
		allow := filepath.Join(dir, "at.allow")
		deny := filepath.Join(dir, "at.deny")
		if _, err := os.Stat(allow); err == nil {
			ok, err := accessFileContains(allow, name)
			if err != nil || !ok {
				fmt.Fprintf(rc.Err, "batch: user %s is not authorized\n", name)
				return 1
			}
			return 0
		}
		if _, err := os.Stat(deny); err == nil {
			blocked, err := accessFileContains(deny, name)
			if err != nil || blocked {
				fmt.Fprintf(rc.Err, "batch: user %s is not authorized\n", name)
				return 1
			}
			return 0
		}
	}
	if os.Geteuid() == 0 {
		return 0
	}
	fmt.Fprintf(rc.Err, "batch: user %s is not authorized\n", name)
	return 1
}

func accessFileContains(path, username string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(strings.SplitN(scanner.Text(), "#", 2)[0])
		if line == username {
			return true, nil
		}
	}
	return false, scanner.Err()
}
