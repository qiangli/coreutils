//go:build windows

package paxcmd

import "os"

// Windows has no uid/gid/device/inode/link-count identity on FileInfo. The
// writers fall back to the portable header defaults, and hardlink members are
// simply never emitted, rather than inventing values an archive would carry.
func identityOf(os.FileInfo) fileIdentity { return fileIdentity{} }
