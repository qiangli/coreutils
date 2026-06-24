// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package dag

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/qiangli/coreutils/pkg/weavecli"
)

// newDagCmd builds the `dag` command. It is the agentic front door: a single
// command that lists targets (--list) or runs them as a dependency DAG. Output
// follows the weavecli envelope convention (DHNT_AGENT=1 forces --json).
func newDagCmd() *cobra.Command {
	var (
		listF, jsonF, plainF, quietF, keepGoing bool
		fileArg                                 string
	)
	cmd := &cobra.Command{
		Use:   "dag [flags] [target ...]",
		Short: "Run markdown-defined targets as a dependency DAG",
		Long: `dag runs targets defined as headings in a markdown file (DAG.md) as a
real dependency graph — an agent-first replacement for make.

Each target is a heading with an optional description, metadata lines
(Requires:/Inputs:/Sources:/Generates:), and a fenced code block run through
the in-process shell. Targets execute in topological order; a target whose
dependency failed is skipped.

  dag --list                 # show targets (add --json for machine output)
  dag build                  # run "build" and its dependencies
  dag test lint              # run several targets and their closures
  dag build VERSION=v1.2     # make-style KEY=VALUE variable overrides
  dag --file pipeline.md ci  # use an explicit file

With no target, dag runs the file's default goal — the frontmatter
"default:" key, or a target named "default" — and otherwise lists the
targets (like a Makefile whose .DEFAULT_GOAL is help).`,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			mode := weavecli.ResolveOutputMode(jsonF, plainF, quietF)
			out, errOut := cmd.OutOrStdout(), cmd.ErrOrStderr()

			path := fileArg
			if path == "" {
				p, err := Discover(".")
				if err != nil {
					return emitErr(errOut, mode, err)
				}
				path = p
			}
			doc, err := ParseFile(path)
			if err != nil {
				return emitErr(errOut, mode, err)
			}
			g, err := BuildGraph(doc)
			if err != nil {
				return emitErr(errOut, mode, err)
			}

			if listF {
				return runList(out, mode, doc)
			}

			// make-style invocation: KEY=VALUE args are variable overrides
			// (injected into every target's environment), the rest are targets.
			targets, overrides := splitOverrides(args)

			if len(targets) == 0 {
				d := defaultTarget(doc)
				if d == "" {
					// No configured default goal — list targets, like a
					// Makefile whose .DEFAULT_GOAL is `help`.
					return runList(out, mode, doc)
				}
				targets = []string{d}
			}

			absPath, _ := filepath.Abs(path)
			eng := &Engine{
				Graph:       g,
				Dir:         filepath.Dir(absPath),
				Env:         append(os.Environ(), overrides...),
				Concurrency: 1,
				FailFast:    !keepGoing,
				Verbose:     mode == weavecli.OutputAuto || mode == weavecli.OutputPlain,
				Capture:     mode == weavecli.OutputJSON,
				Stdout:      out,
				Stderr:      errOut,
			}
			report, err := eng.Run(cmd.Context(), targets...)
			if err != nil {
				return emitErr(errOut, mode, err)
			}
			return runReport(out, errOut, mode, path, targets, report)
		},
	}
	cmd.CompletionOptions.DisableDefaultCmd = true
	cmd.Flags().BoolVarP(&listF, "list", "l", false, "List targets and exit")
	cmd.Flags().BoolVar(&jsonF, "json", false, "Emit machine-readable envelope")
	cmd.Flags().BoolVar(&plainF, "plain", false, "Plain-text output, no banners")
	cmd.Flags().BoolVar(&quietF, "quiet", false, "Suppress banners; final result only")
	cmd.Flags().BoolVarP(&keepGoing, "keep-going", "k", false, "Continue past a failed target")
	cmd.Flags().StringVarP(&fileArg, "file", "f", "", "DAG markdown file (default: discover DAG.md)")
	return cmd
}

// defaultTarget resolves the goal for a no-argument invocation: the frontmatter
// `default:` key (make's .DEFAULT_GOAL) wins, then a target literally named
// "default". Empty means "no default" — the caller lists targets instead of
// guessing one to run.
func defaultTarget(doc *Document) string {
	if doc.Default != "" {
		return doc.Default
	}
	if _, ok := doc.Lookup("default"); ok {
		return "default"
	}
	return ""
}

// splitOverrides separates make-style `KEY=VALUE` variable overrides from target
// names. KEY must be a valid shell identifier; everything else is a target.
func splitOverrides(args []string) (targets, overrides []string) {
	for _, a := range args {
		if i := strings.IndexByte(a, '='); i > 0 && isVarName(a[:i]) {
			overrides = append(overrides, a)
		} else {
			targets = append(targets, a)
		}
	}
	return targets, overrides
}

func isVarName(s string) bool {
	for i, r := range s {
		switch {
		case r == '_':
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
		case i > 0 && r >= '0' && r <= '9':
		default:
			return false
		}
	}
	return s != ""
}

// --- result shapes (envelope Result payloads) ---

type taskSummary struct {
	Name      string   `json:"name"`
	Desc      string   `json:"description,omitempty"`
	Requires  []string `json:"requires,omitempty"`
	Inputs    []string `json:"inputs,omitempty"`
	Sources   []string `json:"sources,omitempty"`
	Generates []string `json:"generates,omitempty"`
	Lang      string   `json:"lang,omitempty"`
}

