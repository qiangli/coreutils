//go:build !unix && !windows

package testcmd

import (
	"errors"
	"os"
	"time"

	"github.com/qiangli/coreutils/tool"
)

// errUnsupportedPlatform is the loud answer for the primaries that need
// a permission model, an owner or an access time this platform does not
// expose. Guessing from mode bits would be a silent wrong answer.
var errUnsupportedPlatform = errors.New("not supported on this platform")

func accessOK(_ *tool.RunContext, _ string, _ byte) (bool, error) {
	return false, errUnsupportedPlatform
}

func ownedByEffective(string, bool) (bool, error) { return false, errUnsupportedPlatform }

func fileTimes(string) (atime, mtime time.Time, err error) {
	return time.Time{}, time.Time{}, errUnsupportedPlatform
}

// isTerminal: never a terminal here, rather than guessing.
func isTerminal(*os.File) bool { return false }
