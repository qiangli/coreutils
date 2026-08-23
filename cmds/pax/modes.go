package paxcmd

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/qiangli/coreutils/pkg/pax"
	"github.com/qiangli/coreutils/tool"
)

// readMode extracts. The destination safety decision is delegated to
// pkg/pax.PlanExtraction, which validates EVERY member before anything is
// written: a hostile archive must not be able to escape the root part-way
// through, which is exactly what a member-by-member loop would allow.
func readMode(rc *tool.RunContext, o *options, patterns []string) int {
	r, err := openArchive(rc, o)
	if err != nil {
		fmt.Fprintf(rc.Err, "pax: %v\n", err)
		return 1
	}
	defer r.Close()

	root := rc.Dir
	if root == "" {
		root, _ = os.Getwd()
	}
	// Plan first. The planner needs the whole stream, so the archive is read
	// once into the plan and once for payloads; a non-seekable stdin therefore
	// needs buffering, which is why the bytes are captured up front.
	data, err := io.ReadAll(r)
	if err != nil {
		fmt.Fprintf(rc.Err, "pax: %v\n", err)
		return 1
	}
	plan, err := pax.PlanExtraction(strings.NewReader(string(data)), root, pax.OSFS{})
	if err != nil {
		fmt.Fprintf(rc.Err, "pax: %v\n", err)
		return 1
	}
	// Two kinds of rejection, and conflating them would be a bug in either
	// direction. An existing destination is a POLICY question that POSIX
	// answers with "overwrite unless -k"; everything else - an escaping path,
	// an unsafe parent, a duplicate destination - is a safety verdict that no
	// flag may override, and it condemns the WHOLE archive rather than one
	// member, so nothing is written at all.
	fatal := false
	overwrite := map[string]string{}
	for _, rej := range plan.Rejected {
		if pax.IsDestinationExists(rej.Reason) {
			if o.noOverwrite {
				continue // -k: silently keep what is already there
			}
			overwrite[rej.Path] = filepath.Join(root, filepath.FromSlash(rej.Path))
			continue
		}
		fmt.Fprintf(rc.Err, "pax: refusing %s: %s\n", rej.Path, rej.Reason)
		fatal = true
	}
	if fatal {
		fmt.Fprintln(rc.Err, "pax: archive rejected; nothing was extracted")
		return 1
	}

	allow := map[string]string{}
	for p, t := range overwrite {
		name := applySubstitutions(o.subst, p)
		if name == "" || !selected(o, patterns, name) {
			continue
		}
		allow[p] = t
	}
	for _, m := range plan.Members {
		name := applySubstitutions(o.subst, m.Path)
		if name == "" || !selected(o, patterns, name) {
			continue
		}
		allow[m.Path] = m.Target
	}

	tr := tar.NewReader(strings.NewReader(string(data)))
	status := 0
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Fprintf(rc.Err, "pax: %v\n", err)
			return 1
		}
		target, ok := allow[h.Name]
		if !ok {
			continue
		}
		if err := extractOne(rc, o, h, tr, target); err != nil {
			fmt.Fprintf(rc.Err, "pax: %s: %v\n", h.Name, err)
			status = 1
			continue
		}
		if o.verbose {
			fmt.Fprintln(rc.Err, h.Name)
		}
	}
	return status
}

