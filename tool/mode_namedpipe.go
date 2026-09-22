package tool

import (
	"io/fs"
	"strings"
	"time"
)

// namedPipeName recognizes the two Win32 spellings of a named-pipe object.
// Named pipes are objects in a namespace, rather than filesystem entries, so
// their only portable stat facts are their name and type.
func namedPipeName(path string) (string, bool) {
	p := strings.ReplaceAll(path, "/", `\`)
	const dir = `\\.\pipe\`
	if !strings.HasPrefix(p, dir) {
		return "", false
	}
	name := strings.TrimPrefix(p, dir)
	if name == "" || strings.ContainsRune(name, '\\') {
		return "", false
	}
	return name, true
}

// namedPipeInfo is the stat-shaped view Windows can provide for a live named
// pipe without opening it. In particular, its zero size and timestamp do not
// describe a backing file: there is none. The 0600 mode matches the shell's
// process-substitution pipe contract; Windows has no pipe mode bits to query.
type namedPipeInfo struct{ name string }

func (i namedPipeInfo) Name() string     { return i.name }
func (namedPipeInfo) Size() int64        { return 0 }
func (namedPipeInfo) Mode() fs.FileMode  { return fs.ModeNamedPipe | 0o600 }
func (namedPipeInfo) ModTime() time.Time { return time.Time{} }
func (namedPipeInfo) IsDir() bool        { return false }
func (namedPipeInfo) Sys() any           { return nil }
