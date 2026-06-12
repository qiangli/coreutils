// Package idcmd implements id(1) per the GNU coreutils manual: print
// user and group information for each specified USER, or for the
// current process when no USER is given. Supported flags: -u -g -G
// -n.
//
// IDs come from os/user, so they are strings throughout: numeric
// uid/gid on unix, SIDs on Windows (the documented best-effort —
// Windows has no uid_t, and the SID is the real identifier). Group
// name resolution falls back to the bare ID when the database has no
// entry, the same way GNU id prints unresolvable IDs without a name.
package idcmd

import (
	"fmt"
	"os/user"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "id",
	Synopsis: "Print user and group information for each USER, or the current process.",
	Usage:    "id [OPTION]... [USER]...",
}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	uFlag := fs.BoolP("user", "u", false, "print only the effective user ID")
	gFlag := fs.BoolP("group", "g", false, "print only the effective group ID")
	GFlag := fs.BoolP("groups", "G", false, "print all group IDs")
	nFlag := fs.BoolP("name", "n", false, "print a name instead of a number, for -ugG")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}

	chosen := 0
	for _, v := range []bool{*uFlag, *gFlag, *GFlag} {
		if v {
			chosen++
		}
	}
	if chosen > 1 {
		return tool.UsageError(rc, cmd, "cannot print \"only\" of more than one choice")
	}
	if *nFlag && chosen == 0 {
		return tool.UsageError(rc, cmd, "cannot print only names or real IDs in default format")
	}

	users := operands
	if len(users) == 0 {
		users = []string{""}
	}
	status := 0
	for _, name := range users {
		u, err := lookupUser(name)
		if err != nil {
			fmt.Fprintf(rc.Err, "id: %q: no such user\n", name)
			status = 1
			continue
		}
		if err := printOne(rc, u, *uFlag, *gFlag, *GFlag, *nFlag); err != nil {
			fmt.Fprintf(rc.Err, "id: %v\n", err)
			status = 1
		}
	}
	return status
}

// lookupUser resolves "" to the current user, otherwise tries the
// name and then (GNU behavior) a literal user ID.
func lookupUser(name string) (*user.User, error) {
	if name == "" {
		return user.Current()
	}
	if u, err := user.Lookup(name); err == nil {
		return u, nil
	}
	return user.LookupId(name)
}

func printOne(rc *tool.RunContext, u *user.User, uFlag, gFlag, GFlag, nFlag bool) error {
	switch {
	case uFlag:
		if nFlag {
			fmt.Fprintf(rc.Out, "%s\n", u.Username)
		} else {
			fmt.Fprintf(rc.Out, "%s\n", u.Uid)
		}
		return nil
	case gFlag:
		if nFlag {
			fmt.Fprintf(rc.Out, "%s\n", groupName(u.Gid))
		} else {
			fmt.Fprintf(rc.Out, "%s\n", u.Gid)
		}
		return nil
	}

	gids, err := groupIDs(u)
	if err != nil {
		return fmt.Errorf("cannot get groups for %q: %v", u.Username, err)
	}

	if GFlag {
		parts := make([]string, 0, len(gids))
		for _, gid := range gids {
			if nFlag {
				parts = append(parts, groupName(gid))
			} else {
				parts = append(parts, gid)
			}
		}
		fmt.Fprintf(rc.Out, "%s\n", strings.Join(parts, " "))
		return nil
	}

	// Default format: uid=U(name) gid=G(gname) groups=g1(n1),...
	var b strings.Builder
	fmt.Fprintf(&b, "uid=%s gid=%s groups=", decorate(u.Uid, u.Username), decorate(u.Gid, lookupGroupName(u.Gid)))
	for i, gid := range gids {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(decorate(gid, lookupGroupName(gid)))
	}
	fmt.Fprintf(rc.Out, "%s\n", b.String())
	return nil
}

// groupIDs returns the user's group IDs with the primary group first
// (GNU id prints the effective gid at the head of the groups list).
func groupIDs(u *user.User) ([]string, error) {
	gids, err := u.GroupIds()
	if err != nil {
		return nil, err
	}
	ordered := []string{u.Gid}
	for _, g := range gids {
		if g != u.Gid {
			ordered = append(ordered, g)
		}
	}
	return ordered, nil
}

// lookupGroupName returns the group's name, or "" when the database
// has no entry for the ID.
func lookupGroupName(gid string) string {
	if g, err := user.LookupGroupId(gid); err == nil {
		return g.Name
	}
	return ""
}

// groupName is lookupGroupName with the GNU -gn fallback: when the
// name is unknown the bare ID is printed.
func groupName(gid string) string {
	if name := lookupGroupName(gid); name != "" {
		return name
	}
	return gid
}

// decorate renders "id(name)", or just "id" when no name resolved.
func decorate(id, name string) string {
	if name == "" {
		return id
	}
	return fmt.Sprintf("%s(%s)", id, name)
}
