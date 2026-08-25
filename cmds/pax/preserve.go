package paxcmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/qiangli/coreutils/tool"
)

// preservation is the effective POSIX -p policy. Times are preserved by
// default; ownership and mode are normal creation attributes unless requested.
type preservation struct {
	owner bool
	mode  bool
	atime bool
	mtime bool
}

func defaultPreservation() preservation {
	return preservation{atime: true, mtime: true}
}

// parsePreservation processes characters in command-line order across both
// concatenated strings and repeated -p occurrences. Only e conflicts with a/m:
// a later e re-enables both times, while a later a or m disables its one time.
func parsePreservation(values []string) (preservation, error) {
	p := defaultPreservation()
	for _, value := range values {
		if value == "" {
			return preservation{}, errors.New("-p requires one or more of a, e, m, o, or p")
		}
		for _, ch := range value {
			switch ch {
			case 'a':
				p.atime = false
			case 'e':
				p.owner, p.mode, p.atime, p.mtime = true, true, true, true
			case 'm':
				p.mtime = false
			case 'o':
				p.owner = true
			case 'p':
				p.mode = true
			default:
				return preservation{}, fmt.Errorf("invalid -p characteristic %q: supported characteristics are a, e, m, o, and p", string(ch))
			}
		}
	}
	return p, nil
}

type preservedAttributes struct {
	uid, gid     int
	mode         os.FileMode
	atime, mtime time.Time
}

var (
	chownExtractedFn = defaultChownExtracted
	chmodExtractedFn = os.Chmod
	timesExtractedFn = defaultSetExtractedTimes
	lstatExtractedFn = os.Lstat
)

// applyPreservedAttributes applies independent attributes in the POSIX order
// needed for set-ID safety: ownership first (chown can clear set-ID), mode
// second, times last. Every requested operation is attempted even if an earlier
// one failed, and errors are joined so the caller diagnoses once while retaining
// the extracted file.
func applyPreservedAttributes(path string, attrs preservedAttributes, policy preservation, symlink bool) error {
	var errs []error
	ownerPreserved := false
	if policy.owner {
		if err := chownExtractedFn(path, attrs.uid, attrs.gid, symlink); err != nil {
			errs = append(errs, fmt.Errorf("preserve owner: %w", err))
		} else {
			ownerPreserved = true
		}
	}

	// POSIX forbids setting S_ISUID/S_ISGID when ownership was not requested or
	// could not be preserved. Symlinks have no portable chmod semantics.
	if !symlink {
		mode := attrs.mode
		needMode := policy.mode
		if !policy.mode {
			if fi, err := lstatExtractedFn(path); err != nil {
				errs = append(errs, fmt.Errorf("inspect mode: %w", err))
			} else {
				mode = fi.Mode()
			}
		}
		if !ownerPreserved {
			cleared := mode &^ (os.ModeSetuid | os.ModeSetgid)
			needMode = needMode || cleared != mode
			mode = cleared
		}
		if needMode {
			if err := chmodExtractedFn(path, mode&(os.ModePerm|os.ModeSetuid|os.ModeSetgid|os.ModeSticky)); err != nil {
				errs = append(errs, fmt.Errorf("preserve mode: %w", err))
			}
		}
	}

	setA := policy.atime && !attrs.atime.IsZero()
	setM := policy.mtime && !attrs.mtime.IsZero()
	if setA || setM {
		if err := timesExtractedFn(path, attrs.atime, setA, attrs.mtime, setM, symlink); err != nil {
			errs = append(errs, fmt.Errorf("preserve times: %w", err))
		}
	}
	return errors.Join(errs...)
}

func normalCreationMode(rc *tool.RunContext, archived os.FileMode) os.FileMode {
	return archived.Perm() &^ invocationUmask(rc)
}

func intermediateDirMode(rc *tool.RunContext) os.FileMode {
	return 0o777 &^ invocationUmask(rc)
}

// mkdirAllNormal creates each missing component at the invocation's effective
// normal directory mode without mutating the process umask. Creating at 0700
// first avoids a permissive host umask exposing broader access before chmod.
func mkdirAllNormal(path string, mode os.FileMode) error {
	var missing []string
	for current := filepath.Clean(path); ; current = filepath.Dir(current) {
		fi, err := os.Stat(current)
		if err == nil {
			if !fi.IsDir() {
				return fmt.Errorf("%s: not a directory", current)
			}
			break
		}
		if !os.IsNotExist(err) {
			return err
		}
		missing = append(missing, current)
		parent := filepath.Dir(current)
		if parent == current {
			return err
		}
	}
	for i := len(missing) - 1; i >= 0; i-- {
		if err := os.Mkdir(missing[i], 0o700); err != nil && !os.IsExist(err) {
			return err
		}
		if err := os.Chmod(missing[i], mode.Perm()); err != nil {
			return err
		}
	}
	return nil
}

func prepareOutputDirectory(path string, finalMode os.FileMode) error {
	if err := mkdirAllNormal(path, finalMode|0o700); err != nil {
		return err
	}
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}
	workingMode := fi.Mode().Perm() | 0o700
	if fi.Mode().Perm() != workingMode {
		return os.Chmod(path, workingMode)
	}
	return nil
}
