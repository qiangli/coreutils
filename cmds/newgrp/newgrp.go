// Package newgrpcmd implements newgrp(1): change the real and effective group
// identification and start a new shell.
//
// # The exit-status rule that shapes the whole command
//
// POSIX: "If newgrp succeeds in creating a new shell execution environment,
// WHETHER OR NOT THE GROUP IDENTIFICATION WAS CHANGED SUCCESSFULLY, the exit
// status shall be the exit status of the shell. Otherwise … >0 An error
// occurred." Together with the requirement that a refused change leave the
// environment unchanged, that makes a denial NOT a reason to exit: newgrp
// writes its diagnostic and starts the shell anyway, with the group as it was.
//
// That reads backwards until you see what newgrp is for. It is normally a shell
// BUILT-IN replacing the user's login shell; if it exited on a denial the user
// would be logged out for mistyping a group name. So the failure modes split:
//
//   - group change refused/failed  → diagnostic, shell starts, group unchanged,
//     status is the shell's.
//   - shell cannot be started      → nothing to hand the user; status >0.
//   - usage error (bad option)     → the command was not understood at all, so
//     no shell is started; status >0.
//
// # What this implementation deliberately does NOT do
//
// It does not execve over itself. Every tool here can run in-process inside an
// embedding shell (see tool.RunContext), where replacing the process image
// would destroy the host. The replacement shell is a CHILD whose status is
// propagated, which is observationally identical for the exit-status contract
// above and differs only in that the caller survives — the reason this repo
// exists.
//
// It also cannot actually change the group without privilege: setgid(2) admits
// an unprivileged caller only to its real or saved-set group id, which is why
// the system newgrp ships setuid-root. When the change is refused by the
// kernel, the rule above applies unchanged — diagnostic, then the shell.
package newgrpcmd

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "newgrp",
	Synopsis: "Change to a new group and start a shell.",
	Usage:    "newgrp [-l] [group]",
}

func init() { cmd.Run = run; tool.Register(cmd) }

// decision is the outcome of the membership/password rules for one (user,
// group) pair. It is a value rather than a bool pair so the three cases can be
// tested directly, without privilege and without a terminal.
type decision int

const (
	// permit: the user may assume the group with no password.
	permit decision = iota
	// challenge: the group has a password and the user is not a member, so the
	// password must be supplied and verified.
	challenge
	// deny: the user is not a member and there is no password that could let
	// them in. POSIX: permission is denied.
	deny
	// unverifiable: a password exists but this host cannot evaluate it (the
	// shadowed database is unreadable, or the hash uses a scheme this build
	// does not implement). Distinct from deny so the diagnostic can say which.
	unverifiable
)

func (d decision) String() string {
	switch d {
	case permit:
		return "permit"
	case challenge:
		return "challenge"
	case deny:
		return "deny"
	default:
		return "unverifiable"
	}
}

// authorize applies the POSIX membership rules.
//
// Membership — primary group, listed member, or supplementary group — is
// permission on its own; no password is asked for even when the group has one,
// because the password exists to admit NON-members.
func authorize(u userInfo, g groupInfo) decision {
	if isMember(u, g) {
		return permit
	}
	if g.PasswordShadowed {
		return unverifiable
	}
	if usableGroupPassword(g.Password) {
		return challenge
	}
	return deny
}

// promptPassword is the seam for reading the group password. Tests replace it;
// nothing in the suite ever needs a terminal.
var promptPassword = readPassword

// spawnShell is the seam for the privileged half: set the group, then run the
// shell. Tests replace it and assert on the spec instead of changing any real
// credential.
var spawnShell = defaultSpawnShell

// shellSpec is everything the spawn needs. Credential is nil when the group is
// not to be changed: the POSIX diagnostic-and-launch-the-shell path.
type shellSpec struct {
	Path       string // the executable
	Argv0      string // argv[0]: "-sh" under -l, "sh" otherwise
	Dir        string // working directory
	UID        string
	Credential *credentialPlan
	Env        []string // nil means use rc.Env
}

// credentialState is obtained through a platform seam, allowing the POSIX
// supplementary-list rules to be tested without changing process credentials.
type credentialState struct {
	RealGID          string
	EffectiveGID     string
	Supplementary    []string
	MaxSupplementary int // <= 0 means the platform did not report a limit
}

