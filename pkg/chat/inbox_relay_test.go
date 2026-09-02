package chat

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/qiangli/coreutils/pkg/bus"
)

func TestInboxRelayCommitsOnlyAfterDelivery(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	var attempts atomic.Int32
	var committed atomic.Bool

	prepare := func() bus.PreparedPreamble {
		if committed.Load() {
			return bus.PreparedPreamble{}
		}
		return bus.NewPreparedPreamble("pending input", func() error {
			committed.Store(true)
			return nil
		})
	}
	deliver := func(p bus.PreparedPreamble) error {
		if attempts.Add(1) == 1 {
			return errors.New("transport busy")
		}
		return p.Commit()
	}

	go runInboxRelayEvery(ctx, done, nil, prepare, deliver, 5*time.Millisecond)
	deadline := time.Now().Add(time.Second)
	for !committed.Load() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !committed.Load() {
		t.Fatal("pending input was not retried and committed after delivery")
	}
	if got := attempts.Load(); got < 2 {
		t.Fatalf("delivery attempts = %d, want retry after failure", got)
	}
}

func TestInboxRelayWaitsForTransportReadiness(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	var ready atomic.Bool
	var delivered atomic.Bool

	go runInboxRelayEvery(ctx, done, ready.Load,
		func() bus.PreparedPreamble { return bus.NewPreparedPreamble("pending", nil) },
		func(bus.PreparedPreamble) error { delivered.Store(true); return nil },
		5*time.Millisecond)
	time.Sleep(20 * time.Millisecond)
	if delivered.Load() {
		t.Fatal("relay delivered while the transport reported an active turn")
	}
	ready.Store(true)
	deadline := time.Now().Add(time.Second)
	for !delivered.Load() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !delivered.Load() {
		t.Fatal("relay did not deliver after the transport became ready")
	}
}
