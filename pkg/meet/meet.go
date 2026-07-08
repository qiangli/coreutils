package meet

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

// NewMeetCmd returns the `bashy meet` command tree.
func NewMeetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "meet",
		Short: "multi-participant deliberation session with a notes-only secretary",
		Long: "Run a turn-taking planning meeting across agentic CLIs and a human.\n" +
			"A dedicated notes-only secretary keeps the minutes and files them to\n" +
			"docs/meetings/ on close. See dhnt/docs/bashy-meet.md.",
	}
	cmd.CompletionOptions.DisableDefaultCmd = true
	cmd.AddCommand(newStartCmd(), newTellCmd(), newRoundCmd(), newCloseCmd(), newListCmd(), newResumeCmd())
	return cmd
}

func humanName() string {
	if u := strings.TrimSpace(os.Getenv("USER")); u != "" {
		return u
	}
	return "you"
}

func drivability(name string) string {
	if _, err := exec.LookPath(name); err != nil {
		return "not installed"
	}
	if name == "codex" && runtime.GOOS == "darwin" {
		return "installed (headless-caveated on macOS — PTY fallback)"
	}
	return "installed"
}

func printPreview(w io.Writer, st *State) {
	fmt.Fprintln(w, "meet: resolved session")
	fmt.Fprintf(w, "  id           %s\n", st.ID)
	fmt.Fprintf(w, "  secretary    %s  role=secretary (notes-only) — %s\n", st.Secretary, drivability(st.Secretary))
	for i, p := range st.Participants {
		label := "participants"
		if i > 0 {
			label = "            "
		}
		fmt.Fprintf(w, "  %s %s  %s\n", label, p, drivability(p))
	}
	if len(st.Participants) == 0 {
		fmt.Fprintln(w, "  participants (none)")
	}
	fmt.Fprintf(w, "  human        %s\n", st.Human)
	dir, _ := storeDir(st.ID)
	fmt.Fprintf(w, "  store        %s/\n", dir)
	fmt.Fprintf(w, "  minutes →    %s\n", minutesPath(st))
	fmt.Fprintf(w, "  turn model   %s round-robin\n", st.Mode)
}

func newStartCmd() *cobra.Command {
	var topic, assistant, mode, out string
	var participants, agenda []string
	var rounds int
	var dry, nonInteractive bool
	cmd := &cobra.Command{
		Use:   "start --topic TEXT [--participant AGENT ...]",
		Short: "start a meeting (enters the REPL unless --non-interactive)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(topic) == "" {
				return fmt.Errorf("meet: --topic is required")
			}
			if assistant == "" {
				assistant = "claude" // dedicated notes-only secretary default
			}
			if mode == "" {
				mode = "sequential"
			}
			cwd, _ := os.Getwd()
			st := &State{
				ID: newID(topic, nowFn()), Topic: topic, Agenda: agenda,
				Secretary: assistant, Participants: participants, Human: humanName(),
				Mode: mode, Status: "open", Cwd: cwd, Out: out, Created: nowFn(),
			}
			w := cmd.OutOrStdout()
			printPreview(w, st)
			if dry {
				fmt.Fprintln(w, "  (dry-run: no agents launched)")
				return nil
			}
			if err := st.save(); err != nil {
				return err
			}
			for _, a := range agenda {
				_, _ = record(st, "agenda", "chair", "", a)
			}
			if nonInteractive {
				for i := 0; i < rounds; i++ {
					q := ""
					if i < len(agenda) {
						q = agenda[i]
					}
					for _, e := range runRound(cmd.Context(), st, q, nil) {
						fmt.Fprintf(w, "%s> %s\n", e.Speaker, oneLine(e.Text))
					}
				}
				path, err := closeMeeting(cmd.Context(), st, true, nil)
				if err != nil {
					return err
				}
				fmt.Fprintf(w, "wrote %s\n", path)
				return nil
			}
			return repl(cmd, st)
		},
	}
	f := cmd.Flags()
	f.StringVar(&topic, "topic", "", "meeting topic (required)")
	f.StringVar(&assistant, "assistant", "", "secretary agent (notes only; default claude)")
	f.StringArrayVar(&participants, "participant", nil, "participant agent (repeatable)")
	f.StringArrayVar(&agenda, "agenda", nil, "agenda item (repeatable)")
	f.StringVar(&mode, "mode", "sequential", "turn mode (P0: sequential)")
	f.StringVar(&out, "out", "docs", "filing target: docs | kb | <path>")
	f.IntVar(&rounds, "rounds", 1, "rounds to run in --non-interactive mode")
	f.BoolVar(&dry, "dry-run", false, "print the resolved session and exit")
	f.BoolVar(&nonInteractive, "non-interactive", false, "run rounds then close, no REPL")
	return cmd
}

