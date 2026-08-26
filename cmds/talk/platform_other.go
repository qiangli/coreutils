//go:build !unix

package talkcmd

import (
	"errors"
	"os"
	"os/user"

	"github.com/qiangli/coreutils/cmds/internal/session"
)

func defaultTalkRoot() string { return os.TempDir() }

func currentOSAccount() (account, error) {
	u, err := user.Current()
	if err != nil {
		return account{}, err
	}
	return account{Name: u.Username, UID: u.Uid}, nil
}

func lookupOSAccount(name string) (account, error) {
	u, err := user.Lookup(name)
	if err != nil {
		return account{}, err
	}
	return account{Name: u.Username, UID: u.Uid}, nil
}

func notifyTerminal(session.Record, account, string) error {
	return errors.New("local terminal notification is unsupported on this platform")
}

func fileOwnerUID(string) (string, error) { return "", errors.New("file uid is unsupported") }
func processAlive(int) bool               { return false }
