package paxcmd

import (
	"archive/tar"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/qiangli/coreutils/cmds/internal/tzenv"
	"github.com/qiangli/coreutils/pkg/locale"
	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "pax",
	Synopsis: "Portable archive interchange.",
	Usage: `pax [-cdnv] [-H|-L] [-f archive] [-s replstr] [pattern...]
  pax -r [-cdiknuv] [-H|-L] [-f archive] [-p string]... [-s replstr] [pattern...]
  pax -w [-dituvX] [-H|-L] [-b blocksize] [-a] [-f archive] [-s replstr] [-x format] [file...]
  pax -r -w [-diklntuvX] [-H|-L] [-p string]... [-s replstr] file... directory`,
}

func init() { cmd.Run = run; tool.Register(cmd) }

type options struct {
	read, write     bool
	archive         string
	verbose         bool
	format          string
	preservation    preservation
	subst           []substitution
	interactive     bool
	link            bool // -l (copy mode only)
	noOverwrite     bool // -k
	newerOnly       bool // -u
	dirsNoDescend   bool // -d
	appendMode      bool // -a
	invertMatch     bool // -c
	selectNoPattern bool // -n

	blocksize     string // -b, parsed into blockBytes after option validation
	blockBytes    int
	blockExplicit bool // true when -b was given, so a char-special sink must not override the default
	archiveTimes  map[string]time.Time
	// links maps a source (device, inode) to the first name archived for it,
	// so later names for the same inode become hardlink members.
	links      map[devIno]string
	paxOptions paxOptions
	timeFormat *locale.TimeFormatter
	now        func() time.Time // invocation-local clock for age-sensitive verbose timestamps
	t, X       bool
	follow     followMode // -H/-L; the last one given wins
	renamer    *interactiveRenamer
	// invalid=rename copy-mode answers are collected before the archive pipe
	// starts, so a terminal failure cannot leave a partially copied tree.
	invalidRenamePlans map[string][]invalidCopyRename
	invalidRenameUsed  map[string]int
}

type invalidCopyRename struct {
	name, link       string
	nameSet, linkSet bool
	skip             bool
}

// followFlag is the pflag value behind -H and -L. Each occurrence overwrites
// options.follow, so with repeated or mixed -H/-L the LAST one on the command
// line wins — pflag calls Set in argument order, including inside clustered
// short options, which a pair of plain bools cannot observe.
type followFlag struct {
	o    *options
	mode followMode
}

func (f *followFlag) String() string { return "false" }
func (f *followFlag) Type() string   { return "bool" }
func (f *followFlag) Set(s string) error {
	v, err := strconv.ParseBool(s)
	if err != nil {
		return err
	}
	if v {
		f.o.follow = f.mode
	}
	return nil
}

