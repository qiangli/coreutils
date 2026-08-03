//go:build !linux && !windows

package multicall

import "os"

func runInheritedSignalHelper(string) { os.Exit(2) }
