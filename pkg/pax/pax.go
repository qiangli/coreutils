// Package pax provides a compatibility-first kernel for POSIX pax-style
// archive inspection and safe extraction planning.
package pax

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

// Kind classifies an archive member in a manifest or extraction plan.
type Kind string

const (
	KindFile     Kind = "file"
	KindDir      Kind = "dir"
	KindSymlink  Kind = "symlink"
	KindHardlink Kind = "hardlink"
	KindOther    Kind = "other"
)

// Member is one archive entry as exposed by the manifest reader.
type Member struct {
	Path       string
	Kind       Kind
	Linkname   string
	Size       int64
	Mode       fs.FileMode
	ModTime    time.Time
	Format     tar.Format
	PAXRecords map[string]string
}

// PlannedMember is one member whose destination passed the complete preflight.
// Extraction is deliberately outside this foundation.
type PlannedMember struct {
	Member
	Target string
}

// RejectedMember is one member the planner refused before mutation.
type RejectedMember struct {
	Member
	Reason string
}

// Plan is the result of scanning and validating an archive for extraction.
type Plan struct {
	Root      string
	Members   []PlannedMember
	Rejected  []RejectedMember
	Unsafe    bool
	Formats   []tar.Format
	FormatsBy map[tar.Format]int
}

// FS is the minimal filesystem boundary the planner needs to inspect existing
// path state without mutating it.
type FS interface {
	Lstat(path string) (fs.FileInfo, error)
	Readlink(path string) (string, error)
}

// OSFS is a passthrough implementation of FS for the local machine.
type OSFS struct{}

func (OSFS) Lstat(path string) (fs.FileInfo, error) { return os.Lstat(path) }
func (OSFS) Readlink(path string) (string, error)   { return os.Readlink(path) }

// ReadManifest reads a tar/ustar/pax stream and returns its member list.
// The Go tar reader already folds pax extended headers into the returned
// header, so this surface exposes the merged view.
func ReadManifest(r io.Reader) ([]Member, error) {
	tr := tar.NewReader(r)
	var out []Member
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		out = append(out, memberFromHeader(hdr))
	}
}

// PlanExtraction scans the entire archive before returning a plan. It never
// mutates the filesystem. Unsupported types, duplicate destinations, escaping
// paths, symlink parents, and invalid hardlink sources are rejected.
func PlanExtraction(r io.Reader, root string, filesystem FS) (*Plan, error) {
	if err := validateRoot(root, filesystem); err != nil {
		return nil, err
	}
	if filesystem == nil {
		filesystem = OSFS{}
	}
	root = filepath.Clean(root)
	plan := &Plan{Root: root, FormatsBy: map[tar.Format]int{}}
	entries := make([]candidate, 0)
	byTarget := make(map[string][]int)
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		member := memberFromHeader(hdr)
		plan.FormatsBy[member.Format]++
		entry := candidate{member: member}
		target, err := secureJoin(root, member.Path)
		if err != nil {
			entry.reject(err.Error())
		} else {
			entry.target = target
			byTarget[target] = append(byTarget[target], len(entries))
			switch member.Kind {
			case KindOther:
				entry.reject("unsupported special archive entry")
			case KindSymlink:
				if _, err := resolveLinkWithinRoot(root, target, member.Linkname); err != nil {
					entry.reject(err.Error())
				}
			case KindHardlink:
				linkTarget, err := secureJoin(root, member.Linkname)
				if err != nil {
					entry.reject(fmt.Sprintf("hardlink target %q: %v", member.Linkname, err))
				} else {
					entry.linkTarget = linkTarget
				}
			}
		}
		entries = append(entries, entry)
	}

	// Duplicates are rejected as a group: a later type cannot silently replace
	// an earlier member, and a repeated type cannot make archive order semantic.
	for target, indexes := range byTarget {
		if len(indexes) < 2 {
			continue
		}
		for _, i := range indexes {
			entries[i].reject(fmt.Sprintf("duplicate destination %q", target))
		}
	}

	valid := make(map[string]int)
	for i := range entries {
		if entries[i].reason != "" {
			continue
		}
		if err := validateExistingTarget(filesystem, entries[i].target, entries[i].member.Kind); err != nil {
			entries[i].reject(err.Error())
			continue
		}
		if err := validateExistingParents(filesystem, root, entries[i].target); err != nil {
			entries[i].reject(err.Error())
			continue
		}
		valid[entries[i].target] = i
	}
	// This second pass deliberately makes a symlink parent unsafe regardless of
	// archive ordering (child-before-parent is just as dangerous as the reverse).
	for changed := true; changed; {
		changed = false
		for i := range entries {
			if entries[i].reason != "" {
				continue
			}
			if err := validateArchiveParents(entries[i].target, root, valid, entries); err != nil {
				entries[i].reject(err.Error())
				delete(valid, entries[i].target)
				changed = true
			}
		}
	}
	// Parent rejection can invalidate a hardlink source, so resolve hardlink
	// chains only after all archive-parent checks have settled.
	for changed := true; changed; {
		changed = false
		for i := range entries {
			if entries[i].reason != "" || entries[i].member.Kind != KindHardlink {
				continue
			}
			if err := validateHardlink(i, root, entries, valid, filesystem); err != nil {
				entries[i].reject(err.Error())
				delete(valid, entries[i].target)
				changed = true
			}
		}
	}
	for _, entry := range entries {
		if entry.reason != "" {
			plan.Rejected = append(plan.Rejected, RejectedMember{Member: entry.member, Reason: entry.reason})
			continue
		}
		plan.Members = append(plan.Members, PlannedMember{Member: entry.member, Target: entry.target})
	}
	plan.Unsafe = len(plan.Rejected) != 0
	plan.Formats = sortedFormats(plan.FormatsBy)
	return plan, nil
}

