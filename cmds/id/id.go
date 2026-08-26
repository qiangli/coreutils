// Package idcmd implements id(1) per the GNU coreutils manual: print
// user and group information for a specified USER, or for the
// current process when no USER is given.
//
// IDs come from os/user, so they are strings throughout: numeric
// uid/gid on unix, SIDs on Windows (the documented best-effort —
// Windows has no uid_t, and the SID is the real identifier). Group
// name resolution falls back to the bare ID when the database has no
// entry, the same way GNU id prints unresolvable IDs without a name.
package idcmd

import (
	"fmt"
	"io"
	"os/user"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "id",
	Synopsis: "Print user and group information for a USER, or the current process.",
	Usage:    "id [OPTION]... [USER]",
}

func init() { cmd.Run = run; tool.Register(cmd) }

// processIDsFn resolves the process's real (real==true) or effective
// (real==false) user and group IDs. It is a package var so tests can inject a
// process whose real and effective IDs differ — the setuid case that exercises
// the default format's euid=/egid= reporting — without actually being setuid.
var processIDsFn = processIDs

// processGroupIDsFn returns the invoking process's supplementary groups. It
// deliberately does not query the passwd/group database: the live process may
// have had its group vector changed since login. Tests replace the seam to
// model that distinction without mutating their own credentials.
var processGroupIDsFn = processGroupIDs

// accountGroupIDsFn returns the group-database affiliations for a named user.
// Keep this separate from processGroupIDsFn: a live process group vector and
// an account's configured memberships are different POSIX data sources.
var accountGroupIDsFn = func(u *user.User) ([]string, error) { return u.GroupIds() }

// groupNameByIDFn is the group-database lookup seam. Besides keeping formatter
// tests independent of the host group database, it makes the unresolvable-ID
// fallback explicit at the point where output is assembled.
var groupNameByIDFn = lookupGroupName

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	uFlag := fs.BoolP("user", "u", false, "print only the effective user ID")
	gFlag := fs.BoolP("group", "g", false, "print only the effective group ID")
	GFlag := fs.BoolP("groups", "G", false, "print all group IDs")
	nFlag := fs.BoolP("name", "n", false, "print a name instead of a number, for -ugG")
	fs.BoolP("all-compat", "a", false, "ignore, for compatibility with other versions")
	rFlag := fs.BoolP("real", "r", false, "print the real ID instead of the effective ID")
	pFlag := fs.BoolP("pretty", "p", false, "make output human-readable")
	zFlag := fs.BoolP("zero", "z", false, "delimit entries with NUL, not newline")
	ignoreFlag := fs.Bool("ignore", false, "ignore unknown users; print nothing for them")
	fs.BoolP("audit-context", "A", false, "no-op: no SELinux audit user context support")
	fs.BoolP("password-db", "P", false, "no-op: no password database sidestep")
	fs.BoolP("selinux-context", "Z", false, "no-op: no SELinux security context support")
	fs.Bool("context", false, "no-op: no SELinux security context support")
	operands, code := tool.Parse(rc, cmd, fs, tool.AliasHelpVersion(args))
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
	if *rFlag && chosen == 0 {
		return tool.UsageError(rc, cmd, "cannot print only names or real IDs in default format")
	}
	if *nFlag && chosen == 0 {
		return tool.UsageError(rc, cmd, "cannot print only names or real IDs in default format")
	}
	if len(operands) > 1 {
		return tool.UsageError(rc, cmd, "extra operand %q", operands[1])
	}
	useName := *nFlag || *pFlag
	term := "\n"
	if *zFlag {
		term = "\x00"
	}

	users := operands
	if len(users) == 0 {
		users = []string{""}
	}
	status := 0
	for _, name := range users {
		u, err := lookupUser(name)
		if err != nil {
			if *ignoreFlag {
				continue
			}
			fmt.Fprintf(rc.Err, "id: %q: no such user\n", name)
			status = 1
			continue
		}
		results, pErr := formatOne(u, *uFlag, *gFlag, *GFlag, useName, *rFlag, name == "")
		if pErr != nil {
			fmt.Fprintf(rc.Err, "id: %v\n", pErr)
			status = 1
			continue
		}
		for _, line := range results {
			str := line + term
			n, err := io.WriteString(rc.Out, str)
			if err == nil && n != len(str) {
				err = io.ErrShortWrite
			}
			if err != nil {
				fmt.Fprintf(rc.Err, "id: write error: %v\n", err)
				return 1
			}
		}
	}
	return status
}

func lookupUser(name string) (*user.User, error) {
	if name == "" {
		return user.Current()
	}
	if u, err := user.Lookup(name); err == nil {
		return u, nil
	}
	return user.LookupId(name)
}

