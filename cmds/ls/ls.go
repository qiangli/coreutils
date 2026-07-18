// Package lscmd implements ls(1) per the GNU coreutils manual for the
// flag subset -l -a -A -d -R -r -t -S -1 -h -i.
//
// Deterministic-output contract: names sort in C-locale byte order and
// color is never emitted. This userland has no terminal, so the output
// matches what GNU ls produces when writing to a non-tty: -1 is the
// default format, and non-printable characters are written literally
// unless -q asks otherwise. The multi-column (-C, -x) and stream (-m)
// formats are still available on request; because there is no terminal
// to measure, their line width comes from -w, else COLUMNS, else the
// 80-column fallback GNU uses off a tty.
//
// Platform note: owner/group in -l come from os/user on unix (falling
// back to the numeric ID when the name cannot be resolved, as GNU
// does); on Windows they are a best-effort SID account-name lookup
// (blank when unavailable), and inode (-i), link count, and block
// count have no portable Windows equivalent — they report 0, 1, and a
// size-derived value respectively.
package lscmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/qiangli/coreutils/tool"
	"github.com/spf13/pflag"
)

var cmd = &tool.Tool{
	Name:     "ls",
	Synopsis: "List information about the FILEs (the current directory by default).",
	Usage:    "ls [OPTION]... [FILE]...",
}

// Run is wired in init: a literal would create an initialization
// cycle (run's flag-error paths reference cmd).
func init() { cmd.Run = run; tool.Register(cmd) }

type options struct {
	long, all, almostAll, dirOnly, recursive bool
	reverse, sortTime, sortSize              bool
	inode, human                             bool
	noGroup, numeric                         bool
	sizeBlocks                               bool
	classify, fileType, slashDirs            bool
	zero, comma                              bool
	unsorted, sortExtension, sortVersion     bool
	groupDirsFirst                           bool
	literal, quoteName, escape               bool
	hideControl                              bool
	deref                                    bool
	si                                       bool
	format                                   fmtKind
	width                                    int
	sizeUnit                                 uint64 // -l size column divisor
	blockUnit                                uint64 // -s / total block divisor
	timeStyle                                timeStyle
	timeSel                                  timeSel
	hide, ignore                             []string
}

// fmtKind is the output format, selected by the last of -l/-1/-C/-x/-m/
// -g/-o/-n/--format on the command line (POSIX: format options are
// mutually exclusive and the last one wins).
type fmtKind int

const (
	fmtOnePerLine fmtKind = iota
	fmtLong
	fmtColumns // -C: entries sorted down the columns
	fmtAcross  // -x: entries sorted across the rows
	fmtCommas  // -m: comma-separated stream
)

// defaultWidth is the line width ls assumes when neither -w nor
// COLUMNS says otherwise. This userland has no terminal to query, so
// the GNU non-tty fallback of 80 columns applies.
const defaultWidth = 80

// unlimitedWidth stands in for "-w 0", which GNU documents as no line
// limit at all.
const unlimitedWidth = 1 << 30

// timeStyle is the -l timestamp rendering selected by --time-style.
type timeStyle int

const (
	styleLocale  timeStyle = iota // "Jan  2 15:04" / "Jan  2  2006"
	styleLongISO                  // "2006-01-02 15:04"
	styleFullISO                  // "2006-01-02 15:04:05.000000000 -0700"
	styleISO                      // "01-02 15:04" recent, "2006-01-02" otherwise
)

// defaultBlockUnit is the unit GNU counts -s blocks and the "total"
// line in when neither --block-size nor -k says otherwise.
const defaultBlockUnit = 1024

// timeSel is the timestamp field shown in -l and used for time
// sorting, selected by --time=WORD, -c, or -u.
type timeSel int

const (
	selMtime timeSel = iota
	selAtime
	selCtime
	selBirth
)

// sysInfo is the platform-dependent slice of an entry's metadata,
// filled by sysOf in sys_unix.go / sys_windows.go.
type sysInfo struct {
	nlink                uint64
	owner, group         string
	blocks512            uint64 // disk usage in 512-byte units
	rdevMajor, rdevMinor uint32 // device numbers for block/char specials
}

type entry struct {
	name   string // display name
	path   string // resolved path for stat operations
	info   os.FileInfo
	tm     time.Time // the --time/-c/-u-selected timestamp
	target string    // symlink target, filled only for -l
}

