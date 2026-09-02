package chat

import (
	"context"
	"time"

	"github.com/qiangli/coreutils/pkg/bus"
)

// managedInboxPoll is deliberately modest: input remains durable in its source,
// and a one-second wake-up is fast compared with an agent turn without turning
// every managed session into a filesystem hot loop.
const managedInboxPoll = time.Second

// runInboxRelay turns durable unified-inbox input into an actual agent turn.
// prepare MUST be non-consuming; deliver owns acknowledgement and may refuse
// while a transport is busy. A refusal leaves every cursor untouched and the
// next poll retries the same input.
func runInboxRelay(ctx context.Context, done <-chan struct{}, ready func() bool,
	prepare func() bus.PreparedPreamble, deliver func(bus.PreparedPreamble) error) {
	runInboxRelayEvery(ctx, done, ready, prepare, deliver, managedInboxPoll)
}

func runInboxRelayEvery(ctx context.Context, done <-chan struct{}, ready func() bool,
	prepare func() bus.PreparedPreamble, deliver func(bus.PreparedPreamble) error, interval time.Duration) {
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-done:
			return
		case <-tick.C:
		}
		if ready != nil && !ready() {
			continue
		}
		pending := prepare()
		if pending.Err() != nil || pending.Text == "" {
			continue
		}
		_ = deliver(pending) // failure is durable: no Commit means retry
	}
}

func (s *Session) startInboxRelay(ctx context.Context) {
	ready := func() bool { return true }
	if s.acp != nil {
		ready = s.acp.idle
	}
	go runInboxRelay(ctx, s.done, ready,
		func() bus.PreparedPreamble { return bus.PrepareForAgent(s.inboxAgent, "") },
		s.deliverPreparedInbox)
}

// deliverPreparedInbox is Say with the already-prepared input preserved. This
// avoids a second snapshot (and duplicate rendering), while retaining the same
// budget gate, transport, metering, and commit-after-delivery contract as Say.
func (s *Session) deliverPreparedInbox(p bus.PreparedPreamble) error {
	if err := p.Err(); err != nil {
		return err
	}
	if p.Text == "" {
		return nil
	}
	if d := s.governTurn(p.Text); !d.Allowed() {
		return nil
	}
	if err := s.say(p.Text); err != nil {
		return err
	}
	recordPreambleAdmission(context.Background(), p)
	if err := p.Commit(); err != nil {
		return err
	}
	recordLaunchUsageTokens(context.Background(), s.launch, estimateTokens(p.Text), 0)
	// The relay owns this unsolicited ACP turn, so it also consumes that turn's
	// completion. Otherwise the completed result would keep the transport
	// non-idle forever and only the first inbox message could be delivered.
	if s.acp != nil {
		return s.waitACPTurn(s.acp.ctx)
	}
	return nil
}
