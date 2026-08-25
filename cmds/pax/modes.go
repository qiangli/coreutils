package paxcmd

import (
	"archive/tar"
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

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

	raw, err := io.ReadAll(r)
	if err != nil {
		fmt.Fprintf(rc.Err, "pax: %v\n", err)
		return 1
	}
	archive, err := decodeArchive(raw)
	if err != nil {
		fmt.Fprintf(rc.Err, "pax: %v\n", err)
		return 1
	}
	raw = archive.tarData
	sel := newSelector(o, patterns)
	var catalog []selectorMember
	scan := tar.NewReader(bytes.NewReader(raw))
	for {
		h, err := scan.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Fprintf(rc.Err, "pax: %v\n", err)
			return 1
		}
		catalog = append(catalog, selectorMember{
			name:  h.Name,
			isDir: h.Typeflag == tar.TypeDir || strings.HasSuffix(h.Name, "/"),
		})
	}
	sel.prime(catalog)

	var rewritten bytes.Buffer
	tr := tar.NewReader(bytes.NewReader(raw))
	tw := tar.NewWriter(&rewritten)

	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Fprintf(rc.Err, "pax: %v\n", err)
			return 1
		}

		isDir := h.Typeflag == tar.TypeDir || strings.HasSuffix(h.Name, "/")
		if !sel.keep(h.Name, isDir) {
			continue
		}

		newName := applySubstitutions(o.subst, h.Name, rc.Err)
		if newName == "" {
			continue
		}

		h.Name = newName
		if h.Typeflag == tar.TypeLink {
			h.Linkname = applySubstitutions(o.subst, h.Linkname, nil)
			if h.Linkname == "" {
				continue
			}
		}
		if err := tw.WriteHeader(h); err != nil {
			fmt.Fprintf(rc.Err, "pax: %v\n", err)
			return 1
		}
		if _, err := io.Copy(tw, tr); err != nil {
			fmt.Fprintf(rc.Err, "pax: %v\n", err)
			return 1
		}
	}
	if err := tw.Close(); err != nil {
		fmt.Fprintf(rc.Err, "pax: %v\n", err)
		return 1
	}

	unmatched := sel.unmatched()
	status := 0
	if len(unmatched) > 0 {
		for _, p := range unmatched {
			fmt.Fprintf(rc.Err, "pax: pattern %q not matched\n", p)
		}
		status = 1
	}

	data := rewritten.Bytes()
	plan, err := pax.PlanExtraction(bytes.NewReader(data), root, pax.OSFS{})
	if err != nil {
		fmt.Fprintf(rc.Err, "pax: %v\n", err)
		return 1
	}

	fatal := false
	// allow is keyed by ARCHIVE POSITION, not by name: an updated archive can
	// carry the same name more than once, and only the occurrence the planner
	// kept may be written. Keying by name would resurrect the stale copies.
	allow := map[int]string{}
	for _, rej := range plan.Rejected {
		if pax.IsDestinationExists(rej.Reason) {
			if o.noOverwrite {
				continue // -k: silently keep what is already there
			}
			allow[rej.Index] = filepath.Join(root, filepath.FromSlash(rej.Path))
			continue
		}
		fmt.Fprintf(rc.Err, "pax: refusing %s: %s\n", rej.Path, rej.Reason)
		fatal = true
	}
	if fatal {
		fmt.Fprintln(rc.Err, "pax: archive rejected; nothing was extracted")
		return 1
	}

	for _, m := range plan.Members {
		allow[m.Index] = m.Target
	}

	tr2 := tar.NewReader(bytes.NewReader(data))
	for index := 0; ; index++ {
		h, err := tr2.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Fprintf(rc.Err, "pax: %v\n", err)
			return 1
		}
		target, ok := allow[index]
		if !ok {
			continue
		}
		if err := extractOne(rc, o, h, tr2, target); err != nil {
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

// archiveSink is the seekable archive file. writeMode takes it through an
// interface rather than *os.File so the append lane's seek, truncate and close
// failures - the paths that decide whether a half-rewritten archive is
// reported or silently accepted - are reachable from tests.
type archiveSink interface {
	io.Reader
	io.Writer
	io.Seeker
	Truncate(size int64) error
	Close() error
}

var openArchiveSink = func(path string, flags int, perm os.FileMode) (archiveSink, error) {
	return os.OpenFile(path, flags, perm)
}

// writeMode creates an archive from the named files.
func writeMode(rc *tool.RunContext, o *options, files []string) int {
	status := 0
	if len(files) == 0 {
		var inputStatus int
		files, inputStatus = readPathnames(rc)
		status = inputStatus
	}
	if o.appendMode && o.format == "cpio" {
		fmt.Fprintln(rc.Err, "pax: appending to cpio archives is not supported")
		return 1
	}

	archivePath := ""
	if o.archive != "" && o.archive != "-" {
		archivePath = resolve(rc, o.archive)
	}
	// -a and -u both have to read the archive they are about to extend. On a
	// pipe there is nothing to read and nothing to seek back to, so the
	// documented semantics cannot be honored - say so rather than silently
	// writing a fresh archive that ignores the option.
	if (o.appendMode || o.newerOnly) && archivePath == "" {
		fmt.Fprintln(rc.Err, "pax: appending or updating requires a seekable archive file")
		return 1
	}
	appendExisting := o.appendMode
	var existing []byte
	if archivePath != "" && (o.appendMode || o.newerOnly) {
		data, err := os.ReadFile(archivePath)
		if err != nil && !os.IsNotExist(err) {
			fmt.Fprintf(rc.Err, "pax: %v\n", err)
			return 1
		}
		if err == nil && len(data) != 0 {
			existing = data
			archive, decodeErr := decodeArchive(data)
			if decodeErr != nil {
				fmt.Fprintf(rc.Err, "pax: %v\n", decodeErr)
				return 1
			}
			if archive.kind == archiveCPIO {
				fmt.Fprintln(rc.Err, "pax: updating or appending to cpio archives is not supported")
				return 1
			}
			existingFormat := "ustar"
			if archive.pax {
				existingFormat = "pax"
			}
			if o.format != existingFormat {
				fmt.Fprintf(rc.Err, "pax: cannot append %s data to an existing %s archive\n", o.format, existingFormat)
				return 1
			}
			if o.newerOnly {
				o.archiveTimes, decodeErr = archiveMemberTimes(archive.tarData)
				if decodeErr != nil {
					fmt.Fprintf(rc.Err, "pax: %v\n", decodeErr)
					return 1
				}
				appendExisting = true
			}
		}
	}

	var out io.Writer = rc.Out
	var file archiveSink
	var blockPrefix []byte
	if archivePath != "" {
		flags := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
		if appendExisting && len(existing) != 0 {
			flags = os.O_RDWR
		}
		var err error
		file, err = openArchiveSink(archivePath, flags, 0o644)
		if err != nil {
			fmt.Fprintf(rc.Err, "pax: %v\n", err)
			return 1
		}
		if appendExisting && len(existing) != 0 {
			end, _, scanErr := scanTar(existing)
			if scanErr != nil {
				fmt.Fprintf(rc.Err, "pax: %v\n", scanErr)
				if closeErr := file.Close(); closeErr != nil {
					fmt.Fprintf(rc.Err, "pax: %v\n", closeErr)
				}
				return 1
			}
			blockStart := end - end%int64(o.blockBytes)
			if _, err = file.Seek(blockStart, io.SeekStart); err != nil {
				fmt.Fprintf(rc.Err, "pax: %v\n", err)
				if closeErr := file.Close(); closeErr != nil {
					fmt.Fprintf(rc.Err, "pax: %v\n", closeErr)
				}
				return 1
			}
			blockPrefix = make([]byte, int(end-blockStart))
			if len(blockPrefix) != 0 {
				if _, err = io.ReadFull(file, blockPrefix); err != nil {
					fmt.Fprintf(rc.Err, "pax: %v\n", err)
					if closeErr := file.Close(); closeErr != nil {
						fmt.Fprintf(rc.Err, "pax: %v\n", closeErr)
					}
					return 1
				}
				if _, err = file.Seek(blockStart, io.SeekStart); err != nil {
					fmt.Fprintf(rc.Err, "pax: %v\n", err)
					if closeErr := file.Close(); closeErr != nil {
						fmt.Fprintf(rc.Err, "pax: %v\n", closeErr)
					}
					return 1
				}
			}
		}
		out = file
	}

	// POSIX pax always writes whole physical blocks; blockBytes is either the
	// explicit -b or the format default, never zero.
	blocker := newBlockWriter(out, o.blockBytes, blockPrefix)
	out = blocker
	if o.format == "cpio" {
		if writeStatus := writeCPIOMode(rc, o, out, files); writeStatus != 0 {
			status = 1
		}
	} else {
		tw := tar.NewWriter(out)
		for _, name := range files {
			diagnosed, err := addPath(rc, o, tw, name)
			if err != nil && !errors.Is(err, errTraversalCycle) {
				fmt.Fprintf(rc.Err, "pax: %s: %v\n", name, err)
			}
			if err != nil || diagnosed {
				status = 1
			}
			if errors.Is(err, errTraversalCycle) {
				break
			}
		}
		if err := tw.Close(); err != nil {
			fmt.Fprintf(rc.Err, "pax: %v\n", err)
			status = 1
		}
	}
	if err := blocker.Close(); err != nil {
		fmt.Fprintf(rc.Err, "pax: %v\n", err)
		status = 1
	}
	if file != nil {
		// A failed physical write may have changed bytes already, but truncating
		// after it would compound the damage. Preparation failures above happen
		// before the first write and therefore leave the archive untouched.
		if appendExisting && len(existing) != 0 && blocker.err == nil {
			if pos, err := file.Seek(0, io.SeekCurrent); err != nil {
				fmt.Fprintf(rc.Err, "pax: %v\n", err)
				status = 1
			} else if err := file.Truncate(pos); err != nil {
				fmt.Fprintf(rc.Err, "pax: %v\n", err)
				status = 1
			}
		}
		if err := file.Close(); err != nil {
			fmt.Fprintf(rc.Err, "pax: %v\n", err)
			status = 1
		}
	}
	return status
}

// readPathnames reads the operand list from standard input, one pathname per
// line. An empty line names no file and an unterminated final line is a
// truncated request; both are reported and the exit status reflects them
// rather than letting pax archive something the caller did not ask for.
func readPathnames(rc *tool.RunContext) ([]string, int) {
	reader := bufio.NewReader(rc.In)
	var paths []string
	status := 0
	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			if line[len(line)-1] == '\n' {
				line = line[:len(line)-1]
			} else {
				fmt.Fprintln(rc.Err, "pax: unterminated pathname on standard input")
				status = 1
			}
			if line == "" {
				fmt.Fprintln(rc.Err, "pax: empty pathname on standard input")
				status = 1
			} else {
				paths = append(paths, line)
			}
		}
		if err != nil {
			if err != io.EOF {
				fmt.Fprintf(rc.Err, "pax: read standard input: %v\n", err)
				status = 1
			}
			break
		}
	}
	return paths, status
}

