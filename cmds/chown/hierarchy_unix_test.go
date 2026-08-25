//go:build unix

package chowncmd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

// linkTree builds the hierarchy the -H/-L/-P rules are defined over:
//
//	d/sub/f
//	d/link -> sub   (a symbolic link to a directory, below the operand)
//	toplink -> d    (a symbolic link to a directory, as the operand)
func linkTree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "d", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "d", "sub", "f"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	symlink(t, "sub", filepath.Join(dir, "d", "link"))
	symlink(t, "d", filepath.Join(dir, "toplink"))
	return dir
}

func symlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}
}

// visited reads the hierarchy a run reached off its -v output. Every
// file keeps the ownership it has, so each line is a "retained" report
// and the sequence is exactly the traversal.
func visited(t *testing.T, out string) string {
	t.Helper()
	var names []string
	for _, line := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
		if line == "" {
			continue
		}
		name, ok := strings.CutPrefix(line, "ownership of '")
		if !ok {
			t.Fatalf("unexpected verbose line %q", line)
		}
		name, _, _ = strings.Cut(name, "'")
		names = append(names, name)
	}
	return strings.Join(names, " ")
}

func TestChownTraversalModes(t *testing.T) {
	u := currentUser(t)
	cases := []struct {
		name    string
		args    []string
		operand string
		want    string
	}{
		// POSIX: -R alone behaves as -P. No symbolic link is followed,
		// so the link is a leaf and its referent is reached only by its
		// real name.
		{"default is physical", []string{"-R"}, "d", "d/link d/sub/f d/sub d"},
		{"explicit -P", []string{"-R", "-P"}, "d", "d/link d/sub/f d/sub d"},
		// -P applies to an operand link too: it is not followed.
		{"-P operand link", []string{"-R", "-P"}, "toplink", "toplink"},
		{"default operand link", []string{"-R"}, "toplink", "toplink"},
		// -H follows a link named on the command line and nothing else.
		{"-H operand link", []string{"-R", "-H"}, "toplink", "toplink/link toplink/sub/f toplink/sub toplink"},
		{"-H interior link", []string{"-R", "-H"}, "d", "d/link d/sub/f d/sub d"},
		// -L follows every link to a directory, so the subtree is
		// reached under both names.
		{"-L interior link", []string{"-R", "-L"}, "d", "d/link/f d/link d/sub/f d/sub d"},
		{"-L operand link", []string{"-R", "-L"}, "toplink", "toplink/link/f toplink/link toplink/sub/f toplink/sub toplink"},
		// POSIX: the three override each other; the last one wins.
		{"-L then -P", []string{"-R", "-L", "-P"}, "d", "d/link d/sub/f d/sub d"},
		{"-P then -L", []string{"-R", "-P", "-L"}, "d", "d/link/f d/link d/sub/f d/sub d"},
		{"-L then -H", []string{"-R", "-L", "-H"}, "d", "d/link d/sub/f d/sub d"},
		{"-H then -L", []string{"-R", "-H", "-L"}, "d", "d/link/f d/link d/sub/f d/sub d"},
		{"clustered last wins", []string{"-RLP"}, "d", "d/link d/sub/f d/sub d"},
		{"clustered -H after -L", []string{"-RL", "-H"}, "toplink", "toplink/link toplink/sub/f toplink/sub toplink"},
		// Without -R the traversal options are ignored entirely.
		{"-L without -R", []string{"-L"}, "d", "d"},
		{"-H without -R", []string{"-H"}, "toplink", "toplink"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := linkTree(t)
			args := append(append([]string{"-v"}, tc.args...), u.Uid, tc.operand)
			out, errb, code := runTool(t, dir, args...)
			if code != 0 || errb != "" {
				t.Fatalf("chown %v: code=%d err=%q", args, code, errb)
			}
			if got := visited(t, out); got != filepath.FromSlash(tc.want) {
				t.Errorf("chown %v reached %q, want %q", args, got, tc.want)
			}
		})
	}
}

// recordChanges replaces the ownership syscall with a recorder. Only
// root may change a file to another owner, so this is how an
// unprivileged test can see which call each file would take; the real
// syscall is still exercised by the self-chown tests.
type recordedChange struct {
	path   string
	uid    int
	gid    int
	follow bool
}

