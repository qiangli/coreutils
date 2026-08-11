package agentlaunch

import (
	"os"
	"path/filepath"
	"slices"
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

func TestResolveAgGeminiVariantsUsesRegistryIDsWithoutEffortFlag(t *testing.T) {
	t.Setenv(UnsafeLaunchEnv, "1")
	for _, tc := range []struct {
		name, model string
	}{
		{"agy-gemini3.5-flash-low", "gemini-3.5-flash-low"},
		{"agy-gemini3.5-flash", "gemini-3.5-flash-high"},
		{"agy-gemini3.1", "gemini-3.1-pro-high"},
	} {
		l, err := ResolveWithCatalog(tc.name, Options{}, NewCatalog)
		if err != nil {
			t.Fatalf("ResolveWithCatalog(%q): %v", tc.name, err)
		}
		want := []string{"agy", "--dangerously-skip-permissions", "--print-timeout", "40m", "--model", tc.model, "-p", "prompt"}
		if got := l.Argv("prompt"); !slices.Equal(got, want) {
			t.Errorf("argv for %s = %q, want %q", tc.name, got, want)
		}
		if slices.Contains(l.Args, "--effort") {
			t.Errorf("argv for %s unexpectedly carries --effort: %q", tc.name, l.Args)
		}
	}
}

func TestResolveCarriesSelectedCredentialNames(t *testing.T) {
	t.Setenv(UnsafeLaunchEnv, "1")
	l, err := ResolveWithCatalog("ycode:glm-5.2", Options{}, testCatalog(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(l.PreserveEnv, "ZAI_API_KEY") {
		t.Errorf("resolved launch missing selected credential name: names=%v", l.PreserveEnv)
	}
	if slices.Contains(l.PreserveEnv, "OPENAI_API_KEY") {
		t.Errorf("resolved launch widened to unrelated credential: names=%v", l.PreserveEnv)
	}
}

func TestResolveCarriesProviderCredentialForDirectAPIHarness(t *testing.T) {
	t.Setenv(UnsafeLaunchEnv, "1")
	root := t.TempDir()
	cat := fleet.New(fleet.WithRoot(root))
	if err := cat.SaveModel(fleet.Model{
		Name: "frontier", Kind: fleet.ModelKindSubscription, Provider: "openai", UpstreamID: "frontier-1",
	}); err != nil {
		t.Fatal(err)
	}
	if err := cat.SaveTool(fleet.Tool{
		Name: "direct", Kind: fleet.ToolKindCLI,
		CLI: fleet.ToolCLI{Launch: fleet.ToolLaunch{
			Exec: "direct --model {model} {prompt}", Credential: fleet.ToolCredentialModelProvider,
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := cat.SaveAgent(fleet.Agent{Name: "reviewer", Tool: "direct", Model: "frontier"}); err != nil {
		t.Fatal(err)
	}

	l, err := ResolveWithCatalog("reviewer", Options{}, testCatalog(root))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(l.PreserveEnv, "OPENAI_API_KEY") {
		t.Errorf("direct-provider launch missing OPENAI_API_KEY: names=%v", l.PreserveEnv)
	}
	if slices.Contains(l.PreserveEnv, "ANTHROPIC_API_KEY") {
		t.Errorf("direct-provider launch widened to unrelated credential: names=%v", l.PreserveEnv)
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

// --- per-run ycode store isolation ------------------------------------------

// envHas is a local lookup for the store-isolation tests.
func envHas(env []string, name string) bool {
	prefix := name + "="
	for _, kv := range env {
		if strings.HasPrefix(kv, prefix) {
			return true
		}
	}
	return false
}

// Two different run identities on the same state dir must yield two different
// stores; one identity must be stable across calls (resume reuse).
func TestYcodeDataDirDistinctPerRunAndStablePerResume(t *testing.T) {
	stateDir := t.TempDir()
	l := Launch{ToolName: YcodeToolName}
	parent := []string{"PATH=/usr/bin"}

	d7 := YcodeDataDir(parent, l, stateDir, "7")
	d8 := YcodeDataDir(parent, l, stateDir, "8")
	d7Again := YcodeDataDir(parent, l, stateDir, "7")

	if d7 == "" || d8 == "" {
		t.Fatalf("expected derived dirs, got %q %q", d7, d8)
	}
	if d7 == d8 {
		t.Fatalf("two runs share one store %q — second worker would die on the lock", d7)
	}
	if d7 != d7Again {
		t.Fatalf("resume of run 7 got a different dir: %q then %q — a resume must reuse, not orphan", d7, d7Again)
	}
	if !strings.HasPrefix(d7, stateDir) {
		t.Errorf("store %q not under state dir %q", d7, stateDir)
	}
}

// A non-ycode tool is never injected — the gate is the tool name, not a
// hardcoded agent name, so claude/codex/opencode are untouched.
func TestYcodeDataDirLeavesNonYcodeToolsAlone(t *testing.T) {
	parent := []string{"PATH=/usr/bin"}
	for _, tool := range []string{"claude", "codex", "opencode", "aider", ""} {
		l := Launch{ToolName: tool}
		if got := YcodeDataDir(parent, l, t.TempDir(), "1"); got != "" {
			t.Errorf("tool %q got a ycode data dir %q — non-ycode tools must be untouched", tool, got)
		}
	}
}

// An operator who set YCODE_DATA_DIR or YCODE_HOME already chose deliberately;
// never override it.
func TestYcodeDataDirRespectsOperatorChoice(t *testing.T) {
	l := Launch{ToolName: YcodeToolName}
	stateDir := t.TempDir()

	for _, setEnv := range [][]string{
		{"PATH=/usr/bin", YcodeDataDirEnv + "=/explicit"},
		{"PATH=/usr/bin", YcodeHomeEnv + "=/explicit/home"},
	} {
		if got := YcodeDataDir(setEnv, l, stateDir, "1"); got != "" {
			t.Errorf("operator choice %v was overridden with %q", setEnv, got)
		}
	}
}

// ApplyYcodeDataDir injects exactly one entry, and never duplicates when called
// twice (idempotent on the same env).
func TestApplyYcodeDataDirInjectsOnceAndIsIdempotent(t *testing.T) {
	l := Launch{ToolName: YcodeToolName}
	parent := []string{"PATH=/usr/bin"}
	env := []string{"PATH=/usr/bin"}

	env = ApplyYcodeDataDir(env, parent, l, t.TempDir(), "5")
	if !envHas(env, YcodeDataDirEnv) {
		t.Fatalf("expected %s injected, got %v", YcodeDataDirEnv, env)
	}
	before := len(env)
	env = ApplyYcodeDataDir(env, parent, l, t.TempDir(), "5")
	if len(env) != before {
		t.Fatalf("ApplyYcodeDataDir duplicated the entry: %v", env)
	}
}

// ApplyYcodeDataDir is a no-op for a non-ycode launch and never panics on a
// zero Launch (the nil/bare-tool path).
func TestApplyYcodeDataDirNoOpForNonYcodeAndZeroLaunch(t *testing.T) {
	parent := []string{"PATH=/usr/bin"}
	for _, l := range []Launch{{ToolName: "claude"}, {}} {
		env := ApplyYcodeDataDir([]string{"PATH=/usr/bin"}, parent, l, t.TempDir(), "9")
		if envHas(env, YcodeDataDirEnv) {
			t.Errorf("launch %+v got an unwanted %s", l, YcodeDataDirEnv)
		}
	}
}