func run(rc *tool.RunContext, args []string) int {
	args = tool.AliasHelpVersion(args)
	fs := tool.NewFlags(cmd.Name)
	// Utility Syntax Guideline 10: the first operand ends option parsing.
	// This is significant for portable archive pathnames such as "-d" and
	// "--", which must remain operands when they follow an earlier operand.
	fs.SetInterspersed(false)
	o := options{now: time.Now}
	fs.BoolVarP(&o.read, "read", "r", false, "read (extract) from the archive")
	fs.BoolVarP(&o.write, "write", "w", false, "write (create) an archive")
	fs.StringVarP(&o.archive, "file", "f", "", "archive pathname (default stdin/stdout)")
	fs.BoolVarP(&o.verbose, "verbose", "v", false, "verbose output")
	fs.StringVarP(&o.format, "format", "x", "pax", "archive format: pax, ustar, or cpio")
	preserve := fs.StringArrayP("preserve", "p", nil, "preserve file attributes (ordered characters a, e, m, o, p; repeatable)")
	subst := fs.StringArrayP("subst", "s", nil, "rewrite member names with an ed-style substitution")
	fs.BoolVarP(&o.interactive, "interactive", "i", false, "rename members interactively")
	fs.BoolVarP(&o.link, "link", "l", false, "hard-link rather than copy where possible")
	fs.BoolVarP(&o.noOverwrite, "keep", "k", false, "do not overwrite existing files")
	fs.BoolVarP(&o.newerOnly, "update", "u", false, "extract or write only newer files")
	fs.BoolVarP(&o.dirsNoDescend, "no-descend", "d", false, "do not descend into directories")
	fs.BoolVarP(&o.appendMode, "append", "a", false, "append to the archive")
	fs.BoolVarP(&o.invertMatch, "complement", "c", false, "select members NOT matching the patterns")
	fs.BoolVarP(&o.selectNoPattern, "first", "n", false, "select only the first match per pattern")

	fs.StringVarP(&o.blocksize, "blocksize", "b", "", "physical block size: decimal factors joined by 'x', each optionally suffixed b (512), k (1024), or m (1048576); must be a positive multiple of 512 up to 32256 (default 10240, or 5120 for -x cpio; to a character-special archive the POSIX device default applies: pax and cpio 5120, ustar 10240)")
	optionArgs := fs.StringArrayP("options", "o", nil, "POSIX pax extended-header and algorithm options (repeatable)")
	fs.BoolVarP(&o.t, "t", "t", false, "reset access times")
	fs.BoolVarP(&o.X, "X", "X", false, "do not descend into directories on a different device")
	fs.VarPF(&followFlag{o: &o, mode: followCmdline}, "H", "H", "follow symlinks named as command-line operands").NoOptDefVal = "true"
	fs.VarPF(&followFlag{o: &o, mode: followAll}, "L", "L", "follow all symlinks").NoOptDefVal = "true"

	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}

	if o.invertMatch && o.selectNoPattern {
		return tool.UsageError(rc, cmd, "-c and -n cannot be used together")
	}
	isList := !o.read && !o.write
	isRead := o.read && !o.write
	isWrite := !o.read && o.write
	o.preservation = defaultPreservation()
	if fs.Changed("preserve") {
		if !o.read {
			return tool.UsageError(rc, cmd, "-p is valid only in read or copy mode")
		}
		policy, err := parsePreservation(*preserve)
		if err != nil {
			return tool.UsageError(rc, cmd, "%v", err)
		}
		o.preservation = policy
	}

	if fs.Changed("blocksize") {
		if !isWrite {
			return tool.UsageError(rc, cmd, "-b is valid only in write mode")
		}
		blockBytes, err := parseBlockSize(o.blocksize)
		if err != nil {
			return tool.UsageError(rc, cmd, "%v", err)
		}
		o.blockBytes = blockBytes
	}
	mode := paxList
	switch {
	case o.read && o.write:
		mode = paxCopy
	case o.read:
		mode = paxRead
	case o.write:
		mode = paxWrite
	}
	parsedOptions, err := parsePAXOptions(*optionArgs, mode, o.format)
	if err != nil {
		return tool.UsageError(rc, cmd, "%v", err)
	}
	o.paxOptions = parsedOptions
	if mode == paxList {
		usesTime := false
		if o.verbose && o.paxOptions.listSet {
			usesTime, err = listFormatUsesTime(o.paxOptions.listFormat)
			if err != nil {
				return tool.UsageError(rc, cmd, "listopt: %v", err)
			}
		}
		if o.verbose && (usesTime || !o.paxOptions.listSet) {
			formatter, resolveErr := locale.ResolveTime(rc.Env)
			if resolveErr != nil {
				fmt.Fprintf(rc.Err, "pax: %v\n", resolveErr)
				return 1
			}
			o.timeFormat = &formatter
		}
	}
	if fs.Changed("t") {
		if isList || isRead {
			return tool.UsageError(rc, cmd, "-t is valid only in write or copy mode")
		}
	}
	// -X is legal only where a hierarchy is traversed; -H/-L are legal in
	// every mode (POSIX lists them in all four synopsis forms) and simply
	// have nothing to do in list and read modes, which traverse no hierarchy.
	if fs.Changed("X") && (isList || isRead) {
		return tool.UsageError(rc, cmd, "-X is valid only in write or copy mode")
	}
	if o.link {
		if !(o.read && o.write) {
			return tool.UsageError(rc, cmd, "-l is valid only in copy mode")
		}
	}
	for _, s := range *subst {
		sub, err := parseSubstitution(s)
		if err != nil {
			return tool.UsageError(rc, cmd, "%v", err)
		}
		o.subst = append(o.subst, sub)
	}
	switch o.format {
	case "pax", "ustar", "cpio":
	default:
		return tool.UsageError(rc, cmd, "unsupported format %q; pax, ustar, and cpio are supported", o.format)
	}
	// Physical blocking is not opt-in: POSIX pax always writes whole blocks.
	// An explicit -b wins; otherwise the format's documented default applies.
	// writeMode may further lower the default to the POSIX character-special
	// value once it can inspect the selected output sink (see charSpecialBlockSize).
	o.blockExplicit = fs.Changed("blocksize")
	if !o.blockExplicit {
		o.blockBytes = defaultBlockSize(o.format)
	}

	if o.interactive {
		r, err := openInteractiveRenamer()
		if err != nil {
			fmt.Fprintf(rc.Err, "pax: interactive rename: %v\n", err)
			return 1
		}
		o.renamer = r
	}

	status := 0
	switch {
	case o.read && o.write:
		status = copyMode(rc, &o, operands)
	case o.read:
		status = readMode(rc, &o, operands)
	case o.write:
		status = writeMode(rc, &o, operands)
	default:
		status = listMode(rc, &o, operands)
	}
	if o.renamer != nil {
		if err := o.renamer.Close(); err != nil {
			fmt.Fprintf(rc.Err, "pax: interactive rename: close /dev/tty: %v\n", err)
			status = 1
		}
	}
	return status
}

