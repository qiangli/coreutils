// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package webconsole

import (
	"runtime/debug"
	"strings"
	"sync"
)

// Build identifies the binary serving this page.
type Build struct {
	Version string `json:"version"`          // module version, or "devel"
	Commit  string `json:"commit,omitempty"` // short vcs revision
	Time    string `json:"time,omitempty"`   // vcs commit time
	Dirty   bool   `json:"dirty,omitempty"`  // built from a modified tree
	Go      string `json:"go,omitempty"`     // toolchain
	Assets  string `json:"assets,omitempty"` // content hash of the launcher script
}

var (
	buildOnce sync.Once
	buildInfo Build
)

// BuildOf reads the build stamp Go embeds at compile time.
//
// It is surfaced on the page for a reason that is not vanity: "am I looking at
// the build I just made?" was unanswerable from the UI, and a browser serving a
// cached script made a fixed bug keep reproducing. A visible commit — plus the
// asset hash, which changes whenever the launcher's own script does — turns that
// into a glance instead of an investigation.
func BuildOf() Build {
	buildOnce.Do(func() {
		b := Build{Version: "devel"}
		if bi, ok := debug.ReadBuildInfo(); ok {
			b.Go = bi.GoVersion
			if v := bi.Main.Version; v != "" && v != "(devel)" {
				b.Version = v
			}
			for _, s := range bi.Settings {
				switch s.Key {
				case "vcs.revision":
					b.Commit = s.Value
					if len(b.Commit) > 12 {
						b.Commit = b.Commit[:12]
					}
				case "vcs.time":
					b.Time = s.Value
				case "vcs.modified":
					b.Dirty = s.Value == "true"
				}
			}
		}
		// The asset hash answers the question the commit cannot: whether the
		// PAGE you are looking at came from this binary or from a cache.
		b.Assets = strings.Trim(etagFor("app.js"), `"`)
		buildInfo = b
	})
	return buildInfo
}

// Short renders the glanceable form: what a user quotes in a bug report.
func (b Build) Short() string {
	s := b.Version
	if b.Commit != "" {
		s += " · " + b.Commit
	}
	if b.Dirty {
		s += "-dirty"
	}
	return s
}
