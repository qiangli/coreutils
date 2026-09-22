//go:build windows

package localecmd

import (
	"path/filepath"

	"github.com/qiangli/coreutils/tool"
)

// defaultHostLocaleProvider is a real Windows provider when Bashy supplies a
// native host executable. No path means no provider: guessing through PATH is
// unsafe because coreutils itself may be the first `locale` found there.
func defaultHostLocaleProvider(rc *tool.RunContext) *hostLocaleProvider {
	path := rc.Getenv(hostLocalePathEnv)
	if path == "" || !filepath.IsAbs(path) {
		return nil
	}
	return &hostLocaleProvider{
		run:         commandHostLocaleRunner(rc, path),
		candidates:  hostLocaleNames(rc.Getenv(hostLocaleNamesEnv)),
		parallelism: 8,
	}
}