// parseBlockSize implements the POSIX pax blocksize grammar. A size is one or
// more factors joined by 'x' (their product), each factor a decimal digit
// string optionally suffixed with a multiplier: 'b' = 512, 'k' = 1024,
// 'm' = 1048576. The product must be positive, a multiple of 512, and no
// larger than the 32256-byte maximum, so "10k" (10240) is accepted while a
// bare "513" is not.
func parseBlockSize(value string) (int, error) {
	invalid := func() error {
		return fmt.Errorf("invalid block size %q: expected decimal factors joined by 'x', each optionally suffixed b (512), k (1024), or m (1048576)", value)
	}
	if value == "" {
		return 0, invalid()
	}
	product := uint64(1)
	for _, factor := range strings.Split(value, "x") {
		if factor == "" {
			return 0, invalid()
		}
		multiplier := uint64(1)
		switch factor[len(factor)-1] {
		case 'b':
			multiplier = 512
		case 'k':
			multiplier = 1024
		case 'm':
			multiplier = 1048576
		}
		digits := factor
		if multiplier != 1 {
			digits = factor[:len(factor)-1]
		}
		if digits == "" {
			return 0, invalid()
		}
		for _, r := range digits {
			if r < '0' || r > '9' {
				return 0, invalid()
			}
		}
		n, err := strconv.ParseUint(digits, 10, 64)
		if err != nil {
			return 0, invalid()
		}
		// Every step is checked: a size expression is attacker-reachable
		// through a script, and a wrapped product would silently become a
		// small, legal-looking block size.
		scaled, err := mulChecked(n, multiplier)
		if err != nil {
			return 0, fmt.Errorf("invalid block size %q: %v", value, err)
		}
		product, err = mulChecked(product, scaled)
		if err != nil {
			return 0, fmt.Errorf("invalid block size %q: %v", value, err)
		}
	}
	if product == 0 {
		return 0, fmt.Errorf("invalid block size %q: must be positive", value)
	}
	if product%512 != 0 {
		return 0, fmt.Errorf("invalid block size %q: must be a multiple of 512 bytes", value)
	}
	if product > maxBlockSize {
		return 0, fmt.Errorf("invalid block size %q: maximum supported size is %d bytes", value, maxBlockSize)
	}
	return int(product), nil
}

// maxBlockSize is the POSIX pax upper bound on -b.
const maxBlockSize = 32256

func mulChecked(a, b uint64) (uint64, error) {
	if a == 0 || b == 0 {
		return 0, nil
	}
	if a > math.MaxUint64/b {
		return 0, fmt.Errorf("size overflows")
	}
	return a * b, nil
}

