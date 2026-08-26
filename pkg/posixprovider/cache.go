// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package posixprovider

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
)

var accountHomeFn = accountHome

// CacheRoot resolves the provider cache root. override is the explicit
// BASHY_BIN_CACHE value supplied by the invocation; an empty value selects the
// authenticated OS account's cache directory.
func CacheRoot(override string) (string, error) {
	if strings.TrimSpace(override) != "" {
		return validateCachePath(override, CacheOverrideEnv)
	}
	home, err := accountHomeFn()
	if err != nil {
		return "", err
	}
	return cacheRootForHome(runtime.GOOS, home)
}

func cacheRootForHome(goos, home string) (string, error) {
	home, err := validateAbsolutePath(home, "authenticated account home", true)
	if err != nil {
		return "", err
	}
	var base string
	switch goos {
	case "darwin":
		base = filepath.Join(home, "Library", "Caches")
	case "windows":
		base = filepath.Join(home, "AppData", "Local")
	default:
		base = filepath.Join(home, ".cache")
	}
	return filepath.Join(base, "bashy", "bin"), nil
}

func validateCachePath(path, source string) (string, error) {
	return validateAbsolutePath(path, source, false)
}

func validateAbsolutePath(path, source string, allowRoot bool) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("%s is empty", source)
	}
	if strings.IndexByte(path, 0) >= 0 {
		return "", fmt.Errorf("%s contains a NUL byte", source)
	}
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("%s must be an absolute path: %q", source, path)
	}
	clean := filepath.Clean(path)
	if !allowRoot && filepath.Dir(clean) == clean {
		return "", fmt.Errorf("%s must not name a filesystem root: %q", source, path)
	}
	return clean, nil
}
