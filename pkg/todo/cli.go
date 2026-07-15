// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package todo

import (
	"encoding/json"
	"fmt"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/qiangli/coreutils/pkg/issue"
)

// storeFunc resolves the store for the current scope (--owner / --repo).
type storeFunc func() (*issue.Store, error)

// NewTodoCmd builds `bashy todo` — the task list, in TWO scopes over one item model:
//
//	default   your personal, host-scoped list (~/.bashy/todo/<owner>/, not committed)
//	--repo    THIS repo's committed list   (<repo>/.bashy/todo/, travels with the clone)
//
// One command; the scope is an explicit flag, never inferred from cwd (so a personal
// note never lands in a repo's history by accident, or vice versa). repoRoot resolves
// the repo for --repo; pass nil to disable that scope.
func NewTodoCmd(repoRoot func() (string, error)) *cobra.Command {
	var owner string
	var repo bool
	root := &cobra.Command{
		Use:   "todo",
		Short: "the task list — personal (default) or --repo (committed)",
		Long: "todo tracks work as simple items (todo -> doing -> done, or blocked) in one of\n" +
			"two SCOPES, chosen by flag:\n\n" +
			"  bashy todo …          your PERSONAL list, per host/user (~/.bashy/todo/<owner>/,\n" +
			"                        NOT committed). Use --owner to keep separate lists (the\n" +
			"                        steward is the default; a fixer uses its run id; a human\n" +
			"                        uses their own).\n" +
			"  bashy todo --repo …   THIS repo's COMMITTED list (<repo>/.bashy/todo/), which\n" +
			"                        travels with the clone and shows up in diffs.\n\n" +
			"The scope is always an explicit flag, never guessed from the working directory.\n\n" +
			"Relation to the other trackers: `bashy sprint` is a TIME-BOX (a window grouping\n" +
			"items); `bashy issue` is the FORMAL committed register (triage, kinds, weave\n" +
			"linkage). todo is the lightweight everyday list — personal or checked in.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().StringVar(&owner, "owner", DefaultOwner, "personal-list owner (steward | a fixer id | a human name)")
	root.PersistentFlags().BoolVar(&repo, "repo", false, "use THIS repo's committed list (.bashy/todo/) instead of your personal list")

	sf := func() (*issue.Store, error) {
		st, _, err := ResolveStore(owner, repo, repoRoot)
		return st, err
	}

	root.AddCommand(
		newAddCmd(sf),
		newListCmd(sf),
		newShowCmd(sf),
		newStatusCmd(sf),
		newDoneCmd(sf),
		newStartCmd(sf),
		newEditCmd(sf),
		newRmCmd(sf),
	)
	return root
}

func emitJSON(cmd *cobra.Command, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), string(b))
	return nil
}

func newAddCmd(sf storeFunc) *cobra.Command {
	var priority, note string
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "add <title>",
		Short: "add a task to the list",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := sf()
			if err != nil {
				return err
			}
			it, err := Add(st, strings.Join(args, " "), note, priority)
			if err != nil {
				return err
			}
			if jsonOut {
				return emitJSON(cmd, map[string]any{"id": it.ID, "status": it.Status, "title": it.Title})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "added %s [%s] — %s\n", it.ID[:8], it.Status, it.Title)
			return nil
		},
	}
	cmd.Flags().StringVar(&priority, "priority", "", "priority tier (p0|p1|p2|p3)")
	cmd.Flags().StringVar(&note, "note", "", "task body/details")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "machine-readable output")
	return cmd
}