// defaultBlockSize is the physical block size pax uses when -b is absent and
// the archive is not known to be a character-special file. POSIX Issue 7
// states default blocksizes only "for character special archive files" and
// leaves every other sink implementation-defined, so this is a bashy choice:
// 10240 bytes (20 512-byte records) for the tar-derived formats and 5120 for
// cpio. Character-special sinks instead take the exact POSIX-mandated value;
// see charSpecialBlockSize.
func defaultBlockSize(format string) int {
	if format == "cpio" {
		return 5120
	}
	return 10240
}

// charSpecialBlockSize is the default physical block size POSIX Issue 7
// mandates when the archive is a character-special file, keyed by -x format:
//
//	ustar -> 10240   pax -> 5120   cpio -> 5120
//
// The pax value is the load-bearing one: it is 5120, NOT the 10240 that ustar
// (and our implementation-defined default) uses, so writing the pax format to
// a device must lower the block size to match the spec exactly. writeMode
// calls this only when the selected output sink reports character-special and
// no explicit -b was supplied.
func charSpecialBlockSize(format string) int {
	if format == "ustar" {
		return 10240
	}
	return 5120
}

// resolve makes a relative operand absolute against the caller's directory
// rather than the process's, which is what an embedded shell requires.
func resolve(rc *tool.RunContext, p string) string {
	if p == "" || filepath.IsAbs(p) {
		return p
	}
	if rc != nil && rc.Dir != "" {
		return filepath.Join(rc.Dir, p)
	}
	return p
}

func openArchive(rc *tool.RunContext, o *options) (io.ReadCloser, error) {
	if o.archive == "" {
		return io.NopCloser(rc.In), nil
	}
	return os.Open(resolve(rc, o.archive))
}

// listMode is pax with neither -r nor -w: report the archive's contents.
func listMode(rc *tool.RunContext, o *options, patterns []string) int {
	return listModeWithOpener(rc, o, patterns, openArchive)
}

type archiveOpener func(*tool.RunContext, *options) (io.ReadCloser, error)

