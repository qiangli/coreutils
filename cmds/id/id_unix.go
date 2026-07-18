//go:build unix

package idcmd

import (
	"os"
	"strconv"
)

func processIDs(real bool) (uid, gid string) {
	if real {
		return strconv.Itoa(os.Getuid()), strconv.Itoa(os.Getgid())
	}
	return strconv.Itoa(os.Geteuid()), strconv.Itoa(os.Getegid())
}
