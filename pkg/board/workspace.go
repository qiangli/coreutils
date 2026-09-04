// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package board

import (
	"context"
	"errors"
	"io/fs"
	"math"
	"path/filepath"
)

// workspaceDiskUsage measures the apparent footprint of one weave workspace.
// WalkDir does not follow symlinks, which keeps a link inside a clone from
// escaping into an unrelated tree. A per-workspace failure is data, not a
// failed weave source: the Workspaces panel must still list the path and say
// that its size is unavailable.
func workspaceDiskUsage(ctx context.Context, path string) (uint64, string) {
	var total uint64
	err := filepath.WalkDir(path, func(_ string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.IsDir() || info.Size() <= 0 {
			return nil
		}
		n := uint64(info.Size())
		if n > math.MaxUint64-total {
			return errors.New("workspace size overflow")
		}
		total += n
		return nil
	})
	if err != nil {
		return 0, err.Error()
	}
	return total, ""
}
