// Package lncmd implements ln(1) per the GNU coreutils manual: make
// hard or symbolic links between files.
//
// Portions adapted from https://github.com/u-root/u-root cmds/core/ln (BSD-3-Clause).
// Changes: rewired to tool framework; reduced to the -s/-f/-v subset;
// operand-form detection kept, RunContext-relative path resolution
// added (symlink TARGET text is stored verbatim, as GNU does).
package lncmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "ln",
	Synopsis: "Make links between files (hard by default, symbolic with --symbolic).",
	Usage: "ln [OPTION]... TARGET LINK_NAME\n" +
		"   or: ln [OPTION]... TARGET\n" +
		"   or: ln [OPTION]... TARGET... DIRECTORY",
}

// Run is wired in init: a literal would create an initialization cycle.
func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	symbolic := fs.BoolP("symbolic", "s", false, "make symbolic links instead of hard links")
	force := fs.BoolP("force", "f", false, "remove existing destination files")
	logical := fs.BoolP("logical", "L", false, "dereference TARGETs that are symbolic links")
	physical := fs.BoolP("physical", "P", false, "make hard links directly to symbolic links")
	backup := fs.StringP("backup", "b", "", "make a backup of each existing destination file")
	fs.Lookup("backup").NoOptDefVal = "existing"
	suffix := fs.StringP("suffix", "S", "~", "override the usual backup suffix")
	interactive := fs.BoolP("interactive", "i", false, "prompt whether to remove destinations")
	noDeref := fs.BoolP("no-dereference", "n", false, "treat LINK_NAME as a normal file if it is a symlink to a directory")
	relative := fs.BoolP("relative", "r", false, "with -s, create links relative to link location")
	targetDir := fs.StringP("target-directory", "t", "", "specify the DIRECTORY in which to create the links")
	noTargetDir := fs.BoolP("no-target-directory", "T", false, "treat LINK_NAME as a normal file always")
	verbose := fs.BoolP("verbose", "v", false, "print name of each linked file")
	replaceMode := replacementMode(args)
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	if len(operands) == 0 {
		return tool.UsageError(rc, cmd, "missing file operand")
	}
	if *targetDir != "" && *noTargetDir {
		return tool.UsageError(rc, cmd, "cannot combine -t and -T")
	}
	if *relative && !*symbolic {
		return tool.UsageError(rc, cmd, "--relative can only be used with --symbolic")
	}
	backupMode := ""
	backupRequested := fs.Changed("backup")
	if fs.Changed("backup") {
		var ok bool
		backupMode, ok = normalizeBackupMode(*backup)
		if !ok {
			return tool.NotSupported(rc, cmd, "--backup="+*backup+" (only none/simple/existing/numbered controls)")
		}
		if backupMode == "none" {
			backupRequested = false
			backupMode = ""
		}
	}
	if !fs.Changed("suffix") {
		if envSuffix := rc.Getenv("SIMPLE_BACKUP_SUFFIX"); envSuffix != "" {
			*suffix = envSuffix
		}
	}
	if *logical && *physical {
		if lastDereferenceMode(args) == "logical" {
			*physical = false
		} else {
			*logical = false
		}
	}
	if replaceMode == "interactive" {
		*force = false
	} else if replaceMode == "force" {
		*interactive = false
	}
	reader := bufio.NewReader(rc.In)

	// Decide which GNU form applies: TARGET, TARGET LINK_NAME, or
	// TARGET... DIRECTORY (last operand is an existing directory).
	targets := operands
	linkName := ""
	dir := *targetDir
	if dir != "" {
		if fi, err := os.Stat(rc.Path(dir)); err != nil || !fi.IsDir() {
			fmt.Fprintf(rc.Err, "ln: target directory '%s' is not a directory\n", dir)
			return 1
		}
	} else if len(operands) > 1 {
		last := operands[len(operands)-1]
		if !*noTargetDir && isDestDir(rc, last, *noDeref) {
			dir = last
			targets = operands[:len(operands)-1]
		} else if len(operands) == 2 {
			targets = operands[:1]
			linkName = last
		} else {
			fmt.Fprintf(rc.Err, "ln: target '%s' is not a directory\n", last)
			return 1
		}
	}

	exit := 0
	for _, target := range targets {
		dest := linkName
		if dest == "" {
			dest = filepath.Base(target)
		}
		if dir != "" {
			dest = filepath.Join(dir, filepath.Base(target))
		}
		destPath := rc.Path(dest)
		linkTarget := target
		if *symbolic && *relative {
			var err error
			linkTarget, err = relativeTarget(rc, target, dest)
			if err != nil {
				fmt.Fprintf(rc.Err, "ln: failed to create symbolic link '%s': %v\n", dest, reason(err))
				exit = 1
				continue
			}
		}
		if *force {
			if !prepareExistingDestination(rc, dest, destPath, backupMode, *suffix, false) {
				exit = 1
				continue
			}
		} else if backupRequested || *interactive {
			ok, skipped := prepareOptionalExistingDestination(rc, reader, dest, destPath, backupMode, *suffix, *interactive)
			if skipped {
				continue
			}
			if !ok {
				exit = 1
				continue
			}
		}
		var err error
		if *symbolic {
			err = os.Symlink(linkTarget, destPath)
		} else {
			err = createHardLink(rc.Path(target), destPath, *logical, *physical)
		}
		if err != nil {
			kind := "hard"
			if *symbolic {
				kind = "symbolic"
			}
			fmt.Fprintf(rc.Err, "ln: failed to create %s link '%s': %v\n", kind, dest, reason(err))
			exit = 1
			continue
		}
		if *verbose {
			arrow := "=>" // GNU prints => for hard links, -> for symlinks
			if *symbolic {
				arrow = "->"
			}
			fmt.Fprintf(rc.Out, "'%s' %s '%s'\n", dest, arrow, linkTarget)
		}
	}
	return exit
}

