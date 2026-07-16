// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package weave

import (
	"slices"
	"strings"
	"testing"
)

// A metered weave agent must get back the ONE key its model declares, after the
// vault scrub. This is the test that was missing when the scrub-without-grant bug
// left every deepseek/kimi weave agent unauthenticated while the fleet recorded it
// as "medium reliability".
func TestGrantAgentModelKeyGrantsDeclaredKey(t *testing.T) {
	// deepseek-v4-pro declares api_key_ref: deepseek -> DEEPSEEK_API_KEY.
	launch := &weaveAgentLaunch{ModelName: "deepseek-v4-pro"}
	scrubbed := []string{"PATH=/usr/bin", "HOME=/home/x"} // vault already stripped
	environ := []string{"DEEPSEEK_API_KEY=sk-test-123", "OTHER=1"}
	got := grantAgentModelKey(scrubbed, launch, environ)
	if !slices.Contains(got, "DEEPSEEK_API_KEY=sk-test-123") {
		t.Fatalf("declared key not granted back after scrub: %v", got)
	}
	// Only the declared key — nothing else from environ leaks through.
	for _, kv := range got {
		if strings.HasPrefix(kv, "OTHER=") {
			t.Fatalf("granted an undeclared env var: %q", kv)
		}
	}
}

// Deny by default: a key the operator's environment does not carry is not granted
// (the agent will fail to authenticate and say so — a real answer, not a leak).
func TestGrantAgentModelKeyDeniesWhenKeyAbsent(t *testing.T) {
	launch := &weaveAgentLaunch{ModelName: "deepseek-v4-pro"}
	got := grantAgentModelKey([]string{"PATH=/usr/bin"}, launch, []string{"OTHER=1"})
	for _, kv := range got {
		if strings.HasPrefix(kv, "DEEPSEEK_API_KEY=") {
			t.Fatalf("granted a key that was not in the operator env: %v", got)
		}
	}
}

// A nil launch or a model with no api_key_ref (e.g. a subscription model) is a no-op.
func TestGrantAgentModelKeyNoOps(t *testing.T) {
	base := []string{"PATH=/usr/bin"}
	if got := grantAgentModelKey(base, nil, []string{"DEEPSEEK_API_KEY=x"}); len(got) != 1 {
		t.Fatalf("nil launch should be a no-op, got %v", got)
	}
	if got := grantAgentModelKey(base, &weaveAgentLaunch{ModelName: ""}, []string{"DEEPSEEK_API_KEY=x"}); len(got) != 1 {
		t.Fatalf("empty model should be a no-op, got %v", got)
	}
}
