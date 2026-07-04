package sdlc

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// The deploy baton: `bashy sdlc promote RUN_ID --env <env>` applies the
// deploy:<env> label to the run's GitHub issue, which fires the deploy Action.
// It is the seam between the private conductor loop and the public deploy plane.
// Promotion to the production env is gated by the prod_approval policy.

func newPromoteCmd() *cobra.Command {
	var dir, env, repo, note, configPath string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "promote RUN_ID --env ENV",
		Short: "apply the deploy:<env> baton label to a run's issue (triggers the deploy Action)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(env) == "" {
				return errors.New("sdlc: --env is required (for example dev, qa, or prod)")
			}
			run, err := LoadRunByID(dir, args[0])
			if err != nil {
				return err
			}
			cfg, _, _ := LoadConfigOrDefault(configPath)
			if RequiresApproval(cfg, env) && !RunApproved(run) {
				return fmt.Errorf("sdlc: promotion to %q requires approval (policy prod_approval=required); run `bashy sdlc approve %s --note ...` first", env, args[0])
			}
			ghRepo, number, ok := resolvePromoteTarget(run, repo)
			if !ok {
				return fmt.Errorf("sdlc: run %s has no GitHub issue reference; pass --repo owner/name (its issue id is %q)", args[0], run.IssueID)
			}
			label, err := PromoteByLabel(cmd.Context(), ghRepo, number, env, GitHubToken())
			if err != nil {
				return err
			}
			if strings.TrimSpace(note) != "" {
				_ = commentGitHubIssue(cmd.Context(), ghRepo, number, note, GitHubToken())
			}
			out := map[string]any{
				"schema_version": schemaVersion,
				"status":         "promoted",
				"reference":      run.ReferenceID,
				"env":            env,
				"label":          label,
				"repo":           ghRepo,
				"issue":          number,
			}
			if asJSON || os.Getenv("BASHY_AGENTIC") != "" {
				b, _ := json.Marshal(out)
				fmt.Fprintln(cmd.OutOrStdout(), string(b))
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "promoted %s → %s (%s#%d)\n", run.ReferenceID, label, ghRepo, number)
			return nil
		},
	}
	cmd.Flags().StringVar(&dir, "runs-dir", ".bashy/sdlc/runs", "local SDLC runs directory")
	cmd.Flags().StringVar(&env, "env", "", "target environment (dev|qa|prod)")
	cmd.Flags().StringVar(&repo, "repo", "", "GitHub repo owner/name; defaults from the run's issue id")
	cmd.Flags().StringVar(&configPath, "config", ".bashy/sdlc.yaml", "SDLC config file (for the prod_approval policy)")
	cmd.Flags().StringVar(&note, "note", "", "optional comment to post on the issue")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print JSON")
	return cmd
}

// RequiresApproval reports whether promoting to env needs an approved run: true
// when env is the configured production environment AND policy prod_approval is
// not "auto" (the default is to require approval). Non-production envs never
// require approval — dev/qa promote freely.
func RequiresApproval(cfg Config, env string) bool {
	if !isProductionEnv(cfg, env) {
		return false
	}
	return !strings.EqualFold(strings.TrimSpace(cfg.Policies["prod_approval"]), "auto")
}

func isProductionEnv(cfg Config, env string) bool {
	env = strings.ToLower(strings.TrimSpace(env))
	if env == "" {
		return false
	}
	for _, cand := range []string{cfg.Deploy.Production.Environment, cfg.Deploy.Production.Name} {
		if c := strings.ToLower(strings.TrimSpace(cand)); c != "" && c == env {
			return true
		}
	}
	return env == "prod" || env == "production"
}

// resolvePromoteTarget derives the GitHub repo + issue number for a run: from the
// run's issue id ("owner/name#N"), or from --repo + a bare-number issue id.
func resolvePromoteTarget(run RunRecord, repoOverride string) (string, int, bool) {
	if repo, n, ok := parseIssueRef(run.IssueID); ok {
		if o := strings.TrimSpace(repoOverride); o != "" {
			repo = o
		}
		return repo, n, true
	}
	if o := strings.TrimSpace(repoOverride); o != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(run.IssueID)); err == nil {
			return o, n, true
		}
	}
	return "", 0, false
}
