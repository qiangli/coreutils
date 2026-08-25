//go:build unix

package findcmd

// Issue 742 / Sprint 79 Unix-only POSIX Issue 7 evidence for find: the
// ownership seam (-nouser and, non-skipped, -nogroup against a really
// unassigned id; -user/-group by name and by decimal id with no name),
// the -newer missing-reference diagnostic, and traversal/read failures
// (an unreadable directory is diagnosed, the walk continues, the run
// exits non-zero). Unassigned ids are chosen dynamically per host so no
// fixed id that happens to resolve somewhere can void the fixture; the
// privileged real-chown fixture stays as supplemental evidence.

import (
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// unassignedID returns a numeric id in the high range for which no user
// (or group) name resolves on this host, so a positive -nouser/
// -nogroup fixture is deterministic everywhere. ok is false when every
// candidate resolves (a pathological host); callers skip.
func unassignedID(group bool) (uint32, bool) {
	oc := newOwnerCache()
	for id := uint32(60000); id < 65535; id++ {
		if !oc.nameExists(id, group) {
			return id, true
		}
	}
	return 0, false
}

// TestFindIssue7NumericUserAndGroupOperandsUseTheIDWhenNoNameExists
// pins the -user/-group lookup rule hermetically: a decimal operand for
// which no user/group name exists is accepted and compared numerically
// (name lookup first, decimal id fallback), so it is never a usage
// error — it simply matches nothing in an unprivileged fixture.
func TestFindIssue7NumericUserAndGroupOperandsUseTheIDWhenNoNameExists(t *testing.T) {
	uid, ok := unassignedID(false)
	if !ok {
		t.Skip("no unassigned uid available on this host")
	}
	gid, ok := unassignedID(true)
	if !ok {
		t.Skip("no unassigned gid available on this host")
	}
	dir := setupTree(t)
	for _, tc := range []struct {
		flag string
		id   string
	}{
		{"-user", strconv.FormatUint(uint64(uid), 10)},
		{"-group", strconv.FormatUint(uint64(gid), 10)},
	} {
		out, errb, code := runFind(t, dir, ".", tc.flag, tc.id)
		if code != 0 || errb != "" || out != "" {
			t.Errorf("%s %s: (%q, %q, %d), want exit 0, no matches, no error",
				tc.flag, tc.id, out, errb, code)
		}
	}
	// The same spelling that names no one is still a valid filter:
	// negated, it selects every file.
	out, _, code := runFind(t, dir, ".", "!", "-user", strconv.FormatUint(uint64(uid), 10), "-type", "f")
	if code != 0 || out == "" {
		t.Errorf("! -user %d: (%q, %d), want the regular files", uid, out, code)
	}
}

// TestFindIssue7NamedGroupOperand is the named -group operand product:
// a real walk over a fresh fixture selects every file by the invoking
// group's name (the name-first lookup path), and -nogroup matches
// nothing there because that name resolves.
func TestFindIssue7NamedGroupOperand(t *testing.T) {
	me, err := user.Current()
	if err != nil || me.Gid == "" {
		t.Skipf("no current group: %v", err)
	}
	g, err := user.LookupGroupId(me.Gid)
	if err != nil {
		t.Skipf("group %s has no name on this host: %v", me.Gid, err)
	}
	dir := setupTree(t)
	all, _, _ := runFind(t, dir, ".")
	out, errb, code := runFind(t, dir, ".", "-group", g.Name)
	if code != 0 || errb != "" || out != all {
		t.Errorf("-group %s: (%q, %q, %d), want every file via the name lookup (all=%q)", g.Name, out, errb, code, all)
	}
	out, _, code = runFind(t, dir, ".", "-nogroup")
	if code != 0 || out != "" {
		t.Errorf("-nogroup with resolvable group: (%q, %d), want no matches", out, code)
	}
}

// TestFindIssue7OwnershipSeamUnassignedOwnerAndGroup is the positive
// -nouser/-nogroup seam evidence, deterministic on every host: the
// primaries are evaluated at the same Unix stat seam the walker uses
// for every visited file, with FileInfo stubs carrying an unassigned
// uid and gid (chosen dynamically so the "unassigned" claim holds
// wherever the test runs). The privileged real-file fixture below is
// the supplemental end-to-end counterpart.
func TestFindIssue7OwnershipSeamUnassignedOwnerAndGroup(t *testing.T) {
	uid, ok := unassignedID(false)
	if !ok {
		t.Skip("no unassigned uid available on this host")
	}
	gid, ok := unassignedID(true)
	if !ok {
		t.Skip("no unassigned gid available on this host")
	}
	w := &walker{owners: newOwnerCache()}

	// Positive paths: an unassigned uid matches -nouser, an unassigned
	// gid matches -nogroup (each stub carries both the id under test
	// and a resolvable counterpart, so the other primary stays false).
	self := uint32(os.Getuid())
	selfGID := uint32(os.Getgid())
	cases := []struct {
		name      string
		stub      statOwnerStub
		userWant  bool // -nouser
		groupWant bool // -nogroup
	}{
		{"unassigned uid", statOwnerStub{uid: uid, gid: selfGID}, true, false},
		{"unassigned gid", statOwnerStub{uid: self, gid: gid}, false, true},
		{"both unassigned", statOwnerStub{uid: uid, gid: gid}, true, true},
		{"both resolvable", statOwnerStub{uid: self, gid: selfGID}, false, false},
	}
	for _, tc := range cases {
		c := &fctx{info: tc.stub, w: w}
		if got := (&noOwnerExpr{}).eval(c); got != tc.userWant {
			t.Errorf("%s: -nouser = %v, want %v", tc.name, got, tc.userWant)
		}
		if got := (&noOwnerExpr{group: true}).eval(c); got != tc.groupWant {
			t.Errorf("%s: -nogroup = %v, want %v", tc.name, got, tc.groupWant)
		}
	}

	// The -user/-group numeric comparison rides the same seam: the
	// unassigned ids are selectable by their decimal spelling.
	if !(&ownerExpr{id: uid}).eval(&fctx{info: statOwnerStub{uid: uid, gid: selfGID}, w: w}) {
		t.Errorf("-user %d did not match the stub carrying that uid", uid)
	}
	if !(&ownerExpr{id: gid, group: true}).eval(&fctx{info: statOwnerStub{uid: self, gid: gid}, w: w}) {
		t.Errorf("-group %d did not match the stub carrying that gid", gid)
	}
}

// statOwnerStub is a FileInfo whose Sys() is a real syscall.Stat_t carrying
// the given uid/gid — the exact structure the -user/-group/-nouser/
// -nogroup primaries read on Unix.
type statOwnerStub struct{ uid, gid uint32 }

func (s statOwnerStub) Name() string       { return "stub" }
func (s statOwnerStub) Size() int64        { return 0 }
func (s statOwnerStub) Mode() os.FileMode  { return 0 }
func (s statOwnerStub) ModTime() time.Time { return time.Time{} }
func (s statOwnerStub) IsDir() bool        { return false }
func (s statOwnerStub) Sys() any {
	return &syscall.Stat_t{Uid: s.uid, Gid: s.gid}
}

// TestFindIssue7NewerMissingReference pins the -newer INPUT_FILES
// failure product: a reference operand that cannot be stat'ed is a
// usage-time diagnostic naming the operand, nothing is walked, and the
// status is 2 (syntax/usage class per the repo's documented deviation
// from GNU's 1).
func TestFindIssue7NewerMissingReference(t *testing.T) {
	dir := setupTree(t)
	out, errb, code := runFind(t, dir, ".", "-newer", "no-such-reference")
	if code != 2 || out != "" || !strings.Contains(errb, "no-such-reference") {
		t.Errorf("-newer missing reference: (%q, %q, %d), want a diagnostic naming the operand, exit 2", out, errb, code)
	}
}

// TestFindIssue7RealOwnershipSeamUnassignedOwner is the supplemental
// privileged fixture: with root the file is really chowned to
// dynamically chosen unassigned ids, and -nouser/-nogroup select
// exactly it end to end. Skipped unprivileged.
func TestFindIssue7RealOwnershipSeamUnassignedOwner(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("chown to an unassigned uid requires root")
	}
	uid, ok := unassignedID(false)
	if !ok {
		t.Skip("no unassigned uid available on this host")
	}
	gid, ok := unassignedID(true)
	if !ok {
		t.Skip("no unassigned gid available on this host")
	}
	dir := t.TempDir()
	writeFile(t, dir, "mine.txt", "x")
	writeFile(t, dir, "orphan.txt", "x")
	if err := os.Chown(filepath.Join(dir, "orphan.txt"), int(uid), int(gid)); err != nil {
		t.Skipf("chown unavailable: %v", err)
	}

	out, errb, code := runFind(t, dir, ".", "-type", "f", "-nouser")
	if code != 0 || errb != "" || out != "./orphan.txt\n" {
		t.Errorf("-nouser: (%q, %q, %d), want exactly ./orphan.txt", out, errb, code)
	}
	out, _, code = runFind(t, dir, ".", "-type", "f", "-nogroup")
	if code != 0 || out != "./orphan.txt\n" {
		t.Errorf("-nogroup: (%q, %d), want exactly ./orphan.txt", out, code)
	}
	// Decimal ids with no name still select the file (numeric fallback).
	out, _, code = runFind(t, dir, ".", "-type", "f", "-user", strconv.FormatUint(uint64(uid), 10))
	if code != 0 || out != "./orphan.txt\n" {
		t.Errorf("-user %d: (%q, %d), want ./orphan.txt", uid, out, code)
	}
	out, _, code = runFind(t, dir, ".", "-type", "f", "-group", strconv.FormatUint(uint64(gid), 10))
	if code != 0 || out != "./orphan.txt\n" {
		t.Errorf("-group %d: (%q, %d), want ./orphan.txt", gid, out, code)
	}
	// Control: root's own file never matches the unowned primaries.
	out, _, _ = runFind(t, dir, ".", "-type", "f", "!", "-nouser", "!", "-nogroup")
	if out != "./mine.txt\n" {
		t.Errorf("! -nouser ! -nogroup: %q, want ./mine.txt", out)
	}
}

