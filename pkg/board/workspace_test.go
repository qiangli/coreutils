// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package board

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWeaveSourceCarriesWorkspaceAndMeasuresItsDiskUsage(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := filepath.Join(home, "repo")
	workspace := filepath.Join(home, "workspace")
	queue := filepath.Join(home, ".bashy", "weave", "fixture")
	for _, dir := range []string{repo, workspace, queue} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(workspace, "payload"), make([]byte, 777), 0o644); err != nil {
		t.Fatal(err)
	}
	record := map[string]any{"root": repo, "items": []map[string]any{{
		"id": 7, "title": "workspace fixture", "state": "todo", "workspace": workspace,
	}}}
	raw, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(queue, "queue.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}

	b := &Board{}
	if err := (weaveSource{}).Load(context.Background(), b, Options{All: true}); err != nil {
		t.Fatalf("load weave source: %v", err)
	}
	if len(b.Runs) != 1 {
		t.Fatalf("runs = %+v", b.Runs)
	}
	if got := b.Runs[0]; got.Workspace != workspace || got.WorkspaceDiskBytes != 777 || got.WorkspaceDiskError != "" {
		t.Fatalf("workspace projection = %+v", got)
	}
}

func TestWorkspaceDiskUsageAndPanel(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "one"), make([]byte, 1024), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "nested", "two"), make([]byte, 2048), 0o644); err != nil {
		t.Fatal(err)
	}

	got, detail := workspaceDiskUsage(context.Background(), root)
	if got != 3072 || detail != "" {
		t.Fatalf("workspaceDiskUsage = %d, %q; want 3072, empty", got, detail)
	}
	_, missing := workspaceDiskUsage(context.Background(), filepath.Join(root, "missing"))
	if missing == "" {
		t.Fatal("missing workspace reported a valid zero-byte footprint")
	}

	b := &Board{Runs: []Run{
		{ID: 1, State: "working", Repo: "/repo/a", Workspace: "/work/small", WorkspaceDiskBytes: 1024},
		{ID: 2, State: "done", Repo: "/repo/b", Workspace: "/work/large", WorkspaceDiskBytes: 2048},
		{ID: 3, State: "failed", Repo: "/repo/c", Workspace: "/work/missing", WorkspaceDiskError: "not found"},
	}}
	v := workspacePanel().Build(b)
	if v.ID != "workspaces" || len(v.Rows) != 3 {
		t.Fatalf("workspace panel = %+v", v)
	}
	if v.Rows[0][0] != "#2" || v.Rows[0][2] != "2.0KiB" {
		t.Errorf("largest workspace is not first: %v", v.Rows[0])
	}
	if !strings.Contains(v.Collapsed, "3.0KiB on disk") || !strings.Contains(v.Collapsed, "1 unavailable") {
		t.Errorf("collapsed workspace summary = %q", v.Collapsed)
	}
	if !strings.HasPrefix(v.Rows[2][2], "unavailable:") {
		t.Errorf("failed measurement is silent: %v", v.Rows[2])
	}
}

func TestWorkspacePanelFollowsRunsAndPrecedesUtilization(t *testing.T) {
	b := &Board{}
	views := DefaultPanels().Build(b)
	positions := map[string]int{}
	for i, view := range views {
		positions[view.ID] = i
	}
	if positions["workspaces"] != positions["runs"]+1 {
		t.Errorf("panel order runs=%d workspaces=%d", positions["runs"], positions["workspaces"])
	}
	if positions["workspaces"] >= positions["utilization"] {
		t.Errorf("workspaces panel %d does not precede utilization %d", positions["workspaces"], positions["utilization"])
	}
}
