//go:build windows

package mesgcmd

import (
	"errors"

	"github.com/qiangli/coreutils/tool"
)

// Windows has no terminal permission bit to control, so mesg has nothing
// truthful to report. Refusing is better than inventing a state.
func defaultTTYName(*tool.RunContext) (string, error) {
	return "", errors.New("terminal message permission is not a Windows concept")
}
