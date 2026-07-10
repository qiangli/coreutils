package chat

import (
	"context"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/pkg/fleet"
)

// pinCatalog points the launcher at the compiled-in baseline only, so a
// developer's own ~/.config/bashy store cannot change what these tests see.
func pinCatalog(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	prev := newCatalog
	newCatalog = func() *fleet.Catalog { return fleet.New(fleet.WithRoot(root)) }
	t.Cleanup(func() { newCatalog = prev })
}

func argv(t *testing.T, name string, opt Options) (string, []string, string) {
	t.Helper()
	l, err := resolveLaunch(name, opt)
	if err != nil {
		t.Fatalf("resolveLaunch(%q): %v", name, err)
	}
	return l.Tool, l.Args, l.Model
}

// THE GOLDEN TEST. Routing launch contracts through the registry must not move
// a single argument for a bare tool name. These are the exact arg lists the
// hardcoded seededProfiles table produced before the registry existed.
func TestBareToolArgvIsUnchangedFromTheLegacyTable(t *testing.T) {
	pinCatalog(t)
	for name, want := range seededProfiles {
		tool, args, model := argv(t, name, Options{})
		if tool != name {
			t.Errorf("%s: tool = %q", name, tool)
		}
		if model != "" {
			t.Errorf("%s: a bare tool name selects no model, got %q", name, model)
		}
		if strings.Join(args, "\x00") != strings.Join(want.Args, "\x00") {
			t.Errorf("%s: args =\n  %q\nwant (legacy table)\n  %q", name, args, want.Args)
		}
	}
}

// The whole point of P2: a binding actually reaches the tool's --model flag.
func TestBindingPassesModel(t *testing.T) {
	pinCatalog(t)

	tool, args, model := argv(t, "claude:opus", Options{})
	if tool != "claude" || model != "opus" {
		t.Fatalf("tool=%q model=%q", tool, model)
	}
	if strings.Join(args, " ") != "--dangerously-skip-permissions --model opus" {
		t.Fatalf("args = %q", args)
	}
}

// The id handed to --model is the PROVIDER's, not our alias: opencode wants
// `deepseek/deepseek-v4`. Passing the alias would name a model that does not
// exist upstream.
func TestModelIsTheProviderSideID(t *testing.T) {
	pinCatalog(t)
	tool, args, model := argv(t, "opencode:deepseek-v4", Options{})
	if tool != "opencode" || model != "deepseek/deepseek-v4" {
		t.Fatalf("tool=%q model=%q", tool, model)
	}
	if strings.Join(args, " ") != "run --model deepseek/deepseek-v4" {
		t.Fatalf("args = %q", args)
	}
}

// A nickname resolves to its binding, and so do its aliases.
func TestNicknameAndAliasSelectTheSameModel(t *testing.T) {
	root := t.TempDir()
	cat := fleet.New(fleet.WithRoot(root))
	if err := cat.SaveAgent(fleet.Agent{
		Name: "007", Aliases: []string{"smarty"}, Tool: "claude", Model: "fable",
	}); err != nil {
		t.Fatal(err)
	}
	prev := newCatalog
	newCatalog = func() *fleet.Catalog { return fleet.New(fleet.WithRoot(root)) }
	t.Cleanup(func() { newCatalog = prev })

	for _, nick := range []string{"007", "smarty"} {
		tool, args, model := argv(t, nick, Options{})
		if tool != "claude" || model != "fable" {
			t.Fatalf("%s: tool=%q model=%q", nick, tool, model)
		}
		if !contains(args, "--model") || !contains(args, "fable") {
			t.Fatalf("%s: args = %q", nick, args)
		}
	}
}

