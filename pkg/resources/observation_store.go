package resources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/qiangli/coreutils/pkg/lockfile"
	"io"
	"os"
	"path/filepath"
	"time"
)

func observationDir(dir string) (string, error) {
	if dir == "" {
		dir = ResourcesStateDir()
	}
	if dir == "" {
		return "", errors.New("resources: no state directory")
	}
	return dir, nil
}
func readObservationJSON(path string, limit int64, value any) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return err
	}
	if int64(len(b)) > limit {
		return fmt.Errorf("resources: state exceeds %d bytes", limit)
	}
	return json.Unmarshal(b, value)
}
func writeObservationJSON(path string, limit int, value any) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(b) > limit {
		return fmt.Errorf("resources: state exceeds %d bytes", limit)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".observation-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err := f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
func observationLock(ctx context.Context, path string) (*lockfile.Lock, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		lock, err := lockfile.TryAcquire(path, lockfile.Holder{Name: "resources", Intent: "derived state"})
		if err == nil {
			return lock, nil
		}
		if !errors.Is(err, lockfile.ErrHeld) {
			return nil, err
		}
		if err := sleepCtx(ctx, 10*time.Millisecond); err != nil {
			return nil, err
		}
	}
}

const maxAlertStateBytes = 2 << 20

func ReadAlertState(ctx context.Context, dir string) (*AlertLedger, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	dir, err := observationDir(dir)
	if err != nil {
		return nil, err
	}
	state := &AlertLedger{SchemaVersion: AlertStateSchema, Entries: map[string]json.RawMessage{}}
	err = readObservationJSON(filepath.Join(dir, "alerts.json"), maxAlertStateBytes, state)
	if os.IsNotExist(err) {
		return state, nil
	}
	if err != nil {
		return nil, err
	}
	if err := validateAlertState(state); err != nil {
		return nil, err
	}
	if state.Entries == nil {
		state.Entries = map[string]json.RawMessage{}
	}
	return state, nil
}
func validateAlertState(state *AlertLedger) error {
	if state.SchemaVersion != AlertStateSchema {
		return errors.New("resources: unsupported alert state schema")
	}
	if len(state.Entries) > 256 {
		return errors.New("resources: alert state exceeds 256 entries")
	}
	for key, value := range state.Entries {
		if key == "" || len(key) > 256 || len(value) > 8192 || !json.Valid(value) {
			return errors.New("resources: invalid or oversized alert entry")
		}
	}
	return nil
}

// UpdateAlertState serializes one short, in-memory derived-state transaction.
// The callback must not perform IO or delivery. Errors leave disk unchanged.
func UpdateAlertState(ctx context.Context, dir string, update func(*AlertLedger) error) (*AlertLedger, error) {
	dir, err := observationDir(dir)
	if err != nil {
		return nil, err
	}
	if update == nil {
		return nil, errors.New("resources: missing alert transaction")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	lock, err := observationLock(ctx, filepath.Join(dir, ".alerts.lock"))
	if err != nil {
		return nil, err
	}
	defer lock.Release()
	state, err := ReadAlertState(ctx, dir)
	if err != nil {
		return nil, err
	}
	revision := state.Revision
	if err := update(state); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	state.Revision = revision + 1
	state.UpdatedAt = time.Now().UTC()
	if err := validateAlertState(state); err != nil {
		return nil, err
	}
	if err := writeObservationJSON(filepath.Join(dir, "alerts.json"), maxAlertStateBytes, state); err != nil {
		return nil, err
	}
	return state, nil
}
