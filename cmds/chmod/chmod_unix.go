//go:build unix

package chmodcmd

import (
	"fmt"
	"io"
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
func apply(rc *tool.RunContext, change *modeChange, files []string, recursive, verbose, changes, silent, preserveRoot bool) int {
	um := umask()
	exit := 0
	v := verbose || changes
	chOnly := changes
	sf := silent
	pr := preserveRoot

	for _, name := range files {
		root := rc.Path(name)
		if recursive && pr && root == "/" {
			fmt.Fprintf(rc.Err, "chmod: it is dangerous to operate recursively on '/'\n")
			fmt.Fprintf(rc.Err, "chmod: use --no-preserve-root to override this failsafe\n")
			exit = 1
			continue
		}
		if !recursive {
			fi, err := os.Stat(root)
			if err != nil {
				if !sf {
					fmt.Fprintf(rc.Err, "chmod: cannot access '%s': %v\n", name, reason(err))
				}
				exit = 1
				continue
			}
			oldBits := fileModeToBits(fi.Mode())
			newBits := change.apply(oldBits, fi.IsDir(), um)
			if oldBits != newBits {
				if err := os.Chmod(root, bitsToFileMode(newBits)); err != nil {
					if !sf {
						fmt.Fprintf(rc.Err, "chmod: changing permissions of '%s': %v\n", name, reason(err))
					}
					exit = 1
					continue
				}
				if v && !chOnly {
					chmodVerbose(rc.Out, name, true, newBits, v, chOnly)
				} else if v {
					chmodVerbose(rc.Out, name, true, newBits, v, chOnly)
				}
			} else {
				if v && !chOnly {
					chmodVerbose(rc.Out, name, false, newBits, v, chOnly)
				}
			}
			continue
		}
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				if !sf {
					fmt.Fprintf(rc.Err, "chmod: cannot access '%s': %v\n", path, reason(err))
				}
				exit = 1
				return nil
			}
			if d.Type()&fs.ModeSymlink != 0 {
				return nil
			}
			fi, err := d.Info()
			if err != nil {
				if !sf {
					fmt.Fprintf(rc.Err, "chmod: cannot access '%s': %v\n", path, reason(err))
				}
				exit = 1
				return nil
			}
			oldBits := fileModeToBits(fi.Mode())
			newBits := change.apply(oldBits, fi.IsDir(), um)
			if oldBits != newBits {
				if err := os.Chmod(path, bitsToFileMode(newBits)); err != nil {
					if !sf {
						fmt.Fprintf(rc.Err, "chmod: changing permissions of '%s': %v\n", path, reason(err))
					}
					exit = 1
					return nil
				}
				if v {
					chmodVerbose(rc.Out, path, true, newBits, v, chOnly)
				}
			} else {
				if v && !chOnly {
					chmodVerbose(rc.Out, path, false, newBits, v, chOnly)
				}
			}
			return nil
		})
		if err != nil {
			if !sf {
				fmt.Fprintf(rc.Err, "chmod: cannot access '%s': %v\n", name, reason(err))
			}
			exit = 1
		}
	}
	return exit
}

func chmodVerbose(out io.Writer, name string, changed bool, newBits uint32, verbose, changes bool) {
	if !verbose {
		return
	}
	if changes && !changed {
		return
	}
	if changed {
		fmt.Fprintf(out, "mode of '%s' changed to %04o\n", name, newBits)
	} else if !changes {
		fmt.Fprintf(out, "mode of '%s' retained as %04o\n", name, newBits)
	}
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
