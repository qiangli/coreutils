//go:build unix

package chgrpcmd

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

func currentGroup(t *testing.T) string {
	t.Helper()
	u, err := user.Current()
	if err != nil {
		t.Skipf("user.Current: %v", err)
	}
	return u.Gid
}

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
// file keeps the group it has, so each line is a "retained" report and
// the sequence is exactly the traversal.
func visited(t *testing.T, out string) string {
	t.Helper()
	var names []string
	for _, line := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
		if line == "" {
			continue
		}
		name, ok := strings.CutPrefix(line, "group of '")
		if !ok {
			t.Fatalf("unexpected verbose line %q", line)
		}
		name, _, _ = strings.Cut(name, "'")
		names = append(names, name)
	}
	return strings.Join(names, " ")
}

func TestChgrpTraversalModes(t *testing.T) {
	gid := currentGroup(t)
	cases := []struct {
		name    string
		args    []string
		operand string
		want    string
	}{
		// POSIX: -R alone behaves as -P. No symbolic link is followed.
		{"default is physical", []string{"-R"}, "d", "d/link d/sub/f d/sub d"},
		{"-P operand link", []string{"-R", "-P"}, "toplink", "toplink"},
		// -H follows a link named on the command line and nothing else.
		{"-H operand link", []string{"-R", "-H"}, "toplink", "toplink/link toplink/sub/f toplink/sub toplink"},
		{"-H interior link", []string{"-R", "-H"}, "d", "d/link d/sub/f d/sub d"},
		// -L follows every link to a directory.
		{"-L interior link", []string{"-R", "-L"}, "d", "d/link/f d/link d/sub/f d/sub d"},
		// POSIX: the three override each other; the last one wins.
		{"-L then -P", []string{"-R", "-L", "-P"}, "d", "d/link d/sub/f d/sub d"},
		{"-P then -L", []string{"-R", "-P", "-L"}, "d", "d/link/f d/link d/sub/f d/sub d"},
		{"-L then -H", []string{"-R", "-L", "-H"}, "toplink", "toplink/link toplink/sub/f toplink/sub toplink"},
		{"-H then -L", []string{"-R", "-H", "-L"}, "d", "d/link/f d/link d/sub/f d/sub d"},
		{"clustered last wins", []string{"-RLP"}, "d", "d/link d/sub/f d/sub d"},
		{"clustered -H after -L", []string{"-RL", "-H"}, "toplink", "toplink/link toplink/sub/f toplink/sub toplink"},
		{"explicit -P", []string{"-R", "-P"}, "d", "d/link d/sub/f d/sub d"},
		{"default operand link", []string{"-R"}, "toplink", "toplink"},
		{"-L operand link", []string{"-R", "-L"}, "toplink", "toplink/link/f toplink/link toplink/sub/f toplink/sub toplink"},
		// Without -R the traversal options are ignored entirely.
		{"-L without -R", []string{"-L"}, "d", "d"},
		{"-H without -R", []string{"-H"}, "toplink", "toplink"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := linkTree(t)
			args := append(append([]string{"-v"}, tc.args...), gid, tc.operand)
			out, errb, code := runTool(t, dir, args...)
			if code != 0 || errb != "" {
				t.Fatalf("chgrp %v: code=%d err=%q", args, code, errb)
			}
			if got := visited(t, out); got != filepath.FromSlash(tc.want) {
				t.Errorf("chgrp %v reached %q, want %q", args, got, tc.want)
			}
		})
	}
}

// recordChanges replaces the ownership syscall with a recorder. Only a
// member of the target group (or root) may perform the change, so this
// is how an unprivileged test can see which call each file would take;
// the real syscall is still exercised by the self-chgrp tests.
func recordChanges(t *testing.T) *[]string {
	t.Helper()
	var calls []string
	restore := changeGroup
	changeGroup = func(path string, gid int, follow bool) error {
		call := "lchown"
		if follow {
			call = "chown"
		}
		calls = append(calls, call+" "+filepath.Base(path)+" "+strconv.Itoa(gid))
		return nil
	}
	t.Cleanup(func() { changeGroup = restore })
	return &calls
}