// GetFlagSet returns the FlagSet containing all ls options, configured with the given command name.
func GetFlagSet(name string) *pflag.FlagSet {
	fs := tool.NewFlags(name)
	fs.BoolP("all", "a", false, "do not ignore entries starting with .")
	fs.BoolP("almost-all", "A", false, "do not list implied . and ..")
	fs.BoolP("directory", "d", false, "list directories themselves, not their contents")
	fs.BoolP("human-readable", "h", false, "with -l, print sizes like 1K 234M 2G etc.")
	fs.BoolP("inode", "i", false, "print the index number of each file")
	fs.BoolP("recursive", "R", false, "list subdirectories recursively")
	fs.BoolP("reverse", "r", false, "reverse order while sorting")
	fs.BoolP("long", "l", false, "display detailed information")
	fs.BoolP("no-group", "G", false, "in a long listing, don't print group names")
	fs.BoolP("numeric-uid-gid", "n", false, "like -l, but list numeric user and group IDs")
	fs.String("format", "", "set display format: long, single-column, commas")
	fs.String("sort", "", "sort by WORD: name, none, time, size, extension")
	fs.StringArray("hide", nil, "do not list implied entries matching shell PATTERN")
	fs.StringArrayP("ignore", "I", nil, "do not list implied entries matching shell PATTERN")
	fs.BoolP("ignore-backups", "B", false, "do not list implied entries ending with ~")
	fs.Bool("zero", false, "end each output line with NUL, not newline")
	fs.Bool("file-type", false, "append file type indicators except '*'")
	fs.StringP("classify", "F", "", "append file type indicators")
	fs.String("indicator-style", "", "append indicator with style WORD: none, slash, file-type, classify")
	fs.BoolP("literal", "N", false, "print entry names without quoting")
	fs.BoolP("quote-name", "Q", false, "enclose entry names in double quotes")
	fs.BoolP("escape", "b", false, "print C-style escapes for nongraphic characters")
	fs.Bool("hide-control-chars", false, "print question marks instead of nongraphic characters")
	fs.Bool("show-control-chars", false, "show nongraphic characters as-is")
	fs.String("quoting-style", "", "set quoting style: literal, c, escape")
	fs.Bool("group-directories-first", false, "group directories before files")
	fs.BoolP("dereference", "L", false, "show file information for symlink referents")
	fs.BoolP("dereference-command-line", "H", false, "follow command-line symlinks")
	fs.Bool("dereference-command-line-symlink-to-dir", false, "follow command-line symlinked directories")
	fs.Bool("author", false, "with -l, print the author of each file")
	fs.Bool("context", false, "print security context when available")
	fs.String("color", "never", "control color output; accepted for compatibility")
	fs.String("hyperlink", "never", "control hyperlink output; accepted for compatibility")
	fs.IntP("width", "w", 0, "set output width; accepted for compatibility")
	fs.IntP("tabsize", "T", 8, "set tab stops; accepted for compatibility")
	fs.String("time", "", "select timestamp field: mtime/modification, atime/access/use, ctime/status, birth/creation")
	fs.String("time-style", "", "set time style for -l; full-iso supported")
	fs.Bool("full-time", false, "like -l --time-style=full-iso")
	fs.String("block-size", "", "scale block counts; supports 1, K, KB")
	fs.Bool("si", false, "print human-readable sizes in powers of 1000")
	fs.BoolP("size", "s", false, "print allocated size of each file, in blocks")
	fs.BoolP("kibibytes", "k", false, "use 1024-byte blocks for allocated sizes")
	fs.BoolP("dired", "D", false, "accepted for compatibility")

	// Short-only options that do not have canonical long options in GNU ls,
	// but are fully supported via the short flag clusters.
	fs.BoolP("1", "1", false, "list one file per line")
	fs.BoolP("t", "t", false, "sort by modification time, newest first")
	fs.BoolP("S", "S", false, "sort by file size, largest first")
	fs.BoolP("v", "v", false, "natural sort of (version) numbers within text")
	fs.BoolP("g", "g", false, "like -l, but do not list owner")
	fs.BoolP("o", "o", false, "like -l, but do not list group information")
	fs.BoolP("C", "C", false, "list entries by columns")
	fs.BoolP("x", "x", false, "list entries by lines instead of by columns")
	fs.BoolP("p", "p", false, "append / indicator to directories")
	fs.BoolP("f", "f", false, "do not sort, enable -a")
	fs.BoolP("U", "U", false, "do not sort; list entries in directory order")
	fs.BoolP("X", "X", false, "sort alphabetically by entry extension")
	fs.BoolP("q", "q", false, "print question marks instead of nongraphic characters")
	fs.BoolP("c", "c", false, "with -lt: sort by, and show, ctime")
	fs.BoolP("u", "u", false, "with -lt: sort by, and show, atime")
	fs.BoolP("m", "m", false, "fill width with a comma separated list of entries")
	fs.BoolP("Z", "Z", false, "print security context when available")

	// Set --version shorthand to -V
	if verFlag := fs.Lookup("version"); verFlag != nil {
		verFlag.Shorthand = "V"
	}

	fs.Lookup("classify").NoOptDefVal = "always"
	fs.Lookup("color").NoOptDefVal = "always"
	fs.Lookup("hyperlink").NoOptDefVal = "always"

	return fs
}

