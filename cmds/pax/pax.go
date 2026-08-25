package paxcmd

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/qiangli/coreutils/pkg/pax"
	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "pax",
	Synopsis: "Portable archive interchange.",
	Usage: `pax [-cdnv] [-f archive] [-s replstr] [pattern...]
  pax -r [-cdiknuv] [-f archive] [-p string] [-s replstr] [pattern...]
  pax -w [-dituvX] [-b blocksize] [-a] [-f archive] [-s replstr] [-x format] [file...]
  pax -r -w [-diklntuvX] [-p string] [-s replstr] file... directory`,
}

func init() { cmd.Run = run; tool.Register(cmd) }

type options struct {
	read, write     bool
	archive         string
	verbose         bool
	format          string
	preserve        string
	subst           []substitution
	interactive     bool
	link            bool
	noOverwrite     bool // -k
	newerOnly       bool // -u
	dirsNoDescend   bool // -d
	appendMode      bool // -a
	invertMatch     bool // -c
	selectNoPattern bool // -n

	blocksize    string // -b, parsed into blockBytes after option validation
	blockBytes   int
	archiveTimes map[string]time.Time
	// links maps a source (device, inode) to the first name archived for it,
	// so later names for the same inode become hardlink members.
	links      map[devIno]string
	optionsStr string // -o
	t, X, H, L bool
}

func run(rc *tool.RunContext, args []string) int {
	args = tool.AliasHelpVersion(args)
	fs := tool.NewFlags(cmd.Name)
	var o options
	fs.BoolVarP(&o.read, "read", "r", false, "read (extract) from the archive")
	fs.BoolVarP(&o.write, "write", "w", false, "write (create) an archive")
	fs.StringVarP(&o.archive, "file", "f", "", "archive pathname (default stdin/stdout)")
	fs.BoolVarP(&o.verbose, "verbose", "v", false, "verbose output")
	fs.StringVarP(&o.format, "format", "x", "pax", "archive format: pax, ustar, or cpio")
	fs.StringVarP(&o.preserve, "preserve", "p", "", "preserve file attributes")
	subst := fs.StringArrayP("subst", "s", nil, "rewrite member names with an ed-style substitution")
	fs.BoolVarP(&o.interactive, "interactive", "i", false, "rename members interactively")
	fs.BoolVarP(&o.link, "link", "l", false, "hard-link rather than copy where possible")
	fs.BoolVarP(&o.noOverwrite, "keep", "k", false, "do not overwrite existing files")
	fs.BoolVarP(&o.newerOnly, "update", "u", false, "extract or write only newer files")
	fs.BoolVarP(&o.dirsNoDescend, "no-descend", "d", false, "do not descend into directories")
	fs.BoolVarP(&o.appendMode, "append", "a", false, "append to the archive")
	fs.BoolVarP(&o.invertMatch, "complement", "c", false, "select members NOT matching the patterns")
	fs.BoolVarP(&o.selectNoPattern, "first", "n", false, "select only the first match per pattern")

	fs.StringVarP(&o.blocksize, "blocksize", "b", "", "physical block size: decimal factors joined by 'x', each optionally suffixed b (512), k (1024), or m (1048576); must be a positive multiple of 512 up to 32256 (default 10240, or 5120 for -x cpio)")
	fs.StringVarP(&o.optionsStr, "options", "o", "", "format-specific options")
	fs.BoolVarP(&o.t, "t", "t", false, "reset access times")
	fs.BoolVarP(&o.X, "X", "X", false, "device boundary")
	fs.BoolVarP(&o.H, "H", "H", false, "follow command-line symlinks")
	fs.BoolVarP(&o.L, "L", "L", false, "follow all symlinks")

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
	if fs.Changed("options") {
		return tool.NotSupported(rc, cmd, "-o")
	}
	if fs.Changed("t") {
		if isList || isRead {
			return tool.UsageError(rc, cmd, "-t is valid only in write or copy mode")
		}
		return tool.NotSupported(rc, cmd, "-t")
	}
	if fs.Changed("X") {
		if isList || isRead {
			return tool.UsageError(rc, cmd, "-X is valid only in write or copy mode")
		}
		return tool.NotSupported(rc, cmd, "-X")
	}
	if fs.Changed("H") {
		return tool.NotSupported(rc, cmd, "-H")
	}
	if fs.Changed("L") {
		return tool.NotSupported(rc, cmd, "-L")
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
	if !fs.Changed("blocksize") {
		o.blockBytes = defaultBlockSize(o.format)
	}

	switch {
	case o.read && o.write:
		return copyMode(rc, &o, operands)
	case o.read:
		return readMode(rc, &o, operands)
	case o.write:
		return writeMode(rc, &o, operands)
	default:
		return listMode(rc, &o, operands)
	}
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

// defaultBlockSize is the physical block size pax uses when -b is absent:
// POSIX fixes 10240 bytes (20 512-byte records) for the tar-derived formats
// and 5120 bytes for cpio.
func defaultBlockSize(format string) int {
	if format == "cpio" {
		return 5120
	}
	return 10240
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
	if o.archive == "" || o.archive == "-" {
		return io.NopCloser(rc.In), nil
	}
	return os.Open(resolve(rc, o.archive))
}

// listMode is pax with neither -r nor -w: report the archive's contents.
func listMode(rc *tool.RunContext, o *options, patterns []string) int {
	r, err := openArchive(rc, o)
	if err != nil {
		fmt.Fprintf(rc.Err, "pax: %v\n", err)
		return 1
	}
	defer r.Close()
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
	members, err := pax.ReadManifest(bytes.NewReader(archive.tarData))
	if err != nil {
		fmt.Fprintf(rc.Err, "pax: %v\n", err)
		return 1
	}

	sel := newSelector(o, patterns)
	catalog := make([]selectorMember, 0, len(members))
	for _, m := range members {
		catalog = append(catalog, selectorMember{
			name:  m.Path,
			isDir: m.Kind == pax.KindDir || strings.HasSuffix(m.Path, "/"),
		})
	}
	sel.prime(catalog)

	for _, m := range members {
		isDir := m.Kind == pax.KindDir || strings.HasSuffix(m.Path, "/")
		if !sel.keep(m.Path, isDir) {
			continue
		}
		name := applySubstitutions(o.subst, m.Path, rc.Err)
		if name == "" {
			continue
		}
		if o.verbose {
			fmt.Fprintf(rc.Out, "%s %2d %-8s %-8s %8d %s %s\n",
				modeString(m), 1, "", "", m.Size, m.ModTime.Format("Jan _2 15:04"), name)
		} else {
			fmt.Fprintln(rc.Out, name)
		}
	}

	unmatched := sel.unmatched()
	if len(unmatched) > 0 {
		for _, p := range unmatched {
			fmt.Fprintf(rc.Err, "pax: pattern %q not matched\n", p)
		}
		return 1
	}

	return 0
}

func modeString(m pax.Member) string {
	s := m.Mode.String()
	switch m.Kind {
	case pax.KindDir:
		if !strings.HasPrefix(s, "d") {
			s = "d" + s
		}
	case pax.KindSymlink:
		if !strings.HasPrefix(s, "L") && !strings.HasPrefix(s, "l") {
			s = "l" + s
		}
	}
	return s
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