// credentialPlan states the privileged operation explicitly. POSIX requires
// both the real and effective group IDs to change; separate fields prevent an
// adapter from silently implementing only half of that contract.
type credentialPlan struct {
	RealGID       string
	EffectiveGID  string
	Supplementary []string
	// When true, the final supplementary entry is the POSIX best-effort
	// append. A kernel-discovered capacity limit may omit that entry, but not
	// the mandatory real/effective GID assignment or other list adjustments.
	HasOptionalSupplementaryAppend bool
}

// errGroupChange marks a spawn failure caused by the credential change rather
// than by the shell itself, so run can retry without it under the POSIX rule.
type errGroupChange struct{ err error }

func (e *errGroupChange) Error() string { return e.err.Error() }
func (e *errGroupChange) Unwrap() error { return e.err }

func run(rc *tool.RunContext, args []string) int {
	// Embedders may reuse a RunContext. A normally exiting shell must not
	// retain the signal reported by an earlier command.
	rc.ExitSignal = 0

	// NOT tool.AliasHelpVersion: it rewrites any short cluster containing an
	// 'h' into --help, and while newgrp's only option is -l today, an operand
	// is not worth risking. tool.Parse registers -h/-V itself.
	fs := tool.NewFlags(cmd.Name)
	login := fs.BoolP("login", "l", false, "make the shell a login shell")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	if len(operands) > 1 {
		// A usage error means the command was not understood, so there is no
		// request to honour and no shell to start.
		return tool.UsageError(rc, cmd, "extra operand %q", operands[1])
	}

	u, err := db.Current()
	if err != nil {
		// Without the password-database entry there is no shell to fall back
		// to and no primary group to revert to: nothing can be done.
		fmt.Fprintf(rc.Err, "newgrp: cannot read the password database: %v\n", err)
		return 1
	}

	credential, changeErr := resolveTargetGroup(rc, u, operands)
	if changeErr != nil {
		// POSIX: diagnose the failed assignment, keep the inherited group
		// credentials, and still create the requested shell environment.
		fmt.Fprintf(rc.Err, "newgrp: %v\n", changeErr)
		credential = nil
	}

	shell := shellPath(rc, u)
	spec := shellSpec{
		Path:       shell,
		Dir:        rc.Dir,
		UID:        u.UID,
		Credential: credential,
		Argv0:      filepath.Base(shell),
	}
	if *login {
		// The leading dash makes the shell read login profiles. The clean base
		// environment matters independently because profiles need not erase
		// exported variables inherited from the old session.
		spec.Argv0 = "-" + spec.Argv0
		if u.Dir != "" {
			spec.Dir = u.Dir
		}
		spec.Env = loginEnvironment(rc, u, shell)
	}

	status, err := spawnShell(rc, spec)
	var groupErr *errGroupChange
	if errors.As(err, &groupErr) {
		// The kernel refused the credential change (the usual cause: this build
		// is not setuid-root). Same rule as a policy denial — say so, then give
		// the user their shell with the group unchanged.
		fmt.Fprintf(rc.Err, "newgrp: cannot change group: %v\n", groupErr)
		spec.Credential = nil
		status, err = spawnShell(rc, spec)
	}
	if err != nil {
		// No shell was created, so the ">0 An error occurred" branch applies.
		fmt.Fprintf(rc.Err, "newgrp: %v\n", err)
		return 1
	}
	return status
}

func resolveTargetGroup(rc *tool.RunContext, u userInfo, operands []string) (*credentialPlan, error) {
	if len(operands) == 0 {
		// No operand: revert to the primary group from the password database.
		// There is nothing to authorize — it is the group the user already has
		// by definition of "primary".
		groups, err := db.GroupsForUser(u.Name)
		if err != nil {
			return nil, fmt.Errorf("cannot derive supplementary groups from the group database: %w", err)
		}
		return &credentialPlan{
			RealGID:       u.GID,
			EffectiveGID:  u.GID,
			Supplementary: dedupeGroups(groups),
		}, nil
	}

	name := operands[0]
	g, err := db.GroupByName(name)
	if errors.Is(err, errNoSuchGroup) {
		// A numeric operand may name a gid directly. Names are tried FIRST: a
		// group literally called "100" must resolve to itself, not to gid 100.
		if gid, ok := canonicalNumericGID(name); ok {
			g, err = db.GroupByID(gid)
		}
	}
	if errors.Is(err, errNoSuchGroup) {
		return nil, fmt.Errorf("no such group: %s", name)
	}
	if err != nil {
		return nil, fmt.Errorf("cannot read the group database: %w", err)
	}

	var authErr error
	switch authorize(u, g) {
	case permit:
		// allowed
	case challenge:
		pw, err := promptPassword(rc, "Password: ")
		if err != nil {
			authErr = fmt.Errorf("cannot read the group password: %w", err)
		} else {
			ok, err := verifyCrypt(g.Password, pw)
			if err != nil {
				authErr = fmt.Errorf("cannot verify the password for group %s: %w", g.Name, err)
			} else if !ok {
				authErr = fmt.Errorf("permission denied")
			}
		}
	case unverifiable:
		authErr = fmt.Errorf("cannot read the group password database for %s", g.Name)
	default:
		authErr = fmt.Errorf("permission denied")
	}

	if authErr != nil {
		return nil, authErr
	}

	current, err := currentCredentials()
	if err != nil {
		return nil, fmt.Errorf("cannot read current group credentials: %w", err)
	}
	return planGroupChange(current, g.GID), nil
}

