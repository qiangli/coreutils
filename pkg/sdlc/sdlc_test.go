package sdlc

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/coreutils/pkg/chat"
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
	for _, name := range []string{"guide", "init", "doctor", "config", "status", "issue", "brief", "delegate", "tick", "runs", "watch", "approve", "rollout", "resolve", "verify", "deploy-status", "guard"} {
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

func TestRunRecordListAndWatch(t *testing.T) {
	dir := t.TempDir()
	res := DelegateResult{
		Conductor: "codex",
		Issue:     Issue{ID: "42", Title: "Add monitor", URL: "https://example.test/42"},
		Brief:     "sensitive conductor brief",
	}
	run, err := NewRunRecord(res, DelegateOptions{RunsDir: dir, Cwd: dir}, chat.Result{
		Agent: "codex",
		Cwd:   dir,
		Args:  []string{"exec", "--sandbox", "danger-full-access", "prompt text"},
	})
	if err != nil {
		t.Fatal(err)
	}
	run.Status = "succeeded"
	run.ExitCode = 0
	run.FinishedAt = time.Now().UTC()
	if err := SaveRunRecord(*run); err != nil {
		t.Fatal(err)
	}
	if err := appendRunLog(*run, "line 1\nline 2\n"); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadRun(run.RunPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(loaded.Command, " "), "prompt text") {
		t.Fatalf("run command should redact prompt: %+v", loaded.Command)
	}
	if loaded.ReferenceID != run.ID {
		t.Fatalf("reference id=%q, want run id %q", loaded.ReferenceID, run.ID)
	}
	runs, err := ListRuns(dir, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].ID != run.ID {
		t.Fatalf("unexpected runs: %+v", runs)
	}
	var out bytes.Buffer
	if err := WatchRun(context.Background(), &out, WatchOptions{RunsDir: dir, RunID: run.ID, Tail: 1}); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, run.ID) || !strings.Contains(got, "line 2") || strings.Contains(got, "line 1") {
		t.Fatalf("unexpected watch output:\n%s", got)
	}
}