func archiveMemberTimes(data []byte) (map[string]time.Time, error) {
	times := make(map[string]time.Time)
	tr := tar.NewReader(bytes.NewReader(data))
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return times, nil
		}
		if err != nil {
			return nil, err
		}
		if old, ok := times[h.Name]; !ok || h.ModTime.After(old) {
			times[h.Name] = h.ModTime
		}
	}
}

// cpioWriter emits the POSIX Issue 7 octet-oriented cpio interchange format.
type cpioWriter struct {
	w      io.Writer
	offset int64
}

func (w *cpioWriter) write(p []byte) error {
	n, err := w.w.Write(p)
	w.offset += int64(n)
	if err == nil && n != len(p) {
		return io.ErrShortWrite
	}
	return err
}

func (w *cpioWriter) pad(alignment int64) error {
	need := (alignment - w.offset%alignment) % alignment
	if need == 0 {
		return nil
	}
	return w.write(make([]byte, need))
}

// add writes one member. id carries the SOURCE metadata (owner and link count
// are written through verbatim) while emitted carries the c_dev/c_ino pair
// chosen by odcIdentities, which is what encodes hardlink identity.
func (w *cpioWriter) add(name string, fi os.FileInfo, id fileIdentity, emitted devIno, data []byte) error {
	mode := uint64(fi.Mode().Perm())
	switch {
	case fi.Mode().IsRegular():
		mode |= 0o100000
	case fi.IsDir():
		mode |= 0o040000
	case fi.Mode()&os.ModeSymlink != 0:
		mode |= 0o120000
	default:
		return fmt.Errorf("unsupported cpio member type %s", fi.Mode())
	}
	name = filepath.ToSlash(name)
	nlink, uid, gid := uint64(1), uint64(0), uint64(0)
	if id.ok {
		if id.nlink > 0 {
			nlink = id.nlink
		}
		uid, gid = id.uid, id.gid
	}
	namesize := uint64(len([]byte(name)) + 1)
	mtime := uint64(0)
	if fi.ModTime().Unix() > 0 {
		mtime = uint64(fi.ModTime().Unix())
	}
	const octal6Max = 0o777777
	for _, field := range []uint64{emitted.dev, emitted.ino, mode, uid, gid, nlink, namesize} {
		if field > octal6Max {
			return fmt.Errorf("cpio member %q exceeds POSIX header limits", name)
		}
	}
	if mtime > 0o77777777777 || uint64(len(data)) > 0o77777777777 {
		return fmt.Errorf("cpio member %q exceeds POSIX header limits", name)
	}
	header := fmt.Sprintf("070707%06o%06o%06o%06o%06o%06o%06o%011o%06o%011o",
		emitted.dev, emitted.ino, mode, uid, gid, nlink, 0, mtime, namesize, len(data))
	if len(header) != 76 {
		return fmt.Errorf("internal cpio header length %d", len(header))
	}
	if err := w.write(append([]byte(header), append([]byte(name), 0)...)); err != nil {
		return err
	}
	return w.write(data)
}

