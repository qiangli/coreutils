package sortcmd

import "sync"

// stringCollator is the small portion of pkg/collate needed by one sort run.
// It deliberately permits fake providers without introducing package globals.
type stringCollator interface {
	Compare(a, b string) (int, error)
	Close() error
}

type collatorOpener func(string) (stringCollator, error)

// collatorAdapter serializes access to a provider and retains the first
// comparison failure. sort's parallel paths may call its comparator from many
// goroutines; this adapter makes even a non-concurrent test/provider safe.
type collatorAdapter struct {
	provider stringCollator
	mu       sync.Mutex
	err      error
}

func newCollatorAdapter(provider stringCollator) *collatorAdapter {
	return &collatorAdapter{provider: provider}
}

func (c *collatorAdapter) Compare(a, b string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return 0
	}
	d, err := c.provider.Compare(a, b)
	if err != nil {
		c.err = err
		return 0
	}
	return d
}

func (c *collatorAdapter) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}

func (c *collatorAdapter) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.provider.Close()
}
