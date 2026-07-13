// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package steward

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// DefaultDir is the host/user-scoped seat: ~/.bashy/steward (or $BASHY_STEWARD_DIR).
//
// HOST-WIDE and cwd-INDEPENDENT, deliberately. A steward is not a property of a
// checkout — it is the human's continuous point of contact across every project
// on the machine. Keying it to a repository would produce one steward per clone,
// which is precisely the thing the singleton is meant to prevent.
func DefaultDir() string {
	if v := os.Getenv("BASHY_STEWARD_DIR"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "bashy-steward")
	}
	return filepath.Join(home, ".bashy", "steward")
}

// Store is the on-disk steward state: one journal (the authority), one seat file
// (liveness only), and a checkpoints directory (projections).
type Store struct{ dir string }

// Open prepares the store directory. The journal records what an agent did across
// every project on the host, so it is owner-only (0700/0600) like the audit log.
func Open(dir string) (*Store, error) {
	if dir == "" {
		dir = DefaultDir()
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("steward: store dir: %w", err)
	}
	return &Store{dir: dir}, nil
}

func (s *Store) Dir() string           { return s.dir }
func (s *Store) journalPath() string   { return filepath.Join(s.dir, "journal.jsonl") }
func (s *Store) seatPath() string      { return filepath.Join(s.dir, "seat.json") }
func (s *Store) lockPath() string      { return filepath.Join(s.dir, "steward.lock") }
func (s *Store) checkpointDir() string { return filepath.Join(s.dir, "checkpoints") }
func (s *Store) transcriptDir() string { return filepath.Join(s.dir, "transcripts") }

// withLock serializes an entire read/decide/write cycle.
//
// The lock is essential exactly here: Claim must READ the journal, decide the
// seat is free, and WRITE its claim — and if two agents interleave those three
// steps, both conclude the seat is vacant and both take it. That is the race the
// singleton contract exists to forbid, reproduced inside the mechanism meant to
// enforce it. A real lock on every supported platform (see lock_*.go).
func (s *Store) withLock(fn func() error) error {
	f, err := os.OpenFile(s.lockPath(), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("steward: lock: %w", err)
	}
	defer f.Close()
	unlock, err := lockFile(f)
	if err != nil {
		return fmt.Errorf("steward: lock: %w", err)
	}
	defer unlock()
	return fn()
}

// Replay walks the journal and returns the valid prefix plus an honest account of
// any unreadable tail. This is the ONLY way state is ever derived: every view in
// this package is a pure function of what Replay returns.
func (s *Store) Replay() (*Replay, error) { return readJournal(s.journalPath()) }

// writeJSONAtomic writes v to path atomically and durably: a reader — possibly a
// successor mid-recovery — sees either the whole previous file or the whole new
// one, never a half-written blend, and a rename that the OS acknowledged survives
// a crash.
func writeJSONAtomic(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')

	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once the rename succeeds

	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	// fsync the DATA before the rename: a rename that lands while the contents are
	// still in the page cache can leave a correctly-named, empty file after a crash.
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	// fsync the DIRECTORY so the rename itself is durable, not just the bytes.
	// Best-effort: some filesystems refuse to open a directory for sync, and a
	// missing directory fsync is a weaker guarantee, not a wrong one.
	if d, err := os.Open(filepath.Dir(path)); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}

func readJSON(path string, v any) (bool, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if err := json.Unmarshal(b, v); err != nil {
		return false, fmt.Errorf("%s: %w", path, err)
	}
	return true, nil
}

// mustUTC normalizes a caller-supplied clock. Every timestamp this package writes
// is UTC: a journal that outlives the session that wrote it must not be readable
// only in the timezone it happened to be born in.
func mustUTC(t time.Time) time.Time {
	if t.IsZero() {
		return time.Now().UTC()
	}
	return t.UTC()
}
