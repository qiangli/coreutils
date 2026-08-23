package newgrpcmd

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

// The group password guarding "secrets" in the fixtures below. Its hash is a
// real $6$ crypt record (see crypt_test.go for the provenance of the scheme),
// so the challenge path is exercised end to end rather than through a stub
// comparison.
const (
	fixturePassword = "Hello world!"
	fixtureHash     = "$6$saltstring$svn8UoSVapNtMuq1ukKS4tPQd8iKwSMHWjl/O817G3uBnIFNjnQJuesI68u4OTLiBFdcbYEdFCoEOfaS35inz1"
)

// fakeDB is a synthetic password/group database. Nothing here needs privilege,
// a real account, or a real group.
type fakeDB struct {
	user   userInfo
	groups []groupInfo
	err    error
}

func (f *fakeDB) Current() (userInfo, error) {
	if f.err != nil {
		return userInfo{}, f.err
	}
	return f.user, nil
}

func (f *fakeDB) GroupByName(name string) (groupInfo, error) {
	for _, g := range f.groups {
		if g.Name == name {
			return g, nil
		}
	}
	return groupInfo{}, errNoSuchGroup
}

func (f *fakeDB) GroupByID(gid string) (groupInfo, error) {
	for _, g := range f.groups {
		if g.GID == gid {
			return g, nil
		}
	}
	return groupInfo{}, errNoSuchGroup
}

func fixtureUser() userInfo {
	return userInfo{
		Name:          "alice",
		UID:           "1000",
		GID:           "1000", // primary group "alice"
		Dir:           "/home/alice",
		Shell:         "/bin/dbshell",
		Supplementary: []string{"1000", "50"},
	}
}

func fixtureGroups() []groupInfo {
	return []groupInfo{
		{Name: "alice", GID: "1000"},                           // her primary group
		{Name: "staff", GID: "20", Members: []string{"alice"}}, // listed member
		{Name: "wheel", GID: "50"},                             // supplementary only
		{Name: "locked", GID: "60", Password: "!"},             // locked, not a member
		{Name: "closed", GID: "70"},                            // no password, not a member
		{Name: "secrets", GID: "80", Password: fixtureHash},    // password, not a member
		{Name: "shadowed", GID: "90", PasswordShadowed: true},  // hash unreadable
		// A group whose NAME is a number, at a different gid. alice is a
		// listed member so the operand resolves rather than being denied,
		// which is what makes the name-before-gid order observable.
		{Name: "100", GID: "999", Members: []string{"alice"}},
	}
}

// spawned records what the privileged half was asked to do, without doing it.
type spawned struct {
	calls  []shellSpec
	status int
	err    error
	// errOnce is returned by the FIRST call only, so a retry can be observed.
	errOnce error
}

func (s *spawned) run(_ *tool.RunContext, spec shellSpec) (int, error) {
	s.calls = append(s.calls, spec)
	if s.errOnce != nil && len(s.calls) == 1 {
		return 0, s.errOnce
	}
	if s.err != nil {
		return 0, s.err
	}
	return s.status, nil
}

type harness struct {
	db      *fakeDB
	spawn   *spawned
	asked   []string // the prompts issued
	answer  string
	promptE error
}

func install(t *testing.T) *harness {
	t.Helper()
	h := &harness{db: &fakeDB{user: fixtureUser(), groups: fixtureGroups()}, spawn: &spawned{}}
	oldDB, oldSpawn, oldPrompt := db, spawnShell, promptPassword
	db = h.db
	spawnShell = h.spawn.run
	promptPassword = func(_ *tool.RunContext, prompt string) (string, error) {
		h.asked = append(h.asked, prompt)
		if h.promptE != nil {
			return "", h.promptE
		}
		return h.answer, nil
	}
	t.Cleanup(func() { db, spawnShell, promptPassword = oldDB, oldSpawn, oldPrompt })
	return h
}

// exec runs the command and only THEN reads the buffers. Reading them in the
// return expression would capture them before run had written anything, and
// every output assertion would pass vacuously.
func runCmd(t *testing.T, env []string, args ...string) (string, string, int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Dir:   "/work",
		Env:   env,
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
	}
	code := run(rc, args)
	return out.String(), errb.String(), code
}

var testEnv = []string{"SHELL=/bin/testsh"}

// --- the decision logic, isolated from any spawn -----------------------------