// canonicalNumericGID recognizes the POSIX non-negative numeric group-ID
// operand and returns its value in database form. In particular, a leading
// zero does not change the ID, while signs are not part of a numeric string.
// The uint32 bound matches the credential adapter's gid representation.
func canonicalNumericGID(value string) (string, bool) {
	if value == "" {
		return "", false
	}
	for _, c := range value {
		if c < '0' || c > '9' {
			return "", false
		}
	}
	n, err := strconv.ParseUint(value, 10, 32)
	if err != nil {
		return "", false
	}
	return strconv.FormatUint(n, 10), true
}

func planGroupChange(current credentialState, target string) *credentialPlan {
	groups := dedupeGroups(current.Supplementary)
	oldInList := slices.Contains(groups, current.EffectiveGID)
	newInList := slices.Contains(groups, target)
	hasOptionalAppend := false

	if oldInList {
		// On systems that normally include the effective gid, retain the old
		// list and add the new effective gid when there is room.
		if !newInList && groupListHasRoom(groups, current.MaxSupplementary) {
			groups = append(groups, target)
			hasOptionalAppend = true
		}
	} else {
		// On systems that normally exclude it, remove the new effective gid and
		// add the old effective gid when there is room.
		if newInList {
			groups = deleteGroup(groups, target)
		}
		if !slices.Contains(groups, current.EffectiveGID) && groupListHasRoom(groups, current.MaxSupplementary) {
			groups = append(groups, current.EffectiveGID)
			hasOptionalAppend = true
		}
	}

	return &credentialPlan{
		RealGID:                        target,
		EffectiveGID:                   target,
		Supplementary:                  groups,
		HasOptionalSupplementaryAppend: hasOptionalAppend,
	}
}

func groupListHasRoom(groups []string, max int) bool { return max <= 0 || len(groups) < max }

func deleteGroup(groups []string, target string) []string {
	out := groups[:0]
	for _, group := range groups {
		if group != target {
			out = append(out, group)
		}
	}
	return out
}

func dedupeGroups(groups []string) []string {
	out := make([]string, 0, len(groups))
	for _, group := range groups {
		if !slices.Contains(out, group) {
			out = append(out, group)
		}
	}
	return out
}

func loginEnvironment(rc *tool.RunContext, u userInfo, shell string) []string {
	env := []string{
		"HOME=" + u.Dir,
		"SHELL=" + shell,
		"USER=" + u.Name,
		"LOGNAME=" + u.Name,
		"PATH=/usr/bin:/bin:/usr/sbin:/sbin",
	}
	// Terminal type and locale/time-zone settings describe the login channel,
	// not mutable session state. Login implementations conventionally retain or
	// reconstruct them, and retaining them avoids reverting diagnostics to C.
	for _, name := range []string{
		"TERM", "TZ", "LANG", "LC_ALL", "LC_COLLATE", "LC_CTYPE",
		"LC_MESSAGES", "LC_MONETARY", "LC_NUMERIC", "LC_TIME", "NLSPATH",
	} {
		if value := rc.Getenv(name); value != "" {
			env = append(env, name+"="+value)
		}
	}
	return env
}

// shellPath chooses the replacement shell: $SHELL, then the password-database
// shell, then /bin/sh. The environment comes from the RunContext, never
// os.Getenv — an embedding shell's environment is not the process's.
func shellPath(rc *tool.RunContext, u userInfo) string {
	if s := rc.Getenv("SHELL"); s != "" {
		return s
	}
	if u.Shell != "" {
		return u.Shell
	}
	return defaultShell
}
