// Portions adapted from https://github.com/u-root/u-root cmds/core/du/du.go (BSD-3-Clause).
// Changes: rewired to the tool framework; GNU flag set -a -b -c -d -h -s
// with GNU defaults (1024-byte block output, hard-link deduplication,
// post-order per-directory reporting); Windows fallback to apparent size.

// Package ducmd implements du(1) per the GNU coreutils manual for the
// flag subset -a -b -c -d/--max-depth -h -s. As GNU documents, sizes
// are disk usage reported in 1024-byte units by default (rounded up);
// -b switches to exact apparent sizes in bytes; -h prints
// human-readable sizes. Hard-linked files are counted once per
// invocation. Traversal does not follow symlinks.
//
// Platform note: disk usage comes from st_blocks on unix; Windows has
// no block count, so usage falls back to the apparent size there.
package ducmd

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "du",
	Synopsis: "Summarize device usage of the set of FILEs, recursively for directories. Supports -D, -H, and -M aliases.",
	Usage:    "du [OPTION]... [FILE]...",
}

// Run is wired in init: a literal would create an initialization
// cycle (run's flag-error paths reference cmd).
func init() { cmd.Run = run; tool.Register(cmd) }

type duRun struct {
	rc            *tool.RunContext
	all           bool
	apparent      bool
	human         bool
	si            bool
	countLinks    bool
	inodes        bool
	separateDirs  bool
	oneFileSystem bool
	showTime      bool
	timeStyle     string
	block         int64
	maxDepth      int
	threshold     int64
	haveThreshold bool
	deref         derefMode
	exit          int
	seen          map[devIno]bool
	excludes      []string
	term          string
}

type derefMode int

const (
	derefNone derefMode = iota
	derefArgs
	derefAll
)

type duEntry struct {
	size int64
	mod  time.Time
	dir  bool
}