func TestAuthorize(t *testing.T) {
	u := fixtureUser()
	byName := map[string]groupInfo{}
	for _, g := range fixtureGroups() {
		byName[g.Name] = g
	}
	for _, tc := range []struct {
		group string
		want  decision
		why   string
	}{
		{"alice", permit, "the primary group needs no permission at all"},
		{"staff", permit, "a listed member needs no password"},
		{"wheel", permit, "a supplementary membership counts even with no member list"},
		{"secrets", challenge, "a non-member may be admitted by the group password"},
		{"closed", deny, "a non-member with no group password is refused"},
		{"locked", deny, "'!' is a locked password, not a password to prompt for"},
		{"shadowed", unverifiable, "an unreadable shadow entry is not the same as no password"},
	} {
		t.Run(tc.group, func(t *testing.T) {
			if got := authorize(u, byName[tc.group]); got != tc.want {
				t.Errorf("authorize(%s) = %v, want %v — %s", tc.group, got, tc.want, tc.why)
			}
		})
	}
}

// A member of a group that ALSO has a password is never asked for it: the
// password exists to admit non-members.
func TestMemberOfAPasswordedGroupIsNotChallenged(t *testing.T) {
	u := fixtureUser()
	g := groupInfo{Name: "secrets", GID: "80", Password: fixtureHash, Members: []string{"alice"}}
	if got := authorize(u, g); got != permit {
		t.Errorf("authorize = %v, want permit: membership outranks the password", got)
	}
}

func TestUsableGroupPassword(t *testing.T) {
	for _, tc := range []struct {
		field string
		want  bool
	}{
		{"", false},
		{"x", false},
		{"*", false},
		{"!", false},
		{"!!", false},
		{fixtureHash, true},
	} {
		if got := usableGroupPassword(tc.field); got != tc.want {
			t.Errorf("usableGroupPassword(%q) = %v, want %v", tc.field, got, tc.want)
		}
	}
}

// --- group selection ----------------------------------------------------------

func TestNoOperandRevertsToThePrimaryGroup(t *testing.T) {
	h := install(t)
	_, errOut, code := runCmd(t, testEnv)
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	if errOut != "" {
		t.Errorf("reverting to the primary group is not a diagnostic case: %q", errOut)
	}
	if len(h.spawn.calls) != 1 || h.spawn.calls[0].GID != "1000" {
		t.Errorf("spawned %+v, want one call at the primary gid 1000", h.spawn.calls)
	}
}

func TestGroupOperandByName(t *testing.T) {
	h := install(t)
	if _, errOut, code := runCmd(t, testEnv, "staff"); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	if h.spawn.calls[0].GID != "20" {
		t.Errorf("gid = %q, want 20", h.spawn.calls[0].GID)
	}
}

// A numeric operand names a gid only AFTER a name lookup misses. A group
// literally called "100" must resolve to itself (gid 999), not to gid 100.
func TestNumericOperandPrefersTheGroupName(t *testing.T) {
	h := install(t)
	if _, errOut, code := runCmd(t, testEnv, "100"); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	if h.spawn.calls[0].GID != "999" {
		t.Errorf("gid = %q, want 999 (the group NAMED \"100\", not gid 100)", h.spawn.calls[0].GID)
	}

	// With no such name, the same operand resolves as a gid.
	h2 := install(t)
	h2.db.groups = []groupInfo{{Name: "alice", GID: "1000"}, {Name: "wheel", GID: "50"}}
	if _, errOut, code := runCmd(t, testEnv, "50"); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	if h2.spawn.calls[0].GID != "50" {
		t.Errorf("gid = %q, want 50 resolved numerically", h2.spawn.calls[0].GID)
	}
}

// --- the POSIX exec-anyway rule -----------------------------------------------

// "If newgrp succeeds in creating a new shell execution environment, WHETHER OR
// NOT the group identification was changed successfully, the exit status shall
// be the exit status of the shell." So a refusal still starts the shell, with
// the group left alone. Exiting instead would log a user out for a typo.
func TestRefusedChangeStillStartsTheShellWithTheGroupUnchanged(t *testing.T) {
	for _, tc := range []struct {
		name    string
		operand string
		want    string
	}{
		{"unknown group", "nosuchgroup", "no such group"},
		{"not a member, no password", "closed", "permission denied"},
		{"not a member, locked password", "locked", "permission denied"},
		{"password database unreadable", "shadowed", "cannot read the group password database"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := install(t)
			h.spawn.status = 7 // the shell's own status must be what comes back
			_, errOut, code := runCmd(t, testEnv, tc.operand)
			if !strings.Contains(errOut, tc.want) {
				t.Errorf("stderr = %q, want it to contain %q", errOut, tc.want)
			}
			if len(h.spawn.calls) != 1 {
				t.Fatalf("the shell must still be started: %d spawns", len(h.spawn.calls))
			}
			if h.spawn.calls[0].GID != "" {
				t.Errorf("gid = %q, want the group left UNCHANGED after a refusal", h.spawn.calls[0].GID)
			}
			if code != 7 {
				t.Errorf("exit %d, want the shell's status 7", code)
			}
		})
	}
}

