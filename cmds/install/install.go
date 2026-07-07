// Package installcmd implements a practical GNU install(1) subset:
// directory creation plus copying regular files with final chmod.
// Supports backup (-b), compare (-C), preserve-timestamps (-p),
// strip (-s, no-op), suffix (-S), and context (-Z, --preserve-context, no-op).
package installcmd

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"unicode"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "install",
	Synopsis: "Copy files and set attributes, or create directories.",
	Usage: "install [OPTION]... SOURCE DEST\n" +
		"   or: install [OPTION]... SOURCE... DIRECTORY\n" +
		"   or: install -d [OPTION]... DIRECTORY...",
}

// Run is wired in init: a literal would create an initialization cycle.
func init() { cmd.Run = run; tool.Register(cmd) }

type installer struct {
	rc       *tool.RunContext
	verbose  bool
	mode     os.FileMode
	failed   bool
	backupFn func(src, dst string) error // backup handler
}

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	dirs := fs.BoolP("directory", "d", false, "treat all arguments as directory names; create all components")
	createParents := fs.BoolP("create-leading-directories", "D", false, "create leading components of the destination")
	modeStr := fs.StringP("mode", "m", "755", "set file mode (octal)")
	targetDir := fs.StringP("target-directory", "t", "", "install into DIRECTORY")
	noTargetDir := fs.BoolP("no-target-directory", "T", false, "treat DEST as a normal file")
	verbose := fs.BoolP("verbose", "v", false, "print the name of each created file or directory")
	owner := fs.StringP("owner", "o", "", "set ownership (not supported)")
	group := fs.StringP("group", "g", "", "set group ownership (not supported)")
	backup := fs.BoolP("backup", "b", false, "make a backup of each existing destination file")
	compare := fs.BoolP("compare", "C", false, "compare content; skip if identical")
	preserveTimestamps := fs.BoolP("preserve-timestamps", "p", false, "apply source timestamps to destination")
	strip := fs.BoolP("strip", "s", false, "strip symbol tables (no-op without native strip)")
	suffix := fs.StringP("suffix", "S", "~", "override the usual backup suffix")
	contextStr := fs.StringP("context", "Z", "", "set SELinux security context (no-op without SELinux)")
	preserveContext := fs.Bool("preserve-context", false, "preserve SELinux security context (no-op without SELinux)")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	_ = contextStr
	_ = preserveContext
	_ = strip // no-op: no native strip support in pure-Go

	if fs.Changed("owner") || fs.Changed("group") {
		_ = owner
		_ = group
		return tool.NotSupported(rc, cmd, "-o/--owner and -g/--group (no shared ownership helper)")
	}
	if runtime.GOOS == "windows" && fs.Changed("mode") {
		return tool.NotSupported(rc, cmd, "-m/--mode on windows (no POSIX mode bits; mapping to read-only would change the documented meaning)")
	}
	mode, errCode := parseMode(rc, *modeStr)
	if errCode >= 0 {
		return errCode
	}
	in := &installer{rc: rc, verbose: *verbose, mode: mode}

	hasBackup := *backup
	backupSuffix := *suffix
	if hasBackup {
		in.backupFn = func(src, dst string) error {
			return makeBackup(rc, dst, backupSuffix, "existing")
		}
	}

	switch {
	case *dirs:
		if *targetDir != "" || *noTargetDir || *createParents || *preserveTimestamps || *compare || hasBackup {
			return tool.UsageError(rc, cmd, "the -d option cannot be combined with -D, -t, -T, -p, -C, or -b")
		}
		if len(operands) == 0 {
			return tool.UsageError(rc, cmd, "missing file operand")
		}
		for _, op := range operands {
			in.makeDir(op)
		}
	case *targetDir != "":
		if *noTargetDir {
			return tool.UsageError(rc, cmd, "cannot combine -t and -T")
		}
		if len(operands) == 0 {
			return tool.UsageError(rc, cmd, "missing file operand")
		}
		if *createParents {
			in.makeParentDir(filepath.Join(*targetDir, "x"))
		}
		for _, src := range operands {
			dst := filepath.Join(*targetDir, filepath.Base(src))
			in.copyFile(src, dst, *createParents, *preserveTimestamps, *compare)
		}
	default:
		switch len(operands) {
		case 0:
			return tool.UsageError(rc, cmd, "missing file operand")
		case 1:
			return tool.UsageError(rc, cmd, "missing destination file operand after '%s'", operands[0])
		}
		dest := operands[len(operands)-1]
		srcs := operands[:len(operands)-1]
		if len(srcs) > 1 && *noTargetDir {
			return tool.UsageError(rc, cmd, "extra operand '%s'", srcs[1])
		}
		if len(srcs) > 1 {
			if !isDir(rc.Path(dest)) {
				fmt.Fprintf(rc.Err, "install: target '%s' is not a directory\n", dest)
				return 1
			}
			for _, src := range srcs {
				in.copyFile(src, filepath.Join(dest, filepath.Base(src)), false, *preserveTimestamps, *compare)
			}
			break
		}
		dst := dest
		if !*noTargetDir && isDir(rc.Path(dest)) {
			dst = filepath.Join(dest, filepath.Base(srcs[0]))
		}
		in.copyFile(srcs[0], dst, *createParents, *preserveTimestamps, *compare)
	}
	if in.failed {
		return 1
	}
	return 0
}

