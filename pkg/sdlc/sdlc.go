package sdlc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/qiangli/coreutils/pkg/chat"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

const schemaVersion = "bashy-sdlc-v1"

// Config describes SDLC's boundary: intake system, agent roles, and deployment
// targets. Implementation details stay delegated to conductor/review/QA agents.
type Config struct {
	Conductor RoleConfig            `json:"conductor" yaml:"conductor"`
	Reviewer  RoleConfig            `json:"reviewer,omitempty" yaml:"reviewer,omitempty"`
	QA        RoleConfig            `json:"qa,omitempty" yaml:"qa,omitempty"`
	Intake    IntakeConfig          `json:"intake" yaml:"intake"`
	Deploy    DeploymentConfig      `json:"deployment" yaml:"deployment"`
	Metadata  map[string]string     `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Policies  map[string]string     `json:"policies,omitempty" yaml:"policies,omitempty"`
	Agents    map[string]RoleConfig `json:"agents,omitempty" yaml:"agents,omitempty"`
}

type RoleConfig struct {
	Agent string `json:"agent" yaml:"agent"`
}

type IntakeConfig struct {
	Provider   string   `json:"provider" yaml:"provider"`
	Repository string   `json:"repository,omitempty" yaml:"repository,omitempty"`
	Query      string   `json:"query,omitempty" yaml:"query,omitempty"`
	Labels     []string `json:"labels,omitempty" yaml:"labels,omitempty"`
}

type DeploymentConfig struct {
	Staging    TargetConfig `json:"staging" yaml:"staging"`
	Production TargetConfig `json:"production" yaml:"production"`
}

type TargetConfig struct {
	Name        string `json:"name,omitempty" yaml:"name,omitempty"`
	Host        string `json:"host,omitempty" yaml:"host,omitempty"`
	Environment string `json:"environment,omitempty" yaml:"environment,omitempty"`
	Command     string `json:"command,omitempty" yaml:"command,omitempty"`
	Healthcheck string `json:"healthcheck,omitempty" yaml:"healthcheck,omitempty"`
	Rollback    string `json:"rollback,omitempty" yaml:"rollback,omitempty"`
}

type Issue struct {
	ID    string
	URL   string
	Title string
	Body  string
}

type DelegateOptions struct {
	ConfigPath string
	Issue      Issue
	IssueText  string
	IssueFile  string
	DryRun     bool
	JSON       bool
	Timeout    time.Duration
	Cwd        string
}

type DelegateResult struct {
	SchemaVersion string      `json:"schema_version"`
	Status        string      `json:"status"`
	ConfigPath    string      `json:"config_path,omitempty"`
	DefaultConfig bool        `json:"default_config,omitempty"`
	Conductor     string      `json:"conductor,omitempty"`
	Issue         Issue       `json:"issue"`
	Brief         string      `json:"brief,omitempty"`
	Chat          chat.Result `json:"chat,omitempty"`
}

type ConfigExplanation struct {
	SchemaVersion string `json:"schema_version"`
	Source        string `json:"source"`
	ConfigPath    string `json:"config_path,omitempty"`
	Conductor     string `json:"conductor"`
	Intake        string `json:"intake"`
	Staging       string `json:"staging"`
	Production    string `json:"production"`
}

type VerifyOptions struct {
	Target  string
	Present []string
	Absent  []string
	Timeout time.Duration
}

type VerifyResult struct {
	SchemaVersion string        `json:"schema_version"`
	Status        string        `json:"status"`
	Target        string        `json:"target"`
	Checks        []VerifyCheck `json:"checks"`
}

type VerifyCheck struct {
	Kind   string `json:"kind"`
	Text   string `json:"text"`
	Status string `json:"status"`
}

type DeployStatus struct {
	SchemaVersion string `json:"schema_version"`
	Repo          string `json:"repo"`
	Workflow      string `json:"workflow,omitempty"`
	RunNumber     int    `json:"run_number,omitempty"`
	Status        string `json:"status,omitempty"`
	Conclusion    string `json:"conclusion,omitempty"`
	HTMLURL       string `json:"html_url,omitempty"`
	HeadSHA       string `json:"head_sha,omitempty"`
	Title         string `json:"title,omitempty"`
}

// NewSDLCCmd returns the `bashy sdlc` command tree.
func NewSDLCCmd() *cobra.Command {
	var opt DelegateOptions
	cmd := &cobra.Command{
		Use:   "sdlc [--issue TEXT | --issue-file PATH]",
		Short: "route intake issues through agentic implementation and deployment gates",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(opt.IssueText) == "" && strings.TrimSpace(opt.IssueFile) == "" &&
				strings.TrimSpace(opt.Issue.Title) == "" && strings.TrimSpace(opt.Issue.Body) == "" {
				return cmd.Help()
			}
			opt.JSON = opt.JSON || os.Getenv("BASHY_AGENTIC") != ""
			res, err := Delegate(cmd.Context(), opt)
			if opt.JSON {
				b, _ := json.Marshal(res)
				fmt.Fprintln(cmd.OutOrStdout(), string(b))
			} else if res.Chat.Output != "" {
				fmt.Fprint(cmd.OutOrStdout(), res.Chat.Output)
				if !strings.HasSuffix(res.Chat.Output, "\n") {
					fmt.Fprintln(cmd.OutOrStdout())
				}
			}
			return err
		},
	}
	cmd.CompletionOptions.DisableDefaultCmd = true
	bindIssueFlags(cmd, &opt)
	bindDelegateFlags(cmd, &opt)
	cmd.AddCommand(
		newInitCmd(), newDoctorCmd(), newConfigCmd(), newStatusCmd(), newIssueCmd(),
		newBriefCmd(), newDelegateCmd(), newTickCmd(), newVerifyCmd(), newDeployStatusCmd(), newGuardCmd(),
	)
	return cmd
}

func newInitCmd() *cobra.Command {
	var configPath string
	var force bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "write a starter SDLC config",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !force {
				if _, err := os.Stat(configPath); err == nil {
					return fmt.Errorf("sdlc: %s already exists; pass --force to overwrite", configPath)
				} else if !errors.Is(err, os.ErrNotExist) {
					return err
				}
			}
			if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(configPath, []byte(DefaultConfigYAML()), 0o644); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", configPath)
			return nil
		},
	}
	cmd.Flags().StringVar(&configPath, "config", ".bashy/sdlc.yaml", "SDLC config file")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite an existing config")
	return cmd
}

func newDoctorCmd() *cobra.Command {
	var configPath string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "validate SDLC config",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := LoadConfig(configPath)
			out := map[string]any{
				"schema_version": schemaVersion,
				"status":         "ok",
				"config_path":    configPath,
			}
			if err != nil {
				out["status"] = "error"
				out["error"] = err.Error()
			} else {
				out["conductor"] = cfg.Conductor.Agent
				out["intake"] = cfg.Intake.Provider
				out["staging"] = cfg.Deploy.Staging.Name
				out["production"] = cfg.Deploy.Production.Name
			}
			if asJSON || os.Getenv("BASHY_AGENTIC") != "" {
				b, _ := json.Marshal(out)
				fmt.Fprintln(cmd.OutOrStdout(), string(b))
			} else if err == nil {
				fmt.Fprintf(cmd.OutOrStdout(), "ok: conductor=%s intake=%s staging=%s production=%s\n",
					cfg.Conductor.Agent, cfg.Intake.Provider, displayTarget(cfg.Deploy.Staging), displayTarget(cfg.Deploy.Production))
			}
			return err
		},
	}
	cmd.Flags().StringVar(&configPath, "config", ".bashy/sdlc.yaml", "SDLC config file")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print a bashy-sdlc-v1 JSON envelope")
	return cmd
}

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "config", Short: "inspect SDLC configuration"}
	cmd.CompletionOptions.DisableDefaultCmd = true
	cmd.AddCommand(newConfigExplainCmd())
	return cmd
}

func newConfigExplainCmd() *cobra.Command {
	var configPath string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "explain",
		Short: "show which SDLC config/defaults are active",
		RunE: func(cmd *cobra.Command, args []string) error {
			exp, err := ExplainConfig(configPath)
			if err != nil {
				return err
			}
			if asJSON || os.Getenv("BASHY_AGENTIC") != "" {
				b, _ := json.Marshal(exp)
				fmt.Fprintln(cmd.OutOrStdout(), string(b))
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "source=%s conductor=%s intake=%s staging=%s production=%s\n",
				exp.Source, exp.Conductor, exp.Intake, exp.Staging, exp.Production)
			return nil
		},
	}
	cmd.Flags().StringVar(&configPath, "config", ".bashy/sdlc.yaml", "SDLC config file")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print JSON")
	return cmd
}

func newStatusCmd() *cobra.Command {
	var configPath string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "summarize local SDLC state",
		RunE: func(cmd *cobra.Command, args []string) error {
			exp, err := ExplainConfig(configPath)
			if err != nil {
				return err
			}
			status := map[string]any{
				"schema_version": schemaVersion,
				"config":         exp,
				"git":            gitSummary("."),
				"issues":         listIssueFiles(".bashy/sdlc/issues"),
			}
			if asJSON || os.Getenv("BASHY_AGENTIC") != "" {
				b, _ := json.Marshal(status)
				fmt.Fprintln(cmd.OutOrStdout(), string(b))
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "config: %s conductor=%s intake=%s\n", exp.Source, exp.Conductor, exp.Intake)
			if g, _ := status["git"].(map[string]any); g != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "git: branch=%v dirty=%v ahead=%v behind=%v\n", g["branch"], g["dirty"], g["ahead"], g["behind"])
			}
			if issues, _ := status["issues"].([]string); len(issues) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "issues: %d local\n", len(issues))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&configPath, "config", ".bashy/sdlc.yaml", "SDLC config file")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print JSON")
	return cmd
}

func newIssueCmd() *cobra.Command {
	var text, file, dir string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "issue",
		Short: "record a local SDLC issue",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 && strings.TrimSpace(text) == "" {
				text = strings.Join(args, " ")
			}
			issue, path, err := SaveLocalIssue(text, file, dir)
			if err != nil {
				return err
			}
			out := map[string]any{"schema_version": schemaVersion, "status": "saved", "path": path, "issue": issue}
			if asJSON || os.Getenv("BASHY_AGENTIC") != "" {
				b, _ := json.Marshal(out)
				fmt.Fprintln(cmd.OutOrStdout(), string(b))
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "saved %s\n", path)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&text, "text", "", "issue/request text")
	cmd.Flags().StringVar(&file, "file", "", "file containing issue/request text")
	cmd.Flags().StringVar(&dir, "dir", ".bashy/sdlc/issues", "local issue directory")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print JSON")
	return cmd
}

func newBriefCmd() *cobra.Command {
	var opt DelegateOptions
	cmd := &cobra.Command{
		Use:   "brief --issue-title TITLE",
		Short: "render the conductor brief without invoking an agent",
		RunE: func(cmd *cobra.Command, args []string) error {
			opt.DryRun = true
			res, err := Prepare(cmd.Context(), opt)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), res.Brief)
			return nil
		},
	}
	bindIssueFlags(cmd, &opt)
	return cmd
}

func newDelegateCmd() *cobra.Command {
	return newDelegateLikeCmd("delegate --issue-title TITLE", "send an intake issue to the configured conductor agent")
}

func newTickCmd() *cobra.Command {
	return newDelegateLikeCmd("tick --issue-title TITLE", "run one externally-triggered SDLC cycle for one intake issue")
}

func newVerifyCmd() *cobra.Command {
	var target string
	var absent, present []string
	var asJSON bool
	var timeout time.Duration
	cmd := &cobra.Command{
		Use:   "verify --url URL --absent TEXT",
		Short: "verify URL/file content with present/absent checks",
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := VerifyContent(cmd.Context(), VerifyOptions{
				Target:  target,
				Present: present,
				Absent:  absent,
				Timeout: timeout,
			})
			if asJSON || os.Getenv("BASHY_AGENTIC") != "" {
				b, _ := json.Marshal(res)
				fmt.Fprintln(cmd.OutOrStdout(), string(b))
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", res.Status, res.Target)
				for _, c := range res.Checks {
					fmt.Fprintf(cmd.OutOrStdout(), "  %s %q: %s\n", c.Kind, c.Text, c.Status)
				}
			}
			return err
		},
	}
	cmd.Flags().StringVar(&target, "url", "", "URL to inspect")
	cmd.Flags().StringVar(&target, "file", "", "local file to inspect")
	cmd.Flags().StringArrayVar(&absent, "absent", nil, "text that must be absent")
	cmd.Flags().StringArrayVar(&present, "present", nil, "text that must be present")
	cmd.Flags().DurationVar(&timeout, "timeout", 20*time.Second, "HTTP timeout")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print JSON")
	return cmd
}

func newDeployStatusCmd() *cobra.Command {
	var repo, branch string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "deploy-status",
		Short: "show latest GitHub Actions/Pages deployment status",
		RunE: func(cmd *cobra.Command, args []string) error {
			if repo == "" {
				repo = inferGitHubRepo(".")
			}
			res, err := GitHubDeployStatus(cmd.Context(), repo, branch)
			if asJSON || os.Getenv("BASHY_AGENTIC") != "" {
				b, _ := json.Marshal(res)
				fmt.Fprintln(cmd.OutOrStdout(), string(b))
			} else if err == nil {
				fmt.Fprintf(cmd.OutOrStdout(), "%s #%v %s/%s %s\n", res.Workflow, res.RunNumber, res.Status, res.Conclusion, res.HTMLURL)
			}
			return err
		},
	}
	cmd.Flags().StringVar(&repo, "repo", "", "GitHub repo owner/name; defaults from origin remote")
	cmd.Flags().StringVar(&branch, "branch", "", "branch filter")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print JSON")
	return cmd
}

func newGuardCmd() *cobra.Command {
	var checks []string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "guard",
		Short: "run local pre-push SDLC guard checks",
		RunE: func(cmd *cobra.Command, args []string) error {
			res := Guard(".", checks)
			if asJSON || os.Getenv("BASHY_AGENTIC") != "" {
				b, _ := json.Marshal(res)
				fmt.Fprintln(cmd.OutOrStdout(), string(b))
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\n", res["status"])
				if items, _ := res["checks"].([]map[string]any); items != nil {
					for _, item := range items {
						fmt.Fprintf(cmd.OutOrStdout(), "  %v: %v\n", item["name"], item["status"])
					}
				}
			}
			if res["status"] != "ok" {
				return errors.New("sdlc: guard failed")
			}
			return nil
		},
	}
	cmd.Flags().StringArrayVar(&checks, "check", nil, "command to run as a guard check")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print JSON")
	return cmd
}

func newDelegateLikeCmd(use, short string) *cobra.Command {
	var opt DelegateOptions
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		RunE: func(cmd *cobra.Command, args []string) error {
			opt.JSON = opt.JSON || os.Getenv("BASHY_AGENTIC") != ""
			res, err := Delegate(cmd.Context(), opt)
			if opt.JSON {
				b, _ := json.Marshal(res)
				fmt.Fprintln(cmd.OutOrStdout(), string(b))
			} else if res.Chat.Output != "" {
				fmt.Fprint(cmd.OutOrStdout(), res.Chat.Output)
				if !strings.HasSuffix(res.Chat.Output, "\n") {
					fmt.Fprintln(cmd.OutOrStdout())
				}
			}
			return err
		},
	}
	bindIssueFlags(cmd, &opt)
	bindDelegateFlags(cmd, &opt)
	return cmd
}

func bindDelegateFlags(cmd *cobra.Command, opt *DelegateOptions) {
	cmd.Flags().BoolVar(&opt.DryRun, "dry-run", false, "print the resolved agent invocation without running it")
	cmd.Flags().BoolVar(&opt.JSON, "json", false, "print a bashy-sdlc-v1 JSON envelope")
	cmd.Flags().DurationVar(&opt.Timeout, "timeout", 0, "conductor timeout, for example 45m")
	cmd.Flags().StringVar(&opt.Cwd, "cwd", "", "working directory for the conductor")
}

func bindIssueFlags(cmd *cobra.Command, opt *DelegateOptions) {
	cmd.Flags().StringVar(&opt.ConfigPath, "config", ".bashy/sdlc.yaml", "SDLC config file")
	cmd.Flags().StringVar(&opt.IssueText, "issue", "", "local issue/request text")
	cmd.Flags().StringVar(&opt.IssueFile, "issue-file", "", "file containing local issue/request text")
	cmd.Flags().StringVar(&opt.Issue.ID, "issue-id", "", "issue number or external issue id")
	cmd.Flags().StringVar(&opt.Issue.URL, "issue-url", "", "issue URL")
	cmd.Flags().StringVar(&opt.Issue.Title, "issue-title", "", "issue title")
	cmd.Flags().StringVar(&opt.Issue.Body, "issue-body", "", "issue body")
	cmd.Flags().String("issue-body-file", "", "file containing the issue body")
	cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		file, _ := cmd.Flags().GetString("issue-body-file")
		if file != "" {
			data, err := os.ReadFile(file)
			if err != nil {
				return fmt.Errorf("sdlc: read issue body file: %w", err)
			}
			opt.Issue.Body = string(data)
		}
		return ApplyIssueRequest(opt)
	}
}

func ApplyIssueRequest(opt *DelegateOptions) error {
	if opt == nil {
		return nil
	}
	text := strings.TrimSpace(opt.IssueText)
	if strings.TrimSpace(opt.IssueFile) != "" {
		data, err := os.ReadFile(opt.IssueFile)
		if err != nil {
			return fmt.Errorf("sdlc: read issue file: %w", err)
		}
		text = strings.TrimSpace(string(data))
	}
	if text == "" {
		return nil
	}
	title, body := splitIssueRequest(text)
	if strings.TrimSpace(opt.Issue.Title) == "" {
		opt.Issue.Title = title
	}
	if strings.TrimSpace(opt.Issue.Body) == "" && body != "" {
		opt.Issue.Body = body
	}
	return nil
}

func splitIssueRequest(text string) (title, body string) {
	text = strings.TrimSpace(text)
	for _, line := range strings.Split(text, "\n") {
		if s := strings.TrimSpace(line); s != "" {
			title = s
			break
		}
	}
	if title == "" {
		title = text
	}
	if strings.Contains(text, "\n") {
		body = text
	}
	return title, body
}

func LoadConfig(path string) (Config, error) {
	cfg, _, err := loadConfig(path, false)
	return cfg, err
}

func LoadConfigOrDefault(path string) (Config, bool, error) {
	return loadConfig(path, true)
}

func ExplainConfig(path string) (ConfigExplanation, error) {
	cfg, usedDefault, err := LoadConfigOrDefault(path)
	if err != nil {
		return ConfigExplanation{SchemaVersion: schemaVersion, ConfigPath: path}, err
	}
	source := "file"
	if usedDefault {
		source = "embedded-default"
	}
	return ConfigExplanation{
		SchemaVersion: schemaVersion,
		Source:        source,
		ConfigPath:    path,
		Conductor:     cfg.Conductor.Agent,
		Intake:        cfg.Intake.Provider,
		Staging:       displayTarget(cfg.Deploy.Staging),
		Production:    displayTarget(cfg.Deploy.Production),
	}, nil
}

func loadConfig(path string, fallback bool) (Config, bool, error) {
	var cfg Config
	data, err := os.ReadFile(path)
	if err != nil {
		if fallback && errors.Is(err, os.ErrNotExist) {
			cfg = DefaultConfig()
			return cfg, true, cfg.Validate()
		}
		return cfg, false, fmt.Errorf("sdlc: read config: %w", err)
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, false, fmt.Errorf("sdlc: parse config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return cfg, false, err
	}
	return cfg, false, nil
}

func DefaultConfig() Config {
	conductor := DefaultConductorAgent()
	return Config{
		Conductor: RoleConfig{Agent: conductor},
		Reviewer:  RoleConfig{Agent: "codex"},
		QA:        RoleConfig{Agent: "codex"},
		Intake:    IntakeConfig{Provider: "local"},
		Deploy: DeploymentConfig{
			Staging:    TargetConfig{Name: "staging"},
			Production: TargetConfig{Name: "production"},
		},
	}
}

func DefaultConfigYAML() string {
	b, err := yaml.Marshal(DefaultConfig())
	if err != nil {
		return "conductor:\n  agent: " + DefaultConductorAgent() + "\nintake:\n  provider: local\n"
	}
	return string(b)
}

// DefaultConductorAgent resolves the operator agent for local interactive SDLC.
// A child process cannot reliably infer its parent chat client, so explicit
// BASHY_* signals win; the rest are best-effort environment fingerprints.
func DefaultConductorAgent() string {
	for _, key := range []string{
		"BASHY_SDLC_CONDUCTOR_AGENT",
		"BASHY_CONDUCTOR_AGENT",
		"BASHY_AGENT_TOOL",
		"BASHY_AGENT_NAME",
		"BASHY_AGENT",
		"AGENT_TOOL",
		"AGENT_NAME",
		"AI_AGENT",
	} {
		if agent := normalizeAgentName(os.Getenv(key)); agent != "" {
			return agent
		}
	}
	env := os.Environ()
	switch {
	case hasAnyEnv(env, "CLAUDECODE", "CLAUDE_CODE", "CLAUDE_SESSION_ID", "CLAUDE_CONFIG_DIR"):
		return "claude"
	case hasAnyEnv(env, "OPENCODE", "OPENCODE_SESSION_ID", "OPENCODE_CONFIG_DIR"):
		return "opencode"
	case hasAnyEnv(env, "AGY", "AGY_SESSION_ID", "ANTIGRAVITY"):
		return "agy"
	case hasAnyEnv(env, "CODEX_SANDBOX", "CODEX_HOME", "CODEX_SESSION_ID", "CODEX_CLI"):
		return "codex"
	default:
		return "codex"
	}
}

func normalizeAgentName(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "claude", "claude-code", "claude_code", "claudecode":
		return "claude"
	case "codex", "openai-codex", "openai_codex":
		return "codex"
	case "opencode", "open-code", "open_code":
		return "opencode"
	case "agy", "antigravity", "anti-gravity", "anti_gravity":
		return "agy"
	default:
		return ""
	}
}

func hasAnyEnv(env []string, keys ...string) bool {
	for _, item := range env {
		name, value, ok := strings.Cut(item, "=")
		if !ok || value == "" {
			continue
		}
		for _, key := range keys {
			if strings.EqualFold(name, key) || strings.HasPrefix(strings.ToUpper(name), strings.ToUpper(key)+"_") {
				return true
			}
		}
	}
	return false
}

func (cfg Config) Validate() error {
	if strings.TrimSpace(cfg.Conductor.Agent) == "" {
		return errors.New("sdlc: conductor.agent is required")
	}
	if strings.TrimSpace(cfg.Intake.Provider) == "" {
		return errors.New("sdlc: intake.provider is required")
	}
	if strings.TrimSpace(cfg.Deploy.Staging.Name) == "" && strings.TrimSpace(cfg.Deploy.Staging.Environment) == "" {
		return errors.New("sdlc: deployment.staging.name or deployment.staging.environment is required")
	}
	if strings.TrimSpace(cfg.Deploy.Production.Name) == "" && strings.TrimSpace(cfg.Deploy.Production.Environment) == "" {
		return errors.New("sdlc: deployment.production.name or deployment.production.environment is required")
	}
	return nil
}

func Prepare(ctx context.Context, opt DelegateOptions) (DelegateResult, error) {
	if err := ApplyIssueRequest(&opt); err != nil {
		return DelegateResult{SchemaVersion: schemaVersion, Status: "error", ConfigPath: opt.ConfigPath}, err
	}
	cfg, usedDefault, err := LoadConfigOrDefault(opt.ConfigPath)
	if err != nil {
		return DelegateResult{SchemaVersion: schemaVersion, Status: "error", ConfigPath: opt.ConfigPath}, err
	}
	if strings.TrimSpace(opt.Issue.Title) == "" {
		return DelegateResult{SchemaVersion: schemaVersion, Status: "error", ConfigPath: opt.ConfigPath}, errors.New("sdlc: --issue-title is required")
	}
	brief := BuildConductorBrief(cfg, opt.Issue)
	return DelegateResult{
		SchemaVersion: schemaVersion,
		Status:        "ready",
		ConfigPath:    opt.ConfigPath,
		DefaultConfig: usedDefault,
		Conductor:     cfg.Conductor.Agent,
		Issue:         opt.Issue,
		Brief:         brief,
	}, nil
}

func Delegate(ctx context.Context, opt DelegateOptions) (DelegateResult, error) {
	res, err := Prepare(ctx, opt)
	if err != nil {
		return res, err
	}
	chatRes, err := chat.Invoke(ctx, chat.Options{
		Agent:       res.Conductor,
		Role:        "conductor",
		Instruction: res.Brief,
		Cwd:         opt.Cwd,
		Timeout:     opt.Timeout,
		DryRun:      opt.DryRun,
	}, nil)
	res.Chat = chatRes
	if err != nil {
		res.Status = "error"
		return res, err
	}
	res.Status = "delegated"
	if opt.DryRun {
		res.Status = "dry-run"
	}
	return res, nil
}

func BuildConductorBrief(cfg Config, issue Issue) string {
	var b strings.Builder
	fmt.Fprintf(&b, "You are the SDLC conductor for this repository.\n\n")
	fmt.Fprintf(&b, "Boundary:\n")
	fmt.Fprintf(&b, "- SDLC owns intake, user approval gates, staging/prod deployment routing, and lifecycle state.\n")
	fmt.Fprintf(&b, "- You own implementation planning and sprint execution. Use bashy sprint/weave and delegate coding, review, and QA work to the available agent fleet.\n")
	fmt.Fprintf(&b, "- Do not deploy to production without explicit human approval.\n\n")
	fmt.Fprintf(&b, "Issue:\n")
	if issue.ID != "" {
		fmt.Fprintf(&b, "ID: %s\n", issue.ID)
	}
	if issue.URL != "" {
		fmt.Fprintf(&b, "URL: %s\n", issue.URL)
	}
	fmt.Fprintf(&b, "Title: %s\n", strings.TrimSpace(issue.Title))
	if strings.TrimSpace(issue.Body) != "" {
		fmt.Fprintf(&b, "Body:\n%s\n", strings.TrimSpace(issue.Body))
	}
	fmt.Fprintf(&b, "\nRouting:\n")
	fmt.Fprintf(&b, "- Intake provider: %s", cfg.Intake.Provider)
	if cfg.Intake.Repository != "" {
		fmt.Fprintf(&b, " (%s)", cfg.Intake.Repository)
	}
	fmt.Fprintf(&b, "\n")
	if cfg.Reviewer.Agent != "" {
		fmt.Fprintf(&b, "- Review agent: %s\n", cfg.Reviewer.Agent)
	}
	if cfg.QA.Agent != "" {
		fmt.Fprintf(&b, "- QA agent: %s\n", cfg.QA.Agent)
	}
	fmt.Fprintf(&b, "- Staging target: %s\n", displayTarget(cfg.Deploy.Staging))
	fmt.Fprintf(&b, "- Production target: %s\n", displayTarget(cfg.Deploy.Production))
	if cfg.Deploy.Staging.Healthcheck != "" {
		fmt.Fprintf(&b, "- Staging healthcheck: %s\n", cfg.Deploy.Staging.Healthcheck)
	}
	if cfg.Deploy.Production.Healthcheck != "" {
		fmt.Fprintf(&b, "- Production healthcheck: %s\n", cfg.Deploy.Production.Healthcheck)
	}
	fmt.Fprintf(&b, "\nExpected loop:\n")
	fmt.Fprintf(&b, "1. Analyze priority/risk and create a sprint plan.\n")
	fmt.Fprintf(&b, "2. Assign implementation work to agentic tools.\n")
	fmt.Fprintf(&b, "3. Merge only after tests and review pass.\n")
	fmt.Fprintf(&b, "4. Prepare staging deployment evidence and request UAT/smoke approval.\n")
	fmt.Fprintf(&b, "5. Iterate on follow-up issues until approved, then prepare production rollout instructions.\n")
	return b.String()
}

func displayTarget(t TargetConfig) string {
	for _, v := range []string{t.Name, t.Environment, t.Host} {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return "unconfigured"
}

func SaveLocalIssue(text, file, dir string) (Issue, string, error) {
	text = strings.TrimSpace(text)
	if strings.TrimSpace(file) != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			return Issue{}, "", fmt.Errorf("sdlc: read issue file: %w", err)
		}
		text = strings.TrimSpace(string(data))
	}
	if text == "" {
		return Issue{}, "", errors.New("sdlc: --text, --file, or positional issue text is required")
	}
	title, body := splitIssueRequest(text)
	issue := Issue{ID: time.Now().UTC().Format("20060102T150405Z"), Title: title, Body: body}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Issue{}, "", err
	}
	slug := slugify(title)
	if slug == "" {
		slug = "issue"
	}
	path := filepath.Join(dir, issue.ID+"-"+slug+".md")
	content := fmt.Sprintf("---\nid: %s\ntitle: %q\ncreated_at: %s\n---\n\n%s\n",
		issue.ID, issue.Title, time.Now().UTC().Format(time.RFC3339), text)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return Issue{}, "", err
	}
	return issue, path, nil
}

func VerifyContent(ctx context.Context, opt VerifyOptions) (VerifyResult, error) {
	target := strings.TrimSpace(opt.Target)
	res := VerifyResult{SchemaVersion: schemaVersion, Target: target, Status: "ok"}
	if target == "" {
		res.Status = "error"
		return res, errors.New("sdlc: --url or --file is required")
	}
	body, err := readTarget(ctx, target, opt.Timeout)
	if err != nil {
		res.Status = "error"
		return res, err
	}
	for _, text := range opt.Present {
		ok := strings.Contains(body, text)
		status := "ok"
		if !ok {
			status, res.Status = "missing", "failed"
		}
		res.Checks = append(res.Checks, VerifyCheck{Kind: "present", Text: text, Status: status})
	}
	for _, text := range opt.Absent {
		ok := !strings.Contains(body, text)
		status := "ok"
		if !ok {
			status, res.Status = "present", "failed"
		}
		res.Checks = append(res.Checks, VerifyCheck{Kind: "absent", Text: text, Status: status})
	}
	if res.Status != "ok" {
		return res, errors.New("sdlc: verify failed")
	}
	return res, nil
}

func readTarget(ctx context.Context, target string, timeout time.Duration) (string, error) {
	if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
		if timeout <= 0 {
			timeout = 20 * time.Second
		}
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			return "", err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return "", fmt.Errorf("sdlc: GET %s: %s", target, resp.Status)
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
		return string(data), err
	}
	data, err := os.ReadFile(target)
	return string(data), err
}

func GitHubDeployStatus(ctx context.Context, repo, branch string) (DeployStatus, error) {
	repo = strings.TrimSpace(repo)
	res := DeployStatus{SchemaVersion: schemaVersion, Repo: repo}
	if repo == "" {
		res.Status = "error"
		return res, errors.New("sdlc: --repo is required and origin is not a GitHub remote")
	}
	url := "https://api.github.com/repos/" + repo + "/actions/runs?per_page=1"
	if branch != "" {
		url += "&branch=" + branch
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return res, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return res, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return res, fmt.Errorf("sdlc: GitHub API: %s", resp.Status)
	}
	var payload struct {
		WorkflowRuns []struct {
			Name        string `json:"name"`
			RunNumber   int    `json:"run_number"`
			Status      string `json:"status"`
			Conclusion  string `json:"conclusion"`
			HTMLURL     string `json:"html_url"`
			HeadSHA     string `json:"head_sha"`
			DisplayName string `json:"display_title"`
		} `json:"workflow_runs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return res, err
	}
	if len(payload.WorkflowRuns) == 0 {
		res.Status = "none"
		return res, nil
	}
	run := payload.WorkflowRuns[0]
	res.Workflow, res.RunNumber, res.Status, res.Conclusion = run.Name, run.RunNumber, run.Status, run.Conclusion
	res.HTMLURL, res.HeadSHA, res.Title = run.HTMLURL, run.HeadSHA, run.DisplayName
	return res, nil
}

