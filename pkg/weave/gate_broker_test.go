package weave

import "testing"

func TestClassifyGate(t *testing.T) {
	tests := []struct {
		name string
		tail string
		kind GateKind
		url  string
	}{
		{
			name: "claude trust",
			tail: "Do you trust the contents of this folder?\n1. Yes, proceed\n2. No, exit",
			kind: GateTrust,
		},
		{
			name: "codex trust",
			tail: "Do you trust this directory?\n1) Yes\n2) No",
			kind: GateTrust,
		},
		{
			name: "agy browser oauth",
			tail: "You are currently not signed in.\nOpen https://accounts.google.com/o/oauth2/v2/auth?client_id=abc to sign in.",
			kind: GateBrowserOAuth,
			url:  "https://accounts.google.com/o/oauth2/v2/auth?client_id=abc",
		},
		{
			name: "codex browser login",
			tail: "Login required. Visit https://auth.openai.com/login?state=xyz and complete authentication.",
			kind: GateBrowserOAuth,
			url:  "https://auth.openai.com/login?state=xyz",
		},
		{
			name: "gh device code",
			tail: "First copy your one-time code: 1A2B-3C4D\nThen press Enter to open github.com in your browser...\nEnter the code at https://github.com/login/device",
			kind: GateDeviceCode,
			url:  "https://github.com/login/device",
		},
		{
			name: "gcloud device code",
			tail: "Go to the following link in your browser:\nhttps://www.google.com/device\nEnter the code: AB12-CD34",
			kind: GateDeviceCode,
			url:  "https://www.google.com/device",
		},
		{
			name: "claude api key",
			tail: "Authentication failed: API key not set. Set ANTHROPIC_API_KEY and retry.",
			kind: GateAPIKey,
		},
		{
			name: "opencode api key",
			tail: "Error: no api key found for provider.",
			kind: GateAPIKey,
		},
		{
			name: "human login no url",
			tail: "Please log in to continue. Run `login` from an interactive terminal.",
			kind: GateHuman,
		},
		{
			name: "unauthorized no route",
			tail: "Authentication required before continuing.",
			kind: GateHuman,
		},
		{
			name: "plain question is none",
			tail: "Do you want me to update the tests as well, or keep the change scoped?",
			kind: GateNone,
		},
		{
			name: "working output is none",
			tail: "go test ./pkg/weave/...\nok  github.com/qiangli/coreutils/pkg/weave  0.243s\nWriting implementation notes.",
			kind: GateNone,
		},
		{
			name: "ordinary url is none",
			tail: "Fetched https://example.com/callbacks.md while researching webhook callback behavior.",
			kind: GateNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyGate(tt.tail)
			if got.Kind != tt.kind {
				t.Fatalf("kind = %q, want %q (signature %q)", got.Kind, tt.kind, got.Signature)
			}
			if got.URL != tt.url {
				t.Fatalf("url = %q, want %q", got.URL, tt.url)
			}
			if tt.kind != GateNone && got.Signature == "" {
				t.Fatalf("signature is empty for %q", tt.kind)
			}
		})
	}
}
