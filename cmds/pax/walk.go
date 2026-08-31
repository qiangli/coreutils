package paxcmd

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

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
	member   string
	abs      string
	fi       os.FileInfo
	followed bool // abs names a symlink whose referent supplied fi
}

var (
	sourceAccessTimeFn   = sourceAccessTime
	restoreSourceTimesFn = restoreSourceTimes
)

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
	if err != nil && rootRelativeSource(rc, name, full) {
		if rooted, rootErr := openRunRoot(rc); rootErr == nil {
			fi, err = rooted.Lstat(filepath.FromSlash(name))
			_ = rooted.Close()
		}
	}
	if err != nil {
		return false, sourceTraversalErr(err)
	}
	// -H and -L both resolve a symlink named on the command line.
	followed := false
	if fi.Mode()&os.ModeSymlink != 0 && o.follow != followNone {
		if fi, err = statSourcePath(rc, name, full); err != nil {
			return false, sourceTraversalErr(err)
		}
		followed = true
	}
	w := &walker{rc: rc, o: o, fn: fn}
	if o.X {
		dev, ok := deviceOf(full, fi)
		if !ok {
			return false, fmt.Errorf("-X: cannot determine the device of %s on this platform", name)
		}
		w.rootDev = dev
	}
	err = w.walk(archiveMemberRoot(name, full), full, fi, followed)
	return w.diagnosed, err
}

func safeRootRelative(name string) bool {
	clean := path.Clean(filepath.ToSlash(name))
	return !filepath.IsAbs(name) && filepath.VolumeName(name) == "" &&
		clean != ".." && !strings.HasPrefix(clean, "../")
}

// rootRelativeSource reports whether member is the run-directory-relative
// spelling of abs, and so whether a call that failed on abs may be retried
// through a handle on the run directory. The check is not redundant with
// safeRootRelative: archiveMemberRoot reduces an absolute or parent-escaping
// operand to its basename, which *looks* safely relative but names a
// different file under rc.Dir. Retrying that would silently archive the
// wrong file.
func rootRelativeSource(rc *tool.RunContext, member, abs string) bool {
	return safeRootRelative(member) && resolve(rc, filepath.FromSlash(member)) == abs
}

func openRunRoot(rc *tool.RunContext) (*os.Root, error) {
	base := rc.Dir
	if base == "" {
		var err error
		base, err = os.Getwd()
		if err != nil {
			return nil, err
		}
	}
	return os.OpenRoot(base)
}

// memberComponents splits an archive member into pathname components, the
// unit every root-relative retry works in.
func memberComponents(member string) []string {
	return strings.FieldsFunc(filepath.ToSlash(member), func(r rune) bool { return r == '/' })
}

// descendRunRoot walks comps down from the run directory one component at a
// time and returns a handle on the last one. Every call it makes carries a
// single component, so a chain whose *resolved* spelling exceeds the
// platform's per-call pathname limit is still reachable even though no single
// call may name it. os.Root's escape checks apply at every step, so the
// descent can reach nothing the caller could not already reach.
func descendRunRoot(rc *tool.RunContext, comps []string) (*os.Root, error) {
	root, err := openRunRoot(rc)
	if err != nil {
		return nil, err
	}
	for _, component := range comps {
		if component == "." {
			continue
		}
		next, err := root.OpenRoot(component)
		_ = root.Close()
		if err != nil {
			return nil, err
		}
		root = next
	}
	return root, nil
}

// sourceDirRoot opens member itself as a directory handle.
func sourceDirRoot(rc *tool.RunContext, member string) (*os.Root, error) {
	return descendRunRoot(rc, memberComponents(member))
}

// sourceParentRoot opens member's parent directory and returns the final
// component, so the operation the caller actually wants stays a single-name
// call relative to that descriptor.
func sourceParentRoot(rc *tool.RunContext, member string) (*os.Root, string, error) {
	parts := memberComponents(member)
	if len(parts) == 0 {
		return nil, "", fmt.Errorf("empty source pathname")
	}
	root, err := descendRunRoot(rc, parts[:len(parts)-1])
	if err != nil {
		return nil, "", err
	}
	return root, parts[len(parts)-1], nil
}

func openSourceFile(rc *tool.RunContext, member, full string) (*os.File, error) {
	f, err := os.Open(full)
	if err == nil || !rootRelativeSource(rc, member, full) {
		return f, err
	}
	root, base, rootErr := sourceParentRoot(rc, member)
	if rootErr != nil {
		return nil, err
	}
	defer root.Close()
	f, rootErr = root.Open(base)
	if rootErr != nil {
		return nil, err
	}
	return f, nil
}

func readSourceLink(rc *tool.RunContext, member, full string) (string, error) {
	target, err := os.Readlink(full)
	if err == nil || !rootRelativeSource(rc, member, full) {
		return target, err
	}
	root, base, rootErr := sourceParentRoot(rc, member)
	if rootErr != nil {
		return "", err
	}
	defer root.Close()
	target, rootErr = root.Readlink(base)
	if rootErr != nil {
		return "", err
	}
	return target, nil
}

// statSourcePath is os.Stat for a source pax has decided to follow (-H on an
// operand, -L anywhere), with the same component-wise retry. The retry keeps
// os.Root's confinement, so it resolves only links that stay inside the run
// directory; a link out of it keeps the original error rather than widening
// what -H/-L may reach. It never fires unless the direct call already failed,
// so no source that resolves today stops resolving.
func statSourcePath(rc *tool.RunContext, member, full string) (os.FileInfo, error) {
	fi, err := os.Stat(full)
	if err == nil || !rootRelativeSource(rc, member, full) {
		return fi, err
	}
	root, base, rootErr := sourceParentRoot(rc, member)
	if rootErr != nil {
		return nil, err
	}
	defer root.Close()
	fi, rootErr = root.Stat(base)
	if rootErr != nil {
		return nil, err
	}
	return fi, nil
}

