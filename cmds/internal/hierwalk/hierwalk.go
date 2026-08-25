// Package hierwalk walks a file hierarchy with the POSIX.1 -H/-L/-P
// symbolic-link semantics shared by the ownership commands (chown,
// chgrp).
//
// POSIX specifies three traversal modes for -R, and they differ only in
// which symbolic links are followed while walking:
//
//	-P  follow no symbolic link (the default)
//	-H  follow a symbolic link named as an operand, nothing below it
//	-L  follow every symbolic link to a directory
//
// The walker owns the traversal decisions only. What a command does to
// each file it is handed — chown or lchown, which ids, which diagnostic
// — stays with the command, because that is where POSIX splits -h
// (which file the change lands on) from -H/-L/-P (which files are
// reached at all).
//
// Directories are handed to Visit in post-order under -R, matching GNU
// coreutils: a directory whose ownership has just been given away may no
// longer be readable, so its contents are processed first. Entries
// within a directory are visited in sorted name order, which keeps
// output deterministic per the repository's agent contract.
package hierwalk

import (
	"os"
	"path/filepath"
)

// sameFile is the directory-identity predicate the cycle check uses. It
// is a variable because a hierarchy that is cyclic without a symbolic
// link needs hard-linked directories, which no unprivileged test can
// create; substituting the predicate exercises the walk's handling of
// such a hierarchy without pretending to build one.
var sameFile = os.SameFile

// Mode selects which symbolic links a recursive walk follows.
type Mode int

const (
	// Physical is -P: no symbolic link is followed. It is the default
	// for -R and the only mode that applies without it.
	Physical Mode = iota
	// CommandLine is -H: a symbolic link named as an operand is
	// followed; links encountered below it are not.
	CommandLine
	// Logical is -L: every symbolic link is followed.
	Logical
)

// Walker walks one operand hierarchy. Every callback is optional except
// Visit; a nil callback drops the event.
//
// Each callback receives both the filesystem path (rooted at the path
// the command resolved through its RunContext) and the display path
// (the same file spelled relative to the operand as the user wrote it),
// so diagnostics never leak the shell's working directory.
type Walker struct {
	// Mode is the symbolic-link traversal mode. It is consulted only
	// when Recursive is set.
	Mode Mode
	// Recursive enables descent into directories (-R).
	Recursive bool

	// Visit is called once for every file in the hierarchy. isLink
	// reports that lstat(path) is a symbolic link — including a link
	// that was followed as a directory, since -h still decides whether
	// the change lands on the link or its referent. followed reports
	// that this entry was a symbolic link the traversal mode resolved,
	// so the entry stands for its referent: chown and chgrp let -h
	// override that per file, but chmod has no portable operation that
	// changes a link's own mode bits, so followed is the only thing
	// that makes a reached link actionable at all.
	Visit func(path, display string, isLink, followed bool)
	// StatError reports that the entry could not be lstat'ed.
	StatError func(path, display string, err error)
	// ReadError reports that a directory's entries could not be read.
	// The directory itself is still visited afterwards.
	ReadError func(path, display string, err error)
	// Cycle reports a directory that is its own ancestor and so was not
	// descended into. It is called only for the physical and
	// command-line modes, where a cycle means a corrupt hierarchy
	// rather than an ordinary symbolic link back up the tree; under
	// -L the directory is handed to Visit instead and the walk simply
	// stops descending.
	Cycle func(path, display string)
	// EnterDir, when non-nil, is consulted before descending into a
	// directory below the operand; returning false skips that subtree
	// without visiting the directory. followed reports that the
	// directory was reached by following a symbolic link.
	EnterDir func(path, display string, followed bool) bool
}

// Walk walks the hierarchy rooted at path, which the caller spells as
// display in its diagnostics.
func (w *Walker) Walk(path, display string) {
	w.walk(path, display, nil, true)
}

func (w *Walker) walk(path, display string, ancestors []os.FileInfo, isOperand bool) {
	li, err := os.Lstat(path)
	if err != nil {
		w.statError(path, display, err)
		return
	}
	isLink := li.Mode()&os.ModeSymlink != 0

	// A link is followed only where the traversal mode says so. A link
	// whose referent cannot be stat'ed (a dangling link) is handed to
	// Visit as an ordinary entry: reporting it belongs to the command,
	// which alone knows whether -h makes the dangling referent
	// irrelevant.
	info, followed := li, false
	if w.Recursive && isLink && w.follows(isOperand) {
		if ti, terr := os.Stat(path); terr == nil {
			info, followed = ti, true
		}
	}

	if !w.Recursive || !info.IsDir() {
		w.visit(path, display, isLink, followed)
		return
	}

	for _, ancestor := range ancestors {
		if !sameFile(ancestor, info) {
			continue
		}
		// Under -L a directory reached again through a symbolic link is
		// an ordinary hierarchy; the walk stops descending and the
		// command still gets the entry. Otherwise the hierarchy itself
		// is cyclic, which is a defect the command must report.
		if w.Mode == Logical {
			w.visit(path, display, isLink, followed)
		} else if w.Cycle != nil {
			w.Cycle(path, display)
		}
		return
	}

	if !isOperand && w.EnterDir != nil && !w.EnterDir(path, display, followed) {
		return
	}

	entries, rerr := os.ReadDir(path)
	if rerr != nil && w.ReadError != nil {
		w.ReadError(path, display, rerr)
	}
	ancestors = append(ancestors, info)
	for _, entry := range entries {
		w.walk(filepath.Join(path, entry.Name()), filepath.Join(display, entry.Name()), ancestors, false)
	}
	w.visit(path, display, isLink, followed)
}

// follows reports whether a symbolic link reached at this position is
// traversed.
func (w *Walker) follows(isOperand bool) bool {
	switch w.Mode {
	case Logical:
		return true
	case CommandLine:
		return isOperand
	default:
		return false
	}
}

func (w *Walker) visit(path, display string, isLink, followed bool) {
	if w.Visit != nil {
		w.Visit(path, display, isLink, followed)
	}
}

func (w *Walker) statError(path, display string, err error) {
	if w.StatError != nil {
		w.StatError(path, display, err)
	}
}