func run(rc *tool.RunContext, args []string) int {
	// -l, -t, -S, -1 have no GNU long form: pre-parse them out of the
	// short-flag clusters before pflag sees the args.
	rest, short := ExtractShort(args, "ltS1gGnoCpfUXQNbqsvCxZHLV")
	fs := GetFlagSet(cmd.Name)
	operands, code := tool.Parse(rc, cmd, fs, rest)
	if code >= 0 {
		return code
	}
	if short['V'] > 0 {
		fmt.Fprintf(rc.Out, "%s (qiangli/coreutils) %s\n", cmd.Name, tool.Version)
		return 0
	}
	ignoreBackups, _ := fs.GetBool("ignore-backups")
	zero, _ := fs.GetBool("zero")
	fileType, _ := fs.GetBool("file-type")
	classify, _ := fs.GetString("classify")
	indicator, _ := fs.GetString("indicator-style")
	literal, _ := fs.GetBool("literal")
	quoteName, _ := fs.GetBool("quote-name")
	escape, _ := fs.GetBool("escape")
	quoting, _ := fs.GetString("quoting-style")
	groupDirsFirst, _ := fs.GetBool("group-directories-first")
	deref, _ := fs.GetBool("dereference")
	derefCL, _ := fs.GetBool("dereference-command-line")
	derefCLDir, _ := fs.GetBool("dereference-command-line-symlink-to-dir")
	timeField, _ := fs.GetString("time")
	fullTime, _ := fs.GetBool("full-time")
	sizeFlag, _ := fs.GetBool("size")
	format, _ := fs.GetString("format")
	sortMode, _ := fs.GetString("sort")

	all, _ := fs.GetBool("all")
	almost, _ := fs.GetBool("almost-all")
	dirOnly, _ := fs.GetBool("directory")
	human, _ := fs.GetBool("human-readable")
	inode, _ := fs.GetBool("inode")
	recursive, _ := fs.GetBool("recursive")
	reverse, _ := fs.GetBool("reverse")
	longFlag, _ := fs.GetBool("long")
	noGroup, _ := fs.GetBool("no-group")
	numeric, _ := fs.GetBool("numeric-uid-gid")
	hide, _ := fs.GetStringArray("hide")
	ignore, _ := fs.GetStringArray("ignore")

	showControl, _ := fs.GetBool("show-control-chars")
	hideControlFlag, _ := fs.GetBool("hide-control-chars")

	opt := options{
		long:           short['l'] > 0 || longFlag,
		all:            all,
		almostAll:      almost,
		dirOnly:        dirOnly,
		recursive:      recursive,
		reverse:        reverse,
		inode:          inode,
		human:          human,
		noGroup:        short['g'] > 0 || short['G'] > 0 || noGroup,
		numeric:        short['n'] > 0 || numeric,
		sortTime:       short['t'] > 0,
		sortSize:       short['S'] > 0,
		sizeBlocks:     short['s'] > 0 || sizeFlag,
		classify:       short['F'] > 0 || classify == "always" || classify == "auto",
		fileType:       fileType,
		slashDirs:      short['p'] > 0,
		zero:           zero,
		comma:          short['m'] > 0,
		unsorted:       short['U'] > 0,
		sortExtension:  short['X'] > 0,
		sortVersion:    short['v'] > 0,
		groupDirsFirst: groupDirsFirst,
		literal:        short['N'] > 0 || literal,
		quoteName:      short['Q'] > 0 || quoteName,
		escape:         short['b'] > 0 || escape,
		hide:           hide,
		ignore:         ignore,
	}
	// -q / --hide-control-chars vs --show-control-chars: last one wins.
	if short['q'] > 0 || hideControlFlag || showControl {
		opt.hideControl = lastFlag(args, 'q', "hide-control-chars") >
			lastFlag(args, 0, "show-control-chars")
	}
	if short['o'] > 0 {
		opt.noGroup = true
	}
	if short['f'] > 0 {
		opt.all, opt.unsorted = true, true
	}
	if short['Z'] > 0 {
		// Security contexts are platform-specific; accept the flag but keep
		// output portable rather than inventing labels.
	}
	if classify != "" && classify != "always" && classify != "auto" && classify != "never" {
		return tool.UsageError(rc, cmd, "unsupported --classify=%s", classify)
	}
	if ignoreBackups {
		opt.ignore = append(opt.ignore, "*~")
	}
	if format != "" {
		if _, ok := formatWord(format); !ok {
			return tool.UsageError(rc, cmd, "unsupported --format=%s", format)
		}
	}
	// The format is whichever format option came last (GNU/POSIX).
	if kind, ok := lastFormat(args, format); ok {
		opt.format = kind
	}
	if fullTime {
		opt.format = fmtLong
	}
	if opt.zero {
		// GNU: --zero implies one name per line.
		opt.format = fmtOnePerLine
	}
	opt.long = opt.format == fmtLong
	opt.comma = opt.format == fmtCommas
	switch sortMode {
	case "":
	case "name":
		opt.sortTime, opt.sortSize, opt.sortExtension, opt.unsorted = false, false, false, false
	case "none":
		opt.unsorted = true
	case "time":
		opt.sortTime = true
	case "size":
		opt.sortSize = true
	case "extension":
		opt.sortExtension = true
	default:
		return tool.UsageError(rc, cmd, "unsupported --sort=%s", sortMode)
	}
	switch indicator {
	case "", "none":
	case "slash":
		opt.slashDirs = true
	case "file-type":
		opt.fileType = true
	case "classify":
		opt.classify = true
	default:
		return tool.UsageError(rc, cmd, "unsupported --indicator-style=%s", indicator)
	}
	switch quoting {
	case "", "literal":
		if quoting == "literal" {
			opt.literal = true
		}
	case "c":
		opt.quoteName = true
	case "escape":
		opt.escape = true
	default:
		return tool.UsageError(rc, cmd, "unsupported --quoting-style=%s", quoting)
	}
	if short['L'] > 0 {
		deref = true
	}
	if short['H'] > 0 {
		derefCL = true
	}
	// -L applies to every entry; -H and its long spelling only to the
	// command-line operands, handled in the operand loop below.
	opt.deref = deref
	opt.width = lineWidth(rc, fs)

	si, _ := fs.GetBool("si")
	blockSize, _ := fs.GetString("block-size")
	kibibytes, _ := fs.GetBool("kibibytes")
	opt.si = si
	if si {
		opt.human = true
	}
	// --block-size scales both the -l size column and the -s block
	// counts; -k pins the block counts to 1 KiB units.
	opt.sizeUnit, opt.blockUnit = 1, defaultBlockUnit
	if blockSize != "" {
		n, err := parseBlockSize(blockSize)
		if err != nil {
			return tool.UsageError(rc, cmd, "invalid --block-size argument '%s'", blockSize)
		}
		opt.sizeUnit, opt.blockUnit = n, n
	}
	if kibibytes {
		opt.blockUnit = defaultBlockUnit
	}

	timeStyleVal, _ := fs.GetString("time-style")
	switch timeStyleVal {
	case "", "locale":
	case "long-iso":
		opt.timeStyle = styleLongISO
	case "full-iso":
		opt.timeStyle = styleFullISO
	case "iso":
		opt.timeStyle = styleISO
	default:
		return tool.UsageError(rc, cmd, "unsupported --time-style=%s", timeStyleVal)
	}
	if fullTime {
		opt.timeStyle = styleFullISO
	}
	switch timeField {
	case "", "mtime", "modification", "atime", "access", "use",
		"ctime", "status", "birth", "creation":
	default:
		return tool.UsageError(rc, cmd, "unsupported --time=%s", timeField)
	}
	// -c, -u, and --time=WORD all set the same timestamp selector; the
	// last occurrence wins (GNU).
	switch lastTimeSelector(args) {
	case 'c':
		opt.timeSel = selCtime
	case 'u':
		opt.timeSel = selAtime
	case 'T':
		switch timeField {
		case "atime", "access", "use":
			opt.timeSel = selAtime
		case "ctime", "status":
			opt.timeSel = selCtime
		case "birth", "creation":
			opt.timeSel = selBirth
		}
	}
	// GNU: a non-mtime timestamp with no explicit sort choice sorts a
	// short-format listing by that timestamp, newest first.
	sortExplicit := short['t'] > 0 || short['S'] > 0 || short['U'] > 0 ||
		short['X'] > 0 || short['v'] > 0 || short['f'] > 0 || fs.Changed("sort")
	if !sortExplicit && !opt.long && opt.timeSel != selMtime {
		opt.sortTime = true
	}
	// GNU last-one-wins pairs: -a vs -A and -t vs -S each set a single
	// internal mode, so the later occurrence wins.
	if opt.all && opt.almostAll {
		if lastFlag(args, 'a', "all") >= lastFlag(args, 'A', "almost-all") {
			opt.almostAll = false
		} else {
			opt.all = false
		}
	}
	if opt.sortTime && opt.sortSize {
		if short['t'] >= short['S'] {
			opt.sortSize = false
		} else {
			opt.sortTime = false
		}
	}

	if len(operands) == 0 {
		operands = []string{"."}
	}

	l := &lister{rc: rc, opt: opt}
	var files, dirs []entry
	for _, op := range operands {
		full := rc.Path(op)
		fi, err := os.Lstat(full)
		if err != nil {
			l.fail(2, "cannot access '%s': %s", op, errMsg(err))
			continue
		}
		e := entry{name: op, path: full, info: fi}
		isDir := fi.IsDir()
		// GNU dereferences command-line symlinks to directories unless
		// -d or -l asks about the link itself.
		if !isDir && fi.Mode()&os.ModeSymlink != 0 && !opt.dirOnly && (!opt.long || deref || derefCL || derefCLDir) {
			if ti, terr := os.Stat(full); terr == nil {
				isDir = ti.IsDir()
				e.info = ti
			}
		}
		e.tm = l.entryTime(e)
		if isDir && !opt.dirOnly {
			dirs = append(dirs, e)
			continue
		}
		if opt.long && fi.Mode()&os.ModeSymlink != 0 && !deref && !derefCL {
			e.target, _ = os.Readlink(full)
		}
		files = append(files, e)
	}

	sortEntries(files, opt)
	sortEntries(dirs, opt)
	if len(files) > 0 {
		l.printBlock(files, false)
		l.wrote = true
	}
	withHeader := len(operands) > 1 || opt.recursive
	for _, d := range dirs {
		l.listDir(d.name, d.path, withHeader)
	}
	return l.exit
}

