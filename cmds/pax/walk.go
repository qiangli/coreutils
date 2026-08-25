package paxcmd

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

// errTraversalCycle is returned after the required diagnostic has already
// been written. POSIX requires pax to terminate when an ancestor loop is
// detected, so callers must stop processing subsequent operands while still
// closing any partially written archive cleanly.
var errTraversalCycle = errors.New("filesystem cycle detected")

// sourceTraversalError distinguishes a source-side lookup/read failure from
// an archive-stream write failure. Copy mode must diagnose the former and
// continue with later operands while keeping the pipe a valid archive; a sink
// failure cannot be recovered that way.
type sourceTraversalError struct{ err error }

func (e *sourceTraversalError) Error() string { return e.err.Error() }
func (e *sourceTraversalError) Unwrap() error { return e.err }

func sourceTraversalFailure(err error) bool {
	var target *sourceTraversalError
	return errors.As(err, &target)
}

func sourceTraversalErr(err error) error {
	if err == nil {
		return nil
	}
	return &sourceTraversalError{err: err}
}

// followMode is the POSIX pax symlink traversal policy. The default is
// physical: symbolic links are archived as symbolic links. -H resolves only
// symlinks named as command-line file operands; -L resolves every symlink,
// named or encountered. The last of repeated -H/-L wins.
type followMode int

const (
	followNone    followMode = iota
	followCmdline            // -H
	followAll                // -L
)

// walkEntry is one source file surfaced by the walker. member is the archive
// pathname. Safe relative spelling is preserved (so "./src" stays "./src");
// unsafe absolute or parent-escaping roots use their basename so the archive
// remains consumable by the fail-closed reader. abs is the filesystem path to
// read from. fi is the lstat result, or the stat result when following a link.
type walkEntry struct {
	member string
	abs    string
	fi     os.FileInfo
}

// deviceOf is the -X device-identity seam. It reports the st_dev of an
// already-statted file, and false where the platform exposes no device
// identity — in which case -X must fail loudly rather than silently archive
// across mount points. Tests substitute it to simulate crossing a device
// boundary without needing a second filesystem.
var deviceOf = func(abs string, fi os.FileInfo) (uint64, bool) {
	id := identityOf(fi)
	return id.dev, id.ok
}

// walkAncestor identifies one directory on the current descent path, for
// ancestor-only cycle detection. Identity is dev/ino where the platform has
// it, else the symlink-resolved absolute path.
type walkAncestor struct {
	id       devIno
	idOK     bool
	resolved string
}

type walker struct {
	rc *tool.RunContext
	o  *options
	fn func(walkEntry) error

	rootDev   uint64
	ancestors []walkAncestor
	diagnosed bool // a source traversal error was reported; exit must be nonzero
}

// walkOperand traverses one command-line file operand under the -H/-L/-X/-d
// policy in effect and calls fn once per source file, in deterministic
// (lexical) order. It is the single traversal shared by the tar writer, the
// cpio writer, and copy mode. The returned bool reports whether a diagnostic
// was already written; the error is fatal for this operand and has not been
// printed, except errTraversalCycle whose required diagnostic is already done.
func walkOperand(rc *tool.RunContext, o *options, name string, fn func(walkEntry) error) (bool, error) {
	full := resolve(rc, name)
	fi, err := os.Lstat(full)
	if err != nil {
		return false, sourceTraversalErr(err)
	}
	// -H and -L both resolve a symlink named on the command line.
	if fi.Mode()&os.ModeSymlink != 0 && o.follow != followNone {
		if fi, err = os.Stat(full); err != nil {
			return false, sourceTraversalErr(err)
		}
	}
	w := &walker{rc: rc, o: o, fn: fn}
	if o.X {
		dev, ok := deviceOf(full, fi)
		if !ok {
			return false, fmt.Errorf("-X: cannot determine the device of %s on this platform", name)
		}
		w.rootDev = dev
	}
	err = w.walk(archiveMemberRoot(name, full), full, fi)
	return w.diagnosed, err
}