type listResult struct {
	File  string        `json:"file"`
	Tasks []taskSummary `json:"tasks"`
}

type runItem struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	ExitCode   int    `json:"exit_code"`
	DurationMS int64  `json:"duration_ms"`
	Stdout     string `json:"stdout,omitempty"`
	Stderr     string `json:"stderr,omitempty"`
	Error      string `json:"error,omitempty"`
}

type runResult struct {
	File    string    `json:"file"`
	Targets []string  `json:"targets"`
	Tasks   []runItem `json:"tasks"`
	Failed  bool      `json:"failed"`
}

func runList(out io.Writer, mode weavecli.OutputMode, doc *Document) error {
	res := listResult{File: doc.Path}
	for _, name := range doc.Order {
		t, _ := doc.Lookup(name)
		res.Tasks = append(res.Tasks, taskSummary{
			Name: t.Name, Desc: t.Desc, Requires: t.Requires,
			Inputs: t.Inputs, Sources: t.Sources, Generates: t.Generates, Lang: t.Lang,
		})
	}
	if mode == weavecli.OutputJSON {
		emitOK(out, res)
		return nil
	}
	for _, t := range res.Tasks {
		if t.Desc != "" {
			fmt.Fprintf(out, "%-24s %s\n", t.Name, firstLine(t.Desc))
		} else {
			fmt.Fprintln(out, t.Name)
		}
		if len(t.Requires) > 0 {
			fmt.Fprintf(out, "%-24s   requires: %s\n", "", strings.Join(t.Requires, ", "))
		}
	}
	return nil
}

func runReport(out, errOut io.Writer, mode weavecli.OutputMode, path string, targets []string, report RunReport) error {
	res := runResult{File: path, Targets: targets, Failed: report.Failed}
	for _, r := range report.Results {
		item := runItem{
			Name: r.Name, Status: r.Status.String(),
			ExitCode: r.ExitCode, DurationMS: r.Duration.Milliseconds(),
			Stdout: r.Stdout, Stderr: r.Stderr,
		}
		if r.Err != nil {
			item.Error = r.Err.Error()
		}
		res.Tasks = append(res.Tasks, item)
	}

	if report.Failed {
		if mode == weavecli.OutputJSON {
			res.Failed = true
			emitErrEnvelope(errOut, mode, weavecli.ExitGenericFail,
				errf(weavecli.ExitGenericFail, "one or more targets failed"), res)
		} else if mode != weavecli.OutputQuiet {
			fmt.Fprintf(errOut, "dag: %d/%d targets failed\n", countFailed(report), len(report.Results))
		}
		return &Error{Code: weavecli.ExitGenericFail, Msg: "one or more targets failed"}
	}

	if mode == weavecli.OutputJSON {
		emitOK(out, res)
	} else if mode != weavecli.OutputQuiet {
		fmt.Fprintf(out, "dag: %d target(s) ok\n", len(report.Results))
	}
	return nil
}

func countFailed(report RunReport) int {
	n := 0
	for _, r := range report.Results {
		if r.Status == StatusFailed || r.Status == StatusSkipped {
			n++
		}
	}
	return n
}

// emitErr writes the error envelope/line and returns the error so cobra's
// Execute() reports non-zero (ExitCodeOf recovers the code in the host).
func emitErr(errOut io.Writer, mode weavecli.OutputMode, err error) error {
	code := ExitCodeOf(err)
	emitErrEnvelope(errOut, mode, code, err, nil)
	return &Error{Code: code, Msg: err.Error()}
}

// emitOK encodes a status=ok dag envelope (schema "dag-v1") to w. Unlike
// weavecli.EmitOK it stamps the dag schema version, not weave's "loom-v2".
func emitOK(w io.Writer, result any) {
	encodeEnvelope(w, weavecli.Envelope{
		SchemaVersion: SchemaVersion,
		Command:       "dag",
		Status:        "ok",
		Result:        result,
	})
}

// emitErrEnvelope encodes a status=error dag envelope (or a plain "dag: msg"
// line outside JSON mode). result is attached when present (e.g. partial run
// state) so an agent sees which targets failed.
func emitErrEnvelope(w io.Writer, mode weavecli.OutputMode, code int, err error, result any) {
	if mode != weavecli.OutputJSON {
		fmt.Fprintf(w, "dag: %s\n", err.Error())
		return
	}
	encodeEnvelope(w, weavecli.Envelope{
		SchemaVersion: SchemaVersion,
		Command:       "dag",
		Status:        "error",
		Result:        result,
		Error:         &weavecli.EnvelopeError{Code: codeString(code), Message: err.Error()},
	})
}

func encodeEnvelope(w io.Writer, e weavecli.Envelope) {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(e)
}

func codeString(code int) string {
	switch code {
	case weavecli.ExitInvalidArg:
		return "invalid_arg"
	case weavecli.ExitPrecondFail:
		return "precondition_failed"
	case weavecli.ExitStateConflict:
		return "state_conflict"
	case weavecli.ExitDepUnhealthy:
		return "dependency_unhealthy"
	case weavecli.ExitOK:
		return "ok"
	default:
		return "generic_failure"
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
