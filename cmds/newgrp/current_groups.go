package newgrpcmd

import (
	"os"
	"strconv"
)

var currentGroups = defaultCurrentGroups

func defaultCurrentGroups() ([]string, error) {
	groups, err := os.Getgroups()
	if err != nil {
		return nil, err
	}
	var res []string
	for _, g := range groups {
		res = append(res, strconv.Itoa(g))
	}
	return res, nil
}