func (w *cpioWriter) close() error {
	info := syntheticFileInfo{name: "TRAILER!!!"}
	if err := w.add("TRAILER!!!", info, fileIdentity{}, devIno{}, nil); err != nil {
		return err
	}
	return w.pad(512)
}

type syntheticFileInfo struct{ name string }

func (f syntheticFileInfo) Name() string       { return f.name }
func (f syntheticFileInfo) Size() int64        { return 0 }
func (f syntheticFileInfo) Mode() os.FileMode  { return 0 }
func (f syntheticFileInfo) ModTime() time.Time { return time.Unix(0, 0) }
func (f syntheticFileInfo) IsDir() bool        { return false }
func (f syntheticFileInfo) Sys() any           { return nil }

// cpioMember is one collected source file.
type cpioMember struct {
	name string
	fi   os.FileInfo
	id   fileIdentity
	data []byte
}

// odcIdentities chooses the c_dev/c_ino pair written for each member. The
// POSIX cpio header gives those fields six octal digits, which a modern inode
// number routinely overflows, so the source values are written verbatim only
// when every member's pair fits; otherwise the whole archive is remapped onto
// small sequential values. Both mappings are one-to-one, so equal source pairs
// - and only equal source pairs - share a c_dev/c_ino, which is what carries
// hardlink identity through the archive.
func odcIdentities(members []cpioMember) []devIno {
	const octal6Max = 0o777777
	exact := true
	for _, m := range members {
		if !m.id.ok || m.id.dev > octal6Max || m.id.ino > octal6Max {
			exact = false
			break
		}
	}
	out := make([]devIno, len(members))
	if exact {
		for i, m := range members {
			out[i] = m.id.key()
		}
		return out
	}
	assigned := make(map[devIno]devIno)
	next := uint64(0)
	for i, m := range members {
		if m.id.ok {
			if v, ok := assigned[m.id.key()]; ok {
				out[i] = v
				continue
			}
		}
		next++
		out[i] = devIno{ino: next}
		if m.id.ok {
			assigned[m.id.key()] = out[i]
		}
	}
	return out
}