// archiveMemberRoot preserves safe relative operand spelling (including a
// leading "./"), but retains the historical basename behavior for absolute
// or parent-escaping operands. That keeps write-mode archives consumable by
// our fail-closed extractor and makes copy mode a genuine write-then-read
// operation instead of generating a member it must reject as unsafe.
func archiveMemberRoot(name, full string) string {
	slashed := filepath.ToSlash(name)
	clean := path.Clean(slashed)
	unsafe := filepath.IsAbs(name) || filepath.VolumeName(name) != "" ||
		clean == ".." || strings.HasPrefix(clean, "../")
	if !unsafe {
		return slashed
	}
	base := filepath.Base(filepath.Clean(full))
	if base == "." || base == string(filepath.Separator) || base == "" {
		return "."
	}
	return filepath.ToSlash(base)
}

func (w *walker) walk(member, abs string, fi os.FileInfo) error {
	if err := w.fn(walkEntry{member: member, abs: abs, fi: fi}); err != nil {
		if sourceTraversalFailure(err) {
			fmt.Fprintf(w.rc.Err, "pax: %s: %v\n", member, err)
			w.diagnosed = true
			return nil
		}
		return err
	}
	if !fi.IsDir() || w.o.dirsNoDescend {
		return nil
	}
	// -X: the directory on the other device is itself archived (it was
	// encountered on the operand's device), but never descended — the same
	// boundary fts(3) FTS_XDEV draws for the BSD pax this option comes from.
	if w.o.X {
		dev, ok := deviceOf(abs, fi)
		if !ok {
			return fmt.Errorf("-X: cannot determine the device of %s on this platform", member)
		}
		if dev != w.rootDev {
			return nil
		}
	}
	// Ancestor-only cycle detection: descending into a directory that is
	// already on the current descent path (reachable only through a followed
	// symlink or an equivalent mount trick) would never terminate. POSIX
	// requires the loop be diagnosed and the pax invocation to terminate.
	anc := walkAncestor{}
	if id := identityOf(fi); id.ok {
		anc.id, anc.idOK = id.key(), true
	} else if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		anc.resolved = resolved
	}
	for _, a := range w.ancestors {
		if (anc.idOK && a.idOK && anc.id == a.id) ||
			(!anc.idOK && !a.idOK && anc.resolved != "" && anc.resolved == a.resolved) {
			fmt.Fprintf(w.rc.Err, "pax: %s: filesystem cycle detected; terminating\n", member)
			w.diagnosed = true
			return errTraversalCycle
		}
	}
	w.ancestors = append(w.ancestors, anc)
	defer func() { w.ancestors = w.ancestors[:len(w.ancestors)-1] }()

	entries, err := os.ReadDir(abs) // sorted by name: deterministic member order
	if err != nil {
		fmt.Fprintf(w.rc.Err, "pax: %s: %v\n", member, err)
		w.diagnosed = true
		return nil
	}
	base := strings.TrimRight(member, "/")
	for _, e := range entries {
		childAbs := filepath.Join(abs, e.Name())
		cfi, err := e.Info() // lstat semantics
		if err != nil {
			fmt.Fprintf(w.rc.Err, "pax: %s/%s: %v\n", base, e.Name(), err)
			w.diagnosed = true
			continue
		}
		// Only -L resolves symlinks encountered below an operand; under -H
		// they are archived as the symlinks they are.
		if cfi.Mode()&os.ModeSymlink != 0 && w.o.follow == followAll {
			if cfi, err = os.Stat(childAbs); err != nil {
				fmt.Fprintf(w.rc.Err, "pax: %s/%s: %v\n", base, e.Name(), err)
				w.diagnosed = true
				continue
			}
		}
		if err := w.walk(base+"/"+e.Name(), childAbs, cfi); err != nil {
			return err
		}
	}
	return nil
}
