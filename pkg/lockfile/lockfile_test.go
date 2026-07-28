package lockfile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestThreeAcquisitionModesAndOwner(t *testing.T) {
	path := filepath.Join(t.TempDir(), "resource.lock")
	since := time.Now().Add(-time.Minute).Round(time.Second)
	holder := Holder{Name: "first", PID: os.Getpid(), Intent: "test", Since: since}

	first, err := TryAcquire(path, holder)
	if err != nil {
		t.Fatalf("TryAcquire first: %v", err)
	}
	defer first.Release()

	got, ok := Owner(path)
	if !ok || got != holder {
		t.Fatalf("Owner = (%+v, %v), want (%+v, true)", got, ok, holder)
	}

	_, err = TryAcquire(path, Holder{Name: "second"})
	if !errors.Is(err, ErrHeld) {
		t.Fatalf("TryAcquire contention = %v, want ErrHeld", err)
	}
	if got, ok := HeldBy(err); !ok || got != holder {
		t.Fatalf("HeldBy = (%+v, %v), want (%+v, true)", got, ok, holder)
	}
	if _, ok := HeldBy(fmt.Errorf("wrapped: %w", err)); !ok {
		t.Fatal("HeldBy did not traverse a wrapped ErrHeld")
	}

	start := time.Now()
	_, err = AcquireWithin(path, 75*time.Millisecond, Holder{Name: "bounded"})
	if !errors.Is(err, ErrHeld) {
		t.Fatalf("AcquireWithin contention = %v, want ErrHeld", err)
	}
	if elapsed := time.Since(start); elapsed < 50*time.Millisecond || elapsed > time.Second {
		t.Fatalf("AcquireWithin elapsed %s, want a bounded wait near 75ms", elapsed)
	}

	acquired := make(chan *Lock, 1)
	acquireErr := make(chan error, 1)
	go func() {
		l, err := Acquire(path, Holder{Name: "blocking"})
		if err != nil {
			acquireErr <- err
			return
		}
		acquired <- l
	}()
	select {
	case <-acquired:
		t.Fatal("Acquire returned while the first holder still owned the lock")
	case err := <-acquireErr:
		t.Fatalf("Acquire failed while waiting: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	if err := first.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}
	var second *Lock
	select {
	case second = <-acquired:
	case err := <-acquireErr:
		t.Fatalf("Acquire after release: %v", err)
	case <-time.After(time.Second):
		t.Fatal("blocking Acquire did not proceed after release")
	}
	if err := second.Release(); err != nil {
		t.Fatalf("second Release: %v", err)
	}
	if err := second.Release(); err != nil {
		t.Fatalf("idempotent Release: %v", err)
	}
	if _, ok := Owner(path); ok {
		t.Fatal("Owner trusted stale metadata after the kernel lock was released")
	}
}

func TestExactlyOneScopedPlatformPair(t *testing.T) {
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(here), "..", ".."))
	scoped := []string{
		"pkg/lockfile",
		"pkg/weave",
		"pkg/meet",
		"pkg/steward",
		"pkg/policy/coord",
		"pkg/policy/audit",
	}
	var got []string
	for _, dir := range scoped {
		entries, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(dir)))
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || (!strings.Contains(name, "lock") && !strings.Contains(name, "flock")) {
				continue
			}
			if strings.HasSuffix(name, "_unix.go") || strings.HasSuffix(name, "_windows.go") {
				got = append(got, filepath.ToSlash(filepath.Join(dir, name)))
			}
		}
	}
	sort.Strings(got)
	want := []string{"pkg/lockfile/lock_unix.go", "pkg/lockfile/lock_windows.go"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("file-lock platform implementations = %v, want the single shared pair %v", got, want)
	}
}