func parseMode(rc *tool.RunContext, s string) (os.FileMode, int) {
	n, err := strconv.ParseUint(s, 8, 32)
	if err == nil && n <= 0o7777 {
		mode := os.FileMode(n & 0o777)
		if n&0o1000 != 0 {
			mode |= os.ModeSticky
		}
		if n&0o2000 != 0 {
			mode |= os.ModeSetgid
		}
		if n&0o4000 != 0 {
			mode |= os.ModeSetuid
		}
		return mode, -1
	}
	if s == "" || allDigits(s) {
		return 0, tool.UsageError(rc, cmd, "invalid mode '%s'", s)
	}
	return 0, tool.NotSupported(rc, cmd, fmt.Sprintf("symbolic mode '%s' for -m/--mode (only octal modes)", s))
}

func allDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

func makeBackup(rc *tool.RunContext, dst, suffix, config string) error {
	backupName := dst + suffix
	if _, err := os.Stat(rc.Path(dst)); os.IsNotExist(err) {
		return nil
	}
	if config == "numbered" || config == "t" {
		for i := 1; i < 1000; i++ {
			candidate := fmt.Sprintf("%s%s.%d~", dst, suffix, i)
			if _, err := os.Stat(rc.Path(candidate)); os.IsNotExist(err) {
				backupName = candidate
				break
			}
		}
	}
	return os.Rename(rc.Path(dst), rc.Path(backupName))
}

func (in *installer) makeDir(name string) {
	if name == "" {
		in.errf("cannot create directory '': No such file or directory")
		return
	}
	created := !isDir(in.rc.Path(name))
	if err := os.MkdirAll(in.rc.Path(name), 0o777); err != nil {
		in.errf("cannot create directory '%s': %s", name, reason(err))
		return
	}
	if err := os.Chmod(in.rc.Path(name), in.mode); err != nil {
		in.errf("cannot change permissions of '%s': %s", name, reason(err))
		return
	}
	if created {
		in.verbosef("install: creating directory '%s'", name)
	}
}

func (in *installer) makeParentDir(dst string) bool {
	parent := filepath.Dir(dst)
	if parent == "." || parent == dst {
		return true
	}
	if err := os.MkdirAll(in.rc.Path(parent), 0o777); err != nil {
		in.errf("cannot create directory '%s': %s", parent, reason(err))
		return false
	}
	return true
}

func (in *installer) copyFile(src, dst string, parents, preserveTs, compare bool) {
	if src == "" {
		in.errf("cannot stat '': No such file or directory")
		return
	}
	fi, err := os.Stat(in.rc.Path(src))
	if err != nil {
		in.errf("cannot stat '%s': %s", src, reason(err))
		return
	}
	if fi.IsDir() {
		in.errf("omitting directory '%s'", src)
		return
	}
	if parents && !in.makeParentDir(dst) {
		return
	}

	if compare {
		srcData, err := os.ReadFile(in.rc.Path(src))
		if err != nil {
			in.errf("cannot read '%s': %s", src, reason(err))
			return
		}
		dstData, err := os.ReadFile(in.rc.Path(dst))
		if err == nil && bytes.Equal(srcData, dstData) {
			in.verbosef("'%s' -> '%s' (unchanged, skipped)", src, dst)
			return
		}
	}

	if di, err := os.Stat(in.rc.Path(dst)); err == nil {
		if os.SameFile(fi, di) {
			in.errf("'%s' and '%s' are the same file", src, dst)
			return
		}
		if di.IsDir() {
			in.errf("cannot overwrite directory '%s' with non-directory", dst)
			return
		}
		if in.backupFn != nil {
			if err := in.backupFn(src, dst); err != nil {
				in.errf("cannot backup '%s': %s", dst, reason(err))
				return
			}
		} else {
			if err := os.Remove(in.rc.Path(dst)); err != nil {
				in.errf("cannot remove '%s': %s", dst, reason(err))
				return
			}
		}
	}
	inp, err := os.Open(in.rc.Path(src))
	if err != nil {
		in.errf("cannot open '%s' for reading: %s", src, reason(err))
		return
	}
	defer inp.Close()
	out, err := os.OpenFile(in.rc.Path(dst), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o777)
	if err != nil {
		in.errf("cannot create regular file '%s': %s", dst, reason(err))
		return
	}
	_, werr := io.Copy(out, inp)
	cerr := out.Close()
	if werr != nil {
		in.errf("error writing '%s': %s", dst, reason(werr))
		return
	}
	if cerr != nil {
		in.errf("error writing '%s': %s", dst, reason(cerr))
		return
	}
	if err := os.Chmod(in.rc.Path(dst), in.mode); err != nil {
		in.errf("cannot change permissions of '%s': %s", dst, reason(err))
		return
	}
	if preserveTs {
		atime := fi.ModTime()
		if err := os.Chtimes(in.rc.Path(dst), atime, atime); err != nil {
			in.errf("cannot set timestamps of '%s': %s", dst, reason(err))
			return
		}
	}
	in.verbosef("'%s' -> '%s'", src, dst)
}

func isDir(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

func (in *installer) errf(format string, a ...any) {
	fmt.Fprintf(in.rc.Err, "install: "+format+"\n", a...)
	in.failed = true
}

func (in *installer) verbosef(format string, a ...any) {
	if in.verbose {
		fmt.Fprintf(in.rc.Out, format+"\n", a...)
	}
}

func reason(err error) string {
	var pe *os.PathError
	if errors.As(err, &pe) {
		err = pe.Err
	}
	if errors.Is(err, fs.ErrNotExist) {
		err = fs.ErrNotExist
	}
	var se *os.SyscallError
	if errors.As(err, &se) {
		err = se.Err
	}
	s := err.Error()
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}