func extractOne(rc *tool.RunContext, o *options, h *tar.Header, r io.Reader, target string) error {
	// -k: an existing destination is never replaced.
	if o.noOverwrite {
		if _, err := os.Lstat(target); err == nil {
			return nil
		}
	}
	// -u: only replace a destination older than the archive member.
	if o.newerOnly {
		if fi, err := os.Lstat(target); err == nil && !h.ModTime.After(fi.ModTime()) {
			return nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	switch h.Typeflag {
	case tar.TypeDir:
		return os.MkdirAll(target, h.FileInfo().Mode().Perm())
	case tar.TypeSymlink:
		_ = os.Remove(target)
		return os.Symlink(h.Linkname, target)
	case tar.TypeLink:
		_ = os.Remove(target)
		return os.Link(filepath.Join(rootOf(target, h.Name), h.Linkname), target)
	case tar.TypeReg:
		f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, h.FileInfo().Mode().Perm())
		if err != nil {
			return err
		}
		if _, err := io.Copy(f, r); err != nil {
			f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
		// -p m: do not restore the archive's mtime.
		if !strings.Contains(o.preserve, "m") {
			_ = os.Chtimes(target, h.AccessTime, h.ModTime)
		}
		return nil
	default:
		return fmt.Errorf("unsupported member type %q", string(h.Typeflag))
	}
}

// rootOf recovers the extraction root from a target and its member name, so a
// hardlink's referent resolves inside the same extraction rather than against
// the process's working directory.
func rootOf(target, name string) string {
	return strings.TrimSuffix(target, filepath.FromSlash(name))
}

// writeMode creates an archive from the named files.
func writeMode(rc *tool.RunContext, o *options, files []string) int {
	var out io.Writer = rc.Out
	var closer io.Closer
	if o.archive != "" && o.archive != "-" {
		flags := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
		if o.appendMode {
			flags = os.O_CREATE | os.O_WRONLY | os.O_APPEND
		}
		f, err := os.OpenFile(resolve(rc, o.archive), flags, 0o644)
		if err != nil {
			fmt.Fprintf(rc.Err, "pax: %v\n", err)
			return 1
		}
		out, closer = f, f
	}
	tw := tar.NewWriter(out)
	status := 0
	for _, name := range files {
		if err := addPath(rc, o, tw, name); err != nil {
			fmt.Fprintf(rc.Err, "pax: %s: %v\n", name, err)
			status = 1
		}
	}
	if err := tw.Close(); err != nil {
		fmt.Fprintf(rc.Err, "pax: %v\n", err)
		status = 1
	}
	if closer != nil {
		_ = closer.Close()
	}
	return status
}

func addPath(rc *tool.RunContext, o *options, tw *tar.Writer, name string) error {
	full := resolve(rc, name)
	fi, err := os.Lstat(full)
	if err != nil {
		return err
	}
	write := func(rel, abs string, fi os.FileInfo) error {
		link := ""
		if fi.Mode()&os.ModeSymlink != 0 {
			if link, err = os.Readlink(abs); err != nil {
				return err
			}
		}
		out := applySubstitutions(o.subst, filepath.ToSlash(rel))
		if out == "" {
			return nil
		}
		h, err := headerFor(out, fi, link)
		if err != nil {
			return err
		}
		h.Format = tarFormat(o.format)
		if err := tw.WriteHeader(h); err != nil {
			return err
		}
		if fi.Mode().IsRegular() {
			f, err := os.Open(abs)
			if err != nil {
				return err
			}
			defer f.Close()
			if _, err := io.Copy(tw, f); err != nil {
				return err
			}
		}
		if o.verbose {
			fmt.Fprintln(rc.Err, out)
		}
		return nil
	}
	if !fi.IsDir() || o.dirsNoDescend {
		return write(name, full, fi)
	}
	return filepath.Walk(full, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, rerr := filepath.Rel(filepath.Dir(full), p)
		if rerr != nil {
			return rerr
		}
		return write(rel, p, info)
	})
}

// copyMode is -r -w: copy a file hierarchy to a directory. It is implemented as
// create-into-a-pipe feeding extract, so the member naming, -s rewriting and
// destination safety rules are IDENTICAL to the archive path rather than a
// second implementation that could drift from it.
func copyMode(rc *tool.RunContext, o *options, operands []string) int {
	if len(operands) < 2 {
		return tool.UsageError(rc, cmd, "copy mode requires at least one file and a target directory")
	}
	dest := operands[len(operands)-1]
	files := operands[:len(operands)-1]
	full := resolve(rc, dest)
	fi, err := os.Stat(full)
	if err != nil || !fi.IsDir() {
		fmt.Fprintf(rc.Err, "pax: %s: not a directory\n", dest)
		return 1
	}

	pr, pw := io.Pipe()
	go func() {
		tw := tar.NewWriter(pw)
		var werr error
		for _, name := range files {
			if e := addPath(rc, o, tw, name); e != nil && werr == nil {
				werr = e
			}
		}
		if e := tw.Close(); e != nil && werr == nil {
			werr = e
		}
		pw.CloseWithError(werr)
	}()

	sub := *o
	sub.read, sub.write = true, false
	sub.archive = ""
	sub.subst = nil // already applied on the write side; applying twice would rewrite a rewrite
	inner := *rc
	inner.Dir = full
	inner.Stdio = rc.Stdio
	inner.In = pr
	return readMode(&inner, &sub, nil)
}