type lister struct {
	rc    *tool.RunContext
	opt   options
	exit  int
	wrote bool
}

func (l *lister) fail(code int, format string, a ...any) {
	fmt.Fprintf(l.rc.Err, "ls: "+format+"\n", a...)
	if code > l.exit {
		l.exit = code
	}
}

// entryTime resolves the selected timestamp field for display and time
// sorting. A field the platform or filesystem cannot provide is
// reported loudly (never approximated by mtime) and renders as the
// zero time.
func (l *lister) entryTime(e entry) time.Time {
	if l.opt.timeSel == selMtime {
		return e.info.ModTime()
	}
	t, err := sysTime(e.info, e.path, l.opt.timeSel)
	if err != nil {
		l.fail(1, "cannot determine %s time of '%s': %s", timeSelName(l.opt.timeSel), e.name, errMsg(err))
		return time.Time{}
	}
	return t
}

func timeSelName(sel timeSel) string {
	switch sel {
	case selAtime:
		return "access"
	case selCtime:
		return "status change"
	case selBirth:
		return "birth"
	}
	return "modification"
}

func (l *lister) listDir(display, full string, header bool) {
	if l.wrote {
		fmt.Fprintln(l.rc.Out)
	}
	l.wrote = true
	if header {
		fmt.Fprintf(l.rc.Out, "%s:\n", display)
	}
	des, err := os.ReadDir(full)
	if err != nil {
		l.fail(1, "cannot open directory '%s': %s", display, errMsg(err))
		return
	}
	var ents []entry
	if l.opt.all {
		// GNU -a lists the implied . and .. entries.
		for _, dot := range []string{".", ".."} {
			p := full
			if dot == ".." {
				p = filepath.Join(full, "..")
			}
			if fi, ferr := os.Stat(p); ferr == nil {
				e := entry{name: dot, path: p, info: fi}
				e.tm = l.entryTime(e)
				ents = append(ents, e)
			}
		}
	}
	for _, de := range des {
		name := de.Name()
		if !l.opt.all && !l.opt.almostAll && strings.HasPrefix(name, ".") {
			continue
		}
		if matchesAny(name, l.opt.ignore) || (!l.opt.all && !l.opt.almostAll && matchesAny(name, l.opt.hide)) {
			continue
		}
		p := filepath.Join(full, name)
		fi, lerr := os.Lstat(p)
		if lerr != nil {
			l.fail(1, "cannot access '%s': %s", joinDisplay(display, name), errMsg(lerr))
			continue
		}
		// -L reports the referenced file rather than the link itself.
		if l.opt.deref && fi.Mode()&os.ModeSymlink != 0 {
			ti, terr := os.Stat(p)
			if terr != nil {
				l.fail(1, "cannot access '%s': %s", joinDisplay(display, name), errMsg(terr))
			} else {
				fi = ti
			}
		}
		e := entry{name: name, path: p, info: fi}
		if l.opt.long && !l.opt.deref && fi.Mode()&os.ModeSymlink != 0 {
			e.target, _ = os.Readlink(p)
		}
		e.tm = l.entryTime(e)
		ents = append(ents, e)
	}
	sortEntries(ents, l.opt)
	// GNU prints the total block count for a directory whenever block
	// counts are shown, which -s does in every format.
	l.printBlock(ents, l.opt.long || l.opt.sizeBlocks)
	if l.opt.recursive {
		for _, e := range ents {
			if e.name == "." || e.name == ".." {
				continue
			}
			if e.info.IsDir() {
				l.listDir(joinDisplay(display, e.name), e.path, true)
			}
		}
	}
}

