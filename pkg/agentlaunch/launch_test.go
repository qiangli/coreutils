package agentlaunch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/pkg/fleet"
)

func testCatalog(root string) CatalogFunc {
	return func() *fleet.Catalog { return fleet.New(fleet.WithRoot(root)) }
}

func TestResolveWithCatalogRendersNicknameFromFleetTemplate(t *testing.T) {
	t.Setenv(UnsafeLaunchEnv, "1")
	root := t.TempDir()
	cat := fleet.New(fleet.WithRoot(root))
	if err := cat.SaveAgent(fleet.Agent{Name: "007", Tool: "claude", Model: "fable"}); err != nil {
		t.Fatal(err)
	}

	l, err := ResolveWithCatalog("007", Options{}, testCatalog(root))
	if err != nil {
		t.Fatal(err)
	}
	if l.Tool != "claude" || l.Nick != "007" || l.Binding() != "claude:fable5" {
		t.Fatalf("launch = %+v", l)
	}
	if strings.Join(l.Args, " ") != "--dangerously-skip-permissions --model claude-fable-5 -p" {
		t.Fatalf("args = %q", l.Args)
	}
}

func TestResolveWithCatalogUsesProviderSideModelID(t *testing.T) {
	t.Setenv(UnsafeLaunchEnv, "1")
	l, err := ResolveWithCatalog("opencode:deepseek-v4-pro", Options{}, testCatalog(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	if l.Model != "deepseek/deepseek-v4-pro" || strings.Join(l.Args, " ") != "run --model deepseek/deepseek-v4-pro" {
		t.Fatalf("launch = %+v", l)
	}
}

func TestResolveCarriesWorkspaceBindingAndPreflight(t *testing.T) {
	t.Setenv(UnsafeLaunchEnv, "1")
	root := t.TempDir()
	cat := fleet.New(fleet.WithRoot(root))
	tool := fleet.Tool{
		Name: "runner", Kind: fleet.ToolKindCLI,
		CLI: fleet.ToolCLI{Launch: fleet.ToolLaunch{
			Exec:                   "runner --unsafe --model {model} -p {prompt}",
			WorkspaceArg:           "--project {workspace}",
			WorkspacePreflightExec: "runner --mode plan --model {model} -p {prompt}",
		}},
	}
	if err := cat.SaveTool(tool); err != nil {
		t.Fatal(err)
	}
	if err := cat.SaveAgent(fleet.Agent{Name: "worker", Tool: "runner", Model: "fable"}); err != nil {
		t.Fatal(err)
	}
	l, err := ResolveWithCatalog("worker", Options{Workspace: fleet.WorkspaceToken}, testCatalog(root))
	if err != nil {
		t.Fatal(err)
	}
	argv := l.Argv("write source")
	if got := strings.Join(argv, " "); !strings.Contains(got, "runner --project {workspace} --unsafe --model") {
		t.Fatalf("worker argv = %q", got)
	}
	if got := strings.Join(l.WorkspacePreflight, " "); !strings.Contains(got, "runner --project {workspace} --mode plan --model") || !strings.Contains(got, "PWD=<absolute-path>") {
		t.Fatalf("preflight argv = %q", got)
	}
	bound, err := RenderWorkspace(argv, "/tmp/allocated work")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(bound, "\x00"); !strings.Contains(got, "--project\x00/tmp/allocated work\x00--unsafe") {
		t.Fatalf("bound argv = %q", bound)
	}
}

func TestRenderWorkspaceFailsClosedOnEmptyPath(t *testing.T) {
	if _, err := RenderWorkspace([]string{"runner", fleet.WorkspaceToken}, ""); err == nil {
		t.Fatal("empty allocated workspace must be refused")
	}
}

func TestRenderWorkspaceReplacesEmbeddedWorkspaceToken(t *testing.T) {
	got, err := RenderWorkspace([]string{"ycode", "--session-dir", "{workspace}/.git/ycode-sessions"}, "/tmp/allocated")
	if err != nil {
		t.Fatal(err)
	}
	if got[2] != "/tmp/allocated/.git/ycode-sessions" {
		t.Fatalf("embedded workspace = %q", got[2])
	}
}

func TestPrincipalEnvStampsOnlyNamedAgents(t *testing.T) {
	base := []string{"PATH=/bin", "BASHY_AGENT_ID=old"}
	named := PrincipalEnv(base, Launch{Nick: "007", Tool: "claude", ToolName: "claude", ModelName: "fable"})
	if !hasKV(named, "BASHY_PRINCIPAL=dhnt:agent/007") || !hasKV(named, "BASHY_AGENT_BINDING=claude:fable") {
		t.Fatalf("named env = %q", named)
	}
	raw := PrincipalEnv(base, Launch{Nick: "claude:opus", Tool: "claude", ToolName: "claude", ModelName: "opus"})
	if len(raw) != len(base) {
		t.Fatalf("raw binding env = %q", raw)
	}
}

func TestSendControlFrameFallsBackToRegularFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ctl")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := SendControlFrame(path, "hello\n"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "hello\n" {
		t.Fatalf("control file = %q", b)
	}
}

func hasKV(env []string, want string) bool {
	for _, kv := range env {
		if kv == want {
			return true
		}
	}
	return false
}