func run(rc *tool.RunContext, args []string) int {
	args = normalizeBlockSizeArgs(args)
	fs := tool.NewFlags(cmd.Name)
	all := fs.BoolP("all", "a", false, "write counts for all files, not just directories")
	apparentSize := fs.BoolP("apparent-size", "A", false, "print apparent sizes, rather than disk usage")
	bytesMode := fs.BoolP("bytes", "b", false, "equivalent to '--apparent-size --block-size=1'")
	blockSize := fs.StringP("block-size", "B", "", "scale sizes by SIZE before printing them")
	derefArgsLong := fs.Bool("dereference-args", false, "dereference only symlinks that are listed on the command line")
	derefArgsShort := fs.BoolP("dereference-args-short", "D", false, "dereference only symlinks that are listed on the command line")
	hFlag := fs.BoolP("dereference-args-H", "H", false, "equivalent to --dereference-args")
	fs.Lookup("dereference-args-short").Hidden = true
	fs.Lookup("dereference-args-H").Hidden = true
	derefAllFlag := fs.BoolP("dereference", "L", false, "dereference all symbolic links")
	noDeref := fs.BoolP("no-dereference", "P", false, "do not follow any symbolic links")
	kib := fs.BoolP("kilobytes", "k", false, "like --block-size=1K")
	mib := fs.BoolP("megabytes", "m", false, "like --block-size=1M")
	fs.BoolP("megabytes-short", "M", false, "like --block-size=1M")
	fs.Lookup("megabytes-short").Hidden = true
	total := fs.BoolP("total", "c", false, "produce a grand total")
	countLinks := fs.BoolP("count-links", "l", false, "count sizes many times if hard linked")
	inodes := fs.Bool("inodes", false, "list inode usage information instead of block usage")
	separateDirs := fs.BoolP("separate-dirs", "S", false, "for directories do not include size of subdirectories")
	oneFileSystem := fs.BoolP("one-file-system", "x", false, "skip directories on different file systems")
	thresholdArg := fs.StringP("threshold", "t", "", "exclude entries smaller than SIZE if positive, or greater than SIZE if negative")
	showTime := fs.String("time", "", "show time of the last modification of any file in the directory")
	if f := fs.Lookup("time"); f != nil {
		f.NoOptDefVal = "mtime"
	}
	timeStyle := fs.String("time-style", "", "time/date format with --time: full-iso, long-iso, iso, or +FORMAT")
	verbose := fs.BoolP("verbose", "v", false, "write counts for all files, not just directories")
	maxDepth := fs.IntP("max-depth", "d", -1, "print the total for a directory (or file, with --all) only if it is N or fewer levels below the command line argument")
	human := fs.BoolP("human-readable", "h", false, "print sizes in human readable format (e.g., 1K 234M 2G)")
	si := fs.Bool("si", false, "like -h, but use powers of 1000 not 1024")
	null := fs.BoolP("null", "0", false, "end each output line with NUL, not newline")
	exclude := fs.StringArray("exclude", nil, "exclude files that match PATTERN")
	excludeFrom := fs.StringArrayP("exclude-from", "X", nil, "read exclude patterns from FILE, one per line")
	files0From := fs.String("files0-from", "", "read input file names from FILE, terminated by NUL; if FILE is -, read names from standard input")
	summarize := fs.BoolP("summarize", "s", false, "display only a total for each argument")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}

	if *verbose {
		*all = true
	}
	if *summarize && *all {
		return tool.UsageError(rc, cmd, "cannot both summarize and show all entries")
	}
	if *summarize && fs.Changed("max-depth") {
		return tool.UsageError(rc, cmd, "warning: summarizing conflicts with --max-depth=%d", *maxDepth)
	}
	if fs.Changed("max-depth") && *maxDepth < 0 {
		return tool.UsageError(rc, cmd, "invalid maximum depth '%d'", *maxDepth)
	}
	var threshold int64
	if fs.Changed("threshold") {
		var err error
		threshold, err = parseSignedSize(*thresholdArg)
		if err != nil {
			return tool.UsageError(rc, cmd, "invalid threshold %q", *thresholdArg)
		}
	}
	if fs.Changed("time-style") && !fs.Changed("time") {
		*showTime = "mtime"
	}
	if fs.Changed("time") {
		switch *showTime {
		case "", "mtime", "modification", "modify":
		default:
			return tool.UsageError(rc, cmd, "unsupported --time=%s", *showTime)
		}
	}
	if fs.Changed("time-style") {
		if err := validateTimeStyle(*timeStyle); err != nil {
			return tool.UsageError(rc, cmd, "%s", err)
		}
	}
	if fs.Changed("files0-from") {
		if len(operands) > 0 {
			return tool.UsageError(rc, cmd, "file operands cannot be combined with --files0-from")
		}
		var err error
		operands, err = readFiles0(rc, *files0From)
		if err != nil {
			fmt.Fprintf(rc.Err, "du: %s: %s\n", *files0From, errMsg(err))
			return 1
		}
	}
	patterns := append([]string{}, *exclude...)
	for _, name := range *excludeFrom {
		more, err := readExcludePatterns(rc, name)
		if err != nil {
			fmt.Fprintf(rc.Err, "du: %s: %s\n", name, errMsg(err))
			return 1
		}
		patterns = append(patterns, more...)
	}

	depth := math.MaxInt
	switch {
	case *summarize:
		depth = 0
	case fs.Changed("max-depth"):
		depth = *maxDepth
	}
	block := int64(1024)
	apparent := *apparentSize || *bytesMode
	if *bytesMode {
		block = 1
	}
	if *kib {
		block = 1024
	}
	if *mib {
		block = 1024 * 1024
	}
	if fs.Changed("block-size") {
		var err error
		block, err = parseBlockSize(*blockSize)
		if err != nil {
			return tool.UsageError(rc, cmd, "invalid block size %q", *blockSize)
		}
	}
	term := "\n"
	if *null {
		term = "\x00"
	}
	deref := derefNone
	if *derefArgsLong || *derefArgsShort || *hFlag {
		deref = derefArgs
	}
	if *derefAllFlag {
		deref = derefAll
	}
	if *noDeref {
		deref = derefNone
	}

	d := &duRun{
		rc:            rc,
		all:           *all,
		apparent:      apparent,
		human:         *human,
		si:            *si,
		countLinks:    *countLinks,
		inodes:        *inodes,
		separateDirs:  *separateDirs,
		oneFileSystem: *oneFileSystem,
		showTime:      fs.Changed("time") || fs.Changed("time-style"),
		timeStyle:     *timeStyle,
		block:         block,
		maxDepth:      depth,
		threshold:     threshold,
		haveThreshold: fs.Changed("threshold"),
		deref:         deref,
		seen:          map[devIno]bool{},
		excludes:      patterns,
		term:          term,
	}

	if len(operands) == 0 {
		operands = []string{"."}
	}
	var grand duEntry
	for _, op := range operands {
		e, ok := d.walk(op, rc.Path(op), 0, nil)
		if ok {
			grand.size += e.size
			grand.mod = maxTime(grand.mod, e.mod)
		}
	}
	if *total {
		d.print(grand.size, "total", grand.mod)
	}
	return d.exit
}