type candidate struct {
	member     Member
	target     string
	linkTarget string
	reason     string
}

func (c *candidate) reject(reason string) {
	if c.reason == "" {
		c.reason = reason
	}
}

func validateRoot(root string, filesystem FS) error {
	if root == "" {
		return fmt.Errorf("pax: destination root is required")
	}
	if !filepath.IsAbs(root) {
		return fmt.Errorf("pax: destination root %q must be absolute", root)
	}
	if filesystem == nil {
		filesystem = OSFS{}
	}
	fi, err := filesystem.Lstat(filepath.Clean(root))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("pax: inspect destination root: %w", err)
	}
	if fi.Mode()&fs.ModeSymlink != 0 || !fi.IsDir() {
		return fmt.Errorf("pax: destination root %q must be a directory, not a symlink", root)
	}
	return nil
}

func validateExistingTarget(filesystem FS, target string, kind Kind) error {
	fi, err := filesystem.Lstat(target)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect existing target %q: %w", target, err)
	}
	if kind == KindDir && fi.IsDir() && fi.Mode()&fs.ModeSymlink == 0 {
		return nil
	}
	return fmt.Errorf("destination %q already exists", target)
}

func validateExistingParents(filesystem FS, root, target string) error {
	parts, err := parentParts(root, target)
	if err != nil {
		return err
	}
	current := root
	for _, part := range parts {
		current = filepath.Join(current, part)
		fi, err := filesystem.Lstat(current)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("inspect parent %q: %w", current, err)
		}
		if fi.Mode()&fs.ModeSymlink != 0 {
			return fmt.Errorf("member %q would traverse existing symlink parent %q", target, current)
		}
		if !fi.IsDir() {
			return fmt.Errorf("member %q has non-directory parent %q", target, current)
		}
	}
	return nil
}

func validateArchiveParents(target, root string, valid map[string]int, entries []candidate) error {
	parts, err := parentParts(root, target)
	if err != nil {
		return err
	}
	current := root
	for _, part := range parts {
		current = filepath.Join(current, part)
		if i, ok := valid[current]; ok {
			switch entries[i].member.Kind {
			case KindDir:
			case KindSymlink:
				return fmt.Errorf("member %q would traverse archive symlink parent %q", target, current)
			default:
				return fmt.Errorf("member %q has archive non-directory parent %q", target, current)
			}
		}
	}
	return nil
}

