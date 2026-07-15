package secrets

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type probeAuth int

const (
	bearerAuth probeAuth = iota
	anthropicAuth
)

type providerProbe struct {
	baseURL string
	path    string
	auth    probeAuth
}

// providerProbes is deliberately data-only so provider endpoints and auth
// conventions stay easy to audit. Aliases are normalized by resolveProvider.
var providerProbes = map[string]providerProbe{
	"moonshot":  {baseURL: "https://api.moonshot.ai/v1", path: "/models", auth: bearerAuth},
	"deepseek":  {baseURL: "https://api.deepseek.com/v1", path: "/models", auth: bearerAuth},
	"openai":    {baseURL: "https://api.openai.com/v1", path: "/models", auth: bearerAuth},
	"zai":       {baseURL: "https://api.z.ai/api/coding/paas/v4", path: "/models", auth: bearerAuth},
	"anthropic": {baseURL: "https://api.anthropic.com/v1", path: "/models", auth: anthropicAuth},
}

var providerAliases = map[string]string{
	"kimi": "moonshot",
	"glm":  "zai",
}

type checkResult struct {
	Name     string `json:"name"`
	Provider string `json:"provider"`
	Status   string `json:"status"`
	HTTPCode int    `json:"http_code"`
}

func newCheckCmd(cfg *Config) *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "check NAME",
		Short: "Check whether a vault API key authenticates with its provider",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			name := args[0]
			provider, ok := resolveProvider(name)
			if !ok {
				return fmt.Errorf("unknown provider for %q (supported: %s)", name, strings.Join(supportedProviderNames(), ", "))
			}

			key, ok := lookupEnvironmentKey(name)
			if !ok {
				client, err := cfg.Resolve()
				if err != nil {
					return err
				}
				key, err = client.Get(name)
				if err != nil {
					return err
				}
			}

			result := probeProvider(c.Context(), &http.Client{Timeout: 10 * time.Second}, name, provider, key, providerProbes[provider])
			if jsonOutput {
				return json.NewEncoder(c.OutOrStdout()).Encode(result)
			}
			fmt.Fprintf(c.OutOrStdout(), "%s (%s): %s\n", result.Name, result.Provider, result.Status)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "emit a JSON verdict")
	return cmd
}

func lookupEnvironmentKey(name string) (string, bool) {
	if value, ok := os.LookupEnv(name); ok {
		return value, true
	}
	entry, ok := GrantAgentKey(os.Environ(), name)
	if !ok {
		return "", false
	}
	_, value, ok := strings.Cut(entry, "=")
	return value, ok
}

func resolveProvider(name string) (string, bool) {
	normalized := strings.ToLower(strings.NewReplacer("_", "-", ".", "-", "/", "-").Replace(name))
	parts := strings.FieldsFunc(normalized, func(r rune) bool { return r == '-' })
	for _, part := range parts {
		if _, ok := providerProbes[part]; ok {
			return part, true
		}
		if provider, ok := providerAliases[part]; ok {
			return provider, true
		}
	}
	return "", false
}

func supportedProviderNames() []string {
	names := make([]string, 0, len(providerProbes)+len(providerAliases))
	for name := range providerProbes {
		names = append(names, name)
	}
	for name := range providerAliases {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func probeProvider(ctx context.Context, client *http.Client, name, provider, key string, probe providerProbe) checkResult {
	result := checkResult{Name: name, Provider: provider, Status: "UNREACHABLE"}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(probe.baseURL, "/")+probe.path, nil)
	if err != nil {
		return result
	}
	if probe.auth == anthropicAuth {
		req.Header.Set("x-api-key", key)
		req.Header.Set("anthropic-version", "2023-06-01")
	} else {
		req.Header.Set("Authorization", "Bearer "+key)
	}

	resp, err := client.Do(req)
	if err != nil {
		return result
	}
	defer resp.Body.Close()
	result.HTTPCode = resp.StatusCode
	switch resp.StatusCode {
	case http.StatusOK:
		result.Status = "VALID"
	case http.StatusUnauthorized, http.StatusForbidden:
		result.Status = "INVALID"
	}
	return result
}
