package newgrpcmd

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/user"
	"slices"
	"strings"
)

// errNoSuchGroup is what a lookup returns when the group database has no such
// entry. It is distinguished from a read failure because the two lead to
// different diagnostics: one is the caller's typo, the other is a broken host.
var errNoSuchGroup = errors.New("no such group")

// userInfo is the password-database record newgrp needs. It is a local type
// rather than *user.User so a test can supply one without a real account and so
// the shell field (which os/user does not expose portably) has a home.
type userInfo struct {
	Name string
	UID  string
	GID  string // the primary group, which is what a bare `newgrp` reverts to
	Dir  string
	// Shell is the login shell from the password database. It is the SECOND
	// choice for the replacement shell, after $SHELL.
	Shell string
	// Supplementary is every group id the user belongs to according to the
	// system, INCLUDING directory-service memberships that never appear in
	// /etc/group. On macOS in particular the group file is nearly empty and
	// this is the only membership evidence there is.
	Supplementary []string
}

// groupInfo is the group-database record newgrp needs.
type groupInfo struct {
	Name    string
	GID     string
	Members []string
	// Password is the crypt(3) hash guarding non-member access, taken from the
	// shadowed group database where one exists and from the group file
	// otherwise. See usableGroupPassword for what the sentinel values mean.
	Password string
	// PasswordShadowed records that the group file pointed at a shadow database
	// ("x") that could not be read. The distinction matters: an unreadable
	// shadow file is "cannot authenticate", not "no password set".
	PasswordShadowed bool
}

// database is the whole system-lookup surface, injected so the decision logic
// can be tested unprivileged against synthetic fixtures.
type database interface {
	Current() (userInfo, error)
	GroupByName(name string) (groupInfo, error)
	GroupByID(gid string) (groupInfo, error)
}

// db is the seam. The default reads the real system databases.
var db database = systemDB{
	groupFile:   "/etc/group",
	gshadowFile: "/etc/gshadow",
}

type systemDB struct {
	groupFile   string
	gshadowFile string
}

func (s systemDB) Current() (userInfo, error) {
	u, err := user.Current()
	if err != nil {
		return userInfo{}, err
	}
	info := userInfo{
		Name: u.Username,
		UID:  u.Uid,
		GID:  u.Gid,
		Dir:  u.HomeDir,
	}
	// os/user does not expose the login shell, so read it from the password
	// file where there is one. A missing or unreadable file is not an error:
	// $SHELL and /bin/sh still stand behind it.
	info.Shell = passwdShell(u.Username)
	if ids, err := u.GroupIds(); err == nil {
		info.Supplementary = ids
	}
	return info, nil
}

func (s systemDB) GroupByName(name string) (groupInfo, error) {
	return s.lookup(func(g groupInfo) bool { return g.Name == name }, func() (*user.Group, error) {
		return user.LookupGroup(name)
	})
}

func (s systemDB) GroupByID(gid string) (groupInfo, error) {
	return s.lookup(func(g groupInfo) bool { return g.GID == gid }, func() (*user.Group, error) {
		return user.LookupGroupId(gid)
	})
}

// lookup reads the group file first and falls back to os/user.
//
// The order is deliberate. The group FILE is the only place the member list and
// the password field live — os/user exposes neither — so a file hit is strictly
// more informative. But on a host whose groups come from a directory service
// the file may not have the entry at all, and there the os/user answer (name +
// gid, no members, no password) is still enough to decide, because
// Supplementary carries the membership.
func (s systemDB) lookup(match func(groupInfo) bool, fallback func() (*user.Group, error)) (groupInfo, error) {
	groups, err := parseGroupFile(s.groupFile)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return groupInfo{}, err
	}
	for _, g := range groups {
		if !match(g) {
			continue
		}
		g.Password, g.PasswordShadowed = s.resolvePassword(g)
		return g, nil
	}

	g, err := fallback()
	if err != nil {
		var unknownName user.UnknownGroupError
		var unknownID user.UnknownGroupIdError
		if errors.As(err, &unknownName) || errors.As(err, &unknownID) {
			return groupInfo{}, errNoSuchGroup
		}
		return groupInfo{}, err
	}
	return groupInfo{Name: g.Name, GID: g.Gid}, nil
}

// resolvePassword returns the hash guarding the group and whether it is
// shadowed-but-unreadable.
//
// "x" in the group file means "the real hash is in the shadowed database". If
// that database cannot be read — which is the NORMAL case for an unprivileged
// caller, since /etc/gshadow is root-only — the answer is not "no password";
// it is "cannot tell", and the caller must refuse rather than assume either way.
func (s systemDB) resolvePassword(g groupInfo) (string, bool) {
	if g.Password != "x" && g.Password != "" {
		return g.Password, false
	}
	hash, err := gshadowPassword(s.gshadowFile, g.Name)
	switch {
	case err == nil:
		return hash, false
	case errors.Is(err, fs.ErrNotExist), errors.Is(err, errNoSuchGroup):
		// No shadowed database on this platform (macOS, the BSDs) or no entry
		// for this group: the group file's own field is authoritative.
		return g.Password, false
	default:
		// Present but unreadable (permission denied): only "x" is a claim that
		// a password exists somewhere we cannot see.
		return "", g.Password == "x"
	}
}

// parseGroupFile reads name:password:gid:member,member entries. Malformed lines
// are skipped rather than failing the whole read: one bad line in /etc/group
// must not lock a user out of every group.
func parseGroupFile(path string) ([]groupInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []groupInfo
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, ":")
		if len(fields) < 3 {
			continue
		}
		g := groupInfo{Name: fields[0], Password: fields[1], GID: fields[2]}
		if len(fields) > 3 && fields[3] != "" {
			g.Members = strings.Split(fields[3], ",")
		}
		out = append(out, g)
	}
	return out, sc.Err()
}

// gshadowPassword reads the hash for name from a shadowed group database
// (name:password:administrators:members).
func gshadowPassword(path, name string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Split(sc.Text(), ":")
		if len(fields) < 2 || fields[0] != name {
			continue
		}
		return fields[1], nil
	}
	if err := sc.Err(); err != nil {
		return "", err
	}
	return "", errNoSuchGroup
}

// usableGroupPassword reports whether the field is a hash that can actually be
// checked. Three sentinels mean "no password can ever match": the empty field
// (no password set at all), and "!" / "*" / "!!" (deliberately locked). POSIX
// distinguishes "has a password" from "has none" and only the former may
// prompt, so collapsing these would either prompt for a password nobody can
// supply or let a locked group through.
func usableGroupPassword(field string) bool {
	switch field {
	case "", "x", "*", "!", "!!":
		return false
	}
	return true
}

// isMember reports whether u belongs to g, by NAME in the group's member list
// or by GID in the user's supplementary set. Both are consulted because neither
// alone is complete: the member list misses directory-service memberships, and
// the supplementary set misses a membership added since the session started.
func isMember(u userInfo, g groupInfo) bool {
	if u.GID == g.GID {
		return true
	}
	if slices.Contains(g.Members, u.Name) {
		return true
	}
	return slices.Contains(u.Supplementary, g.GID)
}

func (g groupInfo) String() string { return fmt.Sprintf("%s(%s)", g.Name, g.GID) }