func TestChgrpSymbolicLinkTargetOfTheChange(t *testing.T) {
	other := strconv.Itoa(atoi(t, currentGroup(t)) + 1)
	cases := []struct {
		name string
		args []string
		want string
	}{
		// A physical -R walk cannot reach a referent, so POSIX has it
		// change the link itself.
		{"-R changes the link", []string{"-R"}, "lchown link " + other},
		// -L reaches referents, so an interior link's referent is what
		// changes unless -h says otherwise.
		{"-L changes the referent", []string{"-R", "-L"}, "chown link " + other},
		{"-L -h changes the link", []string{"-R", "-L", "-h"}, "lchown link " + other},
		// Without -R the default is still to follow the operand link.
		{"no -R follows the link", nil, "chown link " + other},
		{"-h changes the link", []string{"-h"}, "lchown link " + other},
		// The last of -h/--dereference wins, as it does for -H/-L/-P.
		{"--dereference after -h", []string{"-h", "--dereference"}, "chown link " + other},
		{"-h after --dereference", []string{"--dereference", "-h"}, "lchown link " + other},
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
				t.Fatalf("chgrp %v: code=%d err=%q", tc.args, code, errb)
			}
			if got := strings.Join(*calls, "; "); got != tc.want {
				t.Errorf("chgrp %v issued %q, want %q", tc.args, got, tc.want)
			}
		})
	}
}

// POSIX requires an action equivalent to chown() for every selected file,
// even when it already has the requested group. The equality check controls
// reporting only; it must not suppress the ownership syscall.
func TestChgrpUnchangedGroupStillCallsChown(t *testing.T) {
	gid := currentGroup(t)
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "d"), 0o755); err != nil {
		t.Fatal(err)
	}
	setgid := filepath.Join(dir, "d", "setgid")
	if err := os.WriteFile(setgid, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	calls := recordChanges(t)
	if _, errb, code := runTool(t, dir, "-R", gid, "d"); code != 0 || errb != "" {
		t.Fatalf("chgrp -R self: code=%d err=%q", code, errb)
	}
	wantCalls := "lchown setgid " + gid + "; lchown d " + gid
	if got := strings.Join(*calls, "; "); got != wantCalls {
		t.Errorf("chgrp to the group already held issued %q, want %q", got, wantCalls)
	}

	// The same run against the real syscall must have chown(2)'s observable
	// side effect for an unprivileged caller: clearing set-group-ID.
	if err := os.Chmod(setgid, os.ModeSetgid|0o755); err != nil {
		t.Skipf("set-group-ID is unavailable: %v", err)
	}
	fi, err := os.Stat(setgid)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSetgid == 0 {
		t.Skip("the filesystem dropped the set-group-ID bit")
	}
	if os.Geteuid() == 0 {
		t.Skip("set-ID clearing is implementation-defined for privileged callers")
	}
	changeGroup = func(path string, gid int, follow bool) error {
		if follow {
			return os.Chown(path, -1, gid)
		}
		return os.Lchown(path, -1, gid)
	}
	if _, errb, code := runTool(t, dir, "-R", gid, "d"); code != 0 || errb != "" {
		t.Fatalf("chgrp -R self: code=%d err=%q", code, errb)
	}
	if fi, err = os.Stat(setgid); err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSetgid != 0 {
		t.Error("chgrp to the group already held did not clear the set-group-ID bit")
	}
}

func TestChgrpSymbolicLinkCycleTerminates(t *testing.T) {
	gid := currentGroup(t)
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "d", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	symlink(t, "..", filepath.Join(dir, "d", "sub", "up"))
	symlink(t, ".", filepath.Join(dir, "d", "self"))

	out, errb, code := runTool(t, dir, "-v", "-R", "-L", gid, "d")
	if code != 0 || errb != "" {
		t.Fatalf("chgrp -R -L over a cycle: code=%d err=%q", code, errb)
	}
	want := filepath.FromSlash("d/self d/sub/up d/sub d")
	if got := visited(t, out); got != want {
		t.Errorf("chgrp -R -L over a cycle reached %q, want %q", got, want)
	}
}

func TestChgrpDanglingSymbolicLink(t *testing.T) {
	gid := currentGroup(t)
	cases := []struct {
		name    string
		args    []string
		code    int
		wantErr string
	}{
		{"default dereferences", nil, 1, "chgrp: cannot dereference 'dangling': no such file or directory\n"},
		{"-h acts on the link", []string{"-h"}, 0, ""},
		{"-R -L dereferences", []string{"-R", "-L"}, 1, "chgrp: cannot dereference 'dangling': no such file or directory\n"},
		{"-R acts on the link", []string{"-R"}, 0, ""},
		{"--quiet is silent", []string{"--quiet"}, 1, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			symlink(t, "nowhere", filepath.Join(dir, "dangling"))
			recordChanges(t)
			_, errb, code := runTool(t, dir, append(append([]string{}, tc.args...), gid, "dangling")...)
			if code != tc.code {
				t.Errorf("chgrp %v: code=%d, want %d (err=%q)", tc.args, code, tc.code, errb)
			}
			if errb != tc.wantErr {
				t.Errorf("chgrp %v: err=%q, want %q", tc.args, errb, tc.wantErr)
			}
		})
	}
}

