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
	"sort"
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
func readMode(rc *tool.RunContext, o *options, patterns []string) (status int) {
	ownedRenamer := false
	defer func() {
		if ownedRenamer {
			if err := o.renamer.Close(); err != nil {
				fmt.Fprintf(rc.Err, "pax: interactive rename: close /dev/tty: %v\n", err)
				status = 1
			}
			o.renamer = nil
		}
	}()
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
	// A pax archive whose members need no extended records is physically
	// indistinguishable from ustar.  Accept tar input here and reject only the
	// unambiguously non-pax cpio form; otherwise every minimal pax archive
	// would spuriously reject -o.
	if o.paxOptions.needsPAX && archive.kind == archiveCPIO {
		fmt.Fprintln(rc.Err, "pax: -o option is applicable only to a pax archive")
		return 1
	}
	raw, err = filterDeletedPAXRecords(raw, o.paxOptions)
	if err != nil {
		fmt.Fprintf(rc.Err, "pax: %v\n", err)
		return 1
	}
	status = 0
	sel := newSelector(o, patterns)
	var catalog []selectorMember
	var invalidMembers []paxInvalidHeaderFields
	var linkNames []string
	scan := newOptionTarReader(raw, o.paxOptions, false)
	for {
		h, err := scan.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Fprintf(rc.Err, "pax: %v\n", err)
			return 1
		}
		invalid := translatePAXHeaderToLocal(rc, h, o.paxOptions.invalid, false)
		catalog = append(catalog, selectorMember{
			name:  h.Name,
			isDir: h.Typeflag == tar.TypeDir || strings.HasSuffix(h.Name, "/"),
		})
		invalidMembers = append(invalidMembers, invalid)
		linkNames = append(linkNames, h.Linkname)
	}
	sel.prime(catalog)

	// POSIX ordering is selection, then -s substitution, then -i. Resolve every
	// interactive name before extraction begins so an EOF or terminal failure is
	// immediate and cannot leave a partially extracted filesystem. The complete
	// map also lets hard-link targets follow a renamed member even when their
	// archive occurrence precedes that member.
	selected := make(map[int]string)
	renames := make(map[string]string)
	linkRenames := make(map[int]string)
	for index, m := range catalog {
		if !sel.keep(m.name, m.isDir) {
			continue
		}
		subName := applySubstitutions(o.subst, m.name, rc.Err)
		if subName == "" {
			continue
		}
		invalid := invalidMembers[index]
		if invalid.name || invalid.link || invalid.other {
			fmt.Fprintf(rc.Err, "pax: %s: value cannot be translated\n", m.name)
			status = 1
			if o.paxOptions.invalid == "bypass" || invalid.other && o.paxOptions.invalid == "rename" {
				continue
			}
			if (invalid.name || invalid.link) && o.paxOptions.invalid == "rename" && o.renamer == nil {
				r, openErr := openInteractiveRenamer()
				if openErr != nil {
					fmt.Fprintf(rc.Err, "pax: interactive rename: %v\n", openErr)
					return 1
				}
				o.renamer = r
				ownedRenamer = true
			}
		}
		newName, keep, err := renameInteractively(o, subName)
		if err == nil && invalid.name && o.paxOptions.invalid == "rename" && !o.interactive {
			newName, keep, err = o.renamer.rename(subName)
		}
		if err != nil {
			fmt.Fprintf(rc.Err, "pax: interactive rename: %v\n", err)
			return 1
		}
		if !keep {
			continue
		}
		if invalid.link && o.paxOptions.invalid == "rename" {
			linkName := linkNames[index]
			var linkKeep bool
			linkName, linkKeep, err = o.renamer.rename(linkName)
			if err != nil {
				fmt.Fprintf(rc.Err, "pax: interactive link rename: %v\n", err)
				return 1
			}
			if !linkKeep {
				continue
			}
			linkRenames[index] = linkName
		}
		// A -s substitution or interactive replacement can produce a pathname
		// the destination cannot hold even when the archived value was valid.
		// Attempting to create it would fail after extraction had begun;
		// bypass the member with a diagnostic instead, like any other invalid
		// destination value, and keep processing the remaining members.
		if invalidPAXLocalDestinationName(newName) || exceedsDestinationPathLimits(newName) {
			fmt.Fprintf(rc.Err, "pax: %s: pathname cannot be created in the destination hierarchy\n", m.name)
			status = 1
			continue
		}
		selected[index] = newName
		renames[subName] = newName
	}

	var rewritten bytes.Buffer
	tr := newOptionTarReader(raw, o.paxOptions, false)
	tw := tar.NewWriter(&rewritten)

	for index := 0; ; index++ {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Fprintf(rc.Err, "pax: %v\n", err)
			return 1
		}
		translatePAXHeaderToLocal(rc, h, o.paxOptions.invalid, false)

		newName, keep := selected[index]
		if !keep {
			continue
		}

		h.Name = newName
		if h.Typeflag == tar.TypeLink {
			h.Linkname = applySubstitutions(o.subst, h.Linkname, nil)
			if h.Linkname == "" {
				continue
			}
			if renamed, ok := renames[h.Linkname]; ok {
				h.Linkname = renamed
			}
		}
		if linkName, ok := linkRenames[index]; ok {
			h.Linkname = linkName
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
	unsupportedFIFO := ""
	// allow is keyed by ARCHIVE POSITION, not by name: an updated archive can
	// carry the same name more than once, and only the occurrence the planner
	// kept may be written. Keying by name would resurrect the stale copies.
	allow := map[int]string{}
	for _, rej := range plan.Rejected {
		if pax.IsDestinationExists(rej.Reason) {
			if o.noOverwrite {
				continue // -k: silently keep what is already there
			}
			if rej.Kind == pax.KindFIFO && !fifoSupportedForExtraction() {
				unsupportedFIFO = rej.Path
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
		if m.Kind == pax.KindFIFO && !fifoSupportedForExtraction() {
			unsupportedFIFO = m.Path
		}
		allow[m.Index] = m.Target
	}
	if unsupportedFIFO != "" {
		fmt.Fprintf(rc.Err, "pax: refusing %s: FIFO extraction is not supported on this platform\n", unsupportedFIFO)
		fmt.Fprintln(rc.Err, "pax: archive rejected; nothing was extracted")
		return 1
	}

	tr2 := newOptionTarReader(data, paxOptions{}, false)
	var pendingDirs []pendingDirectoryAttributes
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
		pending, err := extractOne(rc, o, h, tr2, target)
		if err != nil {
			fmt.Fprintf(rc.Err, "pax: %s: %v\n", h.Name, err)
			status = 1
			continue
		}
		if pending != nil {
			pendingDirs = append(pendingDirs, *pending)
		}
		if o.verbose {
			fmt.Fprintln(rc.Err, h.Name)
		}
	}
	// Children change their containing directory's times, so directory
	// attributes are finalized deepest-first only after all members exist.
	if finalizePendingDirectories(rc, o, pendingDirs) {
		status = 1
	}
	return status
}

type pendingDirectoryAttributes struct {
	name, path string
	attrs      preservedAttributes
	normalMode os.FileMode
}

func finalizePendingDirectories(rc *tool.RunContext, o *options, pending []pendingDirectoryAttributes) bool {
	// A later archive member for the same directory supersedes an earlier one.
	// Sort distinct paths by depth because archive member order is unrestricted.
	byPath := make(map[string]pendingDirectoryAttributes, len(pending))
	for _, p := range pending {
		byPath[p.path] = p
	}
	pending = pending[:0]
	for _, p := range byPath {
		pending = append(pending, p)
	}
	sort.Slice(pending, func(i, j int) bool {
		return strings.Count(filepath.Clean(pending[i].path), string(filepath.Separator)) >
			strings.Count(filepath.Clean(pending[j].path), string(filepath.Separator))
	})
	failed := false
	for _, p := range pending {
		var errs []error
		if !o.preservation.mode {
			if err := chmodExtractedFn(p.path, p.normalMode.Perm()); err != nil {
				errs = append(errs, fmt.Errorf("set creation mode: %w", err))
			}
		}
		if err := applyPreservedAttributes(p.path, p.attrs, o.preservation, false); err != nil {
			errs = append(errs, err)
		}
		if err := errors.Join(errs...); err != nil {
			fmt.Fprintf(rc.Err, "pax: %s: %v\n", p.name, err)
			failed = true
		}
	}
	return failed
}

func attributesFromHeader(h *tar.Header) preservedAttributes {
	return preservedAttributes{
		uid: h.Uid, gid: h.Gid, mode: h.FileInfo().Mode(),
		atime: h.AccessTime, mtime: h.ModTime,
	}
}

// Kept as a seam so the fail-before-mutation guarantee can be exercised on a
// Unix test host while the unsupported implementation itself remains selected
// by build tags on platforms without mkfifo.
var fifoSupportedForExtraction = fifoSupported

func extractOne(rc *tool.RunContext, o *options, h *tar.Header, r io.Reader, target string) (*pendingDirectoryAttributes, error) {
	// -k: an existing destination is never replaced.
	if o.noOverwrite {
		if _, err := os.Lstat(target); err == nil {
			return nil, nil
		}
	}
	// -u: only replace a destination older than the archive member.
	if o.newerOnly {
		if fi, err := os.Lstat(target); err == nil && !h.ModTime.After(fi.ModTime()) {
			return nil, nil
		}
	}
	if h.Typeflag == tar.TypeFifo && !fifoSupportedForExtraction() {
		return nil, fmt.Errorf("FIFO extraction is not supported on this platform")
	}
	if err := mkdirAllNormal(filepath.Dir(target), intermediateDirMode(rc)); err != nil {
		return nil, err
	}
	attrs := attributesFromHeader(h)
	symlink := false
	switch h.Typeflag {
	case tar.TypeDir:
		normalMode := normalCreationMode(rc, attrs.mode)
		if err := prepareOutputDirectory(target, normalMode); err != nil {
			return nil, err
		}
		return &pendingDirectoryAttributes{name: h.Name, path: target, attrs: attrs, normalMode: normalMode}, nil
	case tar.TypeSymlink:
		_ = os.Remove(target)
		if err := os.Symlink(h.Linkname, target); err != nil {
			return nil, err
		}
		symlink = true
	case tar.TypeLink:
		_ = os.Remove(target)
		if err := os.Link(filepath.Join(rootOf(target, h.Name), h.Linkname), target); err != nil {
			return nil, err
		}
	case tar.TypeFifo:
		_ = os.Remove(target)
		// Create restrictively so an unrelated process umask cannot expose the
		// FIFO before its invocation-relative creation mode is installed.
		if err := makeFIFO(target, 0o600); err != nil {
			return nil, err
		}
		if err := os.Chmod(target, normalCreationMode(rc, attrs.mode)); err != nil {
			_ = os.Remove(target)
			return nil, err
		}
	case tar.TypeReg:
		_, statErr := os.Lstat(target)
		isNew := os.IsNotExist(statErr)
		f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			return nil, err
		}
		if _, err := io.Copy(f, r); err != nil {
			f.Close()
			return nil, err
		}
		if err := f.Close(); err != nil {
			return nil, err
		}
		if isNew {
			if err := os.Chmod(target, normalCreationMode(rc, attrs.mode)); err != nil {
				return nil, err
			}
		}
	default:
		return nil, fmt.Errorf("unsupported member type %q", string(h.Typeflag))
	}
	if err := applyPreservedAttributes(target, attrs, o.preservation, symlink); err != nil {
		return nil, err
	}
	return nil, nil
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
	archivePath := ""
	if o.archive != "" {
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
	var existingCPIO []cpioEntry
	existingCPIOArchive := false
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
				// Append/update rewrites the archive, so the existing archive and
				// selected output format must agree before the destination is opened
				// with O_TRUNC.  Reading newc and crc is supported, but the writer
				// emits only POSIX odc; silently converting either format would make
				// -a/-u destructive even when the operation is rejected later.
				if o.format != "cpio" {
					fmt.Fprintf(rc.Err, "pax: cannot append %s data to an existing cpio archive\n", o.format)
					return 1
				}
				if string(data[:6]) != "070707" {
					fmt.Fprintln(rc.Err, "pax: updating or appending to newc/crc cpio archives is not supported")
					return 1
				}
				existingCPIOArchive = true
				existingCPIO, decodeErr = readCPIOEntries(data)
				if decodeErr != nil {
					fmt.Fprintf(rc.Err, "pax: %v\n", decodeErr)
					return 1
				}
				for _, entry := range existingCPIO {
					if entry.magic != "070707" {
						fmt.Fprintln(rc.Err, "pax: updating or appending to newc/crc cpio archives is not supported")
						return 1
					}
				}
				if o.newerOnly {
					o.archiveTimes = make(map[string]time.Time)
					for _, entry := range existingCPIO {
						mtime := time.Unix(int64(entry.mtime), 0)
						if old, ok := o.archiveTimes[entry.name]; !ok || mtime.After(old) {
							o.archiveTimes[entry.name] = mtime
						}
					}
				}
				appendExisting = true
			} else {
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
	}

	var out io.Writer = rc.Out
	var file archiveSink
	var blockPrefix []byte
	if archivePath != "" {
		flags := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
		if appendExisting && len(existing) != 0 && !existingCPIOArchive {
			flags = os.O_RDWR
		}
		var err error
		file, err = openArchiveSink(archivePath, flags, 0o644)
		if err != nil {
			fmt.Fprintf(rc.Err, "pax: %v\n", err)
			return 1
		}
		if appendExisting && len(existing) != 0 && !existingCPIOArchive {
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
	// POSIX fixes the default block size when the archive file is character
	// special. Inspect the selected sink itself: -f overrides stdout, but
	// stdout remains the archive file when -f is absent (or is "-") and may
	// itself be a terminal or another character device. Looking at a pathname
	// before opening it would both miss that case and introduce a TOCTOU race.
	if !o.blockExplicit {
		if statter, ok := out.(interface {
			Stat() (os.FileInfo, error)
		}); ok {
			info, err := statter.Stat()
			if err != nil {
				fmt.Fprintf(rc.Err, "pax: cannot inspect archive output: %v\n", err)
				if file != nil {
					if closeErr := file.Close(); closeErr != nil {
						fmt.Fprintf(rc.Err, "pax: %v\n", closeErr)
					}
				}
				return 1
			}
			if info.Mode()&os.ModeCharDevice != 0 {
				o.blockBytes = charSpecialBlockSize(o.format)
			}
		}
	}

	// POSIX pax always writes whole physical blocks; blockBytes is either the
	// explicit -b or the format default, never zero.
	blocker := newBlockWriter(out, o.blockBytes, blockPrefix)
	out = blocker
	if o.format == "cpio" {
		if writeStatus := writeCPIOMode(rc, o, out, files, existingCPIO); writeStatus != 0 {
			status = 1
		}
	} else {
		var logical bytes.Buffer
		tw := tar.NewWriter(&logical)
		globalInvalid, err := writeGlobalPAXHeader(rc, o, tw)
		if globalInvalid {
			fmt.Fprintln(rc.Err, "pax: global extended-header value cannot be translated; written as binary")
			status = 1
		}
		if err != nil {
			fmt.Fprintf(rc.Err, "pax: %v\n", err)
			status = 1
		}
		for _, name := range files {
			diagnosed, err := addPath(rc, o, tw, name)
			if err != nil && !errors.Is(err, errTraversalCycle) {
				fmt.Fprintf(rc.Err, "pax: %s: %v\n", name, err)
			}
			if err != nil || diagnosed {
				status = 1
			}
			if errors.Is(err, errInteractiveRename) {
				break
			}
			if errors.Is(err, errTraversalCycle) {
				break
			}
		}
		if err := tw.Close(); err != nil {
			fmt.Fprintf(rc.Err, "pax: %v\n", err)
			status = 1
		}
		logicalData, err := patchLinkdataHeaders(logical.Bytes())
		if err == nil {
			logicalData, err = filterDeletedPAXRecords(logicalData, o.paxOptions)
		}
		if err == nil && o.format == "pax" {
			logicalData, err = patchExtendedHeaderNames(logicalData, o.paxOptions.exthdrName)
		}
		if err != nil {
			fmt.Fprintf(rc.Err, "pax: %v\n", err)
			status = 1
		} else if _, err := out.Write(logicalData); err != nil {
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
	case fi.Mode()&os.ModeNamedPipe != 0:
		mode |= 0o010000
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

// addEntry copies an existing POSIX cpio member without normalizing its
// metadata.  Append and update are implemented as a rewrite because cpio's
// TRAILER!!! record must remain last; preserving the original fields also
// avoids changing an archive merely because it was updated.
func (w *cpioWriter) addEntry(entry cpioEntry) error {
	if entry.magic != "070707" {
		return fmt.Errorf("cannot rewrite cpio format %q", entry.magic)
	}
	if entry.namesize != uint64(len([]byte(entry.name))+1) || entry.filesize != uint64(len(entry.data)) {
		return fmt.Errorf("invalid cpio member %q", entry.name)
	}
	header := fmt.Sprintf("070707%06o%06o%06o%06o%06o%06o%06o%011o%06o%011o",
		entry.dev, entry.ino, entry.mode, entry.uid, entry.gid, entry.nlink,
		entry.rdev, entry.mtime, entry.namesize, entry.filesize)
	if len(header) != 76 {
		return fmt.Errorf("internal cpio header length %d", len(header))
	}
	if err := w.write(append([]byte(header), append([]byte(entry.name), 0)...)); err != nil {
		return err
	}
	return w.write(entry.data)
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
func writeCPIOMode(rc *tool.RunContext, o *options, out io.Writer, files []string, existing []cpioEntry) int {
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
		if errors.Is(err, errInteractiveRename) {
			break
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
	for _, entry := range existing {
		if err := w.addEntry(entry); err != nil {
			fmt.Fprintf(rc.Err, "pax: %s: %v\n", entry.name, err)
			status = 1
		}
	}
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
		out, keep, err := renameInteractively(o, out)
		if err != nil {
			return err
		}
		if !keep {
			return nil
		}
		if !newerThanArchive(o, out, e.fi.ModTime()) {
			return nil
		}
		var data []byte
		var dataErr error
		switch {
		case e.fi.Mode().IsRegular():
			data, dataErr = os.ReadFile(e.abs)
		case e.fi.Mode()&os.ModeSymlink != 0:
			var target string
			target, dataErr = os.Readlink(e.abs)
			data = []byte(target)
		}
		if dataErr != nil {
			return sourceTraversalErr(dataErr)
		}
		*members = append(*members, cpioMember{name: out, fi: e.fi, id: identityOf(e.fi), data: data})
		return nil
	})
}

func addPath(rc *tool.RunContext, o *options, tw *tar.Writer, name string) (bool, error) {
	invalidDiagnosed := false
	diagnosed, err := walkOperand(rc, o, name, func(e walkEntry) error {
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
		var invalidRename invalidCopyRename
		planKey := invalidCopyRenameKey(e)
		if plans := o.invalidRenamePlans[planKey]; len(plans) != 0 {
			index := o.invalidRenameUsed[planKey]
			if index < len(plans) {
				invalidRename = plans[index]
				o.invalidRenameUsed[planKey] = index + 1
			}
		}
		if invalidRename.skip {
			return nil
		}
		if invalidRename.nameSet {
			out = invalidRename.name
		}
		if invalidRename.linkSet {
			link = invalidRename.link
		}
		out, keep, err := renameInteractively(o, out)
		if err != nil {
			return err
		}
		if !keep {
			return nil
		}
		if !newerThanArchive(o, out, fi.ModTime()) {
			return nil
		}
		archiveOut, archiveLink := out, link
		if o.format == "pax" {
			var outEncodingErr, linkEncodingErr error
			archiveOut, outEncodingErr = localTextToArchive(rc, out)
			archiveLink, linkEncodingErr = localTextToArchive(rc, link)
			invalidValue := outEncodingErr != nil || linkEncodingErr != nil || invalidPAXLocalDestinationName(out) || link != "" && invalidPAXLocalDestinationName(link)
			if invalidValue && o.paxOptions.invalid != "binary" && o.paxOptions.invalid != "rename" {
				fmt.Fprintf(rc.Err, "pax: %s: value cannot be encoded as UTF-8; bypassed\n", out)
				invalidDiagnosed = true
				return nil
			}
			if outEncodingErr != nil {
				archiveOut = out
			}
			if linkEncodingErr != nil {
				archiveLink = link
			}
		}
		h, err := headerFor(archiveOut, fi, archiveLink)
		if err != nil {
			return sourceTraversalErr(err)
		}
		h.Format = tarFormat(o.format)
		if h.Format == tar.FormatPAX {
			if err := translatePAXIdentityToArchive(rc, h, o.paxOptions.invalid); err != nil {
				return err
			}
		}
		if o.paxOptions.times || o.read && o.write {
			if atime, ok := sourceAccessTimeFn(fi); ok {
				h.AccessTime = atime
			}
		}
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
				o.links[id.key()] = archiveOut
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
			invalidHeader, err := applyWritePAXOptions(rc, h, o.paxOptions)
			if err != nil {
				return err
			}
			// The preflight examined the effective path/linkpath values, so its
			// replacements take precedence over those same -o overrides.
			if invalidRename.nameSet {
				h.Name = archiveOut
			}
			if invalidRename.linkSet {
				h.Linkname = archiveLink
			}
			if invalidHeader {
				fmt.Fprintf(rc.Err, "pax: %s: value cannot be translated; written as binary\n", out)
				invalidDiagnosed = true
			}
			if hardlink && o.paxOptions.linkdata {
				linkDataSize := fi.Size()
				if effectiveWriteSizeOverride(o.paxOptions) {
					linkDataSize = h.Size
				}
				h.PAXRecords["COREUTILS.linkdata"] = h.Linkname
				h.Typeflag = tar.TypeReg
				h.Linkname = ""
				h.Size = linkDataSize
				hardlink = false
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
			var copyErr error
			if h.Size == fi.Size() {
				copyErr = copySourceFile(tw, f)
			} else {
				copyErr = copyArchiveMember(tw, f, h.Size)
			}
			if copyErr != nil {
				return copyErr
			}
		}
		if o.verbose {
			fmt.Fprintln(rc.Err, out)
		}
		return nil
	})
	return diagnosed || invalidDiagnosed, err
}

// copyArchiveMember writes exactly size bytes. A size extended-header
// override can shorten the live source or extend it; extension bytes are zero,
// matching the contents of the corresponding sparse tail after extraction.
func copyArchiveMember(dst io.Writer, src io.Reader, size int64) error {
	written, err := io.CopyN(dst, src, size)
	if err == nil {
		return nil
	}
	if !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return err
	}
	zeros := make([]byte, 32*1024)
	for remaining := size - written; remaining > 0; {
		chunk := int64(len(zeros))
		if remaining < chunk {
			chunk = remaining
		}
		n, writeErr := dst.Write(zeros[:int(chunk)])
		if writeErr != nil {
			return writeErr
		}
		if int64(n) != chunk {
			return io.ErrShortWrite
		}
		remaining -= chunk
	}
	return nil
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
	if len(operands) < 1 {
		return tool.UsageError(rc, cmd, "copy mode requires a target directory")
	}
	dest := operands[len(operands)-1]
	files := operands[:len(operands)-1]
	inputStatus := 0
	if len(files) == 0 {
		files, inputStatus = readPathnames(rc)
	}
	full := resolve(rc, dest)
	fi, err := os.Stat(full)
	if err != nil || !fi.IsDir() {
		fmt.Fprintf(rc.Err, "pax: %s: not a directory\n", dest)
		return 1
	}
	if o.link && !linkOptionsRequireMaterialCopy(o.paxOptions) {
		needsRename, preflightErr := linkCopyNeedsInvalidRename(rc, o, files)
		if preflightErr != nil {
			fmt.Fprintf(rc.Err, "pax: %v\n", preflightErr)
			return 1
		}
		if !needsRename {
			status := linkCopyMode(rc, o, files, full)
			if inputStatus != 0 && status == 0 {
				status = inputStatus
			}
			return status
		}
	}
	invalidRenameDiagnosed, err := preflightCopyInvalidRenames(rc, o, files)
	if err != nil {
		fmt.Fprintf(rc.Err, "pax: interactive rename: %v\n", err)
		return 1
	}

	pr, pw := io.Pipe()
	writerOptions := *o
	writerOptions.links = nil
	// diagCh carries the write side's already-printed traversal diagnostics to
	// the exit status; the channel receive after readMode is the synchronization
	// point.
	diagCh := make(chan bool, 1)
	go func() {
		tw := tar.NewWriter(pw)
		var werr error
		diagnosed := false
		globalInvalid, err := writeGlobalPAXHeader(rc, &writerOptions, tw)
		if globalInvalid {
			fmt.Fprintln(rc.Err, "pax: global extended-header value cannot be translated; written as binary")
			diagnosed = true
		}
		if err != nil {
			werr = err
		}
		for _, name := range files {
			if werr != nil {
				break
			}
			d, e := addPath(rc, &writerOptions, tw, name)
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
	sub.interactive = false
	sub.renamer = nil // already prompted on the write side; never prompt twice in copy mode
	inner := *rc
	inner.Dir = full
	inner.Stdio = rc.Stdio
	inner.In = pr
	status := readMode(&inner, &sub, nil)
	if (<-diagCh || invalidRenameDiagnosed || inputStatus != 0) && status == 0 {
		status = 1
	}
	return status
}

func preflightCopyInvalidRenames(rc *tool.RunContext, o *options, files []string) (diagnosed bool, retErr error) {
	if o.paxOptions.invalid != "rename" {
		return false, nil
	}
	var renamer *interactiveRenamer
	defer func() {
		if renamer != nil {
			if err := renamer.Close(); retErr == nil && err != nil {
				retErr = err
			}
		}
	}()
	o.invalidRenamePlans = make(map[string][]invalidCopyRename)
	o.invalidRenameUsed = make(map[string]int)
	for _, name := range files {
		_, err := walkOperand(rc, o, name, func(e walkEntry) error {
			plan := invalidCopyRename{}
			out := applySubstitutions(o.subst, e.member, nil)
			if value := o.paxOptions.global["path"]; value != "" {
				out = value
			}
			if value := o.paxOptions.local["path"]; value != "" {
				out = value
			}
			link := ""
			if e.fi.Mode()&os.ModeSymlink != 0 && !e.followed {
				var err error
				link, err = os.Readlink(e.abs)
				if err != nil {
					return err
				}
			}
			if value := o.paxOptions.global["linkpath"]; value != "" {
				link = value
			}
			if value := o.paxOptions.local["linkpath"]; value != "" {
				link = value
			}
			_, nameEncodingErr := localTextToArchive(rc, out)
			_, linkEncodingErr := localTextToArchive(rc, link)
			invalidName := nameEncodingErr != nil || invalidPAXLocalDestinationName(out)
			invalidLink := link != "" && (linkEncodingErr != nil || invalidPAXLocalDestinationName(link))
			if !invalidName && !invalidLink {
				return nil
			}
			diagnosed = true
			fmt.Fprintf(rc.Err, "pax: %s: value cannot be translated\n", out)
			if renamer == nil {
				var err error
				renamer, err = openInteractiveRenamer()
				if err != nil {
					return err
				}
			}
			if invalidName {
				replacement, keep, err := renamer.rename(out)
				if err != nil {
					return err
				}
				if !keep {
					plan.skip = true
					o.invalidRenamePlans[invalidCopyRenameKey(e)] = append(o.invalidRenamePlans[invalidCopyRenameKey(e)], plan)
					return nil
				}
				plan.name, plan.nameSet = replacement, true
			}
			if invalidLink {
				replacement, keep, err := renamer.rename(link)
				if err != nil {
					return err
				}
				if !keep {
					plan.skip = true
					o.invalidRenamePlans[invalidCopyRenameKey(e)] = append(o.invalidRenamePlans[invalidCopyRenameKey(e)], plan)
					return nil
				}
				plan.link, plan.linkSet = replacement, true
			}
			o.invalidRenamePlans[invalidCopyRenameKey(e)] = append(o.invalidRenamePlans[invalidCopyRenameKey(e)], plan)
			return nil
		})
		if err != nil {
			return diagnosed, err
		}
	}
	return diagnosed, nil
}

func invalidCopyRenameKey(e walkEntry) string { return e.abs + "\x00" + e.member }

var errLinkCopyNeedsInvalidRename = errors.New("invalid copy name requires rename")

func linkCopyNeedsInvalidRename(rc *tool.RunContext, o *options, files []string) (bool, error) {
	if o.paxOptions.invalid != "rename" {
		return false, nil
	}
	for _, name := range files {
		_, err := walkOperand(rc, o, name, func(e walkEntry) error {
			out := applySubstitutions(o.subst, e.member, nil)
			if value := o.paxOptions.global["path"]; value != "" {
				out = value
			}
			if value := o.paxOptions.local["path"]; value != "" {
				out = value
			}
			if invalidPAXDestination(rc, out) {
				return errLinkCopyNeedsInvalidRename
			}
			if e.fi.Mode()&os.ModeSymlink != 0 && !e.followed {
				link, readErr := os.Readlink(e.abs)
				if readErr != nil {
					return readErr
				}
				if value := o.paxOptions.global["linkpath"]; value != "" {
					link = value
				}
				if value := o.paxOptions.local["linkpath"]; value != "" {
					link = value
				}
				if invalidPAXDestination(rc, link) {
					return errLinkCopyNeedsInvalidRename
				}
			}
			return nil
		})
		if errors.Is(err, errLinkCopyNeedsInvalidRename) {
			return true, nil
		}
		if err != nil {
			return false, err
		}
	}
	return false, nil
}

// A hard link cannot have ownership, timestamps, size, or symlink contents
// that differ from its source.  When an extended-header override requests one
// of those member attributes, -l's "where possible" condition is false: use
// the archive copy lane rather than silently ignoring the option or mutating
// the source inode through the new name.
func linkOptionsRequireMaterialCopy(options paxOptions) bool {
	for _, values := range []map[string]string{options.global, options.local} {
		for key, value := range values {
			if value == "" {
				continue
			}
			switch key {
			case "atime", "gid", "gname", "linkpath", "mtime", "size", "uid", "uname":
				return true
			}
		}
	}
	return false
}

var linkSourceFn = defaultLinkSource

// linkCopyMode implements copy-mode -l directly against the filesystem. The
// ordinary copy lane deliberately travels through an archive to share its
// extraction safety rules; hard links cannot be represented that way because
// an archive hard-link member links two destination members, not a destination
// member to its live source. safeCopyTarget applies the same extraction planner
// to every transformed name before this lane touches the destination.
func linkCopyMode(rc *tool.RunContext, o *options, files []string, root string) int {
	status := 0
	stop := false
	var pendingDirs []pendingDirectoryAttributes
	for _, name := range files {
		diagnosed, err := walkOperand(rc, o, name, func(e walkEntry) error {
			out := applySubstitutions(o.subst, e.member, rc.Err)
			if out == "" {
				return nil
			}
			out, keep, err := renameInteractively(o, out)
			if err != nil {
				return err
			}
			if !keep {
				return nil
			}
			if value, ok := o.paxOptions.global["path"]; ok && value != "" {
				out = value
			}
			if value, ok := o.paxOptions.local["path"]; ok && value != "" {
				out = value
			}
			target, err := safeCopyTarget(root, out)
			if err != nil {
				return err
			}
			if o.noOverwrite {
				if _, err := os.Lstat(target); err == nil {
					return nil
				}
			}
			if o.newerOnly {
				if dst, err := os.Lstat(target); err == nil && !e.fi.ModTime().After(dst.ModTime()) {
					return nil
				}
			}
			attrErr, err := copyOneByLink(rc, o, e, target)
			if err != nil {
				return err
			}
			if e.fi.IsDir() {
				pendingDirs = append(pendingDirs, pendingDirectoryAttributes{
					name: out, path: target, attrs: attributesFromWalkEntry(e),
					normalMode: normalCreationMode(rc, e.fi.Mode()),
				})
			}
			if attrErr != nil {
				fmt.Fprintf(rc.Err, "pax: %s: %v\n", out, attrErr)
				status = 1
			}
			if o.verbose {
				fmt.Fprintln(rc.Err, out)
			}
			return nil
		})
		if err != nil && !errors.Is(err, errTraversalCycle) {
			fmt.Fprintf(rc.Err, "pax: %s: %v\n", name, err)
		}
		if err != nil || diagnosed {
			status = 1
		}
		if errors.Is(err, errTraversalCycle) || errors.Is(err, errInteractiveRename) {
			stop = true
		}
		if stop {
			break
		}
	}
	if finalizePendingDirectories(rc, o, pendingDirs) {
		status = 1
	}
	return status
}

func safeCopyTarget(root, name string) (string, error) {
	var archive bytes.Buffer
	tw := tar.NewWriter(&archive)
	if err := tw.WriteHeader(&tar.Header{Name: filepath.ToSlash(name), Typeflag: tar.TypeReg, Mode: 0o600}); err != nil {
		return "", err
	}
	if err := tw.Close(); err != nil {
		return "", err
	}
	plan, err := pax.PlanExtraction(bytes.NewReader(archive.Bytes()), root, pax.OSFS{})
	if err != nil {
		return "", err
	}
	if len(plan.Members) == 1 {
		return plan.Members[0].Target, nil
	}
	if len(plan.Rejected) == 1 && pax.IsDestinationExists(plan.Rejected[0].Reason) {
		return filepath.Join(root, filepath.FromSlash(plan.Rejected[0].Path)), nil
	}
	if len(plan.Rejected) != 0 {
		return "", fmt.Errorf("refusing %s: %s", plan.Rejected[0].Path, plan.Rejected[0].Reason)
	}
	return "", fmt.Errorf("refusing invalid destination name %q", name)
}

func attributesFromWalkEntry(e walkEntry) preservedAttributes {
	id := identityOf(e.fi)
	attrs := preservedAttributes{mode: e.fi.Mode(), mtime: e.fi.ModTime()}
	if id.ok {
		attrs.uid, attrs.gid = int(id.uid), int(id.gid)
	}
	if atime, ok := sourceAccessTimeFn(e.fi); ok {
		attrs.atime = atime
	}
	return attrs
}

// copyOneByLink returns preservation failures separately from copy failures so
// the caller can diagnose them, retain the copied file, and continue traversal.
func copyOneByLink(rc *tool.RunContext, o *options, e walkEntry, target string) (error, error) {
	if e.fi.IsDir() {
		return nil, prepareOutputDirectory(target, normalCreationMode(rc, e.fi.Mode()))
	}
	if err := mkdirAllNormal(filepath.Dir(target), intermediateDirMode(rc)); err != nil {
		return nil, err
	}
	source := e.abs
	if e.followed {
		resolved, err := filepath.EvalSymlinks(source)
		if err != nil {
			return nil, sourceTraversalErr(err)
		}
		source = resolved
	}
	if _, err := os.Lstat(target); err == nil {
		// Compare directory entries, not referents. Two distinct symlinks may
		// point at one file but are still distinct source/destination objects;
		// Stat would mistake them for a same-file collision and skip -l.
		sourceInfo, sourceErr := os.Lstat(source)
		targetInfo, targetErr := os.Lstat(target)
		if sourceErr == nil && targetErr == nil && os.SameFile(sourceInfo, targetInfo) {
			return nil, fmt.Errorf("source and destination are the same file")
		}
		if err := os.Remove(target); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	// Without preserved ownership, a set-ID source must be copied so the new
	// inode can have those bits cleared without mutating the live source inode.
	canLink := o.preservation.owner || e.fi.Mode()&(os.ModeSetuid|os.ModeSetgid) == 0
	if canLink {
		if err := linkSourceFn(source, target); err == nil {
			return nil, nil
		}
	}
	// POSIX says "whenever possible": a cross-device filesystem or a platform
	// that cannot hard-link symlinks falls back to the normal copy semantics.
	switch {
	case e.fi.Mode().IsRegular():
		in, err := os.Open(source)
		if err != nil {
			return nil, sourceTraversalErr(err)
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			in.Close()
			return nil, err
		}
		copyErr := copySourceFile(out, in)
		closeOutErr := out.Close()
		closeInErr := in.Close()
		if copyErr != nil {
			return nil, copyErr
		}
		if closeOutErr != nil {
			return nil, closeOutErr
		}
		if closeInErr != nil {
			return nil, sourceTraversalErr(closeInErr)
		}
		if err := os.Chmod(target, normalCreationMode(rc, e.fi.Mode())); err != nil {
			return nil, err
		}
	case e.fi.Mode()&os.ModeSymlink != 0 && !e.followed:
		link, err := os.Readlink(source)
		if err != nil {
			return nil, sourceTraversalErr(err)
		}
		if err := os.Symlink(link, target); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported source type %s", e.fi.Mode())
	}
	attrs := attributesFromWalkEntry(e)
	return applyPreservedAttributes(target, attrs, o.preservation, e.fi.Mode()&os.ModeSymlink != 0 && !e.followed), nil
}