func (c recordedChange) String() string {
	call := "lchown"
	if c.follow {
		call = "chown"
	}
	return call + " " + filepath.Base(c.path) + " " + strconv.Itoa(c.uid) + " " + strconv.Itoa(c.gid)
}

func recordChanges(t *testing.T) *[]recordedChange {
	t.Helper()
	var calls []recordedChange
	restore := changeOwner
	changeOwner = func(path string, uid, gid int, follow bool) error {
		calls = append(calls, recordedChange{path: path, uid: uid, gid: gid, follow: follow})
		return nil
	}
	t.Cleanup(func() { changeOwner = restore })
	return &calls
}

func joinedChanges(calls []recordedChange) string {
	parts := make([]string, 0, len(calls))
	for _, call := range calls {
		parts = append(parts, call.String())
	}
	return strings.Join(parts, "; ")
}

func TestChownSymbolicLinkTargetOfTheChange(t *testing.T) {
	u := currentUser(t)
	other := strconv.Itoa(atoi(t, u.Uid) + 1)
	cases := []struct {
		name string
		args []string
		want string
	}{
		// A physical -R walk cannot reach a referent, so POSIX has it
		// change the link itself.
		{"-R changes the link", []string{"-R"}, "lchown link " + other + " -1"},
		{"-R -P changes the link", []string{"-R", "-P"}, "lchown link " + other + " -1"},
		// -L reaches referents, so an interior link's referent is what
		// changes unless -h says otherwise.
		{"-L changes the referent", []string{"-R", "-L"}, "chown link " + other + " -1"},
		{"-L -h changes the link", []string{"-R", "-L", "-h"}, "lchown link " + other + " -1"},
		// Without -R the default is still to follow the operand link.
		{"no -R follows the link", nil, "chown link " + other + " -1"},
		{"-h changes the link", []string{"-h"}, "lchown link " + other + " -1"},
		// The last of -h/--dereference wins, as it does for -H/-L/-P.
		{"--dereference after -h", []string{"-h", "--dereference"}, "chown link " + other + " -1"},
		{"-h after --dereference", []string{"--dereference", "-h"}, "lchown link " + other + " -1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "target"), nil, 0o644); err != nil {
				t.Fatal(err)
			}
			symlink(t, "target", filepath.Join(dir, "link"))
			calls := recordChanges(t)

			_, errb, code := runTool(t, dir, append(append([]string{}, tc.args...), other, "link")...)
			if code != 0 || errb != "" {
				t.Fatalf("chown %v: code=%d err=%q", tc.args, code, errb)
			}
			if got := joinedChanges(*calls); got != tc.want {
				t.Errorf("chown %v issued %q, want %q", tc.args, got, tc.want)
			}
		})
	}
}

// POSIX requires an action equivalent to chown() for every selected file,
// even when it already has the requested ownership. The equality check
// controls reporting only; it must not suppress the ownership syscall.
func TestChownUnchangedOwnershipStillCallsChown(t *testing.T) {
	u := currentUser(t)
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "d"), 0o755); err != nil {
		t.Fatal(err)
	}
	setuid := filepath.Join(dir, "d", "setuid")
	if err := os.WriteFile(setuid, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	calls := recordChanges(t)
	_, errb, code := runTool(t, dir, "-R", u.Uid+":"+u.Gid, "d")
	if code != 0 || errb != "" {
		t.Fatalf("chown -R self: code=%d err=%q", code, errb)
	}
	wantCalls := "lchown setuid " + u.Uid + " " + u.Gid + "; lchown d " + u.Uid + " " + u.Gid
	if got := joinedChanges(*calls); got != wantCalls {
		t.Errorf("chown to the ids already held issued %q, want %q", got, wantCalls)
	}

	// The same run against the real syscall must have chown(2)'s observable
	// side effect for an unprivileged caller: clearing both set-ID bits.
	if err := os.Chmod(setuid, os.ModeSetuid|os.ModeSetgid|0o755); err != nil {
		t.Skipf("set-user-ID is unavailable: %v", err)
	}
	fi, err := os.Stat(setuid)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&(os.ModeSetuid|os.ModeSetgid) != os.ModeSetuid|os.ModeSetgid {
		t.Skip("the filesystem dropped a set-ID bit")
	}
	if os.Geteuid() == 0 {
		t.Skip("set-ID clearing is implementation-defined for privileged callers")
	}
	changeOwner = func(path string, uid, gid int, follow bool) error {
		if follow {
			return os.Chown(path, uid, gid)
		}
		return os.Lchown(path, uid, gid)
	}
	if _, errb, code := runTool(t, dir, "-R", u.Uid+":"+u.Gid, "d"); code != 0 || errb != "" {
		t.Fatalf("chown -R self: code=%d err=%q", code, errb)
	}
	if fi, err = os.Stat(setuid); err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&(os.ModeSetuid|os.ModeSetgid) != 0 {
		t.Errorf("chown to the ids already held left set-ID mode %v", fi.Mode()&(os.ModeSetuid|os.ModeSetgid))
	}
}