func (l *lister) printBlock(ents []entry, withTotal bool) {
	out := l.rc.Out
	opt := l.opt
	var inoStrs []string
	inoW := 0
	if opt.inode {
		inoStrs = make([]string, len(ents))
		for i, e := range ents {
			inoStrs[i] = strconv.FormatUint(inodeOf(e.info), 10)
			if len(inoStrs[i]) > inoW {
				inoW = len(inoStrs[i])
			}
		}
	}
	// Block counts (-s) and their total are needed by every format, not
	// just -l.
	var blkStrs []string
	blocksW := 0
	var blocks uint64
	if opt.sizeBlocks || opt.long {
		blkStrs = make([]string, len(ents))
		for i, e := range ents {
			b := sysOf(e.info, e.path).blocks512
			blocks += b
			blkStrs[i] = scaledSize(b*512, opt.blockUnit, opt)
			blocksW = max(blocksW, len(blkStrs[i]))
		}
	}
	printTotal := func() {
		if withTotal {
			fmt.Fprintf(out, "total %s\n", scaledSize(blocks*512, opt.blockUnit, opt))
		}
	}

	if !opt.long {
		printTotal()
		// Every non-long format prints the same cell — the -i and -s
		// prefixes belong to the name they annotate.
		cells := make([]string, len(ents))
		for i, e := range ents {
			var b strings.Builder
			if opt.inode {
				fmt.Fprintf(&b, "%*s ", inoW, inoStrs[i])
			}
			if opt.sizeBlocks {
				fmt.Fprintf(&b, "%*s ", blocksW, blkStrs[i])
			}
			b.WriteString(displayName(e, opt))
			cells[i] = b.String()
		}
		switch {
		case opt.zero:
			for _, c := range cells {
				fmt.Fprint(out, c, "\x00")
			}
		case opt.format == fmtCommas:
			printCommas(out, cells, opt.width)
		case opt.format == fmtColumns:
			printColumns(out, cells, opt.width, true)
		case opt.format == fmtAcross:
			printColumns(out, cells, opt.width, false)
		default:
			for _, c := range cells {
				fmt.Fprintln(out, c)
			}
		}
		return
	}

	type row struct {
		mode, nlink, owner, group, size, mtime, name string
	}
	rows := make([]row, len(ents))
	var nlinkW, ownerW, groupW, sizeW int
	now := time.Now()
	for i, e := range ents {
		sys := sysOf(e.info, e.path)
		r := row{
			mode:  modeString(e.info.Mode()),
			nlink: strconv.FormatUint(sys.nlink, 10),
			owner: sys.owner,
			group: sys.group,
			size:  sizeString(e.info, sys, opt),
			mtime: timeString(e.tm, now, opt.timeStyle),
			name:  displayName(e, opt),
		}
		if e.info.Mode()&os.ModeSymlink != 0 && e.target != "" {
			r.name += " -> " + quoteControl(e.target, opt)
		}
		nlinkW = max(nlinkW, len(r.nlink))
		ownerW = max(ownerW, len(r.owner))
		groupW = max(groupW, len(r.group))
		sizeW = max(sizeW, len(r.size))
		rows[i] = r
	}
	printTotal()
	for i, r := range rows {
		if opt.inode {
			fmt.Fprintf(out, "%*s ", inoW, inoStrs[i])
		}
		if opt.sizeBlocks {
			fmt.Fprintf(out, "%*s ", blocksW, blkStrs[i])
		}
		if opt.noGroup || opt.numeric {
			fmt.Fprintf(out, "%s %*s %-*s %*s %s %s\n",
				r.mode, nlinkW, r.nlink, ownerW, r.owner,
				sizeW, r.size, r.mtime, r.name)
		} else {
			fmt.Fprintf(out, "%s %*s %-*s %-*s %*s %s %s\n",
				r.mode, nlinkW, r.nlink, ownerW, r.owner, groupW, r.group,
				sizeW, r.size, r.mtime, r.name)
		}
	}
}

// colGutter is the two-space separation GNU leaves between columns.
const colGutter = 2

// printColumns writes cells in multiple columns within width. With
// down true (-C) entries run down each column before moving right;
// with down false (-x) they run across each row. Trailing padding is
// never written, so a one-column layout is byte-identical to -1.
func printColumns(out io.Writer, cells []string, width int, down bool) {
	if len(cells) == 0 {
		return
	}
	cols, rows, colW := columnLayout(cells, width, down)
	idx := func(r, c int) int {
		if down {
			return r + c*rows
		}
		return r*cols + c
	}
	for r := range rows {
		var line strings.Builder
		for c := range cols {
			i := idx(r, c)
			if i >= len(cells) {
				break
			}
			line.WriteString(cells[i])
			// Pad only when a further cell follows on this line;
			// trailing whitespace is never written.
			if next := idx(r, c+1); c+1 < cols && next < len(cells) {
				line.WriteString(strings.Repeat(" ", colW[c]-cellWidth(cells[i])))
			}
		}
		fmt.Fprintln(out, line.String())
	}
}

