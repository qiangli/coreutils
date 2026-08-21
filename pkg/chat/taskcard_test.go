package chat

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/pkg/agentlaunch"
	"github.com/qiangli/coreutils/pkg/room"
)

// isolatedRoom points the room at a temp dir and clears the uncontained-host
// launch guard, so a headless Invoke reaches the injected runner (the guard is
// about the real fleet, not a fake) without depending on ambient env.
func isolatedRoom(t *testing.T) {
	t.Helper()
	t.Setenv("BASHY_ROOM_DIR", t.TempDir())
	t.Setenv(agentlaunch.UnsafeLaunchEnv, "1")
}

// blockingRunner signals when the agent process would have started and then holds
// the turn open until released, so a test can observe the room while the "process"
// is live. It optionally fails, to prove the card is removed on error too.
type blockingRunner struct {
	started chan struct{}
	release chan struct{}
	err     error
}

func (b *blockingRunner) Run(ctx context.Context, agent string, args []string, cwd string) (string, int, error) {
	close(b.started)
	<-b.release
	if b.err != nil {
		return "", 1, b.err
	}
	return "done\n", 0, nil
}

func oneShotMembers(t *testing.T) []room.Card {
	t.Helper()
	members, err := room.Members()
	if err != nil {
		t.Fatalf("room.Members: %v", err)
	}
	var out []room.Card
	for _, c := range members {
		if c.Mode == "oneshot" {
			out = append(out, c)
		}
	}
	return out
}

func TestInvokePublishesTaskCardWhileRunningAndRemovesOnCompletion(t *testing.T) {
	isolatedRoom(t)

	r := &blockingRunner{started: make(chan struct{}), release: make(chan struct{})}
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = Invoke(context.Background(), Options{
			Agent: "codex", Role: "reviewer", Instruction: "fix the login\nbug in detail", Cwd: t.TempDir(),
		}, r)
	}()

	<-r.started // the process is now "running" — its card must be observable

	live := oneShotMembers(t)
	if len(live) != 1 {
		t.Fatalf("live one-shot cards = %d, want 1", len(live))
	}
	c := live[0]
	if c.Mode != "oneshot" {
		t.Errorf("Mode = %q, want oneshot", c.Mode)
	}
	if c.Role != "reviewer" {
		t.Errorf("Role = %q, want reviewer", c.Role)
	}
	if c.PID != os.Getpid() {
		t.Errorf("PID = %d, want this process %d", c.PID, os.Getpid())
	}
	if c.Tool == "" || c.Cwd == "" {
		t.Errorf("card missing tool/cwd: %+v", c)
	}
	// A safe, concise label: first line only, collapsed whitespace, no newline.
	if c.Task != "fix the login" {
		t.Errorf("Task = %q, want the first line only", c.Task)
	}
	// It is WORK-keyed, not the agent's identity id.
	if !strings.HasPrefix(c.ID, "oneshot-") {
		t.Errorf("card id %q is not work-keyed", c.ID)
	}

	close(r.release)
	<-done

	if got := oneShotMembers(t); len(got) != 0 {
		t.Fatalf("one-shot cards after completion = %d, want 0", len(got))
	}
}

func TestInvokeRemovesTaskCardOnError(t *testing.T) {
	isolatedRoom(t)

	r := &blockingRunner{started: make(chan struct{}), release: make(chan struct{}), err: context.Canceled}
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = Invoke(context.Background(), Options{
			Agent: "codex", Instruction: "run the failing task", Cwd: t.TempDir(),
		}, r)
	}()

	<-r.started
	if len(oneShotMembers(t)) != 1 {
		t.Fatalf("expected one live one-shot card while running")
	}
	close(r.release)
	<-done

	if got := oneShotMembers(t); len(got) != 0 {
		t.Fatalf("one-shot cards after error = %d, want 0 (must be removed on error too)", len(got))
	}
}

func TestConcurrentOneShotsDoNotCollide(t *testing.T) {
	isolatedRoom(t)

	const n = 3
	runners := make([]*blockingRunner, n)
	done := make(chan struct{}, n)
	for i := range runners {
		runners[i] = &blockingRunner{started: make(chan struct{}), release: make(chan struct{})}
		r := runners[i]
		go func() {
			defer func() { done <- struct{}{} }()
			_, _ = Invoke(context.Background(), Options{
				Agent: "codex", Instruction: "concurrent work", Cwd: t.TempDir(),
			}, r)
		}()
	}
	for _, r := range runners {
		<-r.started
	}

	live := oneShotMembers(t)
	if len(live) != n {
		t.Fatalf("concurrent one-shot cards = %d, want %d (they collided)", len(live), n)
	}
	ids := map[string]bool{}
	for _, c := range live {
		if ids[c.ID] {
			t.Fatalf("duplicate card id %q — concurrent one-shots collided", c.ID)
		}
		ids[c.ID] = true
	}

	for _, r := range runners {
		close(r.release)
	}
	for range runners {
		<-done
	}
	if got := oneShotMembers(t); len(got) != 0 {
		t.Fatalf("one-shot cards after all complete = %d, want 0", len(got))
	}
}

func TestDryRunPublishesNoTaskCard(t *testing.T) {
	isolatedRoom(t)

	if _, err := Invoke(context.Background(), Options{
		Agent: "codex", Instruction: "would run", Cwd: t.TempDir(), DryRun: true,
	}, &fakeRunner{}); err != nil {
		t.Fatal(err)
	}
	members, err := room.Members()
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 0 {
		t.Fatalf("dry-run published %d cards, want 0", len(members))
	}
}

func TestTaskLabelIsSafeAndConcise(t *testing.T) {
	long := strings.Repeat("a", 200)
	got := taskLabel(Options{Instruction: "  first\tline  \x00 with\r\nsecond"})
	if got != "first line with" {
		t.Errorf("taskLabel sanitize = %q, want %q", got, "first line with")
	}
	if l := taskLabel(Options{Instruction: long}); len([]rune(l)) != 81 || !strings.HasSuffix(l, "…") {
		t.Errorf("taskLabel truncation = %q (len %d), want 80 runes + ellipsis", l, len([]rune(l)))
	}
	if l := taskLabel(Options{Instruction: ""}); l != "" {
		t.Errorf("empty instruction label = %q, want empty", l)
	}
}
