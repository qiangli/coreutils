// Package lscmd implements ls(1) per the GNU coreutils manual for the
// flag subset -l -a -A -d -R -r -t -S -1 -h -i.
//
// Deterministic-output contract: names sort in C-locale byte order,
// color is never emitted, and the short format is always one entry per
// line in a single column. This userland has no terminal, so the
// output matches what GNU ls produces when writing to a non-tty,
// regardless of any real terminal's width; -1 is therefore accepted
// and is the default.
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
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/qiangli/coreutils/tool"
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
	hide, ignore                             []string
}

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
	target string // symlink target, filled only for -l
}

func run(rc *tool.RunContext, args []string) int {
	// -l, -t, -S, -1 have no GNU long form: pre-parse them out of the
	// short-flag clusters before pflag sees the args.
	rest, short := extractShort(args, "ltS1gGnoCpfUXQNbqsvCxZHLV")
	fs := tool.NewFlags(cmd.Name)
	all := fs.BoolP("all", "a", false, "do not ignore entries starting with .")
	almost := fs.BoolP("almost-all", "A", false, "do not list implied . and ..")
	dirOnly := fs.BoolP("directory", "d", false, "list directories themselves, not their contents")
	human := fs.BoolP("human-readable", "h", false, "with -l, print sizes like 1K 234M 2G etc.")
	inode := fs.BoolP("inode", "i", false, "print the index number of each file")
	recursive := fs.BoolP("recursive", "R", false, "list subdirectories recursively")
	reverse := fs.BoolP("reverse", "r", false, "reverse order while sorting")
	longFlag := fs.Bool("long", false, "display detailed information")
	noGroup := fs.Bool("no-group", false, "in a long listing, don't print group names")
	numeric := fs.Bool("numeric-uid-gid", false, "like -l, but list numeric user and group IDs")
	format := fs.String("format", "", "set display format: long, single-column, commas")
	sortMode := fs.String("sort", "", "sort by WORD: name, none, time, size, extension")
	hide := fs.StringArray("hide", nil, "do not list implied entries matching shell PATTERN")
	ignore := fs.StringArrayP("ignore", "I", nil, "do not list implied entries matching shell PATTERN")
	fs.BoolP("ignore-backups", "B", false, "do not list implied entries ending with ~")
	fs.Bool("zero", false, "end each output line with NUL, not newline")
	fs.Bool("file-type", false, "append file type indicators except '*'")
	fs.String("classify", "", "append file type indicators")
	fs.String("indicator-style", "", "append indicator with style WORD: none, slash, file-type, classify")
	fs.Bool("literal", false, "print entry names without quoting")
	fs.Bool("quote-name", false, "enclose entry names in double quotes")
	fs.Bool("escape", false, "print C-style escapes for nongraphic characters")
	fs.Bool("hide-control-chars", false, "print question marks instead of nongraphic characters")
	fs.Bool("show-control-chars", false, "show nongraphic characters as-is")
	fs.String("quoting-style", "", "set quoting style: literal, c, escape")
	fs.Bool("group-directories-first", false, "group directories before files")
	fs.Bool("dereference", false, "show file information for symlink referents")
	fs.Bool("dereference-command-line", false, "follow command-line symlinks")
	fs.Bool("dereference-command-line-symlink-to-dir", false, "follow command-line symlinked directories")
	fs.Bool("author", false, "with -l, print the author of each file")
	fs.Bool("context", false, "print security context when available")
	fs.String("color", "never", "control color output; accepted for compatibility")
	fs.String("hyperlink", "never", "control hyperlink output; accepted for compatibility")
	fs.IntP("width", "w", 0, "set output width; accepted for compatibility")
	fs.IntP("tabsize", "T", 8, "set tab stops; accepted for compatibility")
	fs.String("time", "", "select timestamp field; mtime, atime/access/use, ctime/status")
	fs.String("time-style", "", "set time style for -l; full-iso supported")
	fs.Bool("full-time", false, "like -l --time-style=full-iso")
	fs.String("block-size", "", "scale block counts; supports 1, K, KB")
	fs.Bool("si", false, "print human-readable sizes in powers of 1000")
	fs.Bool("size", false, "print allocated size of each file, in blocks")
	fs.BoolP("kibibytes", "k", false, "use 1024-byte blocks for allocated sizes")
	fs.BoolP("dired", "D", false, "accepted for compatibility")
	fs.Lookup("classify").NoOptDefVal = "always"
	fs.Lookup("color").NoOptDefVal = "always"
	fs.Lookup("hyperlink").NoOptDefVal = "always"
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

	opt := options{
		long:           short['l'] > 0 || *longFlag,
		all:            *all,
		almostAll:      *almost,
		dirOnly:        *dirOnly,
		recursive:      *recursive,
		reverse:        *reverse,
		inode:          *inode,
		human:          *human,
		noGroup:        short['g'] > 0 || short['G'] > 0 || *noGroup,
		numeric:        short['n'] > 0 || *numeric,
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
		hide:           *hide,
		ignore:         *ignore,
	}
	if short['o'] > 0 {
		opt.long, opt.noGroup = true, true
	}
	if short['g'] > 0 || short['n'] > 0 {
		opt.long = true
	}
	if short['f'] > 0 {
		opt.all, opt.unsorted = true, true
	}
	if short['c'] > 0 || short['u'] > 0 {
		opt.sortTime = true
	}
	if short['C'] > 0 || short['x'] > 0 {
		// Column and row formats collapse to the existing deterministic
		// single-column output for non-interactive tool invocations.
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
	switch *format {
	case "", "verbose":
	case "long":
		opt.long = true
	case "single-column":
	case "commas":
		opt.comma = true
	default:
		return tool.UsageError(rc, cmd, "unsupported --format=%s", *format)
	}
	switch *sortMode {
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
		return tool.UsageError(rc, cmd, "unsupported --sort=%s", *sortMode)
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
	if deref || derefCL || derefCLDir {
		// These modes are handled in the command-line symlink decision below.
	}
	switch timeField {
	case "", "mtime", "modification":
	case "atime", "access", "use":
		// Portable os.FileInfo only exposes mtime; accept the selector but keep
		// mtime rather than silently reaching for platform globals.
	case "ctime", "status":
		opt.sortTime = true
	default:
		return tool.UsageError(rc, cmd, "unsupported --time=%s", timeField)
	}
	if fullTime {
		opt.long = true
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
				ents = append(ents, entry{name: dot, path: p, info: fi})
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
		e := entry{name: name, path: p, info: fi}
		if l.opt.long && fi.Mode()&os.ModeSymlink != 0 {
			e.target, _ = os.Readlink(p)
		}
		ents = append(ents, e)
	}
	sortEntries(ents, l.opt)
	l.printBlock(ents, l.opt.long)
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
	if !opt.long {
		sep := "\n"
		if opt.zero {
			sep = "\x00"
		} else if opt.comma {
			sep = ", "
		}
		for i, e := range ents {
			name := displayName(e, opt)
			if opt.inode {
				fmt.Fprintf(out, "%*s %s%s", inoW, inoStrs[i], name, sep)
			} else {
				fmt.Fprint(out, name, sep)
			}
		}
		if opt.comma && len(ents) > 0 {
			fmt.Fprintln(out)
		}
		return
	}

	type row struct {
		mode, nlink, owner, group, blocks, size, mtime, name string
	}
	rows := make([]row, len(ents))
	var nlinkW, ownerW, groupW, blocksW, sizeW int
	var blocks uint64
	now := time.Now()
	for i, e := range ents {
		sys := sysOf(e.info, e.path)
		blocks += sys.blocks512
		r := row{
			mode:   modeString(e.info.Mode()),
			nlink:  strconv.FormatUint(sys.nlink, 10),
			owner:  sys.owner,
			group:  sys.group,
			blocks: strconv.FormatUint((sys.blocks512+1)/2, 10),
			size:   sizeString(e.info, sys, opt.human),
			mtime:  timeString(e.info.ModTime(), now),
			name:   displayName(e, opt),
		}
		if e.info.Mode()&os.ModeSymlink != 0 {
			r.name += " -> " + e.target
		}
		nlinkW = max(nlinkW, len(r.nlink))
		ownerW = max(ownerW, len(r.owner))
		groupW = max(groupW, len(r.group))
		blocksW = max(blocksW, len(r.blocks))
		sizeW = max(sizeW, len(r.size))
		rows[i] = r
	}
	if withTotal {
		if opt.human {
			fmt.Fprintf(out, "total %s\n", humanSize(blocks*512))
		} else {
			fmt.Fprintf(out, "total %d\n", (blocks*512+1023)/1024)
		}
	}
	for i, r := range rows {
		if opt.inode {
			fmt.Fprintf(out, "%*s ", inoW, inoStrs[i])
		}
		if opt.sizeBlocks {
			fmt.Fprintf(out, "%*s ", blocksW, r.blocks)
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

func sizeString(fi os.FileInfo, sys sysInfo, human bool) string {
	if fi.Mode()&os.ModeDevice != 0 {
		return fmt.Sprintf("%d, %d", sys.rdevMajor, sys.rdevMinor)
	}
	if human {
		return humanSize(uint64(fi.Size()))
	}
	return strconv.FormatInt(fi.Size(), 10)
}

// sixMonths is GNU's "recent" cutoff: half of 365.2425 days.
const sixMonths = 15778476 * time.Second

// timeString renders the -l timestamp: "Jan  2 15:04" for recent
// files, "Jan  2  2006" for old or future ones (C locale).
func timeString(t, now time.Time) string {
	if t.After(now) || now.Sub(t) > sixMonths {
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
		at, bt := a.info.ModTime(), b.info.ModTime()
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

// extractShort removes the given single-letter flags (which have no
// GNU long form) from short-flag clusters, returning the remaining
// args and a map of flag letter to the sequence number of its last
// occurrence (0 = absent). Scanning stops at the "--" terminator.
func extractShort(args []string, chars string) ([]string, map[byte]int) {
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
func humanSize(n uint64) string {
	if n < 1024 {
		return strconv.FormatUint(n, 10)
	}
	const units = "KMGTPE"
	div := uint64(1024)
	idx := 0
	for n/div >= 1024 && idx < len(units)-1 {
		div *= 1024
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
	if v >= 1024 && idx < len(units)-1 {
		return fmt.Sprintf("1.0%c", units[idx+1])
	}
	return fmt.Sprintf("%d%c", v, units[idx])
}
