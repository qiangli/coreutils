package weave

// A sprint goal is the durable contract. Story status and priority remain in
// the repo todo stores; this file only keeps stable references and derives the
// checklist/index every time it is read. That prevents a second, stale copy of
// either progress or ordering from forming on the sprint card.

import (
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/qiangli/coreutils/pkg/issue"
	"github.com/qiangli/coreutils/pkg/room"
	todopkg "github.com/qiangli/coreutils/pkg/todo"
	"github.com/qiangli/coreutils/pkg/weavecli"
	"github.com/spf13/cobra"
)

type sprintStoryRef struct {
	Repo string `json:"repo"`
	ID   string `json:"id"`
}

type sprintGoalItem struct {
	ID           string           `json:"id"`
	Text         string           `json:"text"`
	Stories      []sprintStoryRef `json:"stories,omitempty"`
	GateRequired bool             `json:"gate_required,omitempty"`
	Evidence     string           `json:"evidence,omitempty"`
}

type sprintExecution struct {
	PriorityFirst bool            `json:"priority_first"`
	CurrentFocus  *sprintStoryRef `json:"current_focus,omitempty"`
	Override      string          `json:"override_reason,omitempty"`
}

type sprintStoryState struct {
	Ref      sprintStoryRef `json:"ref"`
	Title    string         `json:"title"`
	Status   string         `json:"status"`
	Priority string         `json:"priority,omitempty"`
	Seq      int            `json:"seq,omitempty"`
	Missing  bool           `json:"missing,omitempty"`
}

// sprintInboxDeliveryLive reports whether mail addressed to this owner can
// actually arrive. It is a THIN PROJECTION of room.OwnerTransportFor and holds
// no logic of its own, deliberately.
//
// It used to carry a private copy of that check, and two predicates answering
// one question about one seat is the defect sprint 105 was opened to fix: the
// board and `bashy agents` disagreed about the same conductor at the same
// instant. A second copy here would have rebuilt exactly that.
//
// ONE BEHAVIOUR CHANGED IN THE MERGE, on purpose rather than by inheritance.
// The old copy accepted CapInboxStream ONLY when the card's Mode was
// "sprint-inbox", so a live `bashy inbox --watch --as X` (Mode "inbox") did not
// count as reachable. It should: an agent holding its own inbox watch open has
// undertaken to read it, which is the entire content of the attached rung. The
// shared predicate counts it, so this now reports reachable in a case that
// previously read as unreachable.
//
// The old copy also required card.Nick == owner. The shared predicate resolves
// through AgentClaimID with the legacy bare-name fallback, which is the same
// identity check every other caller uses.
func sprintInboxDeliveryLive(owner string) bool {
	transport, _ := room.OwnerTransportFor(owner)
	return transport.Deliverable()
}

// sprintReadyLine tells the new conductor the ONE thing it now has to do.
//
// It used to report NOT READY unless a stream was attached, which named a
// condition rather than an action and sent an agent off to arrange machinery.
// Reading your inbox is the whole job: it is how mail arrives and it is what
// keeps the seat live (RefreshSprintOwnerActivity).
func sprintReadyLine(owner string) string {
	return fmt.Sprintf("next: `bashy inbox --as %s` (reads your mail and keeps the seat live; "+
		"`--watch` to stay attached) · procedure: `bashy skills show inbox`", owner)
}

