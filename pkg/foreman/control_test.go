package foreman

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"
)

func TestControlStopDoesNotWaitForActiveTurnStateLock(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &Session{
		store:  NewStore(t.TempDir(), "locked-turn"),
		state:  State{ID: "locked-turn", Status: StatusWorking},
		stopCh: make(chan string, 1),
	}
	cancelled := make(chan struct{})
	finished := make(chan struct{})

	// Model Apply holding this lock for a long-running turn. Stop cancellation
	// must happen before the watcher needs the lock to persist terminal state.
	s.mu.Lock()
	go func() {
		s.watchControlLifetime(context.Background(), func() { close(cancelled) }, ln, make(chan struct{}), time.Time{}, "")
		close(finished)
	}()
	// Let at least one health tick pass first. The regression was the tick itself
	// trying to take s.mu and blocking the watcher before stop could be selected.
	time.Sleep(200 * time.Millisecond)
	s.requestStop("test stop")
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		s.mu.Unlock()
		t.Fatal("stop cancellation waited behind the active turn state lock")
	}
	s.mu.Unlock()
	select {
	case <-finished:
	case <-time.After(3 * time.Second):
		t.Fatal("control watcher did not finish after the turn released its lock")
	}
}

func TestTellReachesSessionOverUnixSocket(t *testing.T) {
	dir := t.TempDir()
	r := &stubRunner{out: "ack"}
	s, err := Start(context.Background(), Options{
		ID:     "sock",
		Goal:   "socket test",
		Agent:  "stub",
		Root:   dir,
		Runner: r,
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan string, 1)
	errc := make(chan error, 1)
	go func() { errc <- s.ServeControl(ctx, ready) }()
	select {
	case <-ready:
	case err := <-errc:
		t.Fatalf("ServeControl: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("control socket did not become ready")
	}

	if _, err := Tell(dir, "sock", "steer over socket"); err != nil {
		t.Fatalf("Tell: %v", err)
	}

	// THE ACK MEANS "ACCEPTED", NOT "FINISHED", and that is deliberate.
	//
	// A turn runs an LLM: it takes minutes. The ack has a 3-second deadline. The
	// old code applied the command inline and only then acked, which meant every
	// `foreman tell` against a real agent died on "i/o timeout" while the agent it
	// had just launched went on working — the command SUCCEEDED and reported
	// failure. It also blocked the listener for the whole turn, so the one moment
	// you most need to say "stop, wrong file" was the one moment the socket would
	// not take the call.
	//
	// So the outcome lands in state.json, which is where `foreman status` reads it
	// from and the only place that can honestly carry the result of a long turn.
	deadline := time.Now().Add(5 * time.Second)
	for {
		if p := r.Prompts(); len(p) == 1 && strings.Contains(p[0], "steer over socket") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("runner prompts = %#v, want socket steering", r.Prompts())
		}
		time.Sleep(20 * time.Millisecond)
	}
	for {
		st, err := NewStore(dir, "sock").LoadState()
		if err != nil {
			t.Fatalf("LoadState: %v", err)
		}
		if st.Status == StatusIdle {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("status = %q, want idle", st.Status)
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-errc:
		if err != nil {
			t.Fatalf("ServeControl exit: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("control server did not stop")
	}
}

func TestServeControlHardRuntimeStopsSession(t *testing.T) {
	dir := t.TempDir()
	s, err := Start(context.Background(), Options{
		ID: "deadline", Goal: "bounded", Root: dir, MaxRuntime: 80 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	ready := make(chan string, 1)
	errC := make(chan error, 1)
	go func() { errC <- s.ServeControl(context.Background(), ready) }()
	select {
	case <-ready:
	case err := <-errC:
		t.Fatalf("ServeControl before ready: %v", err)
	case <-time.After(time.Second):
		t.Fatal("control socket did not become ready")
	}
	select {
	case err := <-errC:
		if err != nil {
			t.Fatalf("ServeControl: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("max runtime did not stop control server")
	}
	st, err := NewStore(dir, "deadline").LoadState()
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if !st.Stopped || st.Status != StatusBlocked || !strings.Contains(st.StopReason, "max runtime 80ms exceeded") {
		t.Fatalf("expired state = %+v", st)
	}
}

func TestServeControlStopCommandReturns(t *testing.T) {
	dir := t.TempDir()
	s, err := Start(context.Background(), Options{
		ID: "stop", Goal: "stoppable", Root: dir, MaxRuntime: time.Minute,
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	ready := make(chan string, 1)
	errC := make(chan error, 1)
	go func() { errC <- s.ServeControl(context.Background(), ready) }()
	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("control socket did not become ready")
	}
	if _, err := SendCommand(dir, "stop", Command{Verb: CommandStop}); err != nil {
		t.Fatalf("stop: %v", err)
	}
	select {
	case err := <-errC:
		if err != nil {
			t.Fatalf("ServeControl: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("stop command left control server running")
	}
}

type cancelRunner struct {
	started chan struct{}
	stopped chan struct{}
}

func (r *cancelRunner) Run(ctx context.Context, _ string, _ []string, _ string) (string, int, error) {
	close(r.started)
	<-ctx.Done()
	close(r.stopped)
	return "", 1, ctx.Err()
}

func TestServeControlStopCancelsActiveTurn(t *testing.T) {
	dir := t.TempDir()
	r := &cancelRunner{started: make(chan struct{}), stopped: make(chan struct{})}
	s, err := Start(context.Background(), Options{
		ID: "active-stop", Goal: "stoppable", Agent: "stub", Root: dir,
		MaxRuntime: time.Minute, Runner: r,
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	ready := make(chan string, 1)
	errC := make(chan error, 1)
	go func() { errC <- s.ServeControl(context.Background(), ready) }()
	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("control socket did not become ready")
	}
	if _, err := SendCommand(dir, "active-stop", Command{Verb: CommandTell, Message: "work"}); err != nil {
		t.Fatalf("tell: %v", err)
	}
	select {
	case <-r.started:
	case <-time.After(time.Second):
		t.Fatal("turn did not start")
	}
	if _, err := SendCommand(dir, "active-stop", Command{Verb: CommandStop}); err != nil {
		t.Fatalf("stop: %v", err)
	}
	select {
	case <-r.stopped:
	case <-time.After(time.Second):
		t.Fatal("stop did not cancel active turn")
	}
	select {
	case err := <-errC:
		if err != nil {
			t.Fatalf("ServeControl: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("control server did not stop")
	}
	st, err := NewStore(dir, "active-stop").LoadState()
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if !st.Stopped || st.Status != StatusDone || st.StopReason != "stopped by operator" {
		t.Fatalf("stopped state = %+v", st)
	}
}

func TestServeControlDeadlineCancelsActiveTurn(t *testing.T) {
	dir := t.TempDir()
	r := &cancelRunner{started: make(chan struct{}), stopped: make(chan struct{})}
	s, err := Start(context.Background(), Options{
		ID: "active-deadline", Goal: "bounded", Agent: "stub", Root: dir,
		MaxRuntime: 80 * time.Millisecond, Runner: r,
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	ready := make(chan string, 1)
	errC := make(chan error, 1)
	go func() { errC <- s.ServeControl(context.Background(), ready) }()
	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("control socket did not become ready")
	}
	if _, err := SendCommand(dir, "active-deadline", Command{Verb: CommandTell, Message: "work"}); err != nil {
		t.Fatalf("tell: %v", err)
	}
	select {
	case <-r.started:
	case <-time.After(time.Second):
		t.Fatal("turn did not start")
	}
	select {
	case <-r.stopped:
	case <-time.After(time.Second):
		t.Fatal("deadline did not cancel active turn")
	}
	select {
	case err := <-errC:
		if err != nil {
			t.Fatalf("ServeControl: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("control server did not stop")
	}
	st, err := NewStore(dir, "active-deadline").LoadState()
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if !st.Stopped || st.Status != StatusBlocked || st.StopReason != "max runtime 80ms exceeded" {
		t.Fatalf("expired state = %+v", st)
	}
}
