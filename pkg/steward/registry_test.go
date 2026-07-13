// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package steward

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// ─── ONE STORE PER SEAT, AND THE DIRECTORY DOES NOT GET A VOTE ────────────────
//
// The seat is one per machine-and-account. scope.json enforced HALF of that — this
// directory belongs to that seat — and the previous revision stopped there, which left the
// other half open: nothing said that seat lives in exactly ONE directory.
//
// So an agent that did not care for the steward it found simply asked for another one, and
// the store it got cheerfully bound ITSELF to the same scope, minted its own epoch ladder
// from an empty journal, and handed over the seat. Two stewards on one host, each holding
// epoch 1, neither able to see the other, neither fenced — because fencing compares epochs
// WITHIN one journal and there were now two. The singleton the whole package exists to
// guarantee was defeated by a flag.
//
// These tests come at that from every direction an agent actually has: the API, the flag,
// the environment variable, a race, and a store handle that was legitimate a moment ago.

// The API route: a second Open, for the same seat, somewhere else.
func TestASecondStoreForTheSameSeatIsRefused(t *testing.T) {
	reg := t.TempDir()
	canonical, alternate := t.TempDir(), t.TempDir()

	if _, err := Open(canonical, WithScopeProvider(testScope("seat")), WithRegistryRoot(reg)); err != nil {
		t.Fatalf("the first store for a seat becomes its store: %v", err)
	}

	_, err := Open(alternate, WithScopeProvider(testScope("seat")), WithRegistryRoot(reg))
	var conflict *ErrScopeDirConflict
	if !errors.As(err, &conflict) {
		t.Fatalf("a SECOND store for a seat that already has one must be refused — it would not be a second view "+
			"of the same seat, it would be a second SEAT, with its own empty journal and its own epoch 1. Got %v", err)
	}
	// Compare CANONICAL forms: macOS hands out temp dirs under /var, which is a symlink to
	// /private/var, and the store resolves that (see canonicalDir) precisely so two spellings
	// of one directory cannot become two seats.
	if want := mustCanonical(t, canonical); conflict.Canonical != want {
		t.Fatalf("the refusal must name where the seat actually lives, got %q want %q", conflict.Canonical, want)
	}
	if !strings.Contains(err.Error(), mustCanonical(t, alternate)) {
		t.Fatalf("…and what was asked for, so the operator can see the difference: %v", err)
	}

	// The seat's own store still opens, of course. The check is a singleton, not a lock-out.
	if _, err := Open(canonical, WithScopeProvider(testScope("seat")), WithRegistryRoot(reg)); err != nil {
		t.Fatalf("the seat's own store must keep opening: %v", err)
	}
}

// The environment route. $BASHY_STEWARD_DIR still SELECTS a directory — a host with a
// mounted volume needs that — but it does not get to mint a second seat with it.
func TestStewardDirEnvCannotMintASecondSeat(t *testing.T) {
	reg := t.TempDir()
	canonical, alternate := t.TempDir(), t.TempDir()

	if _, err := Open(canonical, WithScopeProvider(testScope("seat")), WithRegistryRoot(reg)); err != nil {
		t.Fatalf("Open: %v", err)
	}

	t.Setenv("BASHY_STEWARD_DIR", alternate)
	_, err := Open("", WithScopeProvider(testScope("seat")), WithRegistryRoot(reg))

	var conflict *ErrScopeDirConflict
	if !errors.As(err, &conflict) {
		t.Fatalf("$BASHY_STEWARD_DIR is a way to SAY WHERE the seat lives, not a way to have a second one. Got %v", err)
	}

	// And pointed at the seat's real store, it is honored exactly as before.
	t.Setenv("BASHY_STEWARD_DIR", canonical)
	if _, err := Open("", WithScopeProvider(testScope("seat")), WithRegistryRoot(reg)); err != nil {
		t.Fatalf("pointing the variable AT the seat's store must still work: %v", err)
	}
}

// The flag route, through the CLI the agent actually types at.
func TestCLIDirFlagCannotMintASecondSeat(t *testing.T) {
	reg := t.TempDir()
	canonical, alternate := t.TempDir(), t.TempDir()

	run := func(dir string, args ...string) error {
		t.Setenv("BASHY_PRINCIPAL", "dhnt:agent/tester")
		t.Setenv("BASHY_HOST_ID", "registry-test-machine")
		cmd := NewStewardCmd(WithRegistryRoot(reg))
		cmd.SetOut(new(strings.Builder))
		cmd.SetErr(new(strings.Builder))
		cmd.SetIn(strings.NewReader(""))
		cmd.SetArgs(append([]string{"--dir", dir}, args...))
		return cmd.Execute()
	}

	if err := run(canonical, "status"); err != nil {
		t.Fatalf("the first --dir binds the seat: %v", err)
	}
	err := run(alternate, "status")
	var conflict *ErrScopeDirConflict
	if !errors.As(err, &conflict) {
		t.Fatalf("`steward --dir /tmp/mine` was the whole escape: a fresh store, bound to the same seat, with its "+
			"own epoch ladder. It must be refused. Got %v", err)
	}
}