func Guard(dir string, commands []string) map[string]any {
	checks := []map[string]any{}
	git := gitSummary(dir)
	gitStatus := "ok"
	if dirty, _ := git["dirty"].(bool); dirty {
		gitStatus = "dirty"
	}
	checks = append(checks, map[string]any{"name": "git", "status": gitStatus, "detail": git})
	secretHits := secretScan(dir)
	secretStatus := "ok"
	if len(secretHits) > 0 {
		secretStatus = "failed"
	}
	checks = append(checks, map[string]any{"name": "secrets", "status": secretStatus, "hits": secretHits})
	for _, c := range commands {
		status, output := runGuardCommand(dir, c)
		checks = append(checks, map[string]any{"name": c, "status": status, "output": output})
	}
	status := "ok"
	for _, c := range checks {
		if c["status"] != "ok" {
			status = "failed"
			break
		}
	}
	return map[string]any{"schema_version": schemaVersion, "status": status, "checks": checks}
}

func gitSummary(dir string) map[string]any {
	branch := strings.TrimSpace(runGit(dir, "branch", "--show-current"))
	status := runGit(dir, "status", "--porcelain=v1", "--branch")
	out := map[string]any{"branch": branch, "dirty": false, "ahead": false, "behind": false}
	for _, line := range strings.Split(status, "\n") {
		if strings.HasPrefix(line, "## ") {
			out["ahead"] = strings.Contains(line, "ahead")
			out["behind"] = strings.Contains(line, "behind")
			continue
		}
		if strings.TrimSpace(line) != "" {
			out["dirty"] = true
		}
	}
	return out
}

