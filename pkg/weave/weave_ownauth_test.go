// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package weave

import (
	"slices"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/pkg/secrets"
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
// credential-shaped name outside the own-auth allowlist is left stripped, no
// matter which agent is launching or what model it asked for.
func TestWeavePreserveOwnAuthDoesNotGrantProviderKeys(t *testing.T) {
	scrubbed := []string{"PATH=/usr/bin"}
	environ := []string{
		"ANTHROPIC_API_KEY=sk-ant-live",
		"OPENAI_API_KEY=sk-oai-live",
		"GOOGLE_API_KEY=sk-goog-live",
		"AWS_SECRET_ACCESS_KEY=aws-live",
	}
	got := weavePreserveOwnAuth(scrubbed, environ)
	if len(got) != 1 {
		t.Fatalf("weave must not grant a provider credential: %v", got)
	}
}

// The three sanctioned third-party keys survive the scrub. They are not a
// grant: weave manufactures nothing, it declines to strip what the launcher's
// own environment already carries, because an agent with no persistent key
// store (ycode) has no other place to read its auth from.
func TestWeavePreserveOwnAuthKeepsSanctionedThirdPartyKeys(t *testing.T) {
	for _, name := range []string{"DEEPSEEK_API_KEY", "MOONSHOT_API_KEY", "KIMI_API_KEY"} {
		kv := name + "=sk-own-123"
		got := weavePreserveOwnAuth([]string{"PATH=/usr/bin"}, []string{kv})
		if !slices.Contains(got, kv) {
			t.Errorf("%s is the agent's own env credential and must survive the scrub: %v", name, got)
		}
	}
}

// THE LIVE-LAUNCH ASSERTION.
//
// This asserts the env `weave start` actually assembles for a third-party
// agent, through weaveChildEnv — the same call the spawn makes. That is the
// whole point: run #101 regressed this exact behavior with a green build and
// passing unit tests, because every test checked a piece in isolation and none
// checked what the child would really receive. A test that re-creates the
// assembly instead of calling it cannot catch the caller dropping the step.
func TestWeaveChildEnvKeepsOwnKeyAndStripsOperatorKeys(t *testing.T) {
	// Pin the firewall ON — an operator who has opted out in their real shell
	// must not turn this assertion into a tautology.
	t.Setenv(secrets.AllowAgentSecretsEnv, "0")

	ambient := []string{
		"PATH=/usr/bin",
		"HOME=/home/op",
		"PWD=/origin/repo", // the origin repo — containment must drop this
		"OLDPWD=/origin/elsewhere",
		"DEEPSEEK_API_KEY=sk-deepseek-own",  // the agent's own auth
		"ANTHROPIC_API_KEY=sk-ant-operator", // the operator's vault key
		"OPENAI_API_KEY=sk-oai-operator",
		"AWS_SECRET_ACCESS_KEY=aws-operator",
	}
	it := &weaveItem{ID: 105, Title: "t", Body: "b", Owner: "007-a"}
	got := weaveChildEnv(ambient, "/ws/issue-105", "agent/weave-issue-105", "main", it, nil)

	// What the agent authenticates with must be there, with its real value.
	if !slices.Contains(got, "DEEPSEEK_API_KEY=sk-deepseek-own") {
		t.Errorf("child env lost the agent's own DEEPSEEK_API_KEY — it will die with "+
			"'no LLM provider configured': %v", names(got))
	}
	// The operator's keyring must not be.
	for _, banned := range []string{"ANTHROPIC_API_KEY", "OPENAI_API_KEY", "AWS_SECRET_ACCESS_KEY"} {
		if envHas(got, banned) {
			t.Errorf("child env carries the operator's %s — the credential firewall is open", banned)
		}
	}
	// Containment still holds: the child is pinned to its workspace and cannot
	// read the origin repo's path out of its environment.
	if !slices.Contains(got, "PWD=/ws/issue-105") {
		t.Errorf("PWD not pinned to the workspace: %v", names(got))
	}
	if slices.Contains(got, "PWD=/origin/repo") || envHas(got, "OLDPWD") {
		t.Errorf("the origin repo's path leaked into the child env: %v", got)
	}
	// And the run is still stamped.
	if !slices.Contains(got, "WEAVE_ISSUE=105") || !slices.Contains(got, "WEAVE_AGENT=007-a") {
		t.Errorf("weave stamps missing from child env: %v", names(got))
	}
}

func envHas(env []string, name string) bool {
	for _, kv := range env {
		if strings.HasPrefix(kv, name+"=") {
			return true
		}
	}
	return false
}

// names renders an env as NAMES ONLY — a failure message must never print the
// operator's secrets into a test log.
func names(env []string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		if i := strings.IndexByte(kv, '='); i > 0 {
			out = append(out, kv[:i])
		}
	}
	return out
}
