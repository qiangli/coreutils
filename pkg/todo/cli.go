// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package todo

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/qiangli/coreutils/pkg/issue"
)

// storeFunc resolves the store for the current scope, returning it and a short scope
// label so every command can print WHICH list it is acting on.
type storeFunc func() (*issue.Store, string, error)

// NewTodoCmd builds `bashy todo` — the task list over one item model. The scope is
// AUTO-DETECTED so an agent can just `bashy todo add …` and it lands correctly:
//
//	default   inside a git repo → THAT repo's docs/todo/ (committed); otherwise your
//	          personal host list (~/.bashy/todo/<owner>/)
//	--user    force the personal list even inside a repo
//	--repo    force the repo list (error if not in a git repo)
//	--dir P   point the list at any base directory P
//
// Every command prints a one-line header with the resolved folder, so there is never
// any doubt about which list you are on.
func NewTodoCmd() *cobra.Command {
	var owner, baseDir string
	var forceRepo, forceUser bool
	root := &cobra.Command{
		Use:   "todo",
		Short: "the task list — auto: repo docs/todo/ if in a git repo, else your host list",
		Long: "todo tracks work as simple items (todo -> doing -> done, or blocked). The SCOPE is\n" +
			"auto-detected from where you are, so no flag is needed for the common case:\n\n" +
			"  in a git repo    → THAT repo's docs/todo/ (committed, travels with the clone) —\n" +
			"                     the structured replacement for an ad-hoc TODO.md.\n" +
			"  not in a repo    → your personal host list (~/.bashy/todo/<owner>/, not committed).\n\n" +
			"Overrides: --base-dir <root> shows ANOTHER project's list (<root>/docs/todo/) —\n" +
			"so one agent can travel between repos in a single session; --user forces the\n" +
			"personal list even inside a repo; --repo forces the repo list. Every command prints\n" +
			"a header showing the resolved folder, so which list you are on is never in doubt.\n\n" +
			"Relation to the other trackers: `bashy sprint` is a TIME-BOX (a window grouping\n" +
			"items); `bashy weave` is the execution queue (`weave add --from-todo` seeds a run).",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().StringVar(&owner, "owner", DefaultOwner, "personal-list owner (steward | a fixer id | a human name)")
	root.PersistentFlags().BoolVar(&forceRepo, "repo", false, "force THIS repo's committed list (docs/todo/); error if not in a git repo")
	root.PersistentFlags().BoolVar(&forceUser, "user", false, "force your personal host list (~/.bashy/todo/<owner>/), even inside a repo")
	root.PersistentFlags().StringVar(&baseDir, "base-dir", "", "show the list of ANOTHER project root (<root>/docs/todo/) — travel repos without cd")

	sf := func() (*issue.Store, string, error) {
		st, label, err := ResolveStore(owner, forceRepo, forceUser, baseDir)
		if err != nil {
			return nil, "", err
		}
		// Assign stable running numbers to any legacy items, once, so every command
		// (list, show 3, done 3, …) sees consistent handles. Best-effort.
		_ = EnsureSeq(st)
		return st, label, nil
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

// folder is the resolved on-disk directory of a store (where the item files live).
func folder(st *issue.Store) string { return filepath.Join(st.Root, st.Sub) }

// header is the "which list am I on" line printed before command output. scope is the
// label ResolveStore returns ("repo <root>" | "user <owner>" | "dir <path>"); the
// folder makes the exact location unambiguous.
func header(scope string, st *issue.Store) string {
	word := scope
	if i := strings.IndexByte(scope, ' '); i > 0 {
		word = scope[:i]
	}
	return fmt.Sprintf("todo [%s] %s", word, folder(st))
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
	var dueStr, recurring, assignee string
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "add <title>",
		Short: "add a task to the list",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, _, err := sf()
			if err != nil {
				return err
			}
			due, err := parseDue(dueStr)
			if err != nil {
				return err
			}
			it, err := Add(st, strings.Join(args, " "), note, priority, due, recurring, assignee)
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
	cmd.Flags().StringVar(&dueStr, "due", "", "deadline (e.g. 2026-07-20, +3d)")
	cmd.Flags().StringVar(&recurring, "recurring", "", "cadence (daily, weekly, 24h, cron)")
	cmd.Flags().StringVar(&assignee, "assignee", "", "who is working the item")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "machine-readable output")
	return cmd
}

func newListCmd(sf storeFunc) *cobra.Command {
	var status string
	var jsonOut, all, reverse bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "list tasks (sequential #1 first; --reverse for newest first; open by default, --all includes done)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			st, scope, err := sf()
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
			if reverse {
				slices.Reverse(items)
			}
			if jsonOut {
				return emitJSON(cmd, items)
			}
			// The header names WHICH list — auto-detected scope + exact folder — so
			// there is never confusion about where these tasks live.
			fmt.Fprintln(cmd.OutOrStdout(), header(scope, st))
			if len(items) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no tasks (bashy todo add \"...\")")
				return nil
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			// #  is the stable running number — the short handle for `todo show 3`,
			// `todo done 3`, etc. ID stays for scripts / cross-tool references.
			hasAssignee := false
			for _, it := range items {
				if it.Assignee != "" {
					hasAssignee = true
					break
				}
			}
			headerStr := "#\tID\tSTATUS\tPRIO\tAGE\tDUE"
			if hasAssignee {
				headerStr += "\tASSIGNEE"
			}
			headerStr += "\tTITLE"
			fmt.Fprintln(w, headerStr)
			for _, it := range items {
				dueStr := "-"
				if it.Due != nil {
					if it.Due.Before(time.Now().UTC()) {
						dueStr = "!" + it.Due.Format("2006-01-02")
					} else {
						dueStr = it.Due.Format("2006-01-02")
					}
				}
				row := fmt.Sprintf("%d\t%s\t%s\t%s\t%s\t%s",
					it.Seq, it.ID[:8], it.Status, dash(it.Priority), age(it.Created), dueStr)
				if hasAssignee {
					row += fmt.Sprintf("\t%s", dash(it.Assignee))
				}
				row += fmt.Sprintf("\t%s\n", trunc(it.Title, 60))
				fmt.Fprint(w, row)
			}
			return w.Flush()
		},
	}
	cmd.Flags().StringVar(&status, "status", "", "filter by status (todo|assigned|doing|blocked|done)")
	cmd.Flags().BoolVar(&all, "all", false, "include done tasks")
	cmd.Flags().BoolVar(&reverse, "reverse", false, "newest first (default is sequential, #1 first)")
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
			st, _, err := sf()
			if err != nil {
				return err
			}
			it, err := ResolveRef(st, args[0])
			if err != nil {
				return err
			}
			if jsonOut {
				return emitJSON(cmd, it)
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "#%d  %s  %s\n\n", it.Seq, it.ID, it.Title)
			fmt.Fprintf(w, "  status    %s\n", it.Status)
			if it.Priority != "" {
				fmt.Fprintf(w, "  priority  %s\n", it.Priority)
			}
			fmt.Fprintf(w, "  created   %s\n", it.Created.Format(time.RFC3339))
			if it.Closed != nil {
				fmt.Fprintf(w, "  done      %s\n", it.Closed.Format(time.RFC3339))
			}
			if it.Due != nil {
				fmt.Fprintf(w, "  due       %s\n", it.Due.Format(time.RFC3339))
			}
			if it.Recurring != "" {
				fmt.Fprintf(w, "  recurring %s\n", it.Recurring)
			}
			if it.Assignee != "" {
				fmt.Fprintf(w, "  assignee  %s\n", it.Assignee)
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
			st, _, err := sf()
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
			st, _, err := sf()
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
			st, _, err := sf()
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
	var dueStr, recurring, assignee string
	cmd := &cobra.Command{
		Use:   "edit <id|prefix>",
		Short: "modify a task's title/priority/note",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, _, err := sf()
			if err != nil {
				return err
			}
			it, err := ResolveRef(st, args[0])
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
			if cmd.Flags().Changed("due") {
				due, err := parseDue(dueStr)
				if err != nil {
					return err
				}
				it.Due = due
			}
			if cmd.Flags().Changed("recurring") {
				it.Recurring = recurring
			}
			if cmd.Flags().Changed("assignee") {
				it.Assignee = assignee
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
	cmd.Flags().StringVar(&dueStr, "due", "", "deadline (e.g. 2026-07-20, +3d)")
	cmd.Flags().StringVar(&recurring, "recurring", "", "cadence (daily, weekly, 24h, cron)")
	cmd.Flags().StringVar(&assignee, "assignee", "", "who is working the item")
	return cmd
}

func newRmCmd(sf storeFunc) *cobra.Command {
	return &cobra.Command{
		Use:   "rm <id|prefix>",
		Short: "remove a task",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, _, err := sf()
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

func parseDue(s string) (*time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	if strings.HasPrefix(s, "+") {
		s = s[1:]
		if strings.HasSuffix(s, "d") {
			days, err := strconv.Atoi(s[:len(s)-1])
			if err != nil {
				return nil, fmt.Errorf("invalid relative days: %s", s)
			}
			t := time.Now().UTC().AddDate(0, 0, days)
			return &t, nil
		}
		if strings.HasSuffix(s, "h") {
			hours, err := strconv.Atoi(s[:len(s)-1])
			if err != nil {
				return nil, fmt.Errorf("invalid relative hours: %s", s)
			}
			t := time.Now().UTC().Add(time.Duration(hours) * time.Hour)
			return &t, nil
		}
		if d, err := time.ParseDuration(s); err == nil {
			t := time.Now().UTC().Add(d)
			return &t, nil
		}
		return nil, fmt.Errorf("invalid relative due: +%s", s)
	}
	if t, err := time.Parse("2006-01-02T15:04", s); err == nil {
		return &t, nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return &t, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return &t, nil
	}
	return nil, fmt.Errorf("invalid due date format: %s", s)
}