func listModeWithOpener(rc *tool.RunContext, o *options, patterns []string, open archiveOpener) int {
	if o.timeFormat == nil {
		usesTime := false
		if o.verbose && o.paxOptions.listSet {
			var formatErr error
			usesTime, formatErr = listFormatUsesTime(o.paxOptions.listFormat)
			if formatErr != nil {
				fmt.Fprintf(rc.Err, "pax: listopt: %v\n", formatErr)
				return 1
			}
		}
		if o.verbose && (usesTime || !o.paxOptions.listSet) {
			formatter, resolveErr := locale.ResolveTime(rc.Env)
			if resolveErr != nil {
				fmt.Fprintf(rc.Err, "pax: %v\n", resolveErr)
				return 1
			}
			o.timeFormat = &formatter
		}
	}
	r, err := open(rc, o)
	if err != nil {
		fmt.Fprintf(rc.Err, "pax: %v\n", err)
		return 1
	}
	raw, readErr := io.ReadAll(r)
	closeErr := r.Close()
	if readErr != nil || closeErr != nil {
		if readErr != nil {
			fmt.Fprintf(rc.Err, "pax: %v\n", readErr)
		}
		if closeErr != nil {
			fmt.Fprintf(rc.Err, "pax: close archive: %v\n", closeErr)
		}
		return 1
	}
	archive, err := decodeArchive(raw)
	if err != nil {
		fmt.Fprintf(rc.Err, "pax: %v\n", err)
		return 1
	}
	// A pax stream with no extended records is byte-for-byte ustar.  It is not
	// possible to distinguish those cases, so only cpio is a provably
	// inapplicable input format.
	if o.paxOptions.needsPAX && archive.kind == archiveCPIO {
		fmt.Fprintln(rc.Err, "pax: -o option is applicable only to a pax archive")
		return 1
	}
	tarData, err := filterDeletedPAXRecords(archive.tarData, o.paxOptions)
	if err != nil {
		fmt.Fprintf(rc.Err, "pax: %v\n", err)
		return 1
	}
	tr := newOptionTarReader(tarData, o.paxOptions, true)
	var members []*tar.Header
	var invalidMembers []bool
	for {
		h, nextErr := tr.Next()
		if nextErr == io.EOF {
			break
		}
		if nextErr != nil {
			fmt.Fprintf(rc.Err, "pax: %v\n", nextErr)
			return 1
		}
		invalid := translatePAXHeaderToLocal(rc, h, o.paxOptions.invalid, true)
		copyHeader := *h
		members = append(members, &copyHeader)
		invalidMembers = append(invalidMembers, invalid.name || invalid.link || invalid.other)
	}

	sel := newSelector(o, patterns)
	catalog := make([]selectorMember, 0, len(members))
	for _, h := range members {
		catalog = append(catalog, selectorMember{
			name:  h.Name,
			isDir: h.Typeflag == tar.TypeDir || strings.HasSuffix(h.Name, "/"),
		})
	}
	sel.prime(catalog)

	// Resolve the complete selected-name plan before emitting anything. This is
	// necessary for a link that precedes its target: the displayed target must
	// still follow a later interactive rename.
	status := 0
	effectiveNames := make(map[int]string)
	originalOccurrences := make(map[string][]int)
	now := time.Now()
	if o.now != nil {
		now = o.now()
	}
	for index, h := range members {
		originalOccurrences[h.Name] = append(originalOccurrences[h.Name], index)
	}
	substitutedNames := make(map[int]string)
	substitutedOccurrences := make(map[string][]int)
	for index, h := range members {
		isDir := h.Typeflag == tar.TypeDir || strings.HasSuffix(h.Name, "/")
		if !sel.keep(h.Name, isDir) {
			continue
		}
		if invalidMembers[index] {
			fmt.Fprintf(rc.Err, "pax: %s: value cannot be translated\n", h.Name)
			status = 1
		}
		subName := applySubstitutions(o.subst, h.Name, rc.Err)
		if subName == "" {
			continue
		}
		substitutedNames[index] = subName
		substitutedOccurrences[subName] = append(substitutedOccurrences[subName], index)
		name, keep, err := renameInteractively(o, subName)
		if err != nil {
			fmt.Fprintf(rc.Err, "pax: interactive rename: %v\n", err)
			return 1
		}
		if !keep {
			continue
		}
		effectiveNames[index] = name
	}

	for index, h := range members {
		name, keep := effectiveNames[index]
		if !keep {
			continue
		}
		linkTarget := h.Linkname
		if linkTarget != "" {
			linkTarget = applySubstitutions(o.subst, linkTarget, nil)
			if targetIndex, ok := listTargetOccurrence(index, h.Linkname, linkTarget,
				originalOccurrences, substitutedOccurrences); ok {
				if r, kept := effectiveNames[targetIndex]; kept {
					linkTarget = r
				} else if r, substituted := substitutedNames[targetIndex]; substituted {
					linkTarget = r
				}
			}
		}

		if o.verbose && o.paxOptions.listSet {
			hCopy := effectiveListHeader(h, name, linkTarget)
			line, err := formatPAXList(hCopy, o.paxOptions.listFormat, tzenv.Location(rc.Env), o.timeFormat)
			if err != nil {
				fmt.Fprintf(rc.Err, "pax: listopt: %v\n", err)
				return 1
			}
			if _, err := fmt.Fprintln(rc.Out, line); err != nil {
				fmt.Fprintf(rc.Err, "pax: write error: %v\n", err)
				return 1
			}
		} else if o.verbose {
			stamp, err := verboseListTimestamp(h.ModTime, now, tzenv.Location(rc.Env), o.timeFormat)
			if err != nil {
				fmt.Fprintf(rc.Err, "pax: time format: %v\n", err)
				return 1
			}
			var writeErr error
			nlink := listNlink(h)
			size := h.Size
			if h.Typeflag == tar.TypeSymlink {
				size = int64(len([]byte(linkTarget)))
			}
			switch h.Typeflag {
			case tar.TypeLink:
				_, writeErr = fmt.Fprintf(rc.Out, "%s %2d %-8s %-8s %8d %s %s == %s\n",
					headerModeString(h), nlink, h.Uname, h.Gname, size, stamp, name, linkTarget)
			case tar.TypeSymlink:
				_, writeErr = fmt.Fprintf(rc.Out, "%s %2d %-8s %-8s %8d %s %s -> %s\n",
					headerModeString(h), nlink, h.Uname, h.Gname, size, stamp, name, linkTarget)
			default:
				_, writeErr = fmt.Fprintf(rc.Out, "%s %2d %-8s %-8s %8d %s %s\n",
					headerModeString(h), nlink, h.Uname, h.Gname, size, stamp, name)
			}
			if writeErr != nil {
				fmt.Fprintf(rc.Err, "pax: write error: %v\n", writeErr)
				return 1
			}
		} else {
			if _, err := fmt.Fprintln(rc.Out, name); err != nil {
				fmt.Fprintf(rc.Err, "pax: write error: %v\n", err)
				return 1
			}
		}
	}

	unmatched := sel.unmatched()
	if len(unmatched) > 0 {
		for _, p := range unmatched {
			fmt.Fprintf(rc.Err, "pax: pattern %q not matched\n", p)
		}
		return 1
	}

	return status
}