// minColumnWidth is the floor GNU gives every column when searching
// for a layout, and so bounds the column count it will consider.
const minColumnWidth = 3

// columnLayout picks the largest number of columns whose total line
// length stays under width, mirroring GNU's search: every column
// starts at minColumnWidth, each entry claims a colGutter separator
// except in the last column, and a candidate is valid only when its
// line length is strictly less than the width.
func columnLayout(cells []string, width int, down bool) (cols, rows int, colW []int) {
	maxCols := min(len(cells), max(1, width/minColumnWidth))
	cols, rows, colW = 1, len(cells), []int{minColumnWidth}
	for n := 1; n <= maxCols; n++ {
		r := (len(cells) + n - 1) / n
		w := make([]int, n)
		lineLen := 0
		for c := range w {
			w[c] = minColumnWidth
			lineLen += minColumnWidth
		}
		for i, cell := range cells {
			c := i / r
			if !down {
				c = i % n
			}
			cw := cellWidth(cell)
			if c != n-1 {
				cw += colGutter
			}
			if cw > w[c] {
				lineLen += cw - w[c]
				w[c] = cw
			}
		}
		if lineLen < width {
			cols, rows, colW = n, r, w
		}
	}
	return cols, rows, colW
}

// printCommas writes cells as a comma-separated stream wrapped at
// width (-m). The comma stays on the line it ends, and no separator
// follows the final entry.
func printCommas(out io.Writer, cells []string, width int) {
	pos := 0
	for i, c := range cells {
		w := cellWidth(c)
		if i > 0 {
			if pos+w+2 < width {
				fmt.Fprint(out, ", ")
				pos += 2
			} else {
				fmt.Fprint(out, ",\n")
				pos = 0
			}
		}
		fmt.Fprint(out, c)
		pos += w
	}
	if len(cells) > 0 {
		fmt.Fprintln(out)
	}
}

// cellWidth is the printed width of a name. Names are byte strings in
// this userland's C-locale contract, so one byte is one column.
func cellWidth(s string) int { return len(s) }

// quoteControl applies -q: every non-printable byte becomes '?'. In
// the C locale that is every byte outside the printable ASCII range,
// which is what GNU writes for a name it cannot display.
func quoteControl(s string, opt options) string {
	if !opt.hideControl {
		return s
	}
	b := []byte(s)
	for i, c := range b {
		if c < 0x20 || c >= 0x7f {
			b[i] = '?'
		}
	}
	return string(b)
}

func displayName(e entry, opt options) string {
	name := e.name
	if opt.quoteName {
		name = strconv.Quote(name)
	} else if opt.escape {
		name = strconv.QuoteToASCII(name)
		if len(name) >= 2 {
			name = name[1 : len(name)-1]
		}
	}
	name = quoteControl(name, opt)
	if opt.classify || opt.fileType || opt.slashDirs {
		name += indicator(e, opt.classify, opt.fileType, opt.slashDirs)
	}
	return name
}

func indicator(e entry, classify, fileType, slashDirs bool) string {
	m := e.info.Mode()
	switch {
	case m.IsDir():
		return "/"
	case slashDirs:
		return ""
	case m&os.ModeSymlink != 0:
		return "@"
	case m&os.ModeNamedPipe != 0:
		return "|"
	case m&os.ModeSocket != 0:
		return "="
	case m&os.ModeDevice != 0:
		return ""
	case classify && !fileType && m&0111 != 0:
		return "*"
	default:
		return ""
	}
}

func sizeString(fi os.FileInfo, sys sysInfo, opt options) string {
	if fi.Mode()&os.ModeDevice != 0 {
		return fmt.Sprintf("%d, %d", sys.rdevMajor, sys.rdevMinor)
	}
	return scaledSize(uint64(fi.Size()), opt.sizeUnit, opt)
}

// scaledSize renders n bytes for display: human-readable when -h/--si
// asked for it, otherwise in units of unit, rounded up as GNU does.
func scaledSize(n, unit uint64, opt options) string {
	if opt.human {
		if opt.si {
			return humanSizeBase(n, 1000, "kMGTPE")
		}
		return humanSize(n)
	}
	if unit <= 1 {
		return strconv.FormatUint(n, 10)
	}
	return strconv.FormatUint((n+unit-1)/unit, 10)
}

// parseBlockSize accepts the GNU --block-size spellings: a plain byte
// count, a 1024-based suffix (K, M, G, …, optionally spelled KiB), or a
// 1000-based one (KB, MB, …).
func parseBlockSize(s string) (uint64, error) {
	digits := 0
	for digits < len(s) && isDigit(s[digits]) {
		digits++
	}
	n := uint64(1)
	if digits > 0 {
		v, err := strconv.ParseUint(s[:digits], 10, 64)
		if err != nil || v == 0 {
			return 0, fmt.Errorf("invalid size %q", s)
		}
		n = v
	}
	suffix := s[digits:]
	if suffix == "" {
		if digits == 0 {
			return 0, fmt.Errorf("invalid size %q", s)
		}
		return n, nil
	}
	base := uint64(1024)
	switch {
	case strings.HasSuffix(suffix, "iB"): // KiB, MiB, …
		suffix = suffix[:len(suffix)-2]
	case strings.HasSuffix(suffix, "B"): // KB, MB, … are powers of 1000
		suffix = suffix[:len(suffix)-1]
		base = 1000
	}
	idx := strings.IndexByte("KMGTPE", suffix[0])
	if len(suffix) != 1 || idx < 0 {
		return 0, fmt.Errorf("invalid size %q", s)
	}
	for i := 0; i <= idx; i++ {
		n *= base
	}
	return n, nil
}

// sixMonths is GNU's "recent" cutoff: half of 365.2425 days.
const sixMonths = 15778476 * time.Second