func newListCmd(sf storeFunc) *cobra.Command {
	var status string
	var jsonOut, all bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "list tasks (open by default; --all includes done)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := sf()
			if err != nil {
				return err
			}
			items, err := List(st, status)
			if err != nil {
				return err
			}
			if status == "" && !all {
				var open []*issue.Issue
				for _, it := range items {
					if it.Status != StatusDone {
						open = append(open, it)
					}
				}
				items = open
			}
			if jsonOut {
				return emitJSON(cmd, items)
			}
			if len(items) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no tasks (bashy todo add \"...\")")
				return nil
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tSTATUS\tPRIO\tAGE\tTITLE")
			for _, it := range items {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
					it.ID[:8], it.Status, dash(it.Priority), age(it.Created), trunc(it.Title, 60))
			}
			return w.Flush()
		},
	}
	cmd.Flags().StringVar(&status, "status", "", "filter by status (todo|doing|blocked|done)")
	cmd.Flags().BoolVar(&all, "all", false, "include done tasks")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "machine-readable output")
	return cmd
}

func newShowCmd(sf storeFunc) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "show <id|prefix>",
		Short: "show one task",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := sf()
			if err != nil {
				return err
			}
			it, err := st.Resolve(args[0])
			if err != nil {
				return err
			}
			if jsonOut {
				return emitJSON(cmd, it)
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "%s  %s\n\n", it.ID, it.Title)
			fmt.Fprintf(w, "  status    %s\n", it.Status)
			if it.Priority != "" {
				fmt.Fprintf(w, "  priority  %s\n", it.Priority)
			}
			fmt.Fprintf(w, "  created   %s\n", it.Created.Format(time.RFC3339))
			if it.Closed != nil {
				fmt.Fprintf(w, "  done      %s\n", it.Closed.Format(time.RFC3339))
			}
			if body := strings.TrimSpace(it.Body); body != "" {
				fmt.Fprintf(w, "\n%s\n", body)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "machine-readable output")
	return cmd
}

func newStatusCmd(sf storeFunc) *cobra.Command {
	return &cobra.Command{
		Use:   "status <id|prefix> <todo|doing|blocked|done>",
		Short: "update a task's status",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := sf()
			if err != nil {
				return err
			}
			it, err := SetStatus(st, args[0], args[1])
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s -> %s — %s\n", it.ID[:8], it.Status, it.Title)
			return nil
		},
	}
}

func newDoneCmd(sf storeFunc) *cobra.Command {
	return &cobra.Command{
		Use:   "done <id|prefix>",
		Short: "mark a task done (alias for status done)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := sf()
			if err != nil {
				return err
			}
			it, err := SetStatus(st, args[0], StatusDone)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "done %s — %s\n", it.ID[:8], it.Title)
			return nil
		},
	}
}

func newStartCmd(sf storeFunc) *cobra.Command {
	return &cobra.Command{
		Use:   "start <id|prefix>",
		Short: "mark a task in progress (alias for status doing)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := sf()
			if err != nil {
				return err
			}
			it, err := SetStatus(st, args[0], StatusDoing)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "doing %s — %s\n", it.ID[:8], it.Title)
			return nil
		},
	}
}

func newEditCmd(sf storeFunc) *cobra.Command {
	var title, priority, note string
	cmd := &cobra.Command{
		Use:   "edit <id|prefix>",
		Short: "modify a task's title/priority/note",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := sf()
			if err != nil {
				return err
			}
			it, err := st.Resolve(args[0])
			if err != nil {
				return err
			}
			if title != "" {
				it.Title = title
			}
			if cmd.Flags().Changed("priority") {
				it.Priority = priority
			}
			if cmd.Flags().Changed("note") {
				it.Body = note
			}
			if _, err := st.Save(it); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "edited %s — %s\n", it.ID[:8], it.Title)
			return nil
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "new title")
	cmd.Flags().StringVar(&priority, "priority", "", "new priority (p0|p1|p2|p3)")
	cmd.Flags().StringVar(&note, "note", "", "replace the task body/details")
	return cmd
}

func newRmCmd(sf storeFunc) *cobra.Command {
	return &cobra.Command{
		Use:   "rm <id|prefix>",
		Short: "remove a task",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := sf()
			if err != nil {
				return err
			}
			it, err := Remove(st, args[0])
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "removed %s — %s\n", it.ID[:8], it.Title)
			return nil
		},
	}
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func age(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}