// writeCPIOMode buffers the whole member list before emitting anything: POSIX
// cpio carries a hardlink group's file data on the LAST member of the group,
// which is not knowable while the walk is still running.
func writeCPIOMode(rc *tool.RunContext, o *options, out io.Writer, files []string) int {
	status := 0
	var members []cpioMember
	for _, name := range files {
		diagnosed, err := collectCPIOPath(rc, o, &members, name)
		if err != nil && !errors.Is(err, errTraversalCycle) {
			fmt.Fprintf(rc.Err, "pax: %s: %v\n", name, err)
		}
		if err != nil || diagnosed {
			status = 1
		}
		if errors.Is(err, errTraversalCycle) {
			break
		}
	}
	ids := odcIdentities(members)
	dataHolder := make(map[devIno]int)
	for i, m := range members {
		if m.fi.Mode().IsRegular() && m.id.ok && m.id.nlink > 1 {
			dataHolder[ids[i]] = i
		}
	}
	w := &cpioWriter{w: out}
	for i, m := range members {
		data := m.data
		if holder, ok := dataHolder[ids[i]]; ok && holder != i {
			data = nil
		}
		if err := w.add(m.name, m.fi, m.id, ids[i], data); err != nil {
			fmt.Fprintf(rc.Err, "pax: %s: %v\n", m.name, err)
			status = 1
			continue
		}
		if o.verbose {
			fmt.Fprintln(rc.Err, m.name)
		}
	}
	if err := w.close(); err != nil {
		fmt.Fprintf(rc.Err, "pax: %v\n", err)
		status = 1
	}
	return status
}

