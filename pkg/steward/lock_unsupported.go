// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build !linux && !darwin && !freebsd && !netbsd && !openbsd && !dragonfly && !solaris && !windows

package steward

import "os"

// lockFile fails closed: this platform cannot serialize, so it cannot host a steward.
// See ErrLockUnsupported (lock.go) for why there is no no-op fallback here.
func lockFile(*os.File) (func(), error) { return nil, ErrLockUnsupported }

// LockSupported reports whether this platform can host a steward seat.
func LockSupported() bool { return false }
