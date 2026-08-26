package joincmd

import (
	"sync"

	"github.com/qiangli/coreutils/pkg/collate"
)

// stringCollator is the small portion of pkg/collate that one join run needs.
// Keeping it an interface lets tests supply a fake provider without a package
// global or a real glibc runtime.
type stringCollator interface {
	Compare(a, b string) (int, error)
	Close() error
}

type collatorOpener func(string) (stringCollator, error)

// openCollator is the production opener wired in run. It reaches the shared
// invocation-owned pkg/collate provider, which validates the locale name and
// refuses anything but the ISO-8859-1 aliases before any libc is loaded.
func openCollator(name string) (stringCollator, error) { return collate.Open(name) }

// collatorAdapter serializes access to a provider and retains the first
// comparison failure so join's byte-order comparator signature (int, no error)
// can stay unchanged. The mutex mirrors sort's adapter: even a non-concurrent
// provider is made safe, and Err is consulted once after the join completes.
type collatorAdapter struct {
	provider stringCollator
	mu       sync.Mutex
	err      error
}

func newCollatorAdapter(provider stringCollator) *collatorAdapter {
	return &collatorAdapter{provider: provider}
}

// Compare returns the collation order of a and b. On the first provider error
// it records the error and returns 0 for this and every later comparison, so
// the run finishes deterministically and the caller reports the failure via
// Err rather than emitting a misordered result silently.
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