// A kernel refusal of the credential (the normal case for a build that is not
// setuid-root) follows the same rule: diagnose, then retry without it.
func TestKernelRefusalRetriesWithoutTheCredential(t *testing.T) {
	h := install(t)
	h.spawn.errOnce = &errGroupChange{errors.New("operation not permitted")}
	h.spawn.status = 3
	_, errOut, code := runCmd(t, testEnv, "staff")
	if !strings.Contains(errOut, "cannot change group") {
		t.Errorf("stderr = %q, want a cannot-change-group diagnostic", errOut)
	}
	if len(h.spawn.calls) != 2 {
		t.Fatalf("want two spawns (attempt then retry), got %d", len(h.spawn.calls))
	}
	if h.spawn.calls[0].GID != "20" || h.spawn.calls[1].GID != "" {
		t.Errorf("spawns = %+v, want the first to carry gid 20 and the retry none", h.spawn.calls)
	}
	if code != 3 {
		t.Errorf("exit %d, want the shell's status 3", code)
	}
}

// The other branch of the same clause: no shell was created, so the status is
// an error rather than a shell's.
func TestShellThatCannotStartIsAnError(t *testing.T) {
	h := install(t)
	h.spawn.err = errors.New("cannot run /bin/testsh: no such file")
	_, errOut, code := runCmd(t, testEnv, "staff")
	if code == 0 {
		t.Fatal("a shell that never started must not exit 0")
	}
	if !strings.Contains(errOut, "cannot run") {
		t.Errorf("stderr = %q, want the spawn error", errOut)
	}
}

// A usage error is not "the group change failed": the command was not
// understood, so there is no request to honour and no shell to start.
func TestUsageErrorsStartNoShell(t *testing.T) {
	for _, args := range [][]string{
		{"one", "two"},
		{"--nosuchflag"},
	} {
		t.Run(fmt.Sprint(args), func(t *testing.T) {
			h := install(t)
			_, errOut, code := runCmd(t, testEnv, args...)
			if code == 0 {
				t.Fatalf("args %v must be a usage error", args)
			}
			if len(h.spawn.calls) != 0 {
				t.Errorf("a usage error must start no shell, got %+v", h.spawn.calls)
			}
			if !strings.Contains(errOut, "newgrp") {
				t.Errorf("stderr = %q, want a diagnostic naming the command", errOut)
			}
		})
	}
}

// --- the password challenge ---------------------------------------------------

func TestCorrectGroupPasswordPermitsTheChange(t *testing.T) {
	h := install(t)
	h.answer = fixturePassword
	_, errOut, code := runCmd(t, testEnv, "secrets")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	if len(h.asked) != 1 {
		t.Errorf("want exactly one password prompt, got %v", h.asked)
	}
	if h.spawn.calls[0].GID != "80" {
		t.Errorf("gid = %q, want 80 after a correct password", h.spawn.calls[0].GID)
	}
}

func TestWrongGroupPasswordIsDeniedButStillStartsTheShell(t *testing.T) {
	h := install(t)
	h.answer = "not the password"
	_, errOut, code := runCmd(t, testEnv, "secrets")
	if !strings.Contains(errOut, "permission denied") {
		t.Errorf("stderr = %q, want permission denied", errOut)
	}
	if len(h.spawn.calls) != 1 || h.spawn.calls[0].GID != "" {
		t.Errorf("spawns = %+v, want one shell with the group unchanged", h.spawn.calls)
	}
	if code != 0 {
		t.Errorf("exit %d, want the shell's status", code)
	}
}

func TestNoPromptWhenTheUserIsAlreadyAMember(t *testing.T) {
	h := install(t)
	if _, _, code := runCmd(t, testEnv, "staff"); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if len(h.asked) != 0 {
		t.Errorf("a member must not be prompted, prompts = %v", h.asked)
	}
}

// A prompt that cannot be issued (no terminal) is a failure to authenticate,
// not a silent pass and not a crash.
func TestPromptFailureIsDeniedNotAssumed(t *testing.T) {
	h := install(t)
	h.promptE = errors.New("no terminal available to read a password")
	_, errOut, code := runCmd(t, testEnv, "secrets")
	if !strings.Contains(errOut, "no terminal") {
		t.Errorf("stderr = %q, want the prompt failure reported", errOut)
	}
	if h.spawn.calls[0].GID != "" {
		t.Error("an unanswerable prompt must not grant the group")
	}
	if code != 0 {
		t.Errorf("exit %d, want the shell's status", code)
	}
}