// Binding a model to a tool that cannot select one is a label, not a
// selection. Silently dropping it is how `claude:opus` used to run as
// whatever the tool's config happened to name.
func TestBindingToAToolThatCannotSelectAModelIsAnError(t *testing.T) {
	root := t.TempDir()
	cat := fleet.New(fleet.WithRoot(root))
	if err := cat.SaveTool(fleet.Tool{
		Name: "dumb", Kind: fleet.ToolKindCLI,
		CLI: fleet.ToolCLI{Binary: "dumb", Launch: fleet.ToolLaunch{Exec: "dumb {prompt}"}},
	}); err != nil {
		t.Fatal(err)
	}
	prev := newCatalog
	newCatalog = func() *fleet.Catalog { return fleet.New(fleet.WithRoot(root)) }
	t.Cleanup(func() { newCatalog = prev })

	_, err := resolveLaunch("dumb:opus", Options{})
	if err == nil || !strings.Contains(err.Error(), "label, not a selection") {
		t.Fatalf("err = %v", err)
	}
	// Without a model it launches fine.
	if _, err := resolveLaunch("dumb", Options{}); err != nil {
		t.Fatalf("bare tool must still launch: %v", err)
	}
}

// A tool the registry has never heard of still launches, and still cannot
// be handed a model.
func TestUnregisteredToolFallsBackAndRefusesAModel(t *testing.T) {
	pinCatalog(t)
	tool, args, model := argv(t, "my-own-agent", Options{})
	if tool != "my-own-agent" || len(args) != 0 || model != "" {
		t.Fatalf("tool=%q args=%q model=%q", tool, args, model)
	}
	_, err := resolveLaunch("my-own-agent:opus", Options{})
	if err == nil || !strings.Contains(err.Error(), "no launch template") {
		t.Fatalf("err = %v", err)
	}
}

// An unregistered MODEL passes through verbatim rather than being dropped.
func TestUnregisteredModelPassesThroughVerbatim(t *testing.T) {
	pinCatalog(t)
	_, args, model := argv(t, "claude:some-future-model", Options{})
	if model != "some-future-model" {
		t.Fatalf("model = %q", model)
	}
	if !contains(args, "some-future-model") {
		t.Fatalf("args = %q", args)
	}
}

// --- the codex sandbox contract, preserved -------------------------------

func TestCodexSandboxOverrideStillApplies(t *testing.T) {
	pinCatalog(t)

	_, args, _ := argv(t, "codex", Options{Sandbox: "read-only"})
	if !adjacent(args, "--sandbox", "read-only") {
		t.Fatalf("sandbox override lost: %q", args)
	}

	// danger-full-access maps to the fully non-interactive flag, and the
	// --sandbox pair is dropped entirely.
	_, args, _ = argv(t, "codex", Options{Sandbox: "danger-full-access"})
	if contains(args, "--sandbox") {
		t.Fatalf("--sandbox survived danger-full-access: %q", args)
	}
	if !contains(args, "--dangerously-bypass-approvals-and-sandbox") {
		t.Fatalf("bypass flag missing: %q", args)
	}
}

// The sandbox override and a model selection must coexist: the override
// rewrites a flag, it does not rebuild the argv.
func TestCodexSandboxOverrideCoexistsWithModel(t *testing.T) {
	pinCatalog(t)
	_, args, model := argv(t, "codex:gpt-5.5", Options{Sandbox: "read-only"})
	if model != "gpt-5.5" {
		t.Fatalf("model = %q", model)
	}
	if !adjacent(args, "--sandbox", "read-only") || !adjacent(args, "--model", "gpt-5.5") {
		t.Fatalf("args = %q", args)
	}

	_, args, _ = argv(t, "codex:gpt-5.5", Options{Sandbox: "danger-full-access"})
	if contains(args, "--sandbox") || !adjacent(args, "--model", "gpt-5.5") {
		t.Fatalf("args = %q", args)
	}
}

// --- the prompt stays last ------------------------------------------------

// aider takes its prompt as the value of --message. If the launcher ever
// stopped appending the prompt last, the task text would become the value of
// whatever flag happened to end the template.
func TestPromptRemainsTheFinalArgument(t *testing.T) {
	pinCatalog(t)
	for _, name := range []string{"claude", "codex", "opencode", "aider", "agy", "claude:opus"} {
		l, err := resolveLaunch(name, Options{})
		if err != nil {
			t.Fatal(err)
		}
		full := append(l.Args, "THE PROMPT")
		if full[len(full)-1] != "THE PROMPT" {
			t.Fatalf("%s: prompt is not last: %q", name, full)
		}
	}
	// aider specifically: the prompt must land right after --message.
	l, _ := resolveLaunch("aider", Options{})
	full := append(l.Args, "THE PROMPT")
	if !adjacent(full, "--message", "THE PROMPT") {
		t.Fatalf("aider: %q", full)
	}
}