func collectCPIOPath(rc *tool.RunContext, o *options, members *[]cpioMember, name string) (bool, error) {
	return walkOperand(rc, o, name, func(e walkEntry) error {
		out := applySubstitutions(o.subst, e.member, rc.Err)
		if out == "" {
			return nil
		}
		if !newerThanArchive(o, out, e.fi.ModTime()) {
			return nil
		}
		var data []byte
		var err error
		switch {
		case e.fi.Mode().IsRegular():
			data, err = os.ReadFile(e.abs)
		case e.fi.Mode()&os.ModeSymlink != 0:
			var target string
			target, err = os.Readlink(e.abs)
			data = []byte(target)
		}
		if err != nil {
			return sourceTraversalErr(err)
		}
		*members = append(*members, cpioMember{name: out, fi: e.fi, id: identityOf(e.fi), data: data})
		return nil
	})
}

func addPath(rc *tool.RunContext, o *options, tw *tar.Writer, name string) (bool, error) {
	return walkOperand(rc, o, name, func(e walkEntry) error {
		fi := e.fi
		link := ""
		if fi.Mode()&os.ModeSymlink != 0 {
			var err error
			if link, err = os.Readlink(e.abs); err != nil {
				return sourceTraversalErr(err)
			}
		}
		out := applySubstitutions(o.subst, e.member, rc.Err)
		if out == "" {
			return nil
		}
		if !newerThanArchive(o, out, fi.ModTime()) {
			return nil
		}
		h, err := headerFor(out, fi, link)
		if err != nil {
			return sourceTraversalErr(err)
		}
		h.Format = tarFormat(o.format)
		id := identityOf(fi)
		// A second name for an already-archived inode becomes a hardlink
		// member rather than a second copy of the data, which is what makes
		// the extracted tree share inodes the way the source did.
		hardlink := false
		if fi.Mode().IsRegular() && id.ok && id.nlink > 1 {
			if o.links == nil {
				o.links = make(map[devIno]string)
			}
			if first, seen := o.links[id.key()]; seen {
				h.Typeflag = tar.TypeLink
				h.Linkname = first
				h.Size = 0
				hardlink = true
			} else {
				o.links[id.key()] = out
			}
		}
		if h.Format == tar.FormatPAX {
			if h.PAXRecords == nil {
				h.PAXRecords = make(map[string]string)
			}
			// A basic pax header is otherwise indistinguishable from ustar on
			// disk. This implementation keyword makes the selected format
			// detectable so a later -a can reject a mismatched -x format.
			h.PAXRecords["COREUTILS.format"] = "pax"
			// ustar fields cover the owner but not the source device, inode or
			// link count. pax extended records can, under the long-standing
			// star/schily keywords, so a pax archive carries the identity a
			// cpio header would have held.
			if id.ok {
				h.Uid, h.Gid = int(id.uid), int(id.gid)
				h.PAXRecords["SCHILY.dev"] = strconv.FormatUint(id.dev, 10)
				h.PAXRecords["SCHILY.ino"] = strconv.FormatUint(id.ino, 10)
				h.PAXRecords["SCHILY.nlink"] = strconv.FormatUint(id.nlink, 10)
			}
		}
		if h.Format == tar.FormatUSTAR {
			// USTAR carries mtime but has no fields for atime, ctime, xattrs,
			// or PAX records. FileInfoHeader may populate the extra timestamps
			// from platform stat data; clear them explicitly so asking for
			// -x ustar does not fail merely because the source filesystem
			// exposes richer metadata.
			h.AccessTime = time.Time{}
			h.ChangeTime = time.Time{}
			h.Xattrs = nil
			h.PAXRecords = nil
		}
		if err := tw.WriteHeader(h); err != nil {
			return err
		}
		if fi.Mode().IsRegular() && !hardlink {
			f, err := os.Open(e.abs)
			if err != nil {
				return sourceTraversalErr(err)
			}
			defer f.Close()
			if err := copySourceFile(tw, f); err != nil {
				return err
			}
		}
		if o.verbose {
			fmt.Fprintln(rc.Err, out)
		}
		return nil
	})
}

