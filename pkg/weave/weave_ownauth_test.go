// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package weave

import (
	"slices"
	"strings"
	"testing"
)

// weave must never hand out a provider credential — it only avoids stripping
// a name that is the agent's OWN preconfigured auth (e.g. ycode's
// DHNT_API_KEY / DHNT_BASE_URL from `ycode login`), sourced from the
// launcher's own environ, never manufactured or looked up by model.
func TestWeavePreserveOwnAuthRestoresStrippedOwnAuth(t *testing.T) {
	scrubbed := []string{"PATH=/usr/bin", "HOME=/home/x"} // vault scrub already ran
	environ := []string{"DHNT_API_KEY=own-key-123", "DHNT_BASE_URL=https://dhnt.example", "OTHER=1"}
	got := weavePreserveOwnAuth(scrubbed, environ)
	if !slices.Contains(got, "DHNT_API_KEY=own-key-123") {
		t.Fatalf("own auth key not restored after scrub: %v", got)
	}
	if !slices.Contains(got, "DHNT_BASE_URL=https://dhnt.example") {
		t.Fatalf("own auth base url not restored after scrub: %v", got)
	}
	// Only the declared own-auth names — nothing else from environ leaks through.
	for _, kv := range got {
		if strings.HasPrefix(kv, "OTHER=") {
			t.Fatalf("restored an undeclared env var: %q", kv)
		}
	}
}

// A name absent from the launcher's own environ is not manufactured — the
// agent fails to authenticate and says so, which is a real answer, not a leak.
func TestWeavePreserveOwnAuthNoOpsWhenAbsent(t *testing.T) {
	base := []string{"PATH=/usr/bin"}
	got := weavePreserveOwnAuth(base, []string{"OTHER=1"})
	if len(got) != 1 {
		t.Fatalf("nothing to restore should be a no-op, got %v", got)
	}
}

// A name already present in the scrubbed env (not stripped in the first
// place) is not duplicated.
func TestWeavePreserveOwnAuthDoesNotDuplicate(t *testing.T) {
	scrubbed := []string{"PATH=/usr/bin", "DHNT_API_KEY=already-here"}
	environ := []string{"DHNT_API_KEY=different-value"}
	got := weavePreserveOwnAuth(scrubbed, environ)
	count := 0
	for _, kv := range got {
		if strings.HasPrefix(kv, "DHNT_API_KEY=") {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly one DHNT_API_KEY entry, got %v", got)
	}
	if !slices.Contains(got, "DHNT_API_KEY=already-here") {
		t.Fatalf("existing value should not be overwritten: %v", got)
	}
}

// Weave never looks at the model catalog to decide what to grant — a
// credential-shaped name that is not in the own-auth allowlist (e.g. a
// provider key like DEEPSEEK_API_KEY) is left stripped.
func TestWeavePreserveOwnAuthDoesNotGrantProviderKeys(t *testing.T) {
	scrubbed := []string{"PATH=/usr/bin"}
	environ := []string{"DEEPSEEK_API_KEY=sk-test-123"}
	got := weavePreserveOwnAuth(scrubbed, environ)
	for _, kv := range got {
		if strings.HasPrefix(kv, "DEEPSEEK_API_KEY=") {
			t.Fatalf("weave must not grant a provider credential: %v", got)
		}
	}
}