func normalizeStoryRoot(root string) (string, error) {
	if strings.TrimSpace(root) == "" {
		if found, ok := todopkg.FindGitRoot(); ok {
			root = found
		} else {
			return "", fmt.Errorf("no git repo here; pass --repo <root>")
		}
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

func sprintStoryRoots(s *weaveStory) []string {
	seen := map[string]bool{}
	var roots []string
	for _, root := range s.StoryRoots {
		if r, err := normalizeStoryRoot(root); err == nil && !seen[r] {
			seen[r] = true
			roots = append(roots, r)
		}
	}
	// The current checkout is a useful zero-configuration source, but is never
	// persisted by a read. `sprint track` makes it durable for later handoffs.
	if r, err := normalizeStoryRoot(""); err == nil && !seen[r] {
		roots = append(roots, r)
	}
	return roots
}

func loadSprintStories(s *weaveStory) ([]sprintStoryState, error) {
	seen := map[string]bool{}
	var out []sprintStoryState
	for _, root := range sprintStoryRoots(s) {
		items, err := todopkg.List(todopkg.RepoStore(root), "")
		if err != nil {
			return nil, fmt.Errorf("stories in %s: %w", root, err)
		}
		for _, it := range items {
			if it.Sprint != s.ID {
				continue
			}
			key := root + "\x00" + it.ID
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, sprintStoryState{Ref: sprintStoryRef{Repo: root, ID: it.ID}, Title: it.Title, Status: it.Status, Priority: it.Priority, Seq: it.Seq})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if a, b := todopkg.PriorityRank(out[i].Priority), todopkg.PriorityRank(out[j].Priority); a != b {
			return a < b
		}
		if out[i].Seq != out[j].Seq {
			return out[i].Seq < out[j].Seq
		}
		return out[i].Ref.ID < out[j].Ref.ID
	})
	return out, nil
}

func resolveSprintStory(ref sprintStoryRef) sprintStoryState {
	it, err := todopkg.ResolveRef(todopkg.RepoStore(ref.Repo), ref.ID)
	if err != nil {
		return sprintStoryState{Ref: ref, Status: "missing", Missing: true}
	}
	return sprintStoryState{Ref: sprintStoryRef{Repo: ref.Repo, ID: it.ID}, Title: it.Title, Status: it.Status, Priority: it.Priority, Seq: it.Seq}
}

func sprintGoalDone(g sprintGoalItem) bool {
	if len(g.Stories) == 0 {
		return strings.TrimSpace(g.Evidence) != ""
	}
	for _, ref := range g.Stories {
		story := resolveSprintStory(ref)
		if story.Missing || (story.Status != todopkg.StatusDone && story.Status != issue.StatusClosed) {
			return false
		}
	}
	return !g.GateRequired || strings.TrimSpace(g.Evidence) != ""
}

func sprintGoalDangling(g sprintGoalItem) []string {
	var out []string
	for _, ref := range g.Stories {
		if resolveSprintStory(ref).Missing {
			out = append(out, ref.Repo+"#"+ref.ID)
		}
	}
	return out
}

func sprintUncheckedGoals(s *weaveStory) []string {
	var out []string
	for _, g := range s.Goal {
		if !sprintGoalDone(g) {
			out = append(out, g.ID)
		}
	}
	return out
}

func nextSprintStory(s *weaveStory) (*sprintStoryState, error) {
	stories, err := loadSprintStories(s)
	if err != nil {
		return nil, err
	}
	for i := range stories {
		if stories[i].Status != todopkg.StatusDone && stories[i].Status != issue.StatusClosed && stories[i].Status != todopkg.StatusBlocked {
			return &stories[i], nil
		}
	}
	return nil, nil
}

func renderSprintExecution(w io.Writer, s *weaveStory) {
	fmt.Fprintln(w, "  execution:  PRIORITY-FIRST (P0 → P1 → P2 → P3); lower-priority focus requires a recorded override")
	if s.Execution.CurrentFocus != nil {
		cur := resolveSprintStory(*s.Execution.CurrentFocus)
		fmt.Fprintf(w, "  focus:      [%s/%s] %s (%s)\n", cur.Priority, cur.Status, cur.Title, cur.Ref.ID)
	}
	if next, err := nextSprintStory(s); err == nil && next != nil {
		fmt.Fprintf(w, "  next:       [%s/%s] %s (%s)\n", next.Priority, next.Status, next.Title, next.Ref.ID)
	}
	if len(s.Goal) > 0 {
		fmt.Fprintln(w, "  ── goal checklist (derived from story closure + evidence) ──")
		for _, g := range s.Goal {
			mark := " "
			if sprintGoalDone(g) {
				mark = "x"
			}
			warning := ""
			if dangling := sprintGoalDangling(g); len(dangling) > 0 {
				warning = "  WARNING dangling: " + strings.Join(dangling, ", ")
			}
			fmt.Fprintf(w, "  [%s] %s — %s%s\n", mark, g.ID, g.Text, warning)
		}
	}
}

func newSprintTrackCmd() *cobra.Command {
	var flags weaveOutputFlags
	var repo string
	cmd := &cobra.Command{Use: "track <sprint>", Short: "Add a repo todo store to the sprint's derived story index", Args: cobra.ExactArgs(1)}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("sprint must be an integer: %q", args[0])
		}
		root, err := normalizeStoryRoot(repo)
		if err != nil {
			return err
		}
		return runWeaveStoryMutate(cmd, id, "sprint track", &flags, func(s *weaveStory) (string, error) {
			for _, old := range s.StoryRoots {
				if old == root {
					return fmt.Sprintf("sprint #%d already tracks %s", id, root), nil
				}
			}
			s.StoryRoots = append(s.StoryRoots, root)
			weaveStoryAppend(s, weaveStoryConductorName(s, ""), "system", "tracked story repo "+root)
			return fmt.Sprintf("sprint #%d tracks %s", id, root), nil
		})
	}
	cmd.Flags().StringVar(&repo, "repo", "", "repo root (default current git repo)")
	flags.attach(cmd)
	return cmd
}

func newSprintGoalCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "goal", Short: "Manage the durable, derived sprint goal checklist"}
	cmd.AddCommand(newSprintGoalAddCmd(), newSprintGoalLinkCmd(), newSprintGoalEvidenceCmd())
	return cmd
}

func findSprintGoal(s *weaveStory, id string) *sprintGoalItem {
	for i := range s.Goal {
		if s.Goal[i].ID == id {
			return &s.Goal[i]
		}
	}
	return nil
}

func newSprintGoalAddCmd() *cobra.Command {
	var flags weaveOutputFlags
	var goalID, text, story, repo string
	var gate bool
	cmd := &cobra.Command{Use: "add <sprint>", Short: "Add a required outcome to the sprint checklist (--story covers one in the same step)", Args: cobra.ExactArgs(1)}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return err
		}
		goalID, text = strings.TrimSpace(goalID), strings.TrimSpace(text)
		if goalID == "" || text == "" {
			return fmt.Errorf("--id and --text are required")
		}
		// CREATE AND LINK IN ONE STEP. Covering a newly reported bug was two
		// commands (add, then link), and the second is the one that actually
		// closes the coverage gap — so the common case of "a p0 just arrived,
		// put it in the plan" was exactly the case most likely to be left half
		// done, leaving the plan silently not describing the sprint again.
		var linkRoot string
		var linkItem *issue.Issue
		if strings.TrimSpace(story) != "" {
			root, err := normalizeStoryRoot(repo)
			if err != nil {
				return err
			}
			it, err := todopkg.ResolveRef(todopkg.RepoStore(root), story)
			if err != nil {
				return err
			}
			if it.Sprint != id {
				return fmt.Errorf("story %s belongs to sprint #%d, not #%d", it.ID, it.Sprint, id)
			}
			linkRoot, linkItem = root, it
		}
		return runWeaveStoryMutate(cmd, id, "sprint goal add", &flags, func(s *weaveStory) (string, error) {
			if findSprintGoal(s, goalID) != nil {
				return "", fmt.Errorf("goal item %q already exists", goalID)
			}
			item := sprintGoalItem{ID: goalID, Text: text, GateRequired: gate}
			msg := fmt.Sprintf("sprint #%d added goal %s", id, goalID)
			if linkRoot != "" {
				item.Stories = append(item.Stories, sprintStoryRef{Repo: linkRoot, ID: linkItem.ID})
				found := false
				for _, old := range s.StoryRoots {
					found = found || old == linkRoot
				}
				if !found {
					s.StoryRoots = append(s.StoryRoots, linkRoot)
				}
				msg += " linked to story " + linkItem.ID
			}
			s.Goal = append(s.Goal, item)
			weaveStoryAppend(s, weaveStoryConductorName(s, ""), "system", "added goal item "+goalID)
			if linkRoot != "" {
				weaveStoryAppend(s, weaveStoryConductorName(s, ""), "system",
					"linked story "+linkItem.ID+" to goal "+goalID)
			}
			return msg, nil
		})
	}
	cmd.Flags().StringVar(&goalID, "id", "", "stable checklist id")
	cmd.Flags().StringVar(&text, "text", "", "required outcome")
	cmd.Flags().StringVar(&story, "story", "", "todo id or unique prefix to cover with this goal, in one step")
	cmd.Flags().StringVar(&repo, "repo", "", "repo root holding the story (default: this checkout)")
	cmd.Flags().BoolVar(&gate, "gate-required", false, "require recorded evidence after stories close")
	flags.attach(cmd)
	return cmd
}

