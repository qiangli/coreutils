//go:build windows

package ducmd

import "os"

type devIno struct{ dev, ino uint64 }

// usage on Windows: there is no block count, so disk usage falls back
// to the apparent size (documented platform note).
func (d *duRun) usage(fi os.FileInfo) int64 { return fi.Size() }

// skipHardlink: link counts are not surfaced by os.FileInfo on
// Windows, so hard links are counted per path.
func (d *duRun) skipHardlink(_ os.FileInfo) bool { return false }

func fileDev(_ os.FileInfo) (uint64, bool) { return 0, false }
