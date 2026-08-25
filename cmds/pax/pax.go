package paxcmd

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
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

	blocksize  string // -b
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

	fs.StringVarP(&o.blocksize, "blocksize", "b", "", "block size")
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
		return tool.NotSupported(rc, cmd, "-b")
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
	members, err := pax.ReadManifest(r)
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

var _ = sort.Strings
var _ = time.Now