// paxSixMonths is the GNU-compatible recent-file cutoff already used by
// cmds/ls: half of 365.2425 days. POSIX pax requires its default verbose
// timestamp to have the ls -l shape, which uses the time for a file modified
// within the last six months and the year at the boundary or for an older or
// future timestamp.
const paxSixMonths = 15778476 * time.Second

func verboseListTimestamp(modTime, now time.Time, loc *time.Location, formatter *locale.TimeFormatter) (string, error) {
	format := "%b %e %H:%M"
	if modTime.After(now) || now.Sub(modTime) >= paxSixMonths {
		format = "%b %e  %Y"
	}
	return formatter.Format(modTime.In(loc), format)
}

func listTargetOccurrence(linkIndex int, originalTarget, substitutedTarget string,
	originalOccurrences, substitutedOccurrences map[string][]int) (int, bool) {
	// Prefer the archive identity named by the link header. In particular, a
	// later member whose original name collides with a substituted pathname
	// must not steal an earlier hard link's target.
	occurrences := originalOccurrences[originalTarget]
	if len(occurrences) == 0 {
		occurrences = substitutedOccurrences[substitutedTarget]
	}
	previous := -1
	for _, index := range occurrences {
		if index == linkIndex {
			continue
		}
		if index < linkIndex {
			previous = index
			continue
		}
		if previous >= 0 {
			return previous, true
		}
		return index, true
	}
	if previous >= 0 {
		return previous, true
	}
	return 0, false
}

func effectiveListHeader(h *tar.Header, name, linkTarget string) *tar.Header {
	copyHeader := *h
	copyHeader.Name = name
	copyHeader.Linkname = linkTarget
	if h.PAXRecords != nil {
		copyHeader.PAXRecords = make(map[string]string, len(h.PAXRecords))
		for key, value := range h.PAXRecords {
			copyHeader.PAXRecords[key] = value
		}
		if _, ok := copyHeader.PAXRecords["path"]; ok {
			copyHeader.PAXRecords["path"] = name
		}
		for _, key := range []string{"linkpath", "linkname"} {
			if _, ok := copyHeader.PAXRecords[key]; ok {
				copyHeader.PAXRecords[key] = linkTarget
			}
		}
	}
	return &copyHeader
}

func listNlink(h *tar.Header) uint64 {
	for _, key := range []string{"SCHILY.nlink", "COREUTILS.cpio.c_nlink"} {
		if value, ok := h.PAXRecords[key]; ok {
			if nlink, err := strconv.ParseUint(value, 10, 64); err == nil && nlink > 0 {
				return nlink
			}
		}
	}
	if h.Typeflag == tar.TypeLink {
		return 2
	}
	return 1
}

func headerFor(path string, fi os.FileInfo, link string) (*tar.Header, error) {
	h, err := tar.FileInfoHeader(fi, link)
	if err != nil {
		return nil, err
	}
	h.Name = filepath.ToSlash(path)
	if fi.IsDir() && !strings.HasSuffix(h.Name, "/") {
		h.Name += "/"
	}
	return h, nil
}

func tarFormat(name string) tar.Format {
	if name == "ustar" {
		return tar.FormatUSTAR
	}
	return tar.FormatPAX
}