// THE FIRST BIND IS A RACE, AND IT IS SERIALIZED.
//
// Without a lock, two processes starting together both find no binding, both write one, and
// the last writer silently decides which of two live stewards was real. The canonical lock
// is per-SCOPE and lives outside every store, so the loser reads the winner's binding rather
// than overwriting it.
func TestFirstBindIsSerializedUnderTheCanonicalLock(t *testing.T) {
	const racers = 8
	reg := t.TempDir()

	dirs := make([]string, racers)
	for i := range dirs {
		dirs[i] = t.TempDir()
	}

	var (
		wg     sync.WaitGroup
		mu     sync.Mutex
		won    []string
		failed int
	)
	start := make(chan struct{})
	for i := range racers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start // pile up on the lock together
			s, err := Open(dirs[i], WithScopeProvider(testScope("contested")), WithRegistryRoot(reg))
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				failed++
				return
			}
			won = append(won, s.Dir())
		}()
	}
	close(start)
	wg.Wait()

	if len(won) != 1 {
		t.Fatalf("exactly ONE directory may become the seat's store — %d did (%v). Two winners is two journals, "+
			"two epoch ladders, and two stewards that cannot fence each other.", len(won), won)
	}
	if failed != racers-1 {
		t.Fatalf("every other racer must be refused, got %d refusals of %d", failed, racers-1)
	}
}

// A HANDLE IS NOT A PERMISSION. Open() checks the bindings once, and a Store outlives that
// check — so an agent that opened legitimately and then rebound the scope to a directory of
// its own would keep writing through a handle whose authority was established against a
// world that no longer exists.
//
// Every mutation re-reads both bindings under the canonical lock. A check that is not
// re-taken at the moment of the write is a check that races.
func TestMutationsRevalidateTheBindingUnderTheCanonicalLock(t *testing.T) {
	reg := t.TempDir()
	dir := t.TempDir()

	s, err := Open(dir, WithScopeProvider(testScope("seat")), WithVerifier(verified()), WithRegistryRoot(reg))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	a := agent("a")
	ep := mustClaim(t, s, a, at(0))

	// A write through this handle works — the handle is legitimate.
	if _, err := s.Record(evidenced(a, "api", "before"), ep, at(time.Minute)); err != nil {
		t.Fatalf("Record: %v", err)
	}

	// Now the seat is rebound elsewhere, behind this handle's back — an agent rewriting the
	// registry, or a racing process that won a rebind after the canonical dir went away.
	rebindRegistryTo(t, s, filepath.Join(t.TempDir(), "elsewhere"))

	_, err = s.Record(evidenced(a, "api", "after"), ep, at(2*time.Minute))
	var conflict *ErrScopeDirConflict
	if !errors.As(err, &conflict) {
		t.Fatalf("a steward whose scope has been rebound elsewhere must be refused at its NEXT WRITE, rather than "+
			"journaling into an orphan nobody will ever read. Got %v", err)
	}

	// Every mutation, not just the convenient one. A revalidation that only guarded Record
	// would leave the seat itself — the thing actually worth stealing — unguarded.
	if err := s.Heartbeat(a, ep, at(3*time.Minute)); !errors.As(err, &conflict) {
		t.Fatalf("Heartbeat must revalidate too, got %v", err)
	}
	if _, err := s.Checkpoint(a, ep, "", at(3*time.Minute)); !errors.As(err, &conflict) {
		t.Fatalf("Checkpoint must revalidate too, got %v", err)
	}
	if err := s.Release(a, ep, "", at(3*time.Minute)); !errors.As(err, &conflict) {
		t.Fatalf("Release must revalidate too, got %v", err)
	}
	if _, err := s.Claim(context.Background(), a, SeatRequest{GrantID: "g", Attended: true}, at(3*time.Minute)); !errors.As(err, &conflict) {
		t.Fatalf("Claim must revalidate too, got %v", err)
	}
}

// The store's OWN binding is revalidated as well: a scope.json rewritten under a live
// handle is the same attack from the other side.
func TestMutationsRevalidateTheStoresScopeBinding(t *testing.T) {
	reg := t.TempDir()
	dir := t.TempDir()

	s, err := Open(dir, WithScopeProvider(testScope("seat")), WithVerifier(verified()), WithRegistryRoot(reg))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	a := agent("a")
	ep := mustClaim(t, s, a, at(0))

	// Rewrite the store's scope binding to somebody else's seat.
	b := scopeBinding{
		SchemaVersion: SchemaVersion,
		Scope:         "somebody-else",
		Digest:        "sha256:" + strings.Repeat("e", 64),
		Host:          "other-host",
		BoundAt:       time.Now().UTC(),
	}
	if err := writeJSONAtomic(filepath.Join(dir, "scope.json"), b); err != nil {
		t.Fatal(err)
	}

	_, err = s.Record(evidenced(a, "api", "after"), ep, at(time.Minute))
	var mismatch *ErrScopeMismatch
	if !errors.As(err, &mismatch) {
		t.Fatalf("a store whose binding now names another seat must refuse the write, got %v", err)
	}
}