func formatOne(u *user.User, uFlag, gFlag, GFlag, useName, rFlag, current bool) ([]string, error) {
	var results []string
	uid, gid := u.Uid, u.Gid
	if current {
		uid, gid = processIDsFn(rFlag)
	}

	switch {
	case uFlag:
		val := uid
		if useName {
			if resolved, err := user.LookupId(uid); err == nil {
				val = resolved.Username
			}
		}
		results = append(results, val)
		return results, nil
	case gFlag:
		val := gid
		if useName {
			val = groupName(gid)
		}
		results = append(results, val)
		return results, nil
	case GFlag:
		gids, err := allGroupIDs(u, current, rFlag)
		if err != nil {
			return nil, fmt.Errorf("cannot get groups for %q: %v", u.Username, err)
		}
		parts := make([]string, 0, len(gids))
		for _, gid := range gids {
			if useName {
				parts = append(parts, groupName(gid))
			} else {
				parts = append(parts, gid)
			}
		}
		results = append(results, strings.Join(parts, " "))
		return results, nil
	}

	// POSIX default format: uid=/gid= report the REAL IDs, and the effective
	// IDs are inserted (euid=/egid=) only when they differ from the real ones.
	// For the current process the real/effective pair comes straight from the
	// process so a setuid invocation reports both; for a named USER operand
	// there is no real/effective distinction and only uid=/gid= are shown.
	// This is the real/effective reporting POSIX's default report requires.
	var b strings.Builder
	realUID, realGID := u.Uid, u.Gid
	uidName, gidName := u.Username, groupNameByIDFn(u.Gid)
	if current {
		realUID, realGID = processIDsFn(true)
		uidName, gidName = idUserName(realUID), groupNameByIDFn(realGID)
	}
	fmt.Fprintf(&b, "uid=%s gid=%s", decorate(realUID, uidName), decorate(realGID, gidName))
	if current {
		euid, egid := processIDsFn(false)
		if euid != realUID {
			fmt.Fprintf(&b, " euid=%s", decorate(euid, idUserName(euid)))
		}
		if egid != realGID {
			fmt.Fprintf(&b, " egid=%s", decorate(egid, groupNameByIDFn(egid)))
		}
	}
	gids, err := supplementaryGroupIDs(u, current)
	if err != nil {
		return nil, fmt.Errorf("cannot get groups for %q: %v", u.Username, err)
	}
	if len(gids) > 0 {
		b.WriteString(" groups=")
	}
	for i, gid := range gids {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(decorate(gid, groupNameByIDFn(gid)))
	}
	results = append(results, b.String())
	return results, nil
}

// allGroupIDs implements POSIX id -G. For the invoking process the effective
// and real groups are followed by the actual getgroups(2) vector. For a USER
// operand, the account's primary group and database memberships are used.
func allGroupIDs(u *user.User, current, realFirst bool) ([]string, error) {
	if current {
		_, rgid := processIDsFn(true)
		_, egid := processIDsFn(false)
		ordered := []string{egid, rgid}
		if realFirst {
			ordered[0], ordered[1] = ordered[1], ordered[0]
		}
		gids, err := processGroupIDsFn()
		if err != nil {
			return nil, err
		}
		return uniqueNonempty(append(ordered, gids...)), nil
	}
	gids, err := accountGroupIDsFn(u)
	if err != nil {
		return nil, err
	}
	return uniqueNonempty(append([]string{u.Gid}, gids...)), nil
}

// supplementaryGroupIDs implements the groups= field of the default report.
// For the current process POSIX places its supplementary affiliations here.
// For a named user the field is present only for multiple distinct group
// memberships, and then contains every membership including the initial group.
func supplementaryGroupIDs(u *user.User, current bool) ([]string, error) {
	if current {
		gids, err := processGroupIDsFn()
		if err != nil {
			return nil, err
		}
		return uniqueNonempty(gids), nil
	}
	gids, err := accountGroupIDsFn(u)
	if err != nil {
		return nil, err
	}
	groups := uniqueNonempty(append([]string{u.Gid}, gids...))
	if len(groups) <= 1 {
		return nil, nil
	}
	return groups, nil
}

func uniqueNonempty(ids []string) []string {
	seen := make(map[string]struct{}, len(ids))
	result := make([]string, 0, len(ids))
	for _, id := range ids {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

// idUserName resolves the user name for a numeric uid, used for the effective
// user's name in the default format. An unresolvable uid yields "" so decorate
// prints the bare number, matching how id renders IDs with no database entry.
func idUserName(uid string) string {
	if u, err := user.LookupId(uid); err == nil {
		return u.Username
	}
	return ""
}

func lookupGroupName(gid string) string {
	if g, err := user.LookupGroupId(gid); err == nil {
		return g.Name
	}
	return ""
}

func groupName(gid string) string {
	if name := groupNameByIDFn(gid); name != "" {
		return name
	}
	return gid
}

func decorate(id, name string) string {
	if name == "" {
		return id
	}
	return fmt.Sprintf("%s(%s)", id, name)
}
