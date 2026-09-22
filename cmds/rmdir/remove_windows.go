//go:build windows

package rmdircmd

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/qiangli/coreutils/cmds/internal/pathops"
	"github.com/qiangli/coreutils/tool"
	"golang.org/x/sys/windows"
)

// Windows refuses RemoveDirectory when the directory itself has the
// read-only attribute, even though POSIX rmdir is governed by permissions
// on its parent. The recorded ACL mode remains in place while we clear only
// that attribute for the retry. A failed retry restores the attribute.
func removeDirectory(path string, info os.FileInfo) error {
	parent, err := tool.Stat(filepath.Dir(path))
	if err != nil {
		return err
	}
	if tool.ModeRecorded(parent) && parent.Mode().Perm()&0o300 != 0o300 {
		return fs.ErrPermission
	}
	err = pathops.Remove(path)
	if err == nil || info.Mode().Perm()&0o200 != 0 || !errors.Is(err, fs.ErrPermission) {
		return err
	}
	if changeErr := os.Chmod(path, 0o666); changeErr != nil {
		return err
	}
	retryErr := pathops.Remove(path)
	if retryErr != nil {
		_ = os.Chmod(path, info.Mode().Perm())
	}
	return retryErr
}

// Windows normalizes a terminal ".." before Lstat and RemoveDirectory, so
// even `missing/..` can identify and remove the invocation directory. Check
// the raw prefix before either operation; POSIX permits ENOTEMPTY for a
// valid prefix and requires failure for the terminal ".." itself.
func terminalDotDotError(rc *tool.RunContext, operand string) (error, bool) {
	operand = strings.TrimRight(operand, `/\`)
	if filepath.Base(operand) != ".." {
		return nil, false
	}
	// OperandDir and filepath.Dir clean "f/.." to "." on Windows. Slice
	// the operand itself so the file or missing component stays visible.
	prefix := strings.TrimRight(strings.TrimSuffix(operand, ".."), `/\`)
	if prefix == "" {
		if strings.HasPrefix(operand, "/") || strings.HasPrefix(operand, `\`) {
			prefix = operand[:1]
		} else {
			prefix = "."
		}
	}
	if volume := filepath.VolumeName(operand); volume != "" && prefix == volume &&
		len(operand) > len(volume) && os.IsPathSeparator(operand[len(volume)]) {
		prefix = operand[:len(volume)+1]
	}
	info, err := os.Stat(rc.RawPath(prefix))
	if err != nil {
		return err, true
	}
	if !info.IsDir() {
		return windows.ERROR_DIRECTORY, true
	}
	return syscall.ENOTEMPTY, true
}