func prepareOptionalExistingDestination(rc *tool.RunContext, reader *bufio.Reader, dest, destPath, backupMode, suffix string, interactive bool) (ok, skipped bool) {
	if _, err := os.Lstat(destPath); err != nil {
		if os.IsNotExist(err) {
			return true, false
		}
		fmt.Fprintf(rc.Err, "ln: cannot access '%s': %v\n", dest, reason(err))
		return false, false
	}
	if interactive {
		fmt.Fprintf(rc.Err, "ln: replace '%s'? ", dest)
		yes, err := readYes(reader)
		if err != nil && err != io.EOF {
			fmt.Fprintf(rc.Err, "ln: cannot read response: %v\n", reason(err))
			return false, false
		}
		if !yes {
			return true, true
		}
	}
	return prepareExistingDestination(rc, dest, destPath, backupMode, suffix, true), false
}

func prepareExistingDestination(rc *tool.RunContext, dest, destPath, backupMode, suffix string, mustExist bool) bool {
	if _, err := os.Lstat(destPath); err != nil {
		if os.IsNotExist(err) && !mustExist {
			return true
		}
		fmt.Fprintf(rc.Err, "ln: cannot access '%s': %v\n", dest, reason(err))
		return false
	}
	if backupMode != "" {
		backupPath, err := backupName(destPath, backupMode, suffix)
		if err != nil {
			fmt.Fprintf(rc.Err, "ln: cannot backup '%s': %v\n", dest, reason(err))
			return false
		}
		if err := os.Rename(destPath, backupPath); err != nil {
			fmt.Fprintf(rc.Err, "ln: cannot backup '%s': %v\n", dest, reason(err))
			return false
		}
		return true
	}
	if err := os.Remove(destPath); err != nil {
		fmt.Fprintf(rc.Err, "ln: cannot remove '%s': %v\n", dest, reason(err))
		return false
	}
	return true
}

