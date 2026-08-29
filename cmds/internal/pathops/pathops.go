// Package pathops provides pathname operations that retain ordinary os
// semantics while recovering from a materialized pathname exceeding PATH_MAX.
package pathops

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// Lstat is os.Lstat with a component-relative fallback for paths whose full
// spelling cannot be passed through one system call.
func Lstat(path string) (os.FileInfo, error) {
	fi, err := os.Lstat(path)
	if !errors.Is(err, syscall.ENAMETOOLONG) {
		return fi, err
	}
	root, name, rootErr := splitRoot(path)
	if rootErr != nil {
		return nil, err
	}
	defer root.Close()
	fi, rootErr = root.Lstat(name)
	if rootErr != nil {
		return nil, rootErr
	}
	return fi, nil
}

// Remove is os.Remove with a component-relative fallback for paths whose full
// spelling cannot be passed through one system call.
func Remove(path string) error {
	err := os.Remove(path)
	if !errors.Is(err, syscall.ENAMETOOLONG) {
		return err
	}
	root, name, rootErr := splitRoot(path)
	if rootErr != nil {
		return err
	}
	defer root.Close()
	return root.Remove(name)
}

// RemoveAll is os.RemoveAll with a component-relative fallback when the
// operand itself exceeds PATH_MAX. On supported Unix platforms os.RemoveAll's
// implementation already uses openat-style recursion below its starting
// directory, so descendants never need to be materialized as full paths.
func RemoveAll(path string) error {
	err := os.RemoveAll(path)
	if !errors.Is(err, syscall.ENAMETOOLONG) {
		return err
	}
	root, name, rootErr := splitRoot(path)
	if rootErr != nil {
		return err
	}
	defer root.Close()
	return root.RemoveAll(name)
}

// OpenRoot is os.OpenRoot with the same over-PATH_MAX fallback. The returned
// Root remains anchored to the named directory and must be closed by callers.
func OpenRoot(path string) (*os.Root, error) {
	root, err := os.OpenRoot(path)
	if !errors.Is(err, syscall.ENAMETOOLONG) {
		return root, err
	}
	parent, name, rootErr := splitRoot(path)
	if rootErr != nil {
		return nil, err
	}
	defer parent.Close()
	return parent.OpenRoot(name)
}

// splitRoot anchors an operand at its filesystem volume root and leaves its
// components uncleaned. Not cleaning is load-bearing: a missing/.. prefix must
// still fail at the missing component rather than being rewritten to a
// different object before the filesystem sees it.
func splitRoot(path string) (*os.Root, string, error) {
	if !filepath.IsAbs(path) {
		return openNamedRoot(".", path)
	}
	volume := filepath.VolumeName(path)
	rootPath := volume + string(filepath.Separator)
	rest := path[len(volume):]
	rest = strings.TrimLeftFunc(rest, func(r rune) bool {
		return os.IsPathSeparator(uint8(r))
	})
	if rest == "" {
		rest = "."
	}
	return openNamedRoot(rootPath, rest)
}

func openNamedRoot(path, name string) (*os.Root, string, error) {
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, "", err
	}
	return root, name, nil
}
