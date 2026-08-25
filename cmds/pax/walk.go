package paxcmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

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
// pathname, spelled from the operand exactly as the caller wrote it (POSIX
// stores the pathname "as specified", so "./src" stays "./src", never a
// cleaned relative form). abs is the filesystem path to read from. fi is the
// lstat result, or the stat result when the follow policy resolved a symlink.
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
	diagnosed bool // a cycle was reported on stderr; exit must be nonzero
}

// walkOperand traverses one command-line file operand under the -H/-L/-X/-d
// policy in effect and calls fn once per source file, in deterministic
// (lexical) order. It is the single traversal shared by the tar writer, the
// cpio writer, and copy mode. The returned bool reports whether a diagnostic
// was already written (a filesystem cycle); the error is fatal for this
// operand and has not been printed.
func walkOperand(rc *tool.RunContext, o *options, name string, fn func(walkEntry) error) (bool, error) {
	full := resolve(rc, name)
	fi, err := os.Lstat(full)
	if err != nil {
		return false, err
	}
	// -H and -L both resolve a symlink named on the command line.
	if fi.Mode()&os.ModeSymlink != 0 && o.follow != followNone {
		if fi, err = os.Stat(full); err != nil {
			return false, err
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
	err = w.walk(filepath.ToSlash(name), full, fi)
	return w.diagnosed, err
}

func (w *walker) walk(member, abs string, fi os.FileInfo) error {
	if err := w.fn(walkEntry{member: member, abs: abs, fi: fi}); err != nil {
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
	// requires the loop be diagnosed and the exit status be nonzero; siblings
	// keep archiving.
	anc := walkAncestor{}
	if id := identityOf(fi); id.ok {
		anc.id, anc.idOK = id.key(), true
	} else if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		anc.resolved = resolved
	}
	for _, a := range w.ancestors {
		if (anc.idOK && a.idOK && anc.id == a.id) ||
			(!anc.idOK && !a.idOK && anc.resolved != "" && anc.resolved == a.resolved) {
			fmt.Fprintf(w.rc.Err, "pax: %s: filesystem cycle detected; directory not descended\n", member)
			w.diagnosed = true
			return nil
		}
	}
	w.ancestors = append(w.ancestors, anc)
	defer func() { w.ancestors = w.ancestors[:len(w.ancestors)-1] }()

	entries, err := os.ReadDir(abs) // sorted by name: deterministic member order
	if err != nil {
		return err
	}
	base := strings.TrimRight(member, "/")
	for _, e := range entries {
		childAbs := filepath.Join(abs, e.Name())
		cfi, err := e.Info() // lstat semantics
		if err != nil {
			return err
		}
		// Only -L resolves symlinks encountered below an operand; under -H
		// they are archived as the symlinks they are.
		if cfi.Mode()&os.ModeSymlink != 0 && w.o.follow == followAll {
			if cfi, err = os.Stat(childAbs); err != nil {
				return err
			}
		}
		if err := w.walk(base+"/"+e.Name(), childAbs, cfi); err != nil {
			return err
		}
	}
	return nil
}