func createHardLink(targetPath, destPath string, logical, physical bool) error {
	if logical {
		resolved, err := filepath.EvalSymlinks(targetPath)
		if err != nil {
			return err
		}
		return os.Link(resolved, destPath)
	}
	if physical {
		return hardLinkPhysical(targetPath, destPath)
	}
	return os.Link(targetPath, destPath)
}

func readYes(reader *bufio.Reader) (bool, error) {
	line, err := reader.ReadString('\n')
	if err != nil && len(line) == 0 {
		return false, err
	}
	line = strings.TrimLeft(line, " \t")
	return strings.HasPrefix(line, "y") || strings.HasPrefix(line, "Y"), err
}

func normalizeBackupMode(mode string) (string, bool) {
	switch mode {
	case "none", "off":
		return "none", true
	case "", "existing", "nil":
		return "existing", true
	case "simple", "never":
		return "simple", true
	case "numbered", "t":
		return "numbered", true
	default:
		return "", false
	}
}

func backupName(path, mode, suffix string) (string, error) {
	if mode == "existing" {
		if hasNumberedBackup(path) {
			mode = "numbered"
		} else {
			mode = "simple"
		}
	}
	if mode == "simple" {
		return path + suffix, nil
	}
	n, err := nextNumberedBackup(path)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s.~%d~", path, n), nil
}

func hasNumberedBackup(path string) bool {
	n, err := nextNumberedBackup(path)
	return err == nil && n > 1
}

func nextNumberedBackup(path string) (int, error) {
	dir, base := filepath.Dir(path), filepath.Base(path)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}
	max := 0
	prefix := base + ".~"
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, "~") {
			continue
		}
		n, err := strconv.Atoi(name[len(prefix) : len(name)-1])
		if err == nil && n > max {
			max = n
		}
	}
	return max + 1, nil
}

func replacementMode(args []string) string {
	mode := ""
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			break
		}
		switch {
		case arg == "-f" || arg == "--force":
			mode = "force"
		case arg == "-i" || arg == "--interactive":
			mode = "interactive"
		case strings.HasPrefix(arg, "--"):
			continue
		case strings.HasPrefix(arg, "-") && arg != "-":
			for _, r := range arg[1:] {
				switch r {
				case 'f':
					mode = "force"
				case 'i':
					mode = "interactive"
				}
			}
		}
	}
	return mode
}

func lastDereferenceMode(args []string) string {
	mode := ""
	for _, arg := range args {
		if arg == "--" {
			break
		}
		switch {
		case arg == "-L" || arg == "--logical":
			mode = "logical"
		case arg == "-P" || arg == "--physical":
			mode = "physical"
		case strings.HasPrefix(arg, "--"):
			continue
		case strings.HasPrefix(arg, "-") && arg != "-":
			for _, r := range arg[1:] {
				switch r {
				case 'L':
					mode = "logical"
				case 'P':
					mode = "physical"
				}
			}
		}
	}
	return mode
}

func isDestDir(rc *tool.RunContext, operand string, noDeref bool) bool {
	var (
		fi  os.FileInfo
		err error
	)
	if noDeref {
		fi, err = os.Lstat(rc.Path(operand))
	} else {
		fi, err = os.Stat(rc.Path(operand))
	}
	return err == nil && fi.IsDir()
}

func relativeTarget(rc *tool.RunContext, target, dest string) (string, error) {
	targetAbs := rc.Path(target)
	if !filepath.IsAbs(targetAbs) {
		targetAbs = filepath.Join(rc.Dir, target)
	}
	destDir := filepath.Dir(rc.Path(dest))
	rel, err := filepath.Rel(destDir, targetAbs)
	if err != nil {
		return "", err
	}
	return rel, nil
}

// reason unwraps os wrapper errors and GNU-capitalizes so diagnostics read
// like GNU's (strerror shape).
func reason(err error) error {
	return tool.SysErr(err)
}
