package secrets

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestResolveProvider(t *testing.T) {
	tests := map[string]string{
		"dragon-moonshot": "moonshot",
		"dragon-deepseek": "deepseek",
	}
	for name, want := range tests {
		t.Run(name, func(t *testing.T) {
			got, ok := resolveProvider(name)
			if !ok || got != want {
				t.Fatalf("resolveProvider(%q) = %q, %v; want %q, true", name, got, ok, want)
			}
		})
	}
}

func TestProbeProviderStatus(t *testing.T) {
	for _, tc := range []struct {
		name       string
		code       int
		wantStatus string
	}{
		{name: "valid", code: http.StatusOK, wantStatus: "VALID"},
		{name: "unauthorized", code: http.StatusUnauthorized, wantStatus: "INVALID"},
		{name: "server error", code: http.StatusInternalServerError, wantStatus: "UNREACHABLE"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.code)
			}))
			defer server.Close()

			got := probeProvider(context.Background(), server.Client(), "dragon-moonshot", "moonshot", "test-key", providerProbe{
				baseURL: server.URL,
				path:    "/models",
				auth:    bearerAuth,
			})
			if got.Status != tc.wantStatus || got.HTTPCode != tc.code {
				t.Fatalf("probeProvider() = status %q, code %d; want %q, %d", got.Status, got.HTTPCode, tc.wantStatus, tc.code)
			}
		})
	}
}

func TestProbeProviderTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()

	client := server.Client()
	client.Timeout = 20 * time.Millisecond
	got := probeProvider(context.Background(), client, "dragon-moonshot", "moonshot", "test-key", providerProbe{
		baseURL: server.URL,
		path:    "/models",
		auth:    bearerAuth,
	})
	if got.Status != "UNREACHABLE" || got.HTTPCode != 0 {
		t.Fatalf("probeProvider() = status %q, code %d; want UNREACHABLE, 0", got.Status, got.HTTPCode)
	}
}
