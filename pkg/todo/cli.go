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

// NewTodoCmd builds `bashy todo` — the host-scoped, personal task list. A single
// --owner namespace lets one tool serve the steward (default), a conductor/fixer,
// or a human without forking.
func NewTodoCmd() *cobra.Command {
	var owner string
	root := &cobra.Command{
		Use:   "todo",
		Short: "the host-scoped personal task list (steward / fixer / human)",
		Long: "todo is level 1 of the tracking hierarchy — a per-host, per-user, NON-committed\n" +
			"task list, the equivalent of a human's todo-list app. It complements the other\n" +
			"two levels rather than replacing them:\n\n" +
			"  issue   what is wrong/wanted, per repo, COMMITTED   (`bashy issue`)\n" +
			"  sprint  what a conductor is planning across repos    (`bashy sprint`)\n" +
			"  todo    what YOU/the steward are doing, per host      (this)\n\n" +
			"It reuses the issue register's record format at home scope (~/.bashy/todo/<owner>/):\n" +
			"YAML-frontmatter markdown, content-addressed ids, resolve-by-prefix. Statuses are a\n" +
			"personal lifecycle: todo -> doing -> done (or blocked). Use --owner to keep separate\n" +
			"lists (steward is the default; a fixer uses its run id; a human uses their own).",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().StringVar(&owner, "owner", DefaultOwner, "task-list owner (steward | a fixer id | a human name)")

	root.AddCommand(
		newAddCmd(&owner),
		newListCmd(&owner),
		newShowCmd(&owner),
		newStatusCmd(&owner),
		newDoneCmd(&owner),
		newStartCmd(&owner),
		newEditCmd(&owner),
		newRmCmd(&owner),
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

func newAddCmd(owner *string) *cobra.Command {
	var priority, note string
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "add <title>",
		Short: "add a task to the list",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			title := strings.Join(args, " ")
			it, err := Add(*owner, title, note, priority)
			if err != nil {
				return err
			}
			if jsonOut {
				return emitJSON(cmd, map[string]any{"id": it.ID, "owner": SanitizeOwner(*owner), "status": it.Status, "title": it.Title})
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

func newListCmd(owner *string) *cobra.Command {
	var status string
	var jsonOut, all bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "list tasks (open by default; --all includes done)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			items, err := List(*owner, status)
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

func newShowCmd(owner *string) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "show <id|prefix>",
		Short: "show one task",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := Store(*owner)
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

func newStatusCmd(owner *string) *cobra.Command {
	return &cobra.Command{
		Use:   "status <id|prefix> <todo|doing|blocked|done>",
		Short: "update a task's status",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			it, err := SetStatus(*owner, args[0], args[1])
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s -> %s — %s\n", it.ID[:8], it.Status, it.Title)
			return nil
		},
	}
}

func newDoneCmd(owner *string) *cobra.Command {
	return &cobra.Command{
		Use:   "done <id|prefix>",
		Short: "mark a task done (alias for status done)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			it, err := SetStatus(*owner, args[0], StatusDone)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "done %s — %s\n", it.ID[:8], it.Title)
			return nil
		},
	}
}

func newStartCmd(owner *string) *cobra.Command {
	return &cobra.Command{
		Use:   "start <id|prefix>",
		Short: "mark a task in progress (alias for status doing)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			it, err := SetStatus(*owner, args[0], StatusDoing)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "doing %s — %s\n", it.ID[:8], it.Title)
			return nil
		},
	}
}

func newEditCmd(owner *string) *cobra.Command {
	var title, priority, note string
	cmd := &cobra.Command{
		Use:   "edit <id|prefix>",
		Short: "modify a task's title/priority/note",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := Store(*owner)
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

func newRmCmd(owner *string) *cobra.Command {
	return &cobra.Command{
		Use:   "rm <id|prefix>",
		Short: "remove a task",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			it, err := Remove(*owner, args[0])
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