// sourceDirEntry is one child of a source directory with its lstat already
// applied. Resolving eagerly is the point: fs.DirEntry.Info defers the lstat
// and rebuilds the parent's resolved pathname to perform it, so a directory
// that is only overlong once joined to rc.Dir fails per child even when the
// enumeration itself succeeded. err is that child's own lookup failure, which
// the walker diagnoses individually while continuing with its siblings.
type sourceDirEntry struct {
	name string
	fi   os.FileInfo
	err  error
}

// readSourceDir enumerates a source directory in os.ReadDir's deterministic
// (lexical) order with every child's lstat resolved. The direct read is tried
// first so traversal, symlink, and -X behavior are unchanged wherever it
// works; the root-relative retry is reached only after a real failure.
func readSourceDir(rc *tool.RunContext, member, abs string) ([]sourceDirEntry, error) {
	retryable := rootRelativeSource(rc, member, abs)
	entries, err := os.ReadDir(abs)
	if err != nil {
		if !retryable {
			return nil, err
		}
		retried, rootErr := readSourceDirRootRelative(rc, member)
		if rootErr != nil {
			return nil, err // the direct failure is the one worth reporting
		}
		return retried, nil
	}
	// The enumeration succeeding does not mean the deferred per-child lstats
	// will: os.ReadDir names the directory once, Info names each child. Open
	// the handle lazily so the common case costs nothing.
	var handle *os.Root
	tried := false
	defer func() {
		if handle != nil {
			handle.Close()
		}
	}()
	out := make([]sourceDirEntry, len(entries))
	for i, e := range entries {
		out[i].name = e.Name()
		out[i].fi, out[i].err = e.Info()
		if out[i].err == nil || !retryable {
			continue
		}
		if !tried {
			tried = true
			handle, _ = sourceDirRoot(rc, member)
		}
		if handle == nil {
			continue
		}
		if fi, statErr := handle.Lstat(e.Name()); statErr == nil {
			out[i].fi, out[i].err = fi, nil
		}
	}
	return out, nil
}

// readSourceDirRootRelative lists a directory reached component-wise. Names
// come off the open descriptor and each lstat names one component relative to
// it, so no call in the sequence ever receives the resolved pathname.
func readSourceDirRootRelative(rc *tool.RunContext, member string) ([]sourceDirEntry, error) {
	root, err := sourceDirRoot(rc, member)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	f, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	names, err := f.Readdirnames(-1)
	_ = f.Close()
	if err != nil {
		return nil, err
	}
	slices.Sort(names)
	out := make([]sourceDirEntry, len(names))
	for i, name := range names {
		out[i].name = name
		out[i].fi, out[i].err = root.Lstat(name)
	}
	return out, nil
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

func (w *walker) walk(member, abs string, fi os.FileInfo, followed bool) error {
	if w.o.t {
		atime, ok := sourceAccessTimeFn(fi)
		if !ok {
			fmt.Fprintf(w.rc.Err, "pax: %s: cannot determine source access time on this platform\n", member)
			w.diagnosed = true
		} else {
			defer func() {
				if err := restoreSourceTimesFn(abs, atime, fi.Mode()&os.ModeSymlink != 0 && !followed); err != nil {
					fmt.Fprintf(w.rc.Err, "pax: %s: restore source access time: %v\n", member, err)
					w.diagnosed = true
				}
			}()
		}
	}
	if err := w.fn(walkEntry{member: member, abs: abs, fi: fi, followed: followed}); err != nil {
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

	entries, err := readSourceDir(w.rc, member, abs) // deterministic member order
	if err != nil {
		fmt.Fprintf(w.rc.Err, "pax: %s: %v\n", member, err)
		w.diagnosed = true
		return nil
	}
	if !w.o.t {
		// POSIX directory reads mark the source's access timestamp. Some Linux
		// mounts defer or suppress relatime updates, which made pathname-prefix
		// resolution observably look as though pax never accessed the target.
		// Materialize the same access event while preserving source mtime; -t
		// takes the separate snapshot-and-restore path above.
		_ = os.Chtimes(abs, time.Now(), fi.ModTime())
	}
	base := strings.TrimRight(member, "/")
	for _, e := range entries {
		childMember := base + "/" + e.name
		childAbs := filepath.Join(abs, e.name)
		if e.err != nil { // lstat semantics, already applied by readSourceDir
			fmt.Fprintf(w.rc.Err, "pax: %s: %v\n", childMember, e.err)
			w.diagnosed = true
			continue
		}
		// Only -L resolves symlinks encountered below an operand; under -H
		// they are archived as the symlinks they are.
		cfi, childFollowed := e.fi, false
		if cfi.Mode()&os.ModeSymlink != 0 && w.o.follow == followAll {
			resolved, err := statSourcePath(w.rc, childMember, childAbs)
			if err != nil {
				fmt.Fprintf(w.rc.Err, "pax: %s: %v\n", childMember, err)
				w.diagnosed = true
				continue
			}
			cfi, childFollowed = resolved, true
		}
		if err := w.walk(childMember, childAbs, cfi, childFollowed); err != nil {
			return err
		}
	}
	return nil
}