// SHARED-HOME MULTI-HOST ISOLATION SURVIVES ALL OF THIS.
//
// The registry lives under $HOME, which two machines may well share — and that is fine,
// because the entries are keyed by the SCOPE DIGEST, which includes the machine identity.
// Two machines mounting one home get two keys, two bindings, and two canonical stores. The
// isolation scope.go established is preserved by the registry rather than undone by it,
// and this is the test that says so.
func TestSharedHomeKeepsTwoMachinesIsolated(t *testing.T) {
	reg := t.TempDir() // ONE registry root — a shared home, exactly
	dirA, dirB := t.TempDir(), t.TempDir()

	sa, err := Open(dirA, WithScopeProvider(testScope("machine-a")), WithVerifier(verified()), WithRegistryRoot(reg))
	if err != nil {
		t.Fatalf("machine A: %v", err)
	}
	sb, err := Open(dirB, WithScopeProvider(testScope("machine-b")), WithVerifier(verified()), WithRegistryRoot(reg))
	if err != nil {
		t.Fatalf("machine B must get its OWN seat in a shared home — the registry keys on the machine, not the "+
			"home directory: %v", err)
	}

	if sa.RegistryPath() == sb.RegistryPath() {
		t.Fatal("two machines must not share one binding — that is the merged-journal failure this all exists to stop")
	}

	// And both hold their own seat, at their own epoch 1, without fencing each other.
	epA := mustClaim(t, sa, agent("a"), at(0))
	epB := mustClaim(t, sb, agent("b"), at(0))
	if epA != 1 || epB != 1 {
		t.Fatalf("each machine's seat has its own epoch ladder, got %d and %d", epA, epB)
	}
	if _, err := sa.Record(evidenced(agent("a"), "api", "A's work"), epA, at(time.Minute)); err != nil {
		t.Fatalf("machine A must keep working: %v", err)
	}
	if _, err := sb.Record(evidenced(agent("b"), "api", "B's work"), epB, at(time.Minute)); err != nil {
		t.Fatalf("machine B must keep working: %v", err)
	}
}

// The same directory, spelled differently, is the same directory. A registry that compared
// paths as strings would hand a second seat to whoever typed a trailing slash.
func TestTheSameDirSpelledDifferentlyIsTheSameStore(t *testing.T) {
	reg := t.TempDir()
	dir := t.TempDir()

	if _, err := Open(dir, WithScopeProvider(testScope("seat")), WithRegistryRoot(reg)); err != nil {
		t.Fatalf("Open: %v", err)
	}
	for _, spelling := range []string{
		dir + string(filepath.Separator),
		filepath.Join(dir, "."),
		filepath.Join(dir, "sub", ".."),
	} {
		if _, err := Open(spelling, WithScopeProvider(testScope("seat")), WithRegistryRoot(reg)); err != nil {
			t.Fatalf("%q is the same directory as %q — a path comparison that says otherwise is a second seat "+
				"waiting to happen: %v", spelling, dir, err)
		}
	}
}

// An unreadable binding FAILS CLOSED. A store that cannot find out whether its seat already
// lives somewhere else must not assume it does not: guessing wrong mints a second steward
// on a host that already has one.
func TestAnUnreadableRegistryFailsClosed(t *testing.T) {
	reg := t.TempDir()
	dir := t.TempDir()

	s, err := Open(dir, WithScopeProvider(testScope("seat")), WithRegistryRoot(reg))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := os.WriteFile(s.RegistryPath(), []byte("{ this is not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err = Open(dir, WithScopeProvider(testScope("seat")), WithRegistryRoot(reg))
	var unreadable *ErrRegistryUnreadable
	if !errors.As(err, &unreadable) {
		t.Fatalf("an unreadable binding must refuse the open rather than silently rebinding, got %v", err)
	}
}

// rebindRegistryTo rewrites the seat's canonical binding to point somewhere else — what an
// agent with file access would do, and what a legitimate rebind after a move looks like from
// the inside.
func rebindRegistryTo(t *testing.T, s *Store, dir string) {
	t.Helper()
	var e registryEntry
	found, err := readJSON(s.RegistryPath(), &e)
	if err != nil || !found {
		t.Fatalf("reading the binding: found=%v err=%v", found, err)
	}
	e.Dir = dir
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.RegistryPath(), b, 0o600); err != nil {
		t.Fatal(err)
	}
}

// mustCanonical is the path form the store actually compares. macOS temp dirs live under
// /var, which is a symlink to /private/var, so a test that asserted on the raw t.TempDir()
// string would be testing the symlink rather than the store.
func mustCanonical(t *testing.T, dir string) string {
	t.Helper()
	c, err := canonicalDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	return c
}
