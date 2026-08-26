package talkcmd

import (
	"bufio"
	"context"
	"errors"
	"io"
	"sync"
	"time"
)

// asyncInput exists for embedding/tests whose input is not an OS descriptor.
// Production terminals use polledTerminalInput. Close owns and closes a
// ReadCloser before waiting for the reader, so a cancelled pipe cannot survive
// this invocation and consume input belonging to a later command.
type asyncInput struct {
	r      io.ReadCloser
	events chan inputEvent
	done   chan struct{}
	stop   chan struct{}
	once   sync.Once
}

func newAsyncInput(r io.Reader) (*asyncInput, error) {
	closer, ok := r.(io.ReadCloser)
	if !ok {
		return nil, errors.New("non-file terminal input must be closable")
	}
	a := &asyncInput{r: closer, events: make(chan inputEvent, 1), done: make(chan struct{}), stop: make(chan struct{})}
	go func() {
		defer close(a.done)
		defer close(a.events)
		br := bufio.NewReader(r)
		for {
			data, err := br.ReadBytes('\n')
			if len(data) > 0 {
				select {
				case a.events <- inputEvent{data: data}:
				case <-a.stop:
					return
				}
			}
			if err != nil {
				select {
				case a.events <- inputEvent{err: err}:
				case <-a.stop:
				}
				return
			}
		}
	}()
	return a, nil
}

func (a *asyncInput) Poll(ctx context.Context, wait time.Duration) (inputEvent, bool) {
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case event, ok := <-a.events:
		if !ok {
			return inputEvent{err: io.EOF}, true
		}
		return event, true
	case <-ctx.Done():
		return inputEvent{}, false
	case <-timer.C:
		return inputEvent{}, false
	}
}

func (a *asyncInput) Close() error {
	var closeErr error
	a.once.Do(func() {
		close(a.stop)
		closeErr = a.r.Close()
	})
	// Do not return while a reader still exists. A timeout would merely hide
	// the leak and let that goroutine steal bytes from a later in-process tool.
	<-a.done
	return closeErr
}
