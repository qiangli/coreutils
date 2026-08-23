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

// shellSpec is everything the spawn needs. Gid is "" when the group is NOT to
// be changed — the POSIX "diagnostic written, environment unchanged, shell
// still started" path.
type shellSpec struct {
	Path  string // the executable
	Argv0 string // argv[0]: "-sh" under -l, "sh" otherwise
	Dir   string // working directory
	GID   string // "" = leave the group alone
	UID   string
}

// errGroupChange marks a spawn failure caused by the credential change rather
// than by the shell itself, so run can retry without it under the POSIX rule.
type errGroupChange struct{ err error }

func (e *errGroupChange) Error() string { return e.err.Error() }
func (e *errGroupChange) Unwrap() error { return e.err }

func run(rc *tool.RunContext, args []string) int {
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

	gid, changeErr := resolveTargetGroup(rc, u, operands)
	if changeErr != nil {
		// POSIX: diagnostic, environment unchanged, shell still started.
		fmt.Fprintf(rc.Err, "newgrp: %v\n", changeErr)
		gid = ""
	}

	spec := shellSpec{
		Path:  shellPath(rc, u),
		Dir:   rc.Dir,
		GID:   gid,
		UID:   u.UID,
		Argv0: filepath.Base(shellPath(rc, u)),
	}
	if *login {
		// A login shell is signalled by a leading dash on argv[0] — the
		// convention every shell checks to decide whether to read its
		// login-profile files. Starting from the home directory is the other
		// half of "as if the user logged in again"; the profile the shell then
		// reads rebuilds the rest of the environment, which is why newgrp does
		// not scrub it here (unlike su, there is no target USER to switch to).
		spec.Argv0 = "-" + spec.Argv0
		if u.Dir != "" {
			spec.Dir = u.Dir
		}
	}

	status, err := spawnShell(rc, spec)
	var groupErr *errGroupChange
	if errors.As(err, &groupErr) {
		// The kernel refused the credential change (the usual cause: this build
		// is not setuid-root). Same rule as a policy denial — say so, then give
		// the user their shell with the group unchanged.
		fmt.Fprintf(rc.Err, "newgrp: cannot change group: %v\n", groupErr)
		spec.GID = ""
		status, err = spawnShell(rc, spec)
	}
	if err != nil {
		// No shell was created, so the ">0 An error occurred" branch applies.
		fmt.Fprintf(rc.Err, "newgrp: %v\n", err)
		return 1
	}
	return status
}

// resolveTargetGroup returns the gid to switch to, or an error describing why
// the switch must not happen. An error is never fatal here — see run.
func resolveTargetGroup(rc *tool.RunContext, u userInfo, operands []string) (string, error) {
	if len(operands) == 0 {
		// No operand: revert to the primary group from the password database.
		// There is nothing to authorize — it is the group the user already has
		// by definition of "primary".
		return u.GID, nil
	}

	name := operands[0]
	g, err := db.GroupByName(name)
	if errors.Is(err, errNoSuchGroup) {
		// A numeric operand may name a gid directly. Names are tried FIRST: a
		// group literally called "100" must resolve to itself, not to gid 100.
		if _, convErr := strconv.Atoi(name); convErr == nil {
			g, err = db.GroupByID(name)
		}
	}
	if errors.Is(err, errNoSuchGroup) {
		return "", fmt.Errorf("no such group: %s", name)
	}
	if err != nil {
		return "", fmt.Errorf("cannot read the group database: %w", err)
	}

	switch authorize(u, g) {
	case permit:
		return g.GID, nil

	case challenge:
		pw, err := promptPassword(rc, "Password: ")
		if err != nil {
			return "", fmt.Errorf("cannot read the group password: %w", err)
		}
		ok, err := verifyCrypt(g.Password, pw)
		if err != nil {
			return "", fmt.Errorf("cannot verify the password for group %s: %w", g.Name, err)
		}
		if !ok {
			return "", fmt.Errorf("permission denied")
		}
		return g.GID, nil

	case unverifiable:
		return "", fmt.Errorf("cannot read the group password database for %s", g.Name)

	default:
		return "", fmt.Errorf("permission denied")
	}
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