// --- identity injection ----------------------------------------------------

// The launcher stamps the child with the nickname it is acting as. A bare
// tool gets nothing: a tool is not an agent, and a fabricated nickname would
// put a name in the record that resolves to nothing.
func TestPrincipalEnvStampedOnlyForANamedAgent(t *testing.T) {
	base := []string{"PATH=/bin", "HOME=/home/you"}

	got := principalEnv(base, Launch{Nick: "007", Tool: "claude", ToolName: "claude", Model: "fable", ModelName: "fable"})
	if !hasEnv(got, "BASHY_PRINCIPAL=dhnt:agent/007") ||
		!hasEnv(got, "BASHY_AGENT_ID=007") ||
		!hasEnv(got, "BASHY_AGENT_BINDING=claude:fable") {
		t.Fatalf("env = %q", got)
	}

	same := principalEnv(base, Launch{Nick: "claude", Tool: "claude", ToolName: "claude"})
	if len(same) != len(base) {
		t.Fatalf("a bare tool must not be stamped as an agent: %q", same)
	}

	// A raw binding is not a name a mention can carry, so it is not stamped.
	raw := principalEnv(base, Launch{Nick: "claude:opus", Tool: "claude", ToolName: "claude", Model: "opus", ModelName: "opus"})
	if len(raw) != len(base) {
		t.Fatalf("an un-nicknamed binding must not be stamped: %q", raw)
	}
}

