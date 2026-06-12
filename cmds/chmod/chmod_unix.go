//go:build unix

package chmodcmd

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"

	"github.com/qiangli/coreutils/tool"
)

// apply changes the mode of every operand (and, with -R, everything
// beneath directory operands). Symbolic links encountered during
// recursion are ignored, mirroring GNU chmod; command-line operands
// are dereferenced (os.Stat / os.Chmod follow symlinks).
func apply(rc *tool.RunContext, change *modeChange, files []string, recursive bool) int {
	um := umask()
	exit := 0
	for _, name := range files {
		root := rc.Path(name)
		if !recursive {
			fi, err := os.Stat(root)
			if err != nil {
				fmt.Fprintf(rc.Err, "chmod: cannot access '%s': %v\n", name, reason(err))
				exit = 1
				continue
			}
			if err := chmodOne(root, fi, change, um); err != nil {
				fmt.Fprintf(rc.Err, "chmod: changing permissions of '%s': %v\n", name, reason(err))
				exit = 1
			}
			continue
		}
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				fmt.Fprintf(rc.Err, "chmod: cannot access '%s': %v\n", path, reason(err))
				exit = 1
				return nil
			}
			if d.Type()&fs.ModeSymlink != 0 {
				return nil // GNU chmod never touches symlinks in a traversal
			}
			fi, err := d.Info()
			if err != nil {
				fmt.Fprintf(rc.Err, "chmod: cannot access '%s': %v\n", path, reason(err))
				exit = 1
				return nil
			}
			if err := chmodOne(path, fi, change, um); err != nil {
				fmt.Fprintf(rc.Err, "chmod: changing permissions of '%s': %v\n", path, reason(err))
				exit = 1
			}
			return nil
		})
		if err != nil {
			fmt.Fprintf(rc.Err, "chmod: cannot access '%s': %v\n", name, reason(err))
			exit = 1
		}
	}
	return exit
}

func chmodOne(path string, fi os.FileInfo, change *modeChange, um uint32) error {
	newBits := change.apply(fileModeToBits(fi.Mode()), fi.IsDir(), um)
	return os.Chmod(path, bitsToFileMode(newBits))
}

// umask reads the process umask (set-and-restore — the only portable
// way POSIX offers). Needed for symbolic clauses with no explicit who.
func umask() uint32 {
	old := unix.Umask(0)
	unix.Umask(old)
	return uint32(old) & 0o777
}
