package groupscmd

import (
	"fmt"
	"os/user"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "groups",
	Synopsis: "Print group names a user is in.",
	Usage:    "groups [OPTION]... [USERNAME]...",
}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	users, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	if len(users) == 0 {
		u, err := user.Current()
		if err != nil {
			fmt.Fprintf(rc.Err, "groups: cannot find current user: %v\n", err)
			return 1
		}
		names, err := groupNames(u)
		if err != nil {
			fmt.Fprintf(rc.Err, "groups: cannot get groups: %v\n", err)
			return 1
		}
		fmt.Fprintln(rc.Out, strings.Join(names, " "))
		return 0
	}
	status := 0
	for _, name := range users {
		u, err := user.Lookup(name)
		if err != nil {
			fmt.Fprintf(rc.Err, "groups: %q: no such user\n", name)
			status = 1
			continue
		}
		names, err := groupNames(u)
		if err != nil {
			fmt.Fprintf(rc.Err, "groups: cannot get groups for %q: %v\n", name, err)
			status = 1
			continue
		}
		fmt.Fprintf(rc.Out, "%s : %s\n", name, strings.Join(names, " "))
	}
	return status
}

func groupNames(u *user.User) ([]string, error) {
	gids, err := u.GroupIds()
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	ordered := []string{u.Gid}
	for _, gid := range gids {
		if gid != u.Gid {
			ordered = append(ordered, gid)
		}
	}
	var names []string
	for _, gid := range ordered {
		if seen[gid] {
			continue
		}
		seen[gid] = true
		if g, err := user.LookupGroupId(gid); err == nil && g.Name != "" {
			names = append(names, g.Name)
		} else {
			names = append(names, gid)
		}
	}
	return names, nil
}
