// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build windows

package posixproviderscmd

import "github.com/qiangli/coreutils/tool"

// Windows has no execve-style process replacement.  Provider execution keeps
// using the ordinary child-process path there.
func execProviderDedicated(_ *tool.RunContext, _, _ string, _ []string) (bool, int) {
	return false, 0
}