// POSIX: a failure on one file does not stop the others, and the exit
// status still reports it. Diagnostics name the operand as written, not
// the path the RunContext resolved it to.
func TestChgrpContinuesPastFailures(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads unreadable directories")
	}
	gid := currentGroup(t)
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

	out, errb, code := runTool(t, dir, "-v", "-R", gid, "missing", "d")
	if code != 1 {
		t.Fatalf("code=%d, want 1 (err=%q)", code, errb)
	}
	if !strings.Contains(errb, "chgrp: cannot access 'missing': no such file or directory\n") {
		t.Errorf("missing operand diagnostic=%q", errb)
	}
	if !strings.Contains(errb, "chgrp: cannot read directory '"+filepath.FromSlash("d/locked")+"'") {
		t.Errorf("unreadable directory diagnostic=%q", errb)
	}
	if strings.Contains(errb, dir) {
		t.Errorf("diagnostic leaked the resolved path: %q", errb)
	}
	if got := visited(t, out); got != filepath.FromSlash("d/after d/locked d") {
		t.Errorf("reached %q", got)
	}
}

func TestChgrpRecursiveDereferenceRequiresHOrL(t *testing.T) {
	gid := currentGroup(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	calls := recordChanges(t)
	_, errb, code := runTool(t, dir, "-R", "--dereference", gid, "f")
	if code != 1 || errb != "chgrp: -R --dereference requires either -H or -L\n" {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	if len(*calls) != 0 {
		t.Errorf("a contradictory command line still changed %v", *calls)
	}
	if _, errb, code = runTool(t, dir, "-R", "-L", "--dereference", gid, "f"); code != 0 || errb != "" {
		t.Errorf("-R -L --dereference: code=%d err=%q", code, errb)
	}
	if _, errb, code = runTool(t, dir, "--dereference", gid, "f"); code != 0 || errb != "" {
		t.Errorf("--dereference: code=%d err=%q", code, errb)
	}
}

// POSIX has GROUP looked up as a name first, and read as a number only
// when no such group exists. The lookup is a seam because no test can
// add a group named "42" to the host.
func TestChgrpNameIsPreferredOverNumber(t *testing.T) {
	restore := lookupGroup
	lookupGroup = func(name string) (*user.Group, error) {
		if name == "42" {
			return &user.Group{Gid: "7"}, nil
		}
		return nil, errors.New("no such group")
	}
	t.Cleanup(func() { lookupGroup = restore })

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ spec, want string }{
		{"42", "chown f 7"},  // the group named "42"
		{"43", "chown f 43"}, // no such group: a numeric id
	} {
		calls := recordChanges(t)
		if _, errb, code := runTool(t, dir, tc.spec, "f"); code != 0 || errb != "" {
			t.Fatalf("chgrp %s: code=%d err=%q", tc.spec, code, errb)
		}
		if got := strings.Join(*calls, "; "); got != tc.want {
			t.Errorf("chgrp %s issued %q, want %q", tc.spec, got, tc.want)
		}
	}
}

// --reference supplies an id, not a spec to re-parse: a host holding a
// group whose name is the reference file's numeric id must not capture
// the change.
func TestChgrpReferenceIdIsNotLookedUpAsAName(t *testing.T) {
	restore := lookupGroup
	lookupGroup = func(string) (*user.Group, error) { return &user.Group{Gid: "77"}, nil }
	t.Cleanup(func() { lookupGroup = restore })

	dir := t.TempDir()
	for _, name := range []string{"ref", "f"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	calls := recordChanges(t)
	if _, errb, code := runTool(t, dir, "--reference=ref", "f"); code != 0 || errb != "" {
		t.Fatalf("chgrp --reference: code=%d err=%q", code, errb)
	}
	// The reference file's group is the one f already has, but POSIX still
	// requires the ownership operation to be performed.
	fi, err := os.Stat(filepath.Join(dir, "ref"))
	if err != nil {
		t.Fatal(err)
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		t.Fatal("reference stat did not expose syscall.Stat_t")
	}
	want := "chown f " + strconv.FormatUint(uint64(st.Gid), 10)
	if got := strings.Join(*calls, "; "); got != want {
		t.Errorf("--reference issued %q, want %q", got, want)
	}
}

// The corrupt-hierarchy diagnostic. Detection lives in the shared
// walker, which tests it against a substituted identity predicate; this
// pins the wording and the status the command reports for it.
func TestChgrpCycleDiagnostic(t *testing.T) {
	rc, out, errb := newContext(t)
	reportCycle(rc, "d/sub", chgrpOpts{})
	if out.Len() != 0 {
		t.Errorf("cycle warning went to stdout: %q", out.String())
	}
	want := "chgrp: WARNING: Circular directory structure.\n" +
		"This almost certainly means that you have a corrupted file system.\n" +
		"NOTIFY YOUR SYSTEM ADMINISTRATOR.\n" +
		"The following directory is part of the cycle:\n  d/sub\n"
	if errb.String() != want {
		t.Errorf("cycle warning=%q, want %q", errb.String(), want)
	}
	errb.Reset()
	reportCycle(rc, "d/sub", chgrpOpts{options: options{silent: true}})
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
func TestChgrpTraversalAndDereferenceAreOrthogonal(t *testing.T) {
	other := strconv.Itoa(atoi(t, currentGroup(t)) + 1)
	cases := []struct {
		name string
		args []string
		want string
	}{
		// -P reaches only the operand link, and cannot reach a
		// referent, so the link itself is what changes.
		{"-R -P", []string{"-R", "-P"}, "lchown toplink " + other},
		// -H follows the operand link for the traversal. The change
		// still defaults to the referent, for the operand link and for
		// the interior link the walk does not follow.
		{"-R -H", []string{"-R", "-H"},
			"chown link " + other + "; chown f " + other + "; chown sub " + other + "; chown toplink " + other},
		// -h moves every one of those changes onto the link, without
		// changing which files the walk reached.
		{"-R -H -h", []string{"-R", "-H", "-h"},
			"lchown link " + other + "; lchown f " + other + "; lchown sub " + other + "; lchown toplink " + other},
		// -L additionally follows the interior link, so d/sub is
		// reached twice — once through the link, once by its name.
		{"-R -L", []string{"-R", "-L"},
			"chown f " + other + "; chown link " + other + "; chown f " + other +
				"; chown sub " + other + "; chown toplink " + other},
		{"-R -L -h", []string{"-R", "-L", "-h"},
			"lchown f " + other + "; lchown link " + other + "; lchown f " + other +
				"; lchown sub " + other + "; lchown toplink " + other},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := linkTree(t)
			calls := recordChanges(t)
			_, errb, code := runTool(t, dir, append(append([]string{}, tc.args...), other, "toplink")...)
			if code != 0 || errb != "" {
				t.Fatalf("chgrp %v: code=%d err=%q", tc.args, code, errb)
			}
			if got := strings.Join(*calls, "; "); got != tc.want {
				t.Errorf("chgrp %v issued\n %q, want\n %q", tc.args, got, tc.want)
			}
		})
	}
}

// POSIX -H follows "a symbolic link named as an operand". An operand
// that resolves through more than one link is still one operand, so the
// whole chain is followed; -P follows none of it.
func TestChgrpCommandLineLinkChainIsFollowed(t *testing.T) {
	gid := currentGroup(t)
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "d", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "d", "sub", "f"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	symlink(t, "d", filepath.Join(dir, "hop"))
	symlink(t, "hop", filepath.Join(dir, "top"))

	out, errb, code := runTool(t, dir, "-v", "-R", "-H", gid, "top")
	if code != 0 || errb != "" {
		t.Fatalf("chgrp -R -H: code=%d err=%q", code, errb)
	}
	if got := visited(t, out); got != filepath.FromSlash("top/sub/f top/sub top") {
		t.Errorf("-H over a link chain reached %q", got)
	}
	out, errb, code = runTool(t, dir, "-v", "-R", "-P", gid, "top")
	if code != 0 || errb != "" {
		t.Fatalf("chgrp -R -P: code=%d err=%q", code, errb)
	}
	if got := visited(t, out); got != "top" {
		t.Errorf("-P over a link chain reached %q, want the operand alone", got)
	}
}

// The -v report names the group the file kept, which is not the group
// that was asked for when --from declined the change. Reporting the
// requested id there states something untrue about the file.
func TestChgrpVerboseNamesTheGroupTheFileKept(t *testing.T) {
	gid := currentGroup(t)
	other := strconv.Itoa(atoi(t, gid) + 1)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	calls := recordChanges(t)

	// The file's group is gid, so --from a different group matches
	// nothing and the file keeps gid.
	out, errb, code := runTool(t, dir, "-v", "--from=:"+other, other, "f")
	if code != 0 || errb != "" {
		t.Fatalf("chgrp --from: code=%d err=%q", code, errb)
	}
	if len(*calls) != 0 {
		t.Fatalf("--from mismatch still changed the file: %v", *calls)
	}
	if want := "group of 'f' retained as " + gid + "\n"; out != want {
		t.Errorf("verbose report = %q, want %q", out, want)
	}
}

type failingOutputWriter struct{ err error }

func (w failingOutputWriter) Write([]byte) (int, error) { return 0, w.err }

func TestChgrpOutputFailureSetsStatusAndContinues(t *testing.T) {
	gid := currentGroup(t)
	other := strconv.Itoa(atoi(t, gid) + 1)
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
			if got, want := errb.String(), "chgrp: write error: broken output\n"; got != want {
				t.Errorf("stderr=%q, want %q", got, want)
			}
			if len(*calls) != 2 {
				t.Errorf("ownership work stopped after output failure: calls=%v", *calls)
			}
		})
	}
}
