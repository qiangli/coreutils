package weave

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/qiangli/coreutils/pkg/weavecli"
)

// WeaveCmd holds the top-level weave command for introspection.
var WeaveCmd *cobra.Command

// StubRunE is the placeholder RunE used for subcommands that are not yet wired.
var StubRunE func(cmd *cobra.Command, args []string) error = stubRunE

// stubRunE is the default RunE for subverbs whose orchestration body lands in a later N+1 PR.
func stubRunE(cmd *cobra.Command, args []string) error {
	mode := weavecli.ResolveOutputMode(false, false, false) // default plain
	code := weavecli.EmitError(cmd.ErrOrStderr(), mode, cmd.Name(), weavecli.ExitPrecondFail,
		fmt.Errorf("not yet wired in this build (see N+1 group B/C/D in docs/loom-v2-implementation.md)"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	if code != weavecli.ExitOK {
		return &exitCodeError{code: code}
	}
	return nil
}

// newWeaveCmd builds the `bashy weave ...` top-level group — the v2
// human-facing front door per docs/loom-v2-plan.md. Subverbs
// dispatch through the agent-friendly envelope conventions in
// internal/cli/weavecli; concrete implementations land in
// per-subverb files (weave_add.go, weave_start.go, etc.) so this
// file stays a thin registry.
func newWeaveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "weave",
		Short: "Run agentic tools in isolated, convergent workspaces (v2)",
		// Every weave subverb emits its own structured envelope (or
		// human line) and propagates an *exitCodeError carrying a
		// stable weavecli exit code. cobra's default "Error: ..." +
		// usage dump would double-print on top of the envelope, so we
		// silence both at the parent level — subverbs inherit.
		SilenceErrors: true,
		SilenceUsage:  true,
		Long: `weave is the v2 human/orchestrator front door over the Loom
substrate. Use it to seed a queue of issues, fan agentic tools out
across them in parallel without clobbering each other, and pull
the converged work back into your repo.

Common-case usage:

  bashy weave add "fix null deref in cache" --priority p0
  bashy weave add "refactor user service"
  bashy weave start -- codex                 # claims top of queue
  bashy weave start -- opencode              # claims next
  bashy weave list                           # what's in flight
  bashy weave pull                           # absorb merged work

The kanban project board is opt-in via 'bashy weave init-board';
the default dashboard is the forge's label-filtered issue list view.
See docs/loom-v2-plan.md for the full design.`,
	}

	cmd.AddCommand(newWeaveAddCmd())
	cmd.AddCommand(newWeaveStartCmd())
	cmd.AddCommand(newWeaveNextCmd())
	cmd.AddCommand(newWeavePrioCmd())
	cmd.AddCommand(newWeavePointCmd())
	cmd.AddCommand(newWeaveListCmd())
	cmd.AddCommand(newWeavePauseCmd())
	cmd.AddCommand(newWeaveResumeCmd())
	cmd.AddCommand(newWeaveAutopilotCmd())
	cmd.AddCommand(newWeaveStatusCmd())
	cmd.AddCommand(newWeaveLogCmd())
	cmd.AddCommand(newWeaveSayCmd())
	cmd.AddCommand(newWeavePullCmd())
	cmd.AddCommand(newWeavePruneCmd())
	cmd.AddCommand(newWeaveAbandonCmd())
	cmd.AddCommand(newWeaveKillCmd())
	cmd.AddCommand(newWeaveShellCmd())
	cmd.AddCommand(newWeaveOpenCmd())
	cmd.AddCommand(newWeaveResetCmd())
	cmd.AddCommand(newWeaveInitBoardCmd())
	cmd.AddCommand(newWeaveWaitCmd())
	cmd.AddCommand(newWeaveCheckCmd())

	WeaveCmd = cmd
	return cmd
}

// weaveOutputFlags adds the standard --json/--plain/--quiet flags
// shared across every subverb and returns getters so RunE bodies can
// resolve the OutputMode without re-declaring the flags.
type weaveOutputFlags struct {
	jsonF, plainF, quietF bool
}

func (f *weaveOutputFlags) attach(cmd *cobra.Command) {
	cmd.Flags().BoolVar(&f.jsonF, "json", false, "Emit machine-readable envelope (versioned schema)")
	cmd.Flags().BoolVar(&f.plainF, "plain", false, "Plain-text output, no ANSI or spinners")
	cmd.Flags().BoolVar(&f.quietF, "quiet", false, "Final result line only")
	// Every subverb emits its own envelope and propagates an
	// *exitCodeError. cobra's error handling only consults the leaf
	// command and the absolute root — NOT the `weave` parent — so the
	// parent's SilenceErrors/SilenceUsage don't reach here. Set them on
	// the leaf itself or cobra double-prints "Error: exit N" + a full
	// usage dump on top of the envelope (the confirmation-refusal noise).
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
}

func (f *weaveOutputFlags) mode() weavecli.OutputMode {
	return weavecli.ResolveOutputMode(f.jsonF, f.plainF, f.quietF)
}

// exitCodeError lets RunE propagate a specific exit code while cobra
// still sees an error (so its return-non-zero plumbing fires).
type exitCodeError struct{ code int }

func (e *exitCodeError) Error() string { return fmt.Sprintf("exit %d", e.code) }
func (e *exitCodeError) ExitCode() int { return e.code }