func TestSuperviseRunStreamsLogAndRecordsExit(t *testing.T) {
	shell := "/bin/sh"
	if _, err := os.Stat(shell); err != nil {
		t.Skipf("%s is not available", shell)
	}
	dir := t.TempDir()
	res := DelegateResult{
		Conductor: shell,
		Issue:     Issue{Title: "Supervise stream"},
		Brief:     "brief",
	}
	run, err := NewRunRecord(res, DelegateOptions{RunsDir: dir, Cwd: dir}, chat.Result{
		Agent: shell,
		Cwd:   dir,
		Args:  []string{"-c", "printf 'one\\n'; printf 'two\\n'"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveRunRecord(*run); err != nil {
		t.Fatal(err)
	}
	if err := SuperviseRun(context.Background(), dir, run.ID, []string{shell, "-c", "printf 'one\\n'; printf 'two\\n'"}); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadRun(run.RunPath)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != "succeeded" || loaded.ExitCode != 0 || loaded.FinishedAt.IsZero() {
		t.Fatalf("unexpected supervised run: %+v", loaded)
	}
	data, err := os.ReadFile(run.LogPath)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	for _, want := range []string{"one", "two", "finished status=succeeded exit_code=0"} {
		if !strings.Contains(got, want) {
			t.Fatalf("log missing %q:\n%s", want, got)
		}
	}
}

func TestSuperviseRunRecordsFailure(t *testing.T) {
	shell := "/bin/sh"
	if _, err := os.Stat(shell); err != nil {
		t.Skipf("%s is not available", shell)
	}
	dir := t.TempDir()
	res := DelegateResult{
		Conductor: shell,
		Issue:     Issue{Title: "Supervise failure"},
		Brief:     "brief",
	}
	run, err := NewRunRecord(res, DelegateOptions{RunsDir: dir, Cwd: dir}, chat.Result{
		Agent: shell,
		Cwd:   dir,
		Args:  []string{"-c", "exit 7"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveRunRecord(*run); err != nil {
		t.Fatal(err)
	}
	if err := SuperviseRun(context.Background(), dir, run.ID, []string{shell, "-c", "exit 7"}); err == nil {
		t.Fatal("expected supervisor to return child failure")
	}
	loaded, err := LoadRun(run.RunPath)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != "failed" || loaded.ExitCode != 7 {
		t.Fatalf("unexpected failed run: %+v", loaded)
	}
}

func TestApproveResolveAndRolloutLifecycle(t *testing.T) {
	dir := t.TempDir()
	res := DelegateResult{
		Conductor: "codex",
		Issue:     Issue{ID: "external-42", Title: "Ship change", URL: "https://example.test/issues/42"},
		Brief:     "brief",
	}
	run, err := NewRunRecord(res, DelegateOptions{RunsDir: dir, Cwd: dir}, chat.Result{
		Agent: "codex",
		Cwd:   dir,
		Args:  []string{"exec", "prompt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	run.Status = "succeeded"
	run.FinishedAt = time.Now().UTC()
	if err := SaveRunRecord(*run); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRunByID(dir, "missing"); err == nil {
		t.Fatal("expected missing run lookup to fail")
	}
	approved, err := ApproveRun(dir, run.ReferenceID, "UAT passed", "qa")
	if err != nil {
		t.Fatal(err)
	}
	if !RunApproved(approved) || approved.Status != "approved" || approved.Approval.Note != "UAT passed" {
		t.Fatalf("unexpected approved run: %+v", approved)
	}
	rollout := BuildRolloutInstruction(approved, "push main", "release")
	for _, want := range []string{
		"Production approval granted",
		approved.ReferenceID,
		"External issue ID: external-42",
		"UAT passed",
		"push main",
	} {
		if !strings.Contains(rollout, want) {
			t.Fatalf("rollout instruction missing %q:\n%s", want, rollout)
		}
	}
	resolved, err := ResolveLifecycleRun(dir, run.ReferenceID, "resolved", "deployed", "release")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Status != "resolved" || resolved.Resolution == nil || resolved.Resolution.Note != "deployed" {
		t.Fatalf("unexpected resolved run: %+v", resolved)
	}
}

func TestLocalIssueReferenceLifecycleE2E(t *testing.T) {
	conductor := "/bin/echo"
	if _, err := os.Stat(conductor); err != nil {
		t.Skipf("%s is not available", conductor)
	}
	runsDir := t.TempDir()
	ref := runSDLCCmd(t,
		"--no-config",
		"--conductor", conductor,
		"--runs-dir", runsDir,
		"--issue", "Local reference lifecycle e2e",
	).ref
	if ref == "" {
		t.Fatal("expected delegate output to include sdlc reference")
	}

	run := runSDLCCmd(t, "approve", ref, "--runs-dir", runsDir, "--note", "UAT passed on local staging")
	if !strings.Contains(run.out, "approved "+ref) {
		t.Fatalf("approve output missing reference:\n%s", run.out)
	}

	run = runSDLCCmd(t,
		"rollout", ref,
		"--no-config",
		"--conductor", conductor,
		"--runs-dir", runsDir,
		"--dry-run",
		"--note", "push approved change to main",
	)
	for _, want := range []string{
		"Production approval granted",
		ref,
		"UAT passed on local staging",
		"push approved change to main",
	} {
		if !strings.Contains(run.out, want) {
			t.Fatalf("rollout output missing %q:\n%s", want, run.out)
		}
	}

	run = runSDLCCmd(t, "resolve", ref, "--runs-dir", runsDir, "--status", "resolved", "--note", "production verified")
	if !strings.Contains(run.out, ref+" resolved") {
		t.Fatalf("resolve output missing reference:\n%s", run.out)
	}

	run = runSDLCCmd(t, "runs", "--runs-dir", runsDir)
	for _, want := range []string{ref, "resolved", "approved=true", "resolved=true"} {
		if !strings.Contains(run.out, want) {
			t.Fatalf("runs output missing %q:\n%s", want, run.out)
		}
	}

	record, err := LoadRunByID(runsDir, ref)
	if err != nil {
		t.Fatal(err)
	}
	if record.ReferenceID != ref || record.Approval == nil || record.Resolution == nil {
		t.Fatalf("reference lifecycle not persisted: %+v", record)
	}
}

type sdlcCmdResult struct {
	out string
	ref string
}

func runSDLCCmd(t *testing.T, args ...string) sdlcCmdResult {
	t.Helper()
	cmd := NewSDLCCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("sdlc %s failed: %v\n%s", strings.Join(args, " "), err, out.String())
	}
	got := out.String()
	return sdlcCmdResult{out: got, ref: extractSDLCReference(got)}
}

func extractSDLCReference(out string) string {
	for _, line := range strings.Split(out, "\n") {
		if ref, ok := strings.CutPrefix(strings.TrimSpace(line), "sdlc reference: "); ok {
			return strings.TrimSpace(ref)
		}
	}
	return ""
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