func normalizeBlockSizeArgs(args []string) []string {
	var out []string
	for i, a := range args {
		if a == "--" {
			out = append(out, args[i:]...)
			break
		}
		if len(a) > 2 && strings.HasPrefix(a, "-B") && !strings.HasPrefix(a, "--") {
			out = append(out, "--block-size="+a[2:])
			continue
		}
		if a == "-M" {
			out = append(out, "--block-size=1M")
			continue
		}
		if len(a) > 2 && a[0] == '-' && a[1] != '-' && strings.Contains(a, "M") {
			out = append(out, "--block-size=1M")
			kept := strings.ReplaceAll(a, "M", "")
			if kept != "-" {
				out = append(out, kept)
			}
			continue
		}
		out = append(out, a)
	}
	return out
}

func (d *duRun) walk(display, full string, depth int, rootDev *uint64) (duEntry, bool) {
	if d.excluded(display) {
		return duEntry{}, true
	}
	fi, err := d.stat(full, depth == 0)
	if err != nil {
		fmt.Fprintf(d.rc.Err, "du: cannot access '%s': %s\n", display, errMsg(err))
		d.exit = 1
		return duEntry{}, false
	}
	if d.oneFileSystem {
		if dev, ok := fileDev(fi); ok {
			if rootDev == nil {
				rootDev = &dev
			} else if depth > 0 && dev != *rootDev && fi.IsDir() {
				return duEntry{}, true
			}
		}
	}
	if fi.IsDir() {
		// GNU du counts a directory's own DISK BLOCKS (default/block mode) but NOT
		// its apparent st_size: with -b/--apparent-size the total is the sum of file
		// *contents* only, so a directory contributes 0 to its own apparent size.
		// (GNU `stat` still reports the dir's st_size; GNU `du -b` ignores it.)
		var total int64
		if d.inodes || !d.apparent {
			total = d.amount(fi)
		}
		mod := fi.ModTime()
		ents, rerr := os.ReadDir(full)
		if rerr != nil {
			fmt.Fprintf(d.rc.Err, "du: cannot read directory '%s': %s\n", display, errMsg(rerr))
			d.exit = 1
		}
		for _, de := range ents {
			e, _ := d.walk(joinDisplay(display, de.Name()), filepath.Join(full, de.Name()), depth+1, rootDev)
			if !d.separateDirs || !e.dir {
				total += e.size
			}
			mod = maxTime(mod, e.mod)
		}
		if depth <= d.maxDepth {
			d.print(total, display, mod)
		}
		return duEntry{size: total, mod: mod, dir: true}, true
	}
	if d.skipHardlink(fi) {
		return duEntry{}, true
	}
	total := d.amount(fi)
	// File operands are always reported; files inside the tree only
	// with --all (and within the depth limit).
	if depth == 0 || (d.all && depth <= d.maxDepth) {
		d.print(total, display, fi.ModTime())
	}
	return duEntry{size: total, mod: fi.ModTime()}, true
}