// timeString renders the -l timestamp in the selected style. The
// default (locale) style is C-locale "Jan  2 15:04" for recent files
// and "Jan  2  2006" for old or future ones; the ISO styles are fixed.
func timeString(t, now time.Time, style timeStyle) string {
	recent := !t.After(now) && now.Sub(t) <= sixMonths
	switch style {
	case styleLongISO:
		return t.Format("2006-01-02 15:04")
	case styleFullISO:
		return t.Format("2006-01-02 15:04:05.000000000 -0700")
	case styleISO:
		if recent {
			return t.Format("01-02 15:04")
		}
		return t.Format("2006-01-02")
	}
	if !recent {
		return t.Format("Jan _2  2006")
	}
	return t.Format("Jan _2 15:04")
}

// modeString builds the GNU 10-character permission string
// (type + rwx triplets with setuid/setgid/sticky substitutions).
func modeString(m os.FileMode) string {
	b := []byte("----------")
	switch {
	case m&os.ModeDir != 0:
		b[0] = 'd'
	case m&os.ModeSymlink != 0:
		b[0] = 'l'
	case m&os.ModeCharDevice != 0:
		b[0] = 'c'
	case m&os.ModeDevice != 0:
		b[0] = 'b'
	case m&os.ModeNamedPipe != 0:
		b[0] = 'p'
	case m&os.ModeSocket != 0:
		b[0] = 's'
	}
	const rwx = "rwxrwxrwx"
	perm := m.Perm()
	for i := 0; i < 9; i++ {
		if perm&(1<<uint(8-i)) != 0 {
			b[i+1] = rwx[i]
		}
	}
	if m&os.ModeSetuid != 0 {
		if b[3] == 'x' {
			b[3] = 's'
		} else {
			b[3] = 'S'
		}
	}
	if m&os.ModeSetgid != 0 {
		if b[6] == 'x' {
			b[6] = 's'
		} else {
			b[6] = 'S'
		}
	}
	if m&os.ModeSticky != 0 {
		if b[9] == 'x' {
			b[9] = 't'
		} else {
			b[9] = 'T'
		}
	}
	return string(b)
}

func sortEntries(ents []entry, opt options) {
	if opt.unsorted {
		return
	}
	sort.SliceStable(ents, func(i, j int) bool {
		return compareEntries(ents[i], ents[j], opt) < 0
	})
	// GNU -r reverses the whole comparison, tie-breaks included.
	if opt.reverse {
		for i, j := 0, len(ents)-1; i < j; i, j = i+1, j-1 {
			ents[i], ents[j] = ents[j], ents[i]
		}
	}
}

func compareEntries(a, b entry, opt options) int {
	if opt.groupDirsFirst && a.info.IsDir() != b.info.IsDir() {
		if a.info.IsDir() {
			return -1
		}
		return 1
	}
	switch {
	case opt.sortTime:
		at, bt := a.tm, b.tm
		if at.After(bt) {
			return -1
		}
		if bt.After(at) {
			return 1
		}
	case opt.sortSize:
		if as, bs := a.info.Size(), b.info.Size(); as != bs {
			if as > bs {
				return -1
			}
			return 1
		}
	case opt.sortExtension:
		if ae, be := extensionKey(a.name), extensionKey(b.name); ae != be {
			return strings.Compare(ae, be)
		}
	case opt.sortVersion:
		if c := naturalCompare(a.name, b.name); c != 0 {
			return c
		}
	}
	return strings.Compare(a.name, b.name)
}

func naturalCompare(a, b string) int {
	for len(a) > 0 && len(b) > 0 {
		ra, rb := a[0], b[0]
		if isDigit(ra) && isDigit(rb) {
			ai, bi := 0, 0
			for ai < len(a) && a[ai] == '0' {
				ai++
			}
			for bi < len(b) && b[bi] == '0' {
				bi++
			}
			aj, bj := ai, bi
			for aj < len(a) && isDigit(a[aj]) {
				aj++
			}
			for bj < len(b) && isDigit(b[bj]) {
				bj++
			}
			if la, lb := aj-ai, bj-bi; la != lb {
				if la < lb {
					return -1
				}
				return 1
			}
			if c := strings.Compare(a[ai:aj], b[bi:bj]); c != 0 {
				return c
			}
			if za, zb := ai, bi; za != zb {
				if za > zb {
					return -1
				}
				return 1
			}
			a, b = a[aj:], b[bj:]
			continue
		}
		if ra != rb {
			if ra < rb {
				return -1
			}
			return 1
		}
		a, b = a[1:], b[1:]
	}
	switch {
	case a == "" && b == "":
		return 0
	case a == "":
		return -1
	default:
		return 1
	}
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }

func extensionKey(name string) string {
	i := strings.LastIndexByte(name, '.')
	if i <= 0 || i == len(name)-1 {
		return ""
	}
	return name[i+1:]
}

func matchesAny(name string, patterns []string) bool {
	for _, pat := range patterns {
		ok, err := filepath.Match(pat, name)
		if err == nil && ok {
			return true
		}
	}
	return false
}

// ExtractShort removes the given single-letter flags (which have no
// GNU long form) from short-flag clusters, returning the remaining
// args and a map of flag letter to the sequence number of its last
// occurrence (0 = absent). Scanning stops at the "--" terminator.
func ExtractShort(args []string, chars string) ([]string, map[byte]int) {
	found := map[byte]int{}
	seq := 0
	var rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			rest = append(rest, args[i:]...)
			break
		}
		if len(a) > 1 && a[0] == '-' && a[1] != '-' {
			kept := []byte{'-'}
			for j := 1; j < len(a); j++ {
				seq++
				if strings.IndexByte(chars, a[j]) >= 0 {
					found[a[j]] = seq
				} else {
					kept = append(kept, a[j])
				}
			}
			if len(kept) > 1 {
				rest = append(rest, string(kept))
			}
			continue
		}
		rest = append(rest, a)
	}
	return rest, found
}

