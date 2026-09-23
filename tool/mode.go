package tool

import (
	"io/fs"
	"os"

	"mvdan.cc/sh/v3/winmode"
)

// This file is the framework's whole answer to "what is this file's mode"
// on a platform that has no mode bits.
//
// Windows stores no POSIX permissions. Go derives a FileMode there from one
// attribute: a file is 0666, or 0444 when FILE_ATTRIBUTE_READONLY is set,
// and a directory adds 0111. That is enough to answer "did chmod fail" and
// nothing else — `chmod a=wx f` followed by `test -x f`, `ls -l f` or
// `find -perm` all read back a mode nobody set.
//
// So chmod records the mode in the file's discretionary ACL, the way Cygwin
// does, and every reader of a mode consults the same ACL. [RecordMode] is
// the one writer, [Stat], [Lstat] and [StatInfo] are the one reader, and
// mvdan.cc/sh/v3/winmode holds the representation — shared with the shell
// so that `test -x` in a script and `test -x` in the applet cannot disagree
// about the same file.
//
// Everywhere else the filesystem holds the mode itself and all of this is
// the identity.

// ModesAreRecorded reports whether a mode on this platform lives in a place
// the [io/fs] mode does not reach, so that a tool with something to say
// about an unrepresentable mode can say it.
const ModesAreRecorded = winmode.Supported

// Stat is [os.Stat] reporting the recorded mode.
func Stat(path string) (os.FileInfo, error) {
	fi, err := stat(path)
	if err != nil {
		return fi, err
	}
	return winmode.Apply(path, fi), nil
}

// Lstat is [os.Lstat] reporting the recorded mode. A symbolic link has no
// mode of its own to record, so this differs from [os.Lstat] only for the
// file a link is not.
func Lstat(path string) (os.FileInfo, error) {
	fi, err := lstat(path)
	if err != nil {
		return fi, err
	}
	return winmode.Apply(path, fi), nil
}

// StatInfo reports the recorded mode on a FileInfo obtained some other way
// — a directory entry, an [os.Root] stat — for the file at path, which must
// be the path that FileInfo describes.
func StatInfo(path string, fi os.FileInfo) os.FileInfo {
	return winmode.Apply(path, fi)
}

// SameFile is [os.SameFile] seen through the wrapper [Stat] and friends
// may have put around either FileInfo. os.SameFile recognizes only the
// concrete type os.Stat returns, so a FileInfo carrying a recorded mode
// would otherwise compare unequal to everything — including to itself,
// which is what `test f -ef f` asks. Every -ef, every hard-link identity
// check and every traversal cycle check that can see a [Stat] result goes
// through here.
func SameFile(a, b os.FileInfo) bool {
	return winmode.SameFile(a, b)
}

// ModeRecorded reports whether fi carries a mode somebody set, as opposed
// to one the platform derived from the file's attributes. The two are
// spelled identically in a FileMode and only the first is worth believing:
// a tool that wants to fall back to a platform rule — Windows deciding
// executability by extension, say — asks this first.
func ModeRecorded(fi os.FileInfo) bool {
	return winmode.Recorded(fi)
}

// RecordMode stores mode for path where [Stat] will find it, and is a no-op
// on a platform whose filesystem holds the mode itself — there, chmod(2)
// has already done the whole job and there is nothing to record.
func RecordMode(path string, mode fs.FileMode) error {
	if !winmode.Supported {
		return nil
	}
	return winmode.Set(path, mode)
}