func (d *duRun) stat(full string, root bool) (os.FileInfo, error) {
	if d.deref == derefAll || (root && d.deref == derefArgs) {
		return os.Stat(full)
	}
	return os.Lstat(full)
}

func (d *duRun) amount(fi os.FileInfo) int64 {
	if d.inodes {
		return 1
	}
	return d.usage(fi)
}

func (d *duRun) print(n int64, path string, mod time.Time) {
	if d.haveThreshold {
		if d.threshold >= 0 && n < d.threshold {
			return
		}
		if d.threshold < 0 && n > -d.threshold {
			return
		}
	}
	if d.showTime {
		fmt.Fprintf(d.rc.Out, "%s\t%s\t%s%s", d.fmtSize(n), d.fmtTime(mod), path, d.term)
		return
	}
	fmt.Fprintf(d.rc.Out, "%s\t%s%s", d.fmtSize(n), path, d.term)
}

func (d *duRun) fmtSize(n int64) string {
	if d.inodes {
		return strconv.FormatInt(n, 10)
	}
	switch {
	case d.human:
		return humanSize(uint64(n))
	case d.si:
		return siSize(uint64(n))
	default:
		return strconv.FormatInt(divCeil(n, d.block), 10)
	}
}

func (d *duRun) fmtTime(t time.Time) string {
	if t.IsZero() {
		t = time.Unix(0, 0)
	}
	switch d.timeStyle {
	case "", "long-iso":
		return t.Format("2006-01-02 15:04")
	case "full-iso":
		return t.Format("2006-01-02 15:04:05.000000000 -0700")
	case "iso":
		return t.Format("2006-01-02")
	default:
		if strings.HasPrefix(d.timeStyle, "+") {
			return t.Format(goTimeLayout(d.timeStyle[1:]))
		}
		return t.Format("2006-01-02 15:04")
	}
}

func validateTimeStyle(style string) error {
	switch {
	case style == "", style == "full-iso", style == "long-iso", style == "iso":
		return nil
	case strings.HasPrefix(style, "+"):
		return nil
	default:
		return fmt.Errorf("unsupported --time-style=%s", style)
	}
}

func goTimeLayout(format string) string {
	repl := strings.NewReplacer(
		"%Y", "2006",
		"%y", "06",
		"%m", "01",
		"%d", "02",
		"%H", "15",
		"%M", "04",
		"%S", "05",
		"%F", "2006-01-02",
		"%T", "15:04:05",
		"%%", "%",
	)
	return repl.Replace(format)
}

