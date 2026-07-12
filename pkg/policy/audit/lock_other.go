// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build !linux && !darwin

package audit

import "os"

// lockFile is a no-op on platforms without flock. Appends are still atomic per
// write (O_APPEND), so a single record is never torn; what is lost is the
// cross-process guarantee that two writers cannot read the same head and fork
// the chain. On such a host, prefer a per-process log path. Documented rather
// than silently pretending to lock.
func lockFile(*os.File) (func(), error) {
	return func() {}, nil
}