// repl is the interactive meeting loop.
func repl(cmd *cobra.Command, st *State) error {
	w := cmd.OutOrStdout()
	sc := bufio.NewScanner(cmd.InOrStdin())
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	fmt.Fprintf(w, "\nmeet %s · secretary=%s(notes-only) · participants: %s\n",
		st.ID, st.Secretary, strings.Join(st.Participants, ", "))
	fmt.Fprintln(w, "commands: <text> | @name <text> | /round | /decision <t> | /action owner: task | /agenda <t> | /close")
	fmt.Fprint(w, "you> ")
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			fmt.Fprint(w, "you> ")
			continue
		}
		switch {
		case line == "/close":
			path, err := closeMeeting(cmd.Context(), st, true, nil)
			if err != nil {
				return err
			}
			fmt.Fprintf(w, "wrote %s\n", path)
			return nil
		case line == "/round":
			for _, e := range runRound(cmd.Context(), st, currentAgenda(st), nil) {
				fmt.Fprintf(w, "%s> %s\n", e.Speaker, oneLine(e.Text))
			}
		case strings.HasPrefix(line, "/decision "):
			t := strings.TrimSpace(line[len("/decision "):])
			_, _ = record(st, "decision", st.Human, "", t)
			fmt.Fprintf(w, "⏺ recorded DECISION: %s\n", t)
		case strings.HasPrefix(line, "/action "):
			t := strings.TrimSpace(line[len("/action "):])
			_, _ = record(st, "action", st.Human, "", t)
			fmt.Fprintf(w, "⏺ recorded ACTION: %s\n", t)
		case strings.HasPrefix(line, "/agenda "):
			t := strings.TrimSpace(line[len("/agenda "):])
			st.Agenda = append(st.Agenda, t)
			_ = st.save()
			_, _ = record(st, "agenda", "chair", "", t)
		case strings.HasPrefix(line, "@"):
			name, text, _ := strings.Cut(strings.TrimPrefix(line, "@"), " ")
			ev, _ := runTurn(cmd.Context(), st, name, text, nil)
			fmt.Fprintf(w, "%s> %s\n", ev.Speaker, oneLine(ev.Text))
		default:
			_, _ = record(st, "human", st.Human, "human", line)
		}
		fmt.Fprint(w, "you> ")
	}
	return sc.Err()
}

func newTellCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tell <id> <text...>",
		Short: "append a human contribution to a session",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := loadState(args[0])
			if err != nil {
				return err
			}
			_, err = record(st, "human", st.Human, "human", strings.Join(args[1:], " "))
			return err
		},
	}
}

func newRoundCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "round <id>",
		Short: "run one moderated round across participants",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := loadState(args[0])
			if err != nil {
				return err
			}
			for _, e := range runRound(cmd.Context(), st, currentAgenda(st), nil) {
				fmt.Fprintf(cmd.OutOrStdout(), "%s> %s\n", e.Speaker, oneLine(e.Text))
			}
			return nil
		},
	}
}

func newCloseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "close <id>",
		Short: "run the secretary pass, write and file the minutes",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := loadState(args[0])
			if err != nil {
				return err
			}
			path, err := closeMeeting(cmd.Context(), st, true, nil)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", path)
			return nil
		},
	}
}

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "list saved meetings",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			sessions, err := listSessions()
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			for _, s := range sessions {
				fmt.Fprintf(w, "%-40s  %-8s  %s\n", s.ID, s.Status, s.Topic)
			}
			return nil
		},
	}
}

func newResumeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "resume <id>",
		Short: "reopen a saved meeting in the REPL",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := loadState(args[0])
			if err != nil {
				return err
			}
			return repl(cmd, st)
		},
	}
}