func runGit(dir string, args ...string) string {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return ""
	}
	return string(out)
}

func listIssueFiles(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(out)
	return out
}

func inferGitHubRepo(dir string) string {
	remote := strings.TrimSpace(runGit(dir, "remote", "get-url", "origin"))
	remote = strings.TrimSuffix(remote, ".git")
	switch {
	case strings.HasPrefix(remote, "git@github.com:"):
		return strings.TrimPrefix(remote, "git@github.com:")
	case strings.HasPrefix(remote, "https://github.com/"):
		return strings.TrimPrefix(remote, "https://github.com/")
	default:
		return ""
	}
}

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(api[_-]?key|secret|token|password)\s*[:=]\s*['"]?[A-Za-z0-9_\-]{16,}`),
	regexp.MustCompile(`sk-[A-Za-z0-9]{20,}`),
}

func secretScan(dir string) []string {
	var hits []string
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			if d != nil && d.IsDir() && skipSecretScanDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if len(hits) >= 20 {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil || len(data) > 1<<20 {
			return nil
		}
		for _, re := range secretPatterns {
			if re.Match(data) {
				hits = append(hits, path)
				break
			}
		}
		return nil
	})
	return hits
}

func skipSecretScanDir(name string) bool {
	switch name {
	case ".git", ".next", ".contentlayer", "node_modules", "out", "dist", "build", "vendor", "external", "priorart", "testdata":
		return true
	default:
		return false
	}
}

func runGuardCommand(dir, command string) (string, string) {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return "skipped", ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, fields[0], fields[1:]...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "failed", string(out)
	}
	return "ok", string(out)
}

func slugify(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}
