package secrets

import "testing"

// A metered model needs exactly one credential to answer, and the registry says
// which. The firewall strips the whole keyring; this hands back the one key —
// and no others, which is the difference between a boundary and an outage.
func TestGrantAgentKeyResolvesTheDeclaredRef(t *testing.T) {
	env := []string{
		"PATH=/usr/bin",
		"MOONSHOT_API_KEY=m-secret",
		"DEEPSEEK_API_KEY=d-secret",
		"ANTHROPIC_TOKEN=a-secret",
		"GITHUB_TOKEN=g-secret",
	}
	cases := map[string]string{
		"moonshot":  "MOONSHOT_API_KEY=m-secret",
		"deepseek":  "DEEPSEEK_API_KEY=d-secret",
		"anthropic": "ANTHROPIC_TOKEN=a-secret", // falls through _API_KEY to _TOKEN
	}
	for ref, want := range cases {
		got, ok := GrantAgentKey(env, ref)
		if !ok || got != want {
			t.Errorf("GrantAgentKey(%q) = %q, %v; want %q", ref, got, ok, want)
		}
	}
}

// A ref this host has no key for is a real answer, not a crash: the agent will
// fail to authenticate and SAY so, which is what `agents verify --live` reads.
func TestGrantAgentKeyMissingRef(t *testing.T) {
	if _, ok := GrantAgentKey([]string{"PATH=/usr/bin"}, "moonshot"); ok {
		t.Error("a ref with no key on this host must not resolve")
	}
	if _, ok := GrantAgentKey([]string{"X=1"}, ""); ok {
		t.Error("an empty ref must not resolve")
	}
}

// It grants ONE key. An agent asking for moonshot must not be handed github's.
func TestGrantAgentKeyGrantsNothingElse(t *testing.T) {
	env := []string{"MOONSHOT_API_KEY=m", "GITHUB_TOKEN=g", "OPENAI_API_KEY=o"}
	got, ok := GrantAgentKey(env, "moonshot")
	if !ok || got != "MOONSHOT_API_KEY=m" {
		t.Fatalf("got %q", got)
	}
}
