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

// sameHolder compares a recorded holder to the one that was written.
//
// NOT `==`, and the reason is a real portability bug rather than a nicety.
// Holder goes through JSON, and time.Time carries a *Location that == compares
// by POINTER: a Since written in the local zone comes back attached to
// whichever location time.Parse picked for the offset it read. Where the local
// offset is non-zero that happens to be time.Local, so == passed on a developer
// machine; under TZ=UTC — every minimal CI container — the value marshals with a
// "Z" and parses back into time.UTC, a DIFFERENT pointer from the time.Local
// that time.Now() attached. Same instant, same rendering (the failure printed
// two identical structs), unequal by ==.
//
// So compare the fields, and compare the timestamp as an INSTANT. Which zone a
// diagnostic record round-trips through is not part of the contract; the moment
// it names is.
func sameHolder(a, b Holder) bool {
	return a.Name == b.Name && a.PID == b.PID && a.Intent == b.Intent && a.Since.Equal(b.Since)
}

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
	if !ok || !sameHolder(got, holder) {
		t.Fatalf("Owner = (%+v, %v), want (%+v, true)", got, ok, holder)
	}

	_, err = TryAcquire(path, Holder{Name: "second"})
	if !errors.Is(err, ErrHeld) {
		t.Fatalf("TryAcquire contention = %v, want ErrHeld", err)
	}
	if got, ok := HeldBy(err); !ok || !sameHolder(got, holder) {
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

// THE HOLDER RECORD IS AN INSTANT, NOT A ZONE. A holder recorded in one zone
// must read back as the same moment whatever zone the reader is running in —
// the case that matters is a container on UTC reading a record written on a
// host that was not, and the reverse. This pins the property the suite's own
// `==` comparison used to assume and, under TZ=UTC, got wrong.
func TestHolderTimestampSurvivesZoneRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "zoned.lock")
	// A fixed non-UTC zone that is nobody's Local here, so neither the writer's
	// nor the reader's ambient location can make this pass by accident.
	since := time.Date(2026, 8, 3, 9, 30, 0, 0, time.FixedZone("TEST", 5*3600+1800))
	holder := Holder{Name: "zoned", PID: os.Getpid(), Intent: "zone round trip", Since: since}

	l, err := TryAcquire(path, holder)
	if err != nil {
		t.Fatalf("TryAcquire: %v", err)
	}
	defer l.Release()

	got, ok := Owner(path)
	if !ok {
		t.Fatal("Owner did not report the live holder")
	}
	if !got.Since.Equal(since) {
		t.Errorf("Owner().Since = %s, want the same instant as %s", got.Since, since)
	}
	if !sameHolder(got, holder) {
		t.Errorf("Owner() = %+v, want the recorded holder %+v", got, holder)
	}
	if got.Since.Unix() != since.Unix() {
		t.Errorf("Since round-tripped to a different second: %d vs %d", got.Since.Unix(), since.Unix())
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
