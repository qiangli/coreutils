//go:build unix

package findcmd

import (
	"os"
	"syscall"
	"testing"
	"time"
)

// TestFindIssue7NouserUnownedPositivePath pins the -nouser clause's positive
// path, which the filesystem cannot provide hermetically: creating a file
// owned by an unassigned uid requires root. Instead the primary is evaluated
// directly at its Unix stat seam with a FileInfo stub carrying an unassigned
// uid — the same evaluation the walker performs for every visited file.
func TestFindIssue7NouserUnownedPositivePath(t *testing.T) {
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