// formatWord maps a --format=WORD value to its format, reporting
// whether the word is one GNU defines.
func formatWord(word string) (fmtKind, bool) {
	switch word {
	case "long", "verbose":
		return fmtLong, true
	case "single-column":
		return fmtOnePerLine, true
	case "commas":
		return fmtCommas, true
	case "across", "horizontal":
		return fmtAcross, true
	case "vertical":
		return fmtColumns, true
	}
	return fmtOnePerLine, false
}

// shortFormat maps a short format option letter to its format.
func shortFormat(ch byte) (fmtKind, bool) {
	switch ch {
	case 'l', 'g', 'n', 'o':
		return fmtLong, true
	case '1':
		return fmtOnePerLine, true
	case 'C':
		return fmtColumns, true
	case 'x':
		return fmtAcross, true
	case 'm':
		return fmtCommas, true
	}
	return fmtOnePerLine, false
}

// argTakingShorts are the short options whose value is attached to the
// same argument, so a cluster scan must stop at them rather than read
// the value's characters as further options.
const argTakingShorts = "IwT"

// lastFormat returns the format selected by the last format option in
// args, and whether args contained one at all. formatVal is the parsed
// --format value (already validated), used when --format is the last
// format option. Scanning stops at "--".
func lastFormat(args []string, formatVal string) (fmtKind, bool) {
	kind, found := fmtOnePerLine, false
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			break
		}
		if strings.HasPrefix(a, "--") {
			name := a[2:]
			if eq := strings.IndexByte(name, '='); eq >= 0 {
				name = name[:eq]
			} else if name == "format" {
				i++ // the value is the next argument
			}
			switch name {
			case "format":
				if k, ok := formatWord(formatVal); ok {
					kind, found = k, true
				}
			case "long":
				kind, found = fmtLong, true
			case "full-time":
				kind, found = fmtLong, true
			}
			continue
		}
		if len(a) > 1 && a[0] == '-' {
			for j := 1; j < len(a); j++ {
				if strings.IndexByte(argTakingShorts, a[j]) >= 0 {
					break
				}
				if k, ok := shortFormat(a[j]); ok {
					kind, found = k, true
				}
			}
		}
	}
	return kind, found
}

// lineWidth resolves the output line width for the column and comma
// formats: -w/--width, else COLUMNS, else the 80-column non-tty
// default. GNU documents a width of 0 as "no limit".
func lineWidth(rc *tool.RunContext, fs *pflag.FlagSet) int {
	if fs.Changed("width") {
		w, _ := fs.GetInt("width")
		if w <= 0 {
			return unlimitedWidth
		}
		return w
	}
	if c := rc.Getenv("COLUMNS"); c != "" {
		if n, err := strconv.Atoi(c); err == nil && n > 0 {
			return n
		}
	}
	return defaultWidth
}

// lastTimeSelector scans args for the last of -c, -u, or --time,
// returning 'c', 'u', 'T' (--time), or 0 when none is present;
// scanning stops at the "--" terminator. Only which selector came last
// matters: multiple --time values already resolve last-wins in pflag.
func lastTimeSelector(args []string) byte {
	var kind byte
	for _, a := range args {
		if a == "--" {
			break
		}
		if a == "--time" || strings.HasPrefix(a, "--time=") {
			kind = 'T'
			continue
		}
		if len(a) > 1 && a[0] == '-' && a[1] != '-' {
			for j := 1; j < len(a); j++ {
				if a[j] == 'c' || a[j] == 'u' {
					kind = a[j]
				}
			}
		}
	}
	return kind
}

// lastFlag returns the position (1-based, 0 = absent) of the last
// occurrence of short flag ch or --long among args, scanning GNU-style
// clusters; parsing stops at "--".
func lastFlag(args []string, ch byte, long string) int {
	pos, n := 0, 0
	for _, a := range args {
		if a == "--" {
			break
		}
		if strings.HasPrefix(a, "--") {
			n++
			if a[2:] == long {
				pos = n
			}
			continue
		}
		if len(a) > 1 && a[0] == '-' {
			for j := 1; j < len(a); j++ {
				n++
				if a[j] == ch {
					pos = n
				}
			}
			continue
		}
		n++
	}
	return pos
}

func joinDisplay(dir, name string) string {
	if strings.HasSuffix(dir, "/") || strings.HasSuffix(dir, string(os.PathSeparator)) {
		return dir + name
	}
	return dir + "/" + name
}

func errMsg(err error) string {
	return tool.SysErrString(err)
}

// humanSize renders n bytes in GNU --human-readable form: powers of
// 1024, at most one decimal digit, always rounding up (1025 -> 1.1K).
func humanSize(n uint64) string { return humanSizeBase(n, 1024, "KMGTPE") }

// humanSizeBase is humanSize over an arbitrary base and unit-suffix
// set, so --si can render the same shape in powers of 1000.
func humanSizeBase(n, base uint64, units string) string {
	if n < base {
		return strconv.FormatUint(n, 10)
	}
	div := base
	idx := 0
	for n/div >= base && idx < len(units)-1 {
		div *= base
		idx++
	}
	whole, rem := n/div, n%div
	if whole < 10 {
		tenths := whole*10 + (rem*10+div-1)/div
		if tenths < 100 {
			return fmt.Sprintf("%d.%d%c", tenths/10, tenths%10, units[idx])
		}
		return fmt.Sprintf("10%c", units[idx])
	}
	v := whole
	if rem > 0 {
		v++
	}
	if v >= base && idx < len(units)-1 {
		return fmt.Sprintf("1.0%c", units[idx+1])
	}
	return fmt.Sprintf("%d%c", v, units[idx])
}