// copySourceFile keeps archive sink failures precise. A mid-file source read
// failure is also fatal to this stream: the tar header has already promised
// the full file size, so continuing would leave an invalid member. Lookup,
// open, stat, and readlink failures are classified before a header is emitted
// and remain safely recoverable by copy mode.
func copySourceFile(dst io.Writer, src io.Reader) error {
	buf := make([]byte, 32*1024)
	for {
		n, readErr := src.Read(buf)
		if n > 0 {
			written, writeErr := dst.Write(buf[:n])
			if writeErr != nil {
				return writeErr
			}
			if written != n {
				return io.ErrShortWrite
			}
		}
		if readErr == io.EOF {
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}
}

func newerThanArchive(o *options, name string, mtime time.Time) bool {
	if !o.newerOnly || o.archiveTimes == nil {
		return true
	}
	old, exists := o.archiveTimes[name]
	return !exists || mtime.After(old)
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
	// diagCh carries the write side's already-printed traversal diagnostics to
	// the exit status; the channel receive after readMode is the synchronization
	// point.
	diagCh := make(chan bool, 1)
	go func() {
		tw := tar.NewWriter(pw)
		var werr error
		diagnosed := false
		for _, name := range files {
			d, e := addPath(rc, o, tw, name)
			diagnosed = diagnosed || d
			if errors.Is(e, errTraversalCycle) {
				break
			}
			if sourceTraversalFailure(e) {
				fmt.Fprintf(rc.Err, "pax: %s: %v\n", name, e)
				diagnosed = true
				continue
			}
			if e != nil && werr == nil {
				werr = e
				break
			}
		}
		if e := tw.Close(); e != nil && werr == nil {
			werr = e
		}
		diagCh <- diagnosed
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
	status := readMode(&inner, &sub, nil)
	if <-diagCh && status == 0 {
		status = 1
	}
	return status
}
