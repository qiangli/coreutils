package probe

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/qiangli/coreutils/pkg/browser/internal/cdpactions"
	"github.com/qiangli/coreutils/pkg/browser/wire"
)

const Mode = "probe"

type Client struct {
	url string

	mu        sync.Mutex
	allocCtx  context.Context
	allocStop context.CancelFunc
	ctx       context.Context
	ctxStop   context.CancelFunc
}

func New(url string) *Client {
	if url == "" {
		url = DefaultURL
	}
	return &Client{url: strings.TrimRight(url, "/")}
}

func (c *Client) URL() string { return c.url }

func (c *Client) Available(ctx context.Context) bool {
	hctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(hctx, http.MethodGet, c.url+"/json/version", nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func (c *Client) EnsureReady(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ctx != nil {
		return nil
	}
	if !c.Available(ctx) {
		return fmt.Errorf("probe: no Chrome at %s", c.url)
	}
	allocCtx, allocStop := chromedp.NewRemoteAllocator(context.Background(), c.url)
	cdpCtx, cdpStop := chromedp.NewContext(allocCtx)
	if err := chromedp.Run(cdpCtx); err != nil {
		cdpStop()
		allocStop()
		return fmt.Errorf("probe: attach to %s: %w", c.url, err)
	}
	c.allocCtx, c.allocStop = allocCtx, allocStop
	c.ctx, c.ctxStop = cdpCtx, cdpStop
	return nil
}

func (c *Client) Execute(ctx context.Context, action wire.Action) (*wire.Result, error) {
	c.mu.Lock()
	cdpCtx := c.ctx
	c.mu.Unlock()
	if cdpCtx == nil {
		if err := c.EnsureReady(ctx); err != nil {
			return nil, err
		}
		c.mu.Lock()
		cdpCtx = c.ctx
		c.mu.Unlock()
	}
	if cdpCtx == nil {
		return nil, errors.New("probe: not ready")
	}
	callCtx, cancel := context.WithTimeout(cdpCtx, 30*time.Second)
	defer cancel()
	return cdpactions.Run(callCtx, Mode, action)
}

func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ctxStop != nil {
		c.ctxStop()
		c.ctxStop = nil
	}
	if c.allocStop != nil {
		c.allocStop()
		c.allocStop = nil
	}
	c.ctx = nil
	c.allocCtx = nil
	return nil
}