func maxTime(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

func (d *duRun) excluded(display string) bool {
	if len(d.excludes) == 0 {
		return false
	}
	clean := filepath.ToSlash(display)
	base := path.Base(clean)
	for _, pat := range d.excludes {
		pat = filepath.ToSlash(pat)
		if pat == "" {
			continue
		}
		if ok, _ := path.Match(pat, clean); ok {
			return true
		}
		if ok, _ := path.Match(pat, base); ok {
			return true
		}
		if pat == clean || pat == base {
			return true
		}
	}
	return false
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

func readFiles0(rc *tool.RunContext, name string) ([]string, error) {
	var data []byte
	var err error
	if name == "-" {
		data, err = io.ReadAll(rc.In)
	} else {
		data, err = os.ReadFile(rc.Path(name))
	}
	if err != nil {
		return nil, err
	}
	parts := bytes.Split(data, []byte{0})
	names := make([]string, 0, len(parts))
	for i, p := range parts {
		if len(p) == 0 {
			if i == len(parts)-1 {
				continue
			}
			return nil, fmt.Errorf("invalid zero-length file name")
		}
		names = append(names, string(p))
	}
	return names, nil
}

func readExcludePatterns(rc *tool.RunContext, name string) ([]string, error) {
	var data []byte
	var err error
	if name == "-" {
		data, err = io.ReadAll(rc.In)
	} else {
		data, err = os.ReadFile(rc.Path(name))
	}
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	patterns := make([]string, 0, len(lines))
	for _, ln := range lines {
		if ln == "" {
			continue
		}
		patterns = append(patterns, ln)
	}
	return patterns, nil
}

func parseBlockSize(s string) (int64, error) {
	if s == "" {
		return 0, fmt.Errorf("empty block size")
	}
	mult := int64(1)
	num := strings.ToUpper(strings.TrimSpace(s))
	switch {
	case strings.HasSuffix(num, "KIB"):
		mult, num = 1024, strings.TrimSuffix(num, "KIB")
	case strings.HasSuffix(num, "MIB"):
		mult, num = 1024*1024, strings.TrimSuffix(num, "MIB")
	case strings.HasSuffix(num, "GIB"):
		mult, num = 1024*1024*1024, strings.TrimSuffix(num, "GIB")
	case strings.HasSuffix(num, "TIB"):
		mult, num = 1024*1024*1024*1024, strings.TrimSuffix(num, "TIB")
	case strings.HasSuffix(num, "KB"):
		mult, num = 1000, strings.TrimSuffix(num, "KB")
	case strings.HasSuffix(num, "MB"):
		mult, num = 1000*1000, strings.TrimSuffix(num, "MB")
	case strings.HasSuffix(num, "GB"):
		mult, num = 1000*1000*1000, strings.TrimSuffix(num, "GB")
	case strings.HasSuffix(num, "TB"):
		mult, num = 1000*1000*1000*1000, strings.TrimSuffix(num, "TB")
	case strings.HasSuffix(num, "K"):
		mult, num = 1024, strings.TrimSuffix(num, "K")
	case strings.HasSuffix(num, "M"):
		mult, num = 1024*1024, strings.TrimSuffix(num, "M")
	case strings.HasSuffix(num, "G"):
		mult, num = 1024*1024*1024, strings.TrimSuffix(num, "G")
	case strings.HasSuffix(num, "T"):
		mult, num = 1024*1024*1024*1024, strings.TrimSuffix(num, "T")
	}
	if num == "" {
		num = "1"
	}
	n, err := strconv.ParseInt(num, 10, 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("bad size")
	}
	if n > math.MaxInt64/mult {
		return 0, fmt.Errorf("size overflow")
	}
	return n * mult, nil
}

func parseSignedSize(s string) (int64, error) {
	if s == "" {
		return 0, fmt.Errorf("empty size")
	}
	sign := int64(1)
	if strings.HasPrefix(s, "-") {
		sign = -1
		s = strings.TrimPrefix(s, "-")
	} else if strings.HasPrefix(s, "+") {
		s = strings.TrimPrefix(s, "+")
	}
	n, err := parseBlockSize(s)
	if err != nil {
		return 0, err
	}
	return sign * n, nil
}

func divCeil(n, d int64) int64 {
	if n <= 0 {
		return 0
	}
	return (n + d - 1) / d
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

func siSize(n uint64) string {
	if n < 1000 {
		return strconv.FormatUint(n, 10)
	}
	const units = "KMGTPE"
	div := uint64(1000)
	idx := 0
	for n/div >= 1000 && idx < len(units)-1 {
		div *= 1000
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
	if v >= 1000 && idx < len(units)-1 {
		return fmt.Sprintf("1.0%c", units[idx+1])
	}
	return fmt.Sprintf("%d%c", v, units[idx])
}
