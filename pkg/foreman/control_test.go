package foreman

import (
	"context"
	"strings"
	"testing"
	"time"
)

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

	if err := Tell(dir, "sock", "steer over socket"); err != nil {
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