func newSprintGoalLinkCmd() *cobra.Command {
	var flags weaveOutputFlags
	var repo, story string
	cmd := &cobra.Command{Use: "link <sprint> <goal>", Short: "Link a repo story to a goal item", Args: cobra.ExactArgs(2)}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return err
		}
		root, err := normalizeStoryRoot(repo)
		if err != nil {
			return err
		}
		it, err := todopkg.ResolveRef(todopkg.RepoStore(root), story)
		if err != nil {
			return err
		}
		if it.Sprint != id {
			return fmt.Errorf("story %s belongs to sprint #%d, not #%d", it.ID, it.Sprint, id)
		}
		return runWeaveStoryMutate(cmd, id, "sprint goal link", &flags, func(s *weaveStory) (string, error) {
			g := findSprintGoal(s, args[1])
			if g == nil {
				return "", fmt.Errorf("goal item %q not found", args[1])
			}
			ref := sprintStoryRef{Repo: root, ID: it.ID}
			for _, old := range g.Stories {
				if old == ref {
					return fmt.Sprintf("goal %s already links %s", g.ID, it.ID), nil
				}
			}
			g.Stories = append(g.Stories, ref)
			found := false
			for _, old := range s.StoryRoots {
				found = found || old == root
			}
			if !found {
				s.StoryRoots = append(s.StoryRoots, root)
			}
			weaveStoryAppend(s, weaveStoryConductorName(s, ""), "system", fmt.Sprintf("linked story %s to goal %s", it.ID, g.ID))
			return fmt.Sprintf("sprint #%d goal %s linked %s", id, g.ID, it.ID), nil
		})
	}
	cmd.Flags().StringVar(&repo, "repo", "", "story repo root (default current)")
	cmd.Flags().StringVar(&story, "story", "", "todo id or unique prefix")
	_ = cmd.MarkFlagRequired("story")
	flags.attach(cmd)
	return cmd
}

func newSprintGoalEvidenceCmd() *cobra.Command {
	var flags weaveOutputFlags
	var message string
	cmd := &cobra.Command{Use: "evidence <sprint> <goal>", Short: "Record gate evidence or human approval for a goal item", Args: cobra.ExactArgs(2)}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return err
		}
		if strings.TrimSpace(message) == "" {
			return fmt.Errorf("-m <evidence> required")
		}
		return runWeaveStoryMutate(cmd, id, "sprint goal evidence", &flags, func(s *weaveStory) (string, error) {
			g := findSprintGoal(s, args[1])
			if g == nil {
				return "", fmt.Errorf("goal item %q not found", args[1])
			}
			g.Evidence = strings.TrimSpace(message)
			weaveStoryAppend(s, weaveStoryConductorName(s, ""), "review", "goal "+g.ID+" evidence: "+g.Evidence)
			return fmt.Sprintf("sprint #%d goal %s evidence recorded", id, g.ID), nil
		})
	}
	cmd.Flags().StringVarP(&message, "message", "m", "", "gate result or human approval")
	flags.attach(cmd)
	return cmd
}

