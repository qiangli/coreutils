//go:build !unix

package idcmd

import "os/user"

func processIDs(bool) (uid, gid string) {
	u, err := user.Current()
	if err != nil {
		return "", ""
	}
	return u.Uid, u.Gid
}

func processGroupIDs() ([]string, error) {
	u, err := user.Current()
	if err != nil {
		return nil, err
	}
	return u.GroupIds()
}
