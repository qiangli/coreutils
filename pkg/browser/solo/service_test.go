package solo

import (
	"context"
	"testing"

	"github.com/qiangli/coreutils/pkg/browser/wire"
)

func TestResolveChromePathHonorsMissingExplicitPath(t *testing.T) {
	c := New(Config{ChromePath: "/definitely/not/a/chrome"})
	if got := c.ResolveChromePath(); got != "" {
		t.Fatalf("ResolveChromePath=%q, want empty", got)
	}
}

func TestLiveSoloSkippedWhenNoChrome(t *testing.T) {
	ctx := context.Background()
	c := New(Config{})
	if !c.Available(ctx) {
		t.Skip("no Chrome or Chromium binary found")
	}
	defer c.Close()
	res, err := c.Execute(ctx, wire.Action{Type: wire.ActionEvaluate, Script: "1+1"})
	if err != nil {
		t.Skipf("Chrome found but not launchable in this environment: %v", err)
	}
	if !res.Success || res.Data != "2" {
		t.Fatalf("unexpected live result: %#v", res)
	}
}