func TestChownSymbolicLinkCycleTerminates(t *testing.T) {
	u := currentUser(t)
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "d", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	symlink(t, "..", filepath.Join(dir, "d", "sub", "up"))
	symlink(t, ".", filepath.Join(dir, "d", "self"))

	out, errb, code := runTool(t, dir, "-v", "-R", "-L", u.Uid, "d")
	if code != 0 || errb != "" {
		t.Fatalf("chown -R -L over a cycle: code=%d err=%q", code, errb)
	}
	want := filepath.FromSlash("d/self d/sub/up d/sub d")
	if got := visited(t, out); got != want {
		t.Errorf("chown -R -L over a cycle reached %q, want %q", got, want)
	}
}

func TestChownMutualSymbolicLinkLoopTerminates(t *testing.T) {
	u := currentUser(t)
	dir := t.TempDir()
	for _, name := range []string{"a", "b"} {
		if err := os.Mkdir(filepath.Join(dir, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	symlink(t, filepath.Join("..", "b"), filepath.Join(dir, "a", "toB"))
	symlink(t, filepath.Join("..", "a"), filepath.Join(dir, "b", "toA"))

	out, errb, code := runTool(t, dir, "-v", "-R", "-L", u.Uid, "a")
	if code != 0 || errb != "" {
		t.Fatalf("chown -R -L over a loop: code=%d err=%q", code, errb)
	}
	if got := visited(t, out); got != filepath.FromSlash("a/toB/toA a/toB a") {
		t.Errorf("chown -R -L over a loop reached %q", got)
	}
}

func TestChownDanglingSymbolicLink(t *testing.T) {
	u := currentUser(t)
	cases := []struct {
		name    string
		args    []string
		code    int
		wantErr string
	}{
		{"default dereferences", nil, 1, "chown: cannot dereference 'dangling': no such file or directory\n"},
		{"-h acts on the link", []string{"-h"}, 0, ""},
		{"-R -L dereferences", []string{"-R", "-L"}, 1, "chown: cannot dereference 'dangling': no such file or directory\n"},
		{"-R acts on the link", []string{"-R"}, 0, ""},
		{"-f is silent", []string{"-f"}, 1, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			symlink(t, "nowhere", filepath.Join(dir, "dangling"))
			recordChanges(t)
			_, errb, code := runTool(t, dir, append(append([]string{}, tc.args...), u.Uid, "dangling")...)
			if code != tc.code {
				t.Errorf("chown %v: code=%d, want %d (err=%q)", tc.args, code, tc.code, errb)
			}
			if errb != tc.wantErr {
				t.Errorf("chown %v: err=%q, want %q", tc.args, errb, tc.wantErr)
			}
		})
	}
}

// POSIX: a failure on one file does not stop the others, and the exit
// status still reports it. Diagnostics name the operand as written, not
// the path the RunContext resolved it to.
func TestChownContinuesPastFailures(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads unreadable directories")
	}
	u := currentUser(t)
	dir := t.TempDir()
	locked := filepath.Join(dir, "d", "locked")
	if err := os.MkdirAll(locked, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "d", "after"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Skipf("chmod is unavailable: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	out, errb, code := runTool(t, dir, "-v", "-R", u.Uid, "missing", "d")
	if code != 1 {
		t.Fatalf("code=%d, want 1 (err=%q)", code, errb)
	}
	if !strings.Contains(errb, "chown: cannot access 'missing': no such file or directory\n") {
		t.Errorf("missing operand diagnostic=%q", errb)
	}
	if !strings.Contains(errb, "chown: cannot read directory '"+filepath.FromSlash("d/locked")+"'") {
		t.Errorf("unreadable directory diagnostic=%q", errb)
	}
	if strings.Contains(errb, dir) {
		t.Errorf("diagnostic leaked the resolved path: %q", errb)
	}
	// The operand after the failure, and the rest of the hierarchy
	// around the unreadable directory, are still processed.
	if got := visited(t, out); got != filepath.FromSlash("d/after d/locked d") {
		t.Errorf("reached %q", got)
	}
}

func TestChownRecursiveDereferenceRequiresHOrL(t *testing.T) {
	u := currentUser(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	calls := recordChanges(t)
	_, errb, code := runTool(t, dir, "-R", "--dereference", u.Uid, "f")
	if code != 1 || errb != "chown: -R --dereference requires either -H or -L\n" {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	if len(*calls) != 0 {
		t.Errorf("a contradictory command line still changed %v", *calls)
	}
	// -H or -L makes the request coherent.
	if _, errb, code = runTool(t, dir, "-R", "-L", "--dereference", u.Uid, "f"); code != 0 || errb != "" {
		t.Errorf("-R -L --dereference: code=%d err=%q", code, errb)
	}
	// Without -R there is no traversal to contradict.
	if _, errb, code = runTool(t, dir, "--dereference", u.Uid, "f"); code != 0 || errb != "" {
		t.Errorf("--dereference: code=%d err=%q", code, errb)
	}
}

// POSIX has each half of OWNER[:GROUP] looked up as a name first, and
// read as a number only when no such account exists. The lookup is a
// seam because no test can add an account named "42" to the host.
func TestChownNameIsPreferredOverNumber(t *testing.T) {
	restoreUser, restoreGroup := lookupUser, lookupGroup
	lookupUser = func(name string) (*user.User, error) {
		if name == "42" {
			return &user.User{Uid: "7", Gid: "8"}, nil
		}
		return nil, errors.New("no such user")
	}
	lookupGroup = func(name string) (*user.Group, error) {
		if name == "99" {
			return &user.Group{Gid: "9"}, nil
		}
		return nil, errors.New("no such group")
	}
	t.Cleanup(func() { lookupUser, lookupGroup = restoreUser, restoreGroup })

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	cases := []struct{ spec, want string }{
		{"42", "chown f 7 -1"},    // the account named "42"
		{"43", "chown f 43 -1"},   // no such account: a numeric id
		{"42:", "chown f 7 8"},    // the named account's login group
		{"43:99", "chown f 43 9"}, // group named "99" resolves to 9
		{":99", "chown f -1 9"},   // group only, owner untouched
	}
	for _, tc := range cases {
		calls := recordChanges(t)
		if _, errb, code := runTool(t, dir, tc.spec, "f"); code != 0 || errb != "" {
			t.Fatalf("chown %s: code=%d err=%q", tc.spec, code, errb)
		}
		if got := joinedChanges(*calls); got != tc.want {
			t.Errorf("chown %s issued %q, want %q", tc.spec, got, tc.want)
		}
	}
}

// --reference supplies ids, not a spec to re-parse: a host holding an
// account whose name is the reference file's numeric id must not
// capture the change.
func TestChownReferenceIdsAreNotLookedUpAsNames(t *testing.T) {
	restoreUser := lookupUser
	lookupUser = func(string) (*user.User, error) {
		return &user.User{Uid: "77", Gid: "78"}, nil
	}
	t.Cleanup(func() { lookupUser = restoreUser })

	dir := t.TempDir()
	for _, name := range []string{"ref", "f"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	calls := recordChanges(t)
	if _, errb, code := runTool(t, dir, "--reference=ref", "f"); code != 0 || errb != "" {
		t.Fatalf("chown --reference: code=%d err=%q", code, errb)
	}
	// The reference file is the caller's own, so the ids match, but POSIX
	// still requires the ownership operation to be performed with both ids.
	fi, err := os.Stat(filepath.Join(dir, "ref"))
	if err != nil {
		t.Fatal(err)
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		t.Fatal("reference stat did not expose syscall.Stat_t")
	}
	want := "chown f " + strconv.FormatUint(uint64(st.Uid), 10) + " " + strconv.FormatUint(uint64(st.Gid), 10)
	if got := joinedChanges(*calls); got != want {
		t.Errorf("--reference issued %q, want %q", got, want)
	}
}

func TestChownEmptyReferenceIsStillAnExplicitReference(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	calls := recordChanges(t)
	_, errb, code := runTool(t, dir, "--reference=", "f")
	if code != 1 || !strings.Contains(errb, "chown: cannot stat reference file '':") {
		t.Fatalf("chown --reference=: code=%d err=%q", code, errb)
	}
	if len(*calls) != 0 {
		t.Errorf("an invalid empty reference still changed %v", *calls)
	}
}

// The corrupt-hierarchy diagnostic. Detection lives in the shared
// walker, which tests it against a substituted identity predicate; this
// pins the wording and the status the command reports for it.
func TestChownCycleDiagnostic(t *testing.T) {
	rc, out, errb := newContext(t)
	reportCycle(rc, "d/sub", chownOpts{})
	if out.Len() != 0 {
		t.Errorf("cycle warning went to stdout: %q", out.String())
	}
	want := "chown: WARNING: Circular directory structure.\n" +
		"This almost certainly means that you have a corrupted file system.\n" +
		"NOTIFY YOUR SYSTEM ADMINISTRATOR.\n" +
		"The following directory is part of the cycle:\n  d/sub\n"
	if errb.String() != want {
		t.Errorf("cycle warning=%q, want %q", errb.String(), want)
	}
	errb.Reset()
	reportCycle(rc, "d/sub", chownOpts{options: options{silent: true}})
	if errb.Len() != 0 {
		t.Errorf("-f did not suppress the cycle warning: %q", errb.String())
	}
}

// newContext returns a RunContext whose streams the test owns, for the
// reporting helpers that are called directly.
func newContext(t *testing.T) (*tool.RunContext, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   t.TempDir(),
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
	}
	return rc, &out, &errb
}

func atoi(t *testing.T, s string) int {
	t.Helper()
	n, err := strconv.Atoi(s)
	if err != nil {
		t.Skipf("non-numeric id %q: %v", s, err)
	}
	return n
}

// The two option groups are orthogonal and POSIX defines them
// separately: -H/-L/-P decide which files a recursive walk reaches,
// -h decides which file the change lands on once it is reached. Their
// product is where an implementation that conflates them goes wrong, so
// every combination is pinned over one hierarchy that contains both an
// operand link and an interior link.
func TestChownTraversalAndDereferenceAreOrthogonal(t *testing.T) {
	u := currentUser(t)
	other := strconv.Itoa(atoi(t, u.Uid) + 1)
	ownerOnly := other + " -1"
	cases := []struct {
		name string
		args []string
		want string
	}{
		// -P reaches only the operand link, and cannot reach a
		// referent, so the link itself is what changes.
		{"-R -P", []string{"-R", "-P"}, "lchown toplink " + ownerOnly},
		// -H follows the operand link for the traversal. The change
		// still defaults to the referent, for the operand link and for
		// the interior link the walk does not follow.
		{"-R -H", []string{"-R", "-H"},
			"chown link " + ownerOnly + "; chown f " + ownerOnly + "; chown sub " + ownerOnly + "; chown toplink " + ownerOnly},
		// -h moves every one of those changes onto the link, without
		// changing which files the walk reached.
		{"-R -H -h", []string{"-R", "-H", "-h"},
			"lchown link " + ownerOnly + "; lchown f " + ownerOnly + "; lchown sub " + ownerOnly + "; lchown toplink " + ownerOnly},
		// -L additionally follows the interior link, so d/sub is
		// reached twice — once through the link, once by its name.
		{"-R -L", []string{"-R", "-L"},
			"chown f " + ownerOnly + "; chown link " + ownerOnly + "; chown f " + ownerOnly +
				"; chown sub " + ownerOnly + "; chown toplink " + ownerOnly},
		{"-R -L -h", []string{"-R", "-L", "-h"},
			"lchown f " + ownerOnly + "; lchown link " + ownerOnly + "; lchown f " + ownerOnly +
				"; lchown sub " + ownerOnly + "; lchown toplink " + ownerOnly},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := linkTree(t)
			calls := recordChanges(t)
			_, errb, code := runTool(t, dir, append(append([]string{}, tc.args...), other, "toplink")...)
			if code != 0 || errb != "" {
				t.Fatalf("chown %v: code=%d err=%q", tc.args, code, errb)
			}
			if got := joinedChanges(*calls); got != tc.want {
				t.Errorf("chown %v issued\n %q, want\n %q", tc.args, got, tc.want)
			}
		})
	}
}

// POSIX -H follows "a symbolic link named as an operand". An operand
// that resolves through more than one link is still one operand: the
// whole chain is followed, and only the chain's first link is the file
// -h would act on.
func TestChownCommandLineLinkChainIsFollowed(t *testing.T) {
	u := currentUser(t)
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "d", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "d", "sub", "f"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	symlink(t, "d", filepath.Join(dir, "hop"))
	symlink(t, "hop", filepath.Join(dir, "top"))

	out, errb, code := runTool(t, dir, "-v", "-R", "-H", u.Uid, "top")
	if code != 0 || errb != "" {
		t.Fatalf("chown -R -H: code=%d err=%q", code, errb)
	}
	if got := visited(t, out); got != filepath.FromSlash("top/sub/f top/sub top") {
		t.Errorf("-H over a link chain reached %q", got)
	}
	// -P does not follow it, whatever the chain resolves to.
	out, errb, code = runTool(t, dir, "-v", "-R", "-P", u.Uid, "top")
	if code != 0 || errb != "" {
		t.Fatalf("chown -R -P: code=%d err=%q", code, errb)
	}
	if got := visited(t, out); got != "top" {
		t.Errorf("-P over a link chain reached %q, want the operand alone", got)
	}
}

// POSIX Utility Syntax Guideline 10: "--" ends the options, and what
// follows is operands even when spelled like an option. The traversal
// pre-scan must honor it too, or a file named "-H" would silently
// switch a recursive walk into command-line mode instead of being
// operated on.
func TestChownDoubleDashEndsOptions(t *testing.T) {
	u := currentUser(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "-H"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	out, errb, code := runTool(t, dir, "-v", "-R", "--", u.Uid, "-H")
	if code != 0 || errb != "" {
		t.Fatalf("chown -R -- uid -H: code=%d err=%q", code, errb)
	}
	if got := visited(t, out); got != "-H" {
		t.Errorf("reached %q, want the file named -H alone", got)
	}
	// -h after "--" is likewise an operand, not --no-dereference.
	if err := os.WriteFile(filepath.Join(dir, "-h"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	out, errb, code = runTool(t, dir, "-v", "--", u.Uid, "-h")
	if code != 0 || errb != "" {
		t.Fatalf("chown -- uid -h: code=%d err=%q", code, errb)
	}
	if got := visited(t, out); got != "-h" {
		t.Errorf("reached %q, want the file named -h alone", got)
	}
}

// POSIX gives '-' no special meaning for chown: it names a file called
// "-", it does not select standard input.
func TestChownDashOperandIsAFileName(t *testing.T) {
	u := currentUser(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "-"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	out, errb, code := runTool(t, dir, "-v", u.Uid, "-")
	if code != 0 || errb != "" {
		t.Fatalf("chown uid -: code=%d err=%q", code, errb)
	}
	if got := visited(t, out); got != "-" {
		t.Errorf("reached %q, want the file named - alone", got)
	}
}

type failingOutputWriter struct{ err error }

func (w failingOutputWriter) Write([]byte) (int, error) { return 0, w.err }

func TestChownOutputFailureSetsStatusAndContinues(t *testing.T) {
	u := currentUser(t)
	other := strconv.Itoa(atoi(t, u.Uid) + 1)
	for _, flag := range []string{"-v", "-c"} {
		t.Run(flag, func(t *testing.T) {
			dir := t.TempDir()
			for _, name := range []string{"first", "second"} {
				if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			calls := recordChanges(t)
			var errb bytes.Buffer
			rc := &tool.RunContext{
				Ctx: context.Background(),
				Dir: dir,
				Stdio: tool.Stdio{
					In:  strings.NewReader(""),
					Out: failingOutputWriter{err: errors.New("broken output")},
					Err: &errb,
				},
			}

			if code := cmd.Run(rc, []string{flag, other, "first", "second"}); code != 1 {
				t.Errorf("code=%d, want 1", code)
			}
			if got, want := errb.String(), "chown: write error: broken output\n"; got != want {
				t.Errorf("stderr=%q, want %q", got, want)
			}
			if len(*calls) != 2 {
				t.Errorf("ownership work stopped after output failure: calls=%v", *calls)
			}
		})
	}
}