// A hash this build cannot evaluate must be refused with a diagnostic that says
// so — never treated as a mismatch (which blames the user) and never as a pass.
func TestUnsupportedHashSchemeIsRefusedExplicitly(t *testing.T) {
	h := install(t)
	h.db.groups = append(h.db.groups, groupInfo{Name: "future", GID: "111", Password: "$y$j9T$salt$digest"})
	h.answer = "anything"
	_, errOut, code := runCmd(t, testEnv, "future")
	if !strings.Contains(errOut, "cannot verify") {
		t.Errorf("stderr = %q, want a cannot-verify diagnostic", errOut)
	}
	if h.spawn.calls[0].GID != "" {
		t.Error("an unevaluable hash must not grant the group")
	}
	if code != 0 {
		t.Errorf("exit %d, want the shell's status", code)
	}
}

// --- shell selection and argv construction ------------------------------------

func TestShellSelectionOrder(t *testing.T) {
	for _, tc := range []struct {
		name  string
		env   []string
		shell string
		want  string
	}{
		{"$SHELL wins", []string{"SHELL=/bin/envsh"}, "/bin/dbshell", "/bin/envsh"},
		{"password database next", nil, "/bin/dbshell", "/bin/dbshell"},
		{"default last", nil, "", defaultShell},
		{"empty $SHELL falls through", []string{"SHELL="}, "/bin/dbshell", "/bin/dbshell"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := install(t)
			h.db.user.Shell = tc.shell
			if _, errOut, code := runCmd(t, tc.env); code != 0 {
				t.Fatalf("exit %d, stderr %q", code, errOut)
			}
			if h.spawn.calls[0].Path != tc.want {
				t.Errorf("shell = %q, want %q", h.spawn.calls[0].Path, tc.want)
			}
		})
	}
}

// argv[0] carries the login-shell signal: a leading dash is how every shell
// decides to read its profile files. Without it, -l would be silently inert.
func TestLoginShellArgv0AndDirectory(t *testing.T) {
	h := install(t)
	if _, errOut, code := runCmd(t, testEnv, "-l"); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	got := h.spawn.calls[0]
	if got.Argv0 != "-testsh" {
		t.Errorf("argv[0] = %q, want %q", got.Argv0, "-testsh")
	}
	if got.Dir != "/home/alice" {
		t.Errorf("dir = %q, want the home directory", got.Dir)
	}
}

func TestNonLoginShellArgv0AndDirectory(t *testing.T) {
	h := install(t)
	if _, errOut, code := runCmd(t, testEnv); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	got := h.spawn.calls[0]
	if got.Argv0 != "testsh" {
		t.Errorf("argv[0] = %q, want the bare basename", got.Argv0)
	}
	if got.Dir != "/work" {
		t.Errorf("dir = %q, want the invocation directory unchanged", got.Dir)
	}
}

// -l is still a login shell when the group change was refused: the two are
// independent, and dropping the login flag on a refusal would compound one
// surprise with another.
func TestLoginFlagSurvivesARefusedChange(t *testing.T) {
	h := install(t)
	if _, _, code := runCmd(t, testEnv, "-l", "closed"); code != 0 {
		t.Fatalf("exit %d", code)
	}
	got := h.spawn.calls[0]
	if got.Argv0 != "-testsh" || got.GID != "" {
		t.Errorf("spec = %+v, want a login shell with no group change", got)
	}
}

func TestUnreadablePasswordDatabaseIsFatal(t *testing.T) {
	h := install(t)
	h.db.err = errors.New("nss is down")
	_, errOut, code := runCmd(t, testEnv)
	if code == 0 {
		t.Fatal("without the caller's own record there is no shell and no primary group to revert to")
	}
	if len(h.spawn.calls) != 0 {
		t.Error("no shell may be started when the user record is unreadable")
	}
	if !strings.Contains(errOut, "password database") {
		t.Errorf("stderr = %q", errOut)
	}
}

func TestHelpAndVersionStartNoShell(t *testing.T) {
	for _, arg := range []string{"--help", "--version", "-h", "-V"} {
		t.Run(arg, func(t *testing.T) {
			h := install(t)
			out, errOut, code := runCmd(t, testEnv, arg)
			if code != 0 {
				t.Fatalf("%s exit %d, stderr %q", arg, code, errOut)
			}
			if len(h.spawn.calls) != 0 {
				t.Errorf("%s must not start a shell", arg)
			}
			if !strings.Contains(out, "newgrp") {
				t.Errorf("%s output = %q", arg, out)
			}
		})
	}
}
