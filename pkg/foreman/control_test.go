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
	if len(r.prompts) != 1 || !strings.Contains(r.prompts[0], "steer over socket") {
		t.Fatalf("runner prompts = %#v, want socket steering", r.prompts)
	}
	st, err := NewStore(dir, "sock").LoadState()
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if st.Status != StatusIdle {
		t.Fatalf("status = %q, want idle", st.Status)
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