func newSprintNextCmd() *cobra.Command {
	var flags weaveOutputFlags
	cmd := &cobra.Command{Use: "next <sprint>", Short: "Show the highest-priority runnable sprint story", Args: cobra.ExactArgs(1)}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return err
		}
		dir, err := weaveStoryDir(cmd, flags.mode(), "sprint next")
		if err != nil {
			return err
		}
		q, err := loadWeaveQueue(dir)
		if err != nil {
			return err
		}
		s := findWeaveStory(q, id)
		if s == nil {
			return fmt.Errorf("sprint #%d not found", id)
		}
		next, err := nextSprintStory(s)
		if err != nil {
			return err
		}
		if flags.mode() == weavecli.OutputJSON {
			return ec(emitOK(cmd.OutOrStdout(), flags.mode(), "sprint next", map[string]any{"sprint": id, "story": next}))
		}
		if next == nil {
			fmt.Fprintf(cmd.OutOrStdout(), "sprint next: sprint #%d has no runnable unchecked story\n", id)
			return nil
		}
		fmt.Fprintf(cmd.OutOrStdout(), "sprint next: [%s/%s] %s — %s\n", next.Priority, next.Status, next.Ref.ID, next.Title)
		return nil
	}
	flags.attach(cmd)
	return cmd
}

func newSprintFocusCmd() *cobra.Command {
	var flags weaveOutputFlags
	var repo, override string
	cmd := &cobra.Command{Use: "focus <sprint> <story>", Short: "Set current focus, enforcing priority-first execution", Args: cobra.ExactArgs(2)}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return err
		}
		root, err := normalizeStoryRoot(repo)
		if err != nil {
			return err
		}
		it, err := todopkg.ResolveRef(todopkg.RepoStore(root), args[1])
		if err != nil {
			return err
		}
		if it.Sprint != id {
			return fmt.Errorf("story %s belongs to sprint #%d, not #%d", it.ID, it.Sprint, id)
		}
		return runWeaveStoryMutate(cmd, id, "sprint focus", &flags, func(s *weaveStory) (string, error) {
			owner := weaveStoryConductorName(s, "")
			if !sprintInboxDeliveryLive(owner) {
				return "", fmt.Errorf("sprint owner %s has no verified managed inbox delivery; launch it through Bashy (a terminal `inbox --watch` alone cannot wake the agent)", owner)
			}
			next, err := nextSprintStory(s)
			if err != nil {
				return "", err
			}
			if next != nil && todopkg.PriorityRank(it.Priority) > todopkg.PriorityRank(next.Priority) && strings.TrimSpace(override) == "" {
				return "", fmt.Errorf("priority-first policy: %s is %s while runnable %s is %s; pass --override <reason> to record an exception", it.ID, it.Priority, next.Ref.ID, next.Priority)
			}
			ref := sprintStoryRef{Repo: root, ID: it.ID}
			s.Execution = sprintExecution{PriorityFirst: true, CurrentFocus: &ref, Override: strings.TrimSpace(override)}
			body := fmt.Sprintf("focused story %s (%s)", it.ID, it.Priority)
			if s.Execution.Override != "" {
				body += "; priority override: " + s.Execution.Override
			}
			weaveStoryAppend(s, weaveStoryConductorName(s, ""), "decision", body)
			return fmt.Sprintf("sprint #%d focus %s", id, it.ID), nil
		})
	}
	cmd.Flags().StringVar(&repo, "repo", "", "story repo root (default current)")
	cmd.Flags().StringVar(&override, "override", "", "required reason when bypassing a runnable higher-priority story")
	flags.attach(cmd)
	return cmd
}
