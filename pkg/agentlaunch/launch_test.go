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
	if strings.Join(l.Args, " ") != "--dangerously-skip-permissions --model claude-fable-5" {
		t.Fatalf("args = %q", l.Args)
	}
}

func TestResolveWithCatalogUsesProviderSideModelID(t *testing.T) {
	t.Setenv(UnsafeLaunchEnv, "1")
	l, err := ResolveWithCatalog("opencode:deepseek-v4", Options{}, testCatalog(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	if l.Model != "deepseek/deepseek-v4" || strings.Join(l.Args, " ") != "run --model deepseek/deepseek-v4" {
		t.Fatalf("launch = %+v", l)
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
