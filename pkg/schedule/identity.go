package schedule

import (
	"fmt"
	"os/user"
)

// Identity is the authenticated OS account used for job ownership and mail.
type Identity struct {
	UID  string
	Name string
}

// AuthenticatedIdentity never trusts LOGNAME or USER from an invocation.
func AuthenticatedIdentity() (Identity, error) {
	current, err := user.Current()
	if err != nil {
		return Identity{}, fmt.Errorf("cannot identify current user: %w", err)
	}
	if current.Uid == "" || current.Username == "" {
		return Identity{}, fmt.Errorf("current user has no stable UID or login name")
	}
	return Identity{UID: current.Uid, Name: current.Username}, nil
}
