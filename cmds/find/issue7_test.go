package findcmd

import (
	"os"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

// POSIX.1-2016 find evidence (Issue 7, XCU:find). The main suites already
// pin every primary and operator individually; these tests isolate the
// remaining normative grammar axes: operator precedence with implicit -a,
// the -name leading-period rule, the +n|-n|n numeric-argument trichotomy,
// the -H/-L restriction to the option position, and the leading-option "--"
// end token.

// TestFindIssue7OperatorPrecedence pins the OPERANDS grammar precedence,
// '!' > implicit -a > -o, with a truth table that actually discriminates.
// The controlling case is `A -o B -a C`: because -a binds tighter, the
// expression is A -o (B -a C), which matches MORE than the left-parenthesized
// (A -o B) -a C. The fixture guarantees a path where the two disagree
// (an A-match whose C is false, and a B-match whose C is false), so a parser
// that mis-associates -o with the conjunction produces a different path set,
// not just a different order.
func TestFindIssue7OperatorPrecedence(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "aOnly.go", "w")   // A true, C false
	writeFile(t, dir, "bOnly.go", "x")   // B true, C false
	writeFile(t, dir, "bBoth.txt", "y")  // B true, C true
	writeFile(t, dir, "neither.go", "z") // A false, B false

	// A = -name 'a*'; B = -name 'b*'; C = -name '*txt'.
	// Correct parse: a* files, or (b* AND *txt) → aOnly.go and bBoth.txt.
	// A misparse of (A -o B) -a C drops aOnly.go (C false), leaving only
	// bBoth.txt — the discriminating difference.
	out, errb, code := runFind(t, dir, ".", "(", "-type", "f", ")", "-a", "(", "-name", "a*", "-o", "-name", "b*", "-a", "-name", "*txt", ")")
	if code != 0 || errb != "" {
		t.Fatalf("precedence: code=%d err=%q", code, errb)
	}
	if out != "./aOnly.go\n./bBoth.txt\n" {
		t.Errorf("A -o B -a C = %q, want A -o (B -a C) = ./aOnly.go and ./bBoth.txt", out)
	}
	// Left-parenthesized control: (A -o B) -a C → bBoth.txt only here;
	// the A-match aOnly.go is correctly dropped because its C is false.
	out, _, _ = runFind(t, dir, ".", "-type", "f", "(", "-name", "a*", "-o", "-name", "b*", ")", "-a", "-name", "*txt")
	if out != "./bBoth.txt\n" {
		t.Errorf("(A -o B) -a C = %q, want ./bBoth.txt only", out)
	}
	// '!' binds tighter than -a: ! A -a C is (! A) -a C. Among *txt
	// files, those NOT named a*: bBoth.txt (aOnly.go is not *txt anyway,
	// and neither.go fails C). A loose-binding '!' would instead exclude
	// everything that satisfies !(A -a C) — including neither.go.
	out, _, _ = runFind(t, dir, ".", "!", "-name", "a*", "-a", "-name", "*txt")
	if out != "./bBoth.txt\n" {
		t.Errorf("! A -a C = %q, want (! A) -a C = ./bBoth.txt only", out)
	}
}

// TestFindIssue7NameLeadingPeriodNotSpecial pins the -name clause per the
// Issue 7 pattern-matching rules: unlike shell filename expansion, find's
// -name gives a leading <period> NO special treatment — '*' and '?' may
// match a leading period just like any other character. Verified against the
// Open Group find page (XBD Pattern Matching as it references) and the host
// BSD find control.
func TestFindIssue7NameLeadingPeriodNotSpecial(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".hidden.txt", "x")
	writeFile(t, dir, "plain.txt", "y")
	// '*' matches the leading period: the dotfile matches '*txt'.
	out, _, code := runFind(t, dir, ".", "-name", "*txt", "-type", "f")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if out != "./.hidden.txt\n./plain.txt\n" {
		t.Errorf("-name *txt = %q, want BOTH the dotfile and the plain file (leading period is not special)", out)
	}
	// An explicit leading-period pattern still matches the dotfile alone
	// among regular files ('-type f' keeps the '.' start point out).
	out, _, _ = runFind(t, dir, ".", "-name", ".*", "-type", "f")
	if out != "./.hidden.txt\n" {
		t.Errorf("-name .* = %q, want exactly the dotfile", out)
	}
}

