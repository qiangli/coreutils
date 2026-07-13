package agentctl

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// The first thing several agent CLIs do in a directory they have not seen before
// is stop and ask whether they may read it. On a human's terminal that is a
// reasonable question. To an unattended launcher it is a wall: the agent produces
// nothing, exits nothing, and eventually trips an idle timeout that reports the
// wrong cause entirely.
//
// There are two ways past it, and a fleet wants both.
//
// PREVENTION (this file): write the tool's own config so the prompt never
// appears. Quiet, instant, and it works even without a terminal.
//
// CURE (pkg/agentpty's gate classifier): watch the live output, recognise the
// prompt, and press the key. Needed because a preseed cannot cover every prompt
// every tool will ever invent, and because a config file can be stale.
//
// Prevention is tried first because a prompt that never appears cannot be
// mis-answered.

// ApplyTrustPreseed writes whatever config suppresses a tool's first-run trust
// prompt for this workspace. An unknown preseed is a no-op, not an error: a tool
// we do not have a recipe for is one whose prompt we will clear reactively
// instead, and failing the launch would be strictly worse than trying.
func ApplyTrustPreseed(workspace, preseed string) error {
	switch strings.TrimSpace(preseed) {
	case "", "none":
		return nil
	case ".claude.json":
		return preseedClaudeTrust(workspace)
	case "opencode.json":
		return os.WriteFile(filepath.Join(workspace, "opencode.json"),
			[]byte(`{"permission":{"edit":"allow","bash":"allow","webfetch":"allow","external_directory":"allow"}}`), 0o600)
	default:
		return nil
	}
}

// preseedClaudeTrust marks one workspace as already-trusted in ~/.claude.json.
//
// It MERGES into the existing document rather than writing a fresh one. That file
// is the user's real claude config — their projects, their onboarding state — and
// clobbering it to skip a prompt would be a spectacular trade.
func preseedClaudeTrust(workspace string) error {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return fmt.Errorf("agentctl: empty workspace")
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
	return os.WriteFile(path, append(b, '\n'), 0o600)
}