func validateHardlink(i int, root string, entries []candidate, valid map[string]int, filesystem FS) error {
	seen := map[int]bool{}
	for {
		if seen[i] {
			return fmt.Errorf("hardlink %q has a cyclic archive source", entries[i].member.Path)
		}
		seen[i] = true
		target := entries[i].linkTarget
		if source, ok := valid[target]; ok {
			switch entries[source].member.Kind {
			case KindFile:
				return nil
			case KindHardlink:
				i = source
				continue
			default:
				return fmt.Errorf("hardlink target %q is not a regular file", entries[i].member.Linkname)
			}
		}
		if err := validateExistingParents(filesystem, root, target); err != nil {
			return fmt.Errorf("hardlink target %q: %v", entries[i].member.Linkname, err)
		}
		fi, err := filesystem.Lstat(target)
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("hardlink target %q does not exist in the archive or destination", entries[i].member.Linkname)
		}
		if err != nil {
			return fmt.Errorf("inspect hardlink target %q: %w", entries[i].member.Linkname, err)
		}
		if fi.Mode()&fs.ModeSymlink != 0 || !fi.Mode().IsRegular() {
			return fmt.Errorf("hardlink target %q is not a regular file", entries[i].member.Linkname)
		}
		return nil
	}
}

func parentParts(root, target string) ([]string, error) {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return nil, err
	}
	parts := strings.Split(filepath.Dir(rel), string(filepath.Separator))
	if len(parts) == 1 && parts[0] == "." {
		return nil, nil
	}
	return parts, nil
}

func memberFromHeader(hdr *tar.Header) Member {
	return Member{Path: filepath.ToSlash(hdr.Name), Kind: kindFromHeader(hdr), Linkname: filepath.ToSlash(hdr.Linkname), Size: hdr.Size, Mode: hdr.FileInfo().Mode(), ModTime: hdr.ModTime, Format: hdr.Format, PAXRecords: cloneMap(hdr.PAXRecords)}
}

func kindFromHeader(hdr *tar.Header) Kind {
	switch hdr.Typeflag {
	case tar.TypeReg, tar.TypeRegA:
		return KindFile
	case tar.TypeDir:
		return KindDir
	case tar.TypeSymlink:
		return KindSymlink
	case tar.TypeLink:
		return KindHardlink
	default:
		return KindOther
	}
}

func secureJoin(root, member string) (string, error) {
	name := strings.TrimSuffix(filepath.ToSlash(member), "/")
	if name == "" || name == "." {
		return "", fmt.Errorf("empty member path")
	}
	if strings.HasPrefix(name, "/") {
		return "", fmt.Errorf("absolute member path %q", member)
	}
	native := filepath.FromSlash(name)
	if filepath.IsAbs(native) || filepath.VolumeName(native) != "" {
		return "", fmt.Errorf("absolute member path %q", member)
	}
	target := filepath.Join(root, native)
	if !withinRoot(root, target) || samePath(root, target) {
		return "", fmt.Errorf("member path %q escapes or names root", member)
	}
	return target, nil
}

func resolveLinkWithinRoot(root, linkPath, linkname string) (string, error) {
	if strings.TrimSpace(linkname) == "" {
		return "", fmt.Errorf("empty symlink target")
	}
	native := filepath.FromSlash(linkname)
	if filepath.IsAbs(native) || filepath.VolumeName(native) != "" {
		return "", fmt.Errorf("symlink target %q is absolute", linkname)
	}
	target := filepath.Clean(filepath.Join(filepath.Dir(linkPath), native))
	if !withinRoot(root, target) {
		return "", fmt.Errorf("symlink target %q escapes root", linkname)
	}
	return target, nil
}

func withinRoot(root, target string) bool {
	root, target = filepath.Clean(root), filepath.Clean(target)
	if samePath(root, target) {
		return true
	}
	if runtime.GOOS == "windows" {
		root, target = strings.ToLower(root), strings.ToLower(target)
	}
	return strings.HasPrefix(target, root+string(filepath.Separator))
}

func samePath(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

func cloneMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func sortedFormats(counts map[tar.Format]int) []tar.Format {
	if len(counts) == 0 {
		return nil
	}
	out := make([]tar.Format, 0, len(counts))
	for f := range counts {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
