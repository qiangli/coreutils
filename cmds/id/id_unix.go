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

func processGroupIDs() ([]string, error) {
	gids, err := os.Getgroups()
	if err != nil {
		return nil, err
	}
	result := make([]string, len(gids))
	for i, gid := range gids {
		result[i] = strconv.Itoa(gid)
	}
	return result, nil
}