// TestFindIssue7UnreadableDirectoryTraversal pins the
// CONSEQUENCES_OF_ERRORS clause for a directory the process cannot
// read: the failure is diagnosed on stderr naming the directory, its
// contents are not visited, every other path is still evaluated in
// preorder, and the run exits non-zero — and an unreadable start point
// does not stop later operands from being processed.
func TestFindIssue7UnreadableDirectoryTraversal(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads through mode-0000 directories")
	}
	dir := setupTree(t)
	locked := filepath.Join(dir, "locked")
	writeFile(t, dir, "locked/hidden.txt", "x")
	if err := os.Chmod(locked, 0o0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(locked, 0o755) }) // let TempDir cleanup remove it

	out, errb, code := runFind(t, dir, ".", "-name", "*.txt")
	want := "./a.txt\n./empty.txt\n./skipme/e.txt\n./sub/c.txt\n"
	if out != want {
		t.Errorf("unreadable directory walk: out=%q, want %q (hidden.txt not visited)", out, want)
	}
	if code != 1 || !strings.Contains(errb, "locked") {
		t.Errorf("unreadable directory: code=%d err=%q, want exit 1 and a diagnostic naming locked", code, errb)
	}

	// The same failure class for a start point operand, aggregated with
	// a good later operand (EXIT_STATUS: >0 when a start point cannot
	// be descended; later operands continue).
	out, errb, code = runFind(t, dir, "locked", "sub", "-name", "c.txt")
	if code != 1 || out != "sub/c.txt\n" || !strings.Contains(errb, "locked") {
		t.Errorf("unreadable start point: (%q, %q, %d), want sub/c.txt, diagnostic, exit 1", out, errb, code)
	}
}