// An un-nicknamed binding still launches with the right model; it just has no
// principal identity to sign with.
func TestUnNicknamedBindingKeepsItsRawName(t *testing.T) {
	root := t.TempDir()
	prev := newCatalog
	newCatalog = func() *fleet.Catalog { return fleet.New(fleet.WithRoot(root), fleet.WithoutLocalStore()) }
	t.Cleanup(func() { newCatalog = prev })

	// aider:opus is a legal binding with no seeded agent behind it.
	l, err := resolveLaunch("aider:opus", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if l.Nick != "aider:opus" || l.Tool != "aider" || l.Model != "opus" {
		t.Fatalf("launch = %+v", l)
	}
}

// A nested launch must not inherit its parent's identity.
func TestPrincipalEnvOverwritesAnInheritedIdentity(t *testing.T) {
	base := []string{"BASHY_PRINCIPAL=dhnt:agent/old", "BASHY_AGENT_ID=old", "PATH=/bin"}
	got := principalEnv(base, Launch{Nick: "007", Tool: "claude", ToolName: "claude", Model: "fable", ModelName: "fable"})
	if hasEnv(got, "BASHY_AGENT_ID=old") || hasEnv(got, "BASHY_PRINCIPAL=dhnt:agent/old") {
		t.Fatalf("stale identity survived: %q", got)
	}
	if !hasEnv(got, "BASHY_AGENT_ID=007") {
		t.Fatalf("env = %q", got)
	}
}

// Invoke threads the resolved launch to the runner without widening the
// Runner interface that meet, foreman, and sdlc implement against.
func TestInvokeCarriesTheLaunchToTheRunner(t *testing.T) {
	pinCatalog(t)
	var seen Launch
	var ok bool
	r := runnerFunc(func(ctx context.Context, agent string, args []string, cwd string) (string, int, error) {
		seen, ok = LaunchFrom(ctx)
		return "", 0, nil
	})
	if _, err := Invoke(context.Background(), Options{Agent: "claude:opus", Instruction: "hi"}, r); err != nil {
		t.Fatal(err)
	}
	if !ok || seen.Tool != "claude" || seen.Model != "opus" {
		t.Fatalf("launch = %+v ok=%v", seen, ok)
	}
}

// The result envelope records what was asked for and what was selected.
func TestResultRecordsNickAndModel(t *testing.T) {
	pinCatalog(t)
	res, err := Invoke(context.Background(), Options{Agent: "claude:opus", Instruction: "hi", DryRun: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	// The registry seeds an agent for this binding, so the CANONICAL nickname
	// is what gets recorded — a name that `whois` and `@mentions` can resolve.
	if res.Agent != "claude" || res.Nick != "claude-opus" || res.Model != "opus" {
		t.Fatalf("res = %+v", res)
	}
	// Agent stays the executable, so the dry-run line is still runnable.
	if !strings.HasPrefix(res.Output, "claude --dangerously-skip-permissions --model opus ") {
		t.Fatalf("dry run = %q", res.Output)
	}
}

// A bare tool leaves Nick and Model empty, so nothing downstream changes.
func TestResultUnchangedForABareTool(t *testing.T) {
	pinCatalog(t)
	res, err := Invoke(context.Background(), Options{Agent: "codex", Instruction: "hi", DryRun: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Agent != "codex" || res.Nick != "" || res.Model != "" {
		t.Fatalf("res = %+v", res)
	}
}

// --- helpers ---------------------------------------------------------------

type runnerFunc func(context.Context, string, []string, string) (string, int, error)

func (f runnerFunc) Run(ctx context.Context, a string, args []string, cwd string) (string, int, error) {
	return f(ctx, a, args, cwd)
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

func adjacent(ss []string, a, b string) bool {
	for i := 0; i+1 < len(ss); i++ {
		if ss[i] == a && ss[i+1] == b {
			return true
		}
	}
	return false
}

func hasEnv(env []string, kv string) bool { return contains(env, kv) }

// The binding recorded for attribution must be the registry's tool:model, not
// the executable path. A binding written with a path would never match the
// capability matrix, whose rows are keyed by tool name.
func TestBindingUsesRegistryNamesNotThePath(t *testing.T) {
	root := t.TempDir()
	cat := fleet.New(fleet.WithRoot(root))
	if err := cat.SaveTool(fleet.Tool{
		Name: "echoer", Kind: fleet.ToolKindCLI,
		CLI: fleet.ToolCLI{
			Binary: "/opt/bin/echoer-v2",
			Launch: fleet.ToolLaunch{Exec: "echoer --model {model} {prompt}"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := cat.SaveAgent(fleet.Agent{Name: "007", Tool: "echoer", Model: "deepseek-v4"}); err != nil {
		t.Fatal(err)
	}
	prev := newCatalog
	newCatalog = func() *fleet.Catalog { return fleet.New(fleet.WithRoot(root)) }
	t.Cleanup(func() { newCatalog = prev })

	l, err := resolveLaunch("007", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if l.Tool != "/opt/bin/echoer-v2" {
		t.Fatalf("the executable is the declared binary: %q", l.Tool)
	}
	if l.Binding() != "echoer:deepseek-v4" {
		t.Fatalf("Binding() = %q, want the registry names", l.Binding())
	}
	// The provider-side id is what reaches --model.
	if l.Model != "deepseek/deepseek-v4" {
		t.Fatalf("Model = %q", l.Model)
	}
	env := principalEnv(nil, l)
	if !hasEnv(env, "BASHY_AGENT_BINDING=echoer:deepseek-v4") {
		t.Fatalf("env = %q", env)
	}
}

// A tool whose binary differs from its name is still a bare tool. Stamping it
// as an agent would invent a principal that resolves to nothing.
func TestBareToolWithADifferentBinaryIsNotStamped(t *testing.T) {
	root := t.TempDir()
	cat := fleet.New(fleet.WithRoot(root))
	if err := cat.SaveTool(fleet.Tool{
		Name: "cursor", Kind: fleet.ToolKindCLI,
		CLI: fleet.ToolCLI{Binary: "cursor-agent", Launch: fleet.ToolLaunch{Exec: "cursor-agent {prompt}"}},
	}); err != nil {
		t.Fatal(err)
	}
	prev := newCatalog
	newCatalog = func() *fleet.Catalog { return fleet.New(fleet.WithRoot(root)) }
	t.Cleanup(func() { newCatalog = prev })

	l, err := resolveLaunch("cursor", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if l.Tool != "cursor-agent" || l.ToolName != "cursor" {
		t.Fatalf("launch = %+v", l)
	}
	base := []string{"PATH=/bin"}
	if got := principalEnv(base, l); len(got) != len(base) {
		t.Fatalf("a bare tool must not be stamped as an agent: %q", got)
	}
}
