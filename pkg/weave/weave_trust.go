package weave

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type weaveTrustLaunch struct {
	Preseed string
	Clear   string
}

func weaveTrustLaunchFor(toolName string) weaveTrustLaunch {
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		return weaveTrustLaunch{}
	}
	t, ok := fleetCatalog().Tool(toolName)
	if !ok || !t.IsCLI() {
		return weaveTrustLaunch{}
	}
	l := t.CLI.Launch
	return weaveTrustLaunch{Preseed: l.TrustPreseed, Clear: l.TrustClear}
}

func weaveTrustClearPayload(spec string) (string, bool) {
	method, payload, ok := strings.Cut(strings.TrimSpace(spec), ":")
	if !ok || strings.TrimSpace(method) != "say" {
		return "", false
	}
	return payload, true
}

func weaveApplyTrustPreseed(workspace, preseed string) error {
	preseed = strings.TrimSpace(preseed)
	switch preseed {
	case "", "none":
		return nil
	case ".claude.json":
		return weavePreseedClaudeTrust(workspace)
	case "opencode.json":
		return os.WriteFile(filepath.Join(workspace, preseed), []byte(`{"permission":{"edit":"allow","bash":"allow","webfetch":"allow","external_directory":"allow"}}`), 0o600)
	default:
		return nil
	}
}

func weavePreseedClaudeTrust(workspace string) error {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return fmt.Errorf("empty workspace")
	}
	abs, err := filepath.Abs(workspace)
	if err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	path := filepath.Join(home, ".claude.json")
	var doc map[string]any
	if b, err := os.ReadFile(path); err == nil && len(strings.TrimSpace(string(b))) > 0 {
		if err := json.Unmarshal(b, &doc); err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
	} else {
		doc = map[string]any{}
	}
	projects, ok := doc["projects"].(map[string]any)
	if !ok {
		projects = map[string]any{}
		doc["projects"] = projects
	}
	project, ok := projects[abs].(map[string]any)
	if !ok {
		project = map[string]any{}
		projects[abs] = project
	}
	project["hasTrustDialogAccepted"] = true
	project["hasCompletedProjectOnboarding"] = true
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(path, b, 0o600)
}
