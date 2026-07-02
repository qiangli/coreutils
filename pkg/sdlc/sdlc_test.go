package sdlc

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sdlc.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func clearAgentDetectionEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"BASHY_SDLC_CONDUCTOR_AGENT",
		"BASHY_CONDUCTOR_AGENT",
		"BASHY_AGENT_TOOL",
		"BASHY_AGENT_NAME",
		"BASHY_AGENT",
		"AGENT_TOOL",
		"AGENT_NAME",
		"AI_AGENT",
		"CLAUDECODE",
		"CLAUDE_CODE",
		"CLAUDE_SESSION_ID",
		"CLAUDE_CONFIG_DIR",
		"OPENCODE",
		"OPENCODE_SESSION_ID",
		"OPENCODE_CONFIG_DIR",
		"AGY",
		"AGY_SESSION_ID",
		"ANTIGRAVITY",
		"CODEX_SANDBOX",
		"CODEX_HOME",
		"CODEX_SESSION_ID",
		"CODEX_CLI",
	} {
		t.Setenv(key, "")
	}
}

func TestLoadConfigValidatesBoundary(t *testing.T) {
	path := writeConfig(t, `
conductor:
  agent: claude
reviewer:
  agent: codex
qa:
  agent: codex
intake:
  provider: github
  repository: owner/repo
deployment:
  staging:
    name: staging
    healthcheck: https://staging.example.test/health
  production:
    name: production
`)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Conductor.Agent != "claude" || cfg.Intake.Provider != "github" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestLoadConfigOrDefaultUsesCodexLocalFallback(t *testing.T) {
	clearAgentDetectionEnv(t)
	cfg, usedDefault, err := LoadConfigOrDefault(filepath.Join(t.TempDir(), "missing.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !usedDefault {
		t.Fatal("expected embedded default config")
	}
	if cfg.Conductor.Agent != "codex" || cfg.Intake.Provider != "local" {
		t.Fatalf("unexpected default config: %+v", cfg)
	}
}

func TestDefaultConductorAgentUsesExplicitOverride(t *testing.T) {
	clearAgentDetectionEnv(t)
	t.Setenv("BASHY_AGENT", "claude")
	if got := DefaultConductorAgent(); got != "claude" {
		t.Fatalf("DefaultConductorAgent=%q, want claude", got)
	}
}

func TestDefaultConductorAgentDetectsOpenCode(t *testing.T) {
	clearAgentDetectionEnv(t)
	t.Setenv("OPENCODE_SESSION_ID", "session-1")
	if got := DefaultConductorAgent(); got != "opencode" {
		t.Fatalf("DefaultConductorAgent=%q, want opencode", got)
	}
}

func TestDefaultConfigUsesDetectedConductor(t *testing.T) {
	clearAgentDetectionEnv(t)
	t.Setenv("BASHY_SDLC_CONDUCTOR_AGENT", "agy")
	cfg := DefaultConfig()
	if cfg.Conductor.Agent != "agy" {
		t.Fatalf("default conductor=%q, want agy", cfg.Conductor.Agent)
	}
}

func TestLoadConfigStaysStrict(t *testing.T) {
	_, err := LoadConfig(filepath.Join(t.TempDir(), "missing.yaml"))
	if err == nil {
		t.Fatal("expected missing config error")
	}
}

func TestCommandSurfaceIncludesTriggerEntrypoint(t *testing.T) {
	cmd := NewSDLCCmd()
	have := map[string]bool{}
	for _, c := range cmd.Commands() {
		have[c.Name()] = true
	}
	for _, name := range []string{"guide", "init", "doctor", "config", "status", "issue", "brief", "delegate", "tick", "verify", "deploy-status", "guard"} {
		if !have[name] {
			t.Fatalf("missing subcommand %q", name)
		}
	}
}

func TestGuideIsSelfContainedRuntimeHelp(t *testing.T) {
	cmd := NewSDLCCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"guide"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{
		"bashy sdlc --issue",
		"--issue-file",
		".bashy/sdlc.yaml",
		"danger-full-access",
		"bashy web inspect",
		"deploy-status",
		"Production gate",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("guide missing %q:\n%s", want, got)
		}
	}
}

func TestExplainConfigReportsEmbeddedDefault(t *testing.T) {
	clearAgentDetectionEnv(t)
	exp, err := ExplainConfig(filepath.Join(t.TempDir(), "missing.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if exp.Source != "embedded-default" || exp.Intake != "local" || exp.Conductor != "codex" {
		t.Fatalf("unexpected explanation: %+v", exp)
	}
}

func TestApplyConfigOverridesCoversRoutingFields(t *testing.T) {
	cfg := DefaultConfig()
	cfg = ApplyConfigOverrides(cfg, ConfigOverrides{
		ConductorAgent:        "claude",
		ReviewerAgent:         "codex",
		QAAgent:               "opencode",
		IntakeProvider:        "github",
		IntakeRepo:            "owner/repo",
		IntakeQuery:           "is:issue is:open",
		IntakeLabels:          []string{" bug ", "", "release"},
		StagingName:           "stage",
		StagingHost:           "stage.example.test",
		StagingEnvironment:    "staging",
		StagingCommand:        "deploy stage",
		StagingHealthcheck:    "https://stage.example.test/health",
		StagingRollback:       "rollback stage",
		ProductionName:        "prod",
		ProductionHost:        "example.test",
		ProductionEnvironment: "production",
		ProductionCommand:     "deploy prod",
		ProductionHealthcheck: "https://example.test/health",
		ProductionRollback:    "rollback prod",
		Metadata:              []string{"team=platform", "ignored", "risk=low"},
		Policies:              []string{"prod_approval=required"},
		Agents:                []string{"security=codex", "docs=opencode"},
	})
	if cfg.Conductor.Agent != "claude" || cfg.Reviewer.Agent != "codex" || cfg.QA.Agent != "opencode" {
		t.Fatalf("agent overrides not applied: %+v", cfg)
	}
	if cfg.Intake.Provider != "github" || cfg.Intake.Repository != "owner/repo" || cfg.Intake.Query != "is:issue is:open" {
		t.Fatalf("intake overrides not applied: %+v", cfg.Intake)
	}
	if got := strings.Join(cfg.Intake.Labels, ","); got != "bug,release" {
		t.Fatalf("labels=%q", got)
	}
	if cfg.Deploy.Staging.Name != "stage" || cfg.Deploy.Staging.Healthcheck == "" || cfg.Deploy.Staging.Rollback == "" {
		t.Fatalf("staging overrides not applied: %+v", cfg.Deploy.Staging)
	}
	if cfg.Deploy.Production.Name != "prod" || cfg.Deploy.Production.Healthcheck == "" || cfg.Deploy.Production.Rollback == "" {
		t.Fatalf("production overrides not applied: %+v", cfg.Deploy.Production)
	}
	if cfg.Metadata["team"] != "platform" || cfg.Metadata["risk"] != "low" {
		t.Fatalf("metadata overrides not applied: %+v", cfg.Metadata)
	}
	if cfg.Policies["prod_approval"] != "required" {
		t.Fatalf("policy overrides not applied: %+v", cfg.Policies)
	}
	if cfg.Agents["security"].Agent != "codex" || cfg.Agents["docs"].Agent != "opencode" {
		t.Fatalf("agent map overrides not applied: %+v", cfg.Agents)
	}
}

func TestPrepareNoConfigUsesCLIOverrides(t *testing.T) {
	path := writeConfig(t, `
conductor:
  agent: claude
intake:
  provider: github
  repository: from/file
deployment:
  staging:
    name: file-staging
  production:
    name: file-production
`)
	res, err := Prepare(context.Background(), DelegateOptions{
		ConfigPath: path,
		Config: ConfigOverrides{
			NoConfig:        true,
			ConductorAgent:  "codex",
			ReviewerAgent:   "opencode",
			QAAgent:         "agy",
			IntakeProvider:  "jira",
			IntakeRepo:      "TEAM/PROJ",
			StagingName:     "cli-staging",
			ProductionName:  "cli-production",
			StagingHost:     "staging.example.test",
			ProductionHost:  "example.test",
			IntakeLabels:    []string{"uat"},
			StagingRollback: "rollback staging",
		},
		Issue: Issue{Title: "Use CLI config"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Review agent: opencode",
		"QA agent: agy",
		"Intake provider: jira (TEAM/PROJ)",
		"Staging target: cli-staging",
		"Production target: cli-production",
	} {
		if !strings.Contains(res.Brief, want) {
			t.Fatalf("brief missing %q:\n%s", want, res.Brief)
		}
	}
	if strings.Contains(res.Brief, "from/file") || strings.Contains(res.Brief, "file-staging") {
		t.Fatalf("brief should ignore file config under --no-config:\n%s", res.Brief)
	}
	if !res.DefaultConfig || res.Conductor != "codex" {
		t.Fatalf("unexpected prepare result: %+v", res)
	}
}

func TestSaveLocalIssue(t *testing.T) {
	issue, path, err := SaveLocalIssue("Add status command\n\nDetails", "", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if issue.Title != "Add status command" || !strings.Contains(issue.Body, "Details") {
		t.Fatalf("unexpected issue: %+v", issue)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "Add status command") {
		t.Fatalf("saved issue missing content: %s", data)
	}
}

func TestVerifyContentFilePresentAbsent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "page.html")
	if err := os.WriteFile(path, []byte("<h1>Cloudbox</h1><h2>Startups</h2>"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := VerifyContent(context.Background(), VerifyOptions{
		Target:  path,
		Present: []string{"Cloudbox"},
		Absent:  []string{"Miscellaneous"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "ok" || len(res.Checks) != 2 {
		t.Fatalf("unexpected verify result: %+v", res)
	}
}

func TestInitWritesValidConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".bashy", "sdlc.yaml")
	cmd := NewSDLCCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"init", "--config", path})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "wrote") {
		t.Fatalf("missing init output: %q", out.String())
	}
	if _, err := LoadConfig(path); err != nil {
		t.Fatal(err)
	}
}

func TestApplyIssueRequestFromInlineText(t *testing.T) {
	opt := DelegateOptions{IssueText: "Add local issue intake"}
	if err := ApplyIssueRequest(&opt); err != nil {
		t.Fatal(err)
	}
	if opt.Issue.Title != "Add local issue intake" {
		t.Fatalf("title=%q", opt.Issue.Title)
	}
	if opt.Issue.Body != "" {
		t.Fatalf("single-line issue should not duplicate body, got %q", opt.Issue.Body)
	}
}

func TestApplyIssueRequestFromFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "issue.md")
	if err := os.WriteFile(path, []byte("Add local issue intake\n\nDetails go here.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	opt := DelegateOptions{IssueFile: path}
	if err := ApplyIssueRequest(&opt); err != nil {
		t.Fatal(err)
	}
	if opt.Issue.Title != "Add local issue intake" {
		t.Fatalf("title=%q", opt.Issue.Title)
	}
	if !strings.Contains(opt.Issue.Body, "Details go here.") {
		t.Fatalf("body=%q", opt.Issue.Body)
	}
}

func TestRootCommandAcceptsLocalIssueDryRun(t *testing.T) {
	cmd := NewSDLCCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--config", filepath.Join(t.TempDir(), "missing.yaml"), "--issue", "Add CLI issue intake", "--dry-run", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, `"status":"dry-run"`) ||
		!strings.Contains(got, `"default_config":true`) ||
		!strings.Contains(got, `"conductor":"codex"`) ||
		!strings.Contains(got, `--sandbox`) ||
		!strings.Contains(got, `danger-full-access`) ||
		!strings.Contains(got, "Add CLI issue intake") {
		t.Fatalf("unexpected root dry-run output: %s", got)
	}
}

func TestBuildConductorBriefStatesDelegationBoundary(t *testing.T) {
	cfg := Config{
		Conductor: RoleConfig{Agent: "claude"},
		Reviewer:  RoleConfig{Agent: "codex"},
		QA:        RoleConfig{Agent: "codex"},
		Intake:    IntakeConfig{Provider: "github", Repository: "owner/repo"},
		Deploy: DeploymentConfig{
			Staging:    TargetConfig{Name: "staging"},
			Production: TargetConfig{Name: "production"},
		},
	}
	brief := BuildConductorBrief(cfg, Issue{ID: "42", Title: "Ship SDLC loop", Body: "Make it work"})
	for _, want := range []string{
		"SDLC owns intake",
		"You own implementation planning",
		"Do not deploy to production without explicit human approval",
		"Title: Ship SDLC loop",
		"Review agent: codex",
		"QA agent: codex",
	} {
		if !strings.Contains(brief, want) {
			t.Fatalf("brief missing %q:\n%s", want, brief)
		}
	}
}

func TestDelegateDryRunUsesConductorAgent(t *testing.T) {
	path := writeConfig(t, `
conductor:
  agent: claude
intake:
  provider: github
deployment:
  staging:
    name: staging
  production:
    name: production
`)
	res, err := Delegate(context.Background(), DelegateOptions{
		ConfigPath: path,
		Issue:      Issue{Title: "Add deployment gate"},
		DryRun:     true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "dry-run" || res.Chat.Agent != "claude" {
		t.Fatalf("unexpected delegate result: %+v", res)
	}
	if !strings.Contains(res.Chat.Output, "claude --dangerously-skip-permissions") {
		t.Fatalf("dry-run output missing resolved conductor invocation: %q", res.Chat.Output)
	}
}