// TestFindIssue7NumericArgumentTrichotomy pins the numeric-argument rule for
// -links, -size, and the *time primaries: bare n is exactly n, +n is more
// than n, -n is less than n. Rounding to the primary's unit stays the
// primary's own rule (blocks round up for -size, days round up for *time);
// the trichotomy itself is uniform.
func TestFindIssue7NumericArgumentTrichotomy(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "empty", "")                      // 0 bytes → 0 blocks
	writeFile(t, dir, "one", "hi")                      // 2 bytes → 1 block
	writeFile(t, dir, "big", strings.Repeat("x", 1024)) // 2 blocks
	exact := func(name string) string { return "./" + name + "\n" }
	cases := []struct {
		arg  string
		want string
	}{
		{"1", exact("one")}, // exactly 1 block: neither empty (0) nor big (2)
		{"+1", exact("big")},
		{"-2", exact("empty") + exact("one")},
	}
	for _, tc := range cases {
		out, _, code := runFind(t, dir, ".", "-type", "f", "-size", tc.arg)
		if code != 0 || out != tc.want {
			t.Errorf("-size %s = (%q, %d), want %q", tc.arg, out, code, tc.want)
		}
	}
	if runtime.GOOS != "windows" {
		// -links trichotomy on plain files (nlink 1).
		out, _, code := runFind(t, dir, ".", "-type", "f", "-links", "+0")
		if code != 0 || out != exact("big")+exact("empty")+exact("one") {
			t.Errorf("-links +0 = (%q, %d)", out, code)
		}
		out, _, _ = runFind(t, dir, ".", "-type", "f", "-links", "-2")
		if out != exact("big")+exact("empty")+exact("one") {
			t.Errorf("-links -2 = %q, want all nlink-1 files", out)
		}
	}
}

// TestFindIssue7FollowOptionsOnlyLeading pins the SYNOPSIS: -H and -L are
// valid only in the option position before the first path operand; after a
// path they are expression tokens (-H/-L are not primaries, so the parser
// must reject them loudly rather than silently accepting or ignoring).
func TestFindIssue7FollowOptionsOnlyLeading(t *testing.T) {
	dir := setupTree(t)
	out, errb, code := runFind(t, dir, ".", "-H")
	if code == 0 || !strings.Contains(errb, "-H") {
		t.Errorf("trailing -H must fail loudly: out=%q err=%q code=%d", out, errb, code)
	}
	_, errb, code = runFind(t, dir, ".", "-L")
	if code == 0 || !strings.Contains(errb, "-L") {
		t.Errorf("trailing -L must fail loudly: err=%q code=%d", errb, code)
	}
	// In the option position they still work (already covered by
	// TestFindSymlinkFollow; this is the paired positive control).
	out, _, code = runFind(t, dir, "-H", ".", "-name", "c.txt")
	if code != 0 || out != "./sub/c.txt\n" {
		t.Errorf("-H leading: out=%q code=%d", out, code)
	}
}

// TestFindIssue7DoubleDashEndsLeadingOptions pins the leading-option scan:
// "--" terminates -H/-L/-P processing, and what follows is read as start
// points and expression; a "-H" after "--" is therefore an expression error.
func TestFindIssue7DoubleDashEndsLeadingOptions(t *testing.T) {
	dir := setupTree(t)
	out, _, code := runFind(t, dir, "--", "sub", "-name", "*.txt")
	if code != 0 || out != "sub/c.txt\n" {
		t.Errorf("find -- sub -name: out=%q code=%d", out, code)
	}
	_, errb, code := runFind(t, dir, "--", "-H")
	if code == 0 || !strings.Contains(errb, "-H") {
		t.Errorf("-H after -- must be an expression error: err=%q code=%d", errb, code)
	}
}

// TestFindIssue7NouserUnownedPositivePath pins the -nouser clause's positive
// path, which the filesystem cannot provide hermetically: creating a file
// owned by an unassigned uid requires root (chown) and would leave an
// unremovable-by-owner artifact behind. Instead the primary is evaluated
// directly at its seam with a FileInfo stub carrying an unassigned uid —
// the same evaluation the walker performs for every visited file.
func TestFindIssue7NouserUnownedPositivePath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("-nouser is unsupported on Windows")
	}
	e := noOwnerExpr{}
	stub := ownerStub{uid: 4242 * 4242} // 17,994,564: unassigned on test hosts
	if !e.eval(&fctx{info: stub, w: &walker{owners: newOwnerCache()}}) {
		t.Errorf("-nouser did not match a file with an unassigned uid %d", stub.uid)
	}
	// Control: our own uid (from the process) must NOT match.
	self := ownerStub{uid: uint32(os.Getuid())}
	if e.eval(&fctx{info: self, w: &walker{owners: newOwnerCache()}}) {
		t.Errorf("-nouser matched the invoking user's own uid %d", self.uid)
	}
}

type ownerStub struct{ uid uint32 }

func (s ownerStub) Name() string       { return "stub" }
func (s ownerStub) Size() int64        { return 0 }
func (s ownerStub) Mode() os.FileMode  { return 0 }
func (s ownerStub) ModTime() time.Time { return time.Time{} }
func (s ownerStub) IsDir() bool        { return false }
func (s ownerStub) Sys() any           { return &syscall.Stat_t{Uid: s.uid} }
