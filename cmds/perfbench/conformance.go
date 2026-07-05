package perfbenchcmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os/exec" // DEV-ONLY measurement exception — see the package doc.
	"path/filepath"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

// A Case is one invocation compared byte-for-byte between the GNU arm and the
// bashy arm. Cases come from three tiers (spec §4.2): authored flag matrices
// (cmds/<name>/conformance_test.go), mined benchmark invocations, and a seeded
// differential fuzzer. This runner executes an already-materialized case list.
type Case struct {
	Cmd   string   `json:"cmd"`
	Argv  []string `json:"argv"` // args after the command name
	Stdin []byte   `json:"-"`    // optional
	// Fixtures are corpus files the case reads; resolved via rc.Path.
	Fixtures []string `json:"fixtures,omitempty"`
}

// Outcome is the per-case verdict.
type Outcome string

const (
	Match    Outcome = "match"     // byte-identical stdout+stderr+exit
	Diff     Outcome = "diff"      // a fidelity bug
	LoudSkip Outcome = "loud-skip" // bashy returned the contract's "flag not supported" — honest partial
)

// CaseResult records one comparison.
type CaseResult struct {
	Cmd     string   `json:"cmd"`
	Argv    []string `json:"argv"`
	Outcome Outcome  `json:"outcome"`
	Detail  string   `json:"detail,omitempty"` // first-diff summary when Outcome==Diff
}

// runConformance diffs each command's output against GNU and reports a
// per-command / per-group conformance score.
func runConformance(rc *tool.RunContext, args []string) int {
	opt := defaultPerfOptions(rc)
	_ = args // TODO: parse --group, --cmd, --cases <file>

	cases := authoredCases() // the empirical frequency-head matrix (cases.go)
	_ = builtinCases         // superseded; kept for reference
	var results []CaseResult
	for _, c := range cases {
		results = append(results, diffCase(rc, c, opt))
	}
	return emitConformance(rc, opt.Format, results)
}

// diffCase runs one case against both arms and compares.
func diffCase(rc *tool.RunContext, c Case, opt perfOptions) CaseResult {
	gnuOut, gnuErr, gnuCode := runGNU(rc, c, opt)
	byOut, byErr, byCode := runBashyInproc(rc, c)

	res := CaseResult{Cmd: c.Cmd, Argv: c.Argv}
	switch {
	case bytes.Equal(gnuOut, byOut) && bytes.Equal(gnuErr, byErr) && gnuCode == byCode:
		res.Outcome = Match
	case byCode != 0 && looksUnsupported(byErr):
		// bashy loudly declined an unsupported flag — honest partial, not a bug.
		res.Outcome = LoudSkip
	default:
		res.Outcome = Diff
		res.Detail = firstDiff(gnuOut, byOut, gnuErr, byErr, gnuCode, byCode)
	}
	return res
}

func runGNU(rc *tool.RunContext, c Case, opt perfOptions) (out, errb []byte, code int) {
	bin := filepath.Join(opt.GNUPrefix, "bin", c.Cmd)
	cmd := exec.Command(bin, c.Argv...)
	// GNU tools put argv[0] verbatim in their error prefix; exec sets it to the
	// absolute path. Force the bare name so the prefix matches in-process bashy
	// (which uses the bare command name) — otherwise every error case false-DIFFs.
	cmd.Args[0] = c.Cmd
	cmd.Dir = rc.Dir
	cmd.Env = rc.Env
	if c.Stdin != nil {
		cmd.Stdin = bytes.NewReader(c.Stdin)
	}
	var o, e bytes.Buffer
	cmd.Stdout, cmd.Stderr = &o, &e
	code = 0
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			code = 127
		}
	}
	return o.Bytes(), e.Bytes(), code
}

// runBashyInproc runs the case through the in-process tool — the exact code
// path bashy uses in-shell — so conformance measures what ships, not a spawn.
func runBashyInproc(rc *tool.RunContext, c Case) (out, errb []byte, code int) {
	t := tool.Lookup(c.Cmd)
	if t == nil {
		return nil, []byte(fmt.Sprintf("%s: not implemented\n", c.Cmd)), 127
	}
	var o, e bytes.Buffer
	sub := &tool.RunContext{
		Ctx: rc.Ctx, Dir: rc.Dir, Env: rc.Env, FS: rc.FS,
		Stdio: tool.Stdio{In: stdinReader(c.Stdin), Out: &o, Err: &e},
	}
	code = t.Run(sub, c.Argv)
	return o.Bytes(), e.Bytes(), code
}

// emitConformance rolls up per-command and per-group conformance.
func emitConformance(rc *tool.RunContext, format string, results []CaseResult) int {
	// per-command tallies
	type tally struct{ match, diff, skip int }
	perCmd := map[string]*tally{}
	for _, r := range results {
		t := perCmd[r.Cmd]
		if t == nil {
			t = &tally{}
			perCmd[r.Cmd] = t
		}
		switch r.Outcome {
		case Match:
			t.match++
		case Diff:
			t.diff++
		case LoudSkip:
			t.skip++
		}
	}
	// group lookup for a per-group roll-up
	groupOf := map[string]Group{}
	for _, p := range Inventory {
		groupOf[p.Name] = p.Group
	}

	if format == "json" {
		enc := json.NewEncoder(rc.Out)
		enc.SetIndent("", "  ")
		_ = enc.Encode(map[string]any{"schema": "perfbench-conformance-v1", "cases": results})
		return conformanceExit(results)
	}

	var b bytes.Buffer
	fmt.Fprintln(&b, "| command | group | match | diff | loud-skip | conformance% |")
	fmt.Fprintln(&b, "|---|---|---|---|---|---|")
	for cmd, t := range perCmd {
		denom := t.match + t.diff
		pct := 100.0
		if denom > 0 {
			pct = 100 * float64(t.match) / float64(denom)
		}
		fmt.Fprintf(&b, "| %s | %s | %d | %d | %d | %.1f |\n",
			cmd, groupOf[cmd], t.match, t.diff, t.skip, pct)
	}
	// List the actual divergences — the actionable output.
	var diffs []CaseResult
	for _, r := range results {
		if r.Outcome == Diff {
			diffs = append(diffs, r)
		}
	}
	if len(diffs) > 0 {
		fmt.Fprintf(&b, "\n## DIFFs (%d)\n", len(diffs))
		for _, r := range diffs {
			fmt.Fprintf(&b, "- `%s %s`\n  %s\n", r.Cmd, strings.Join(r.Argv, " "), r.Detail)
		}
	}
	rc.Out.Write(b.Bytes())
	return conformanceExit(results)
}

// conformanceExit is nonzero if any case DIFFed — the gate signal.
func conformanceExit(results []CaseResult) int {
	for _, r := range results {
		if r.Outcome == Diff {
			return 1
		}
	}
	return 0
}

// --- helpers (stubs / heuristics to flesh out) ---

func looksUnsupported(stderr []byte) bool {
	// The contract emits a clear "not supported"/"unknown flag" on rejection.
	// TODO: tighten to the exact tool.NotSupported/UsageError wording.
	s := bytes.ToLower(stderr)
	return bytes.Contains(s, []byte("not supported")) ||
		bytes.Contains(s, []byte("unsupported")) ||
		bytes.Contains(s, []byte("not every gnu flag is implemented")) || // the contract's signature loud-fail
		bytes.Contains(s, []byte("unknown flag")) ||
		bytes.Contains(s, []byte("unknown shorthand flag")) || // pflag wording
		bytes.Contains(s, []byte("invalid option"))
}

func firstDiff(gO, bO, gE, bE []byte, gC, bC int) string {
	var parts []string
	if gC != bC {
		parts = append(parts, fmt.Sprintf("exit gnu=%d bashy=%d", gC, bC))
	}
	if !bytes.Equal(gO, bO) {
		parts = append(parts, fmt.Sprintf("stdout gnu=%q bashy=%q", trunc(gO), trunc(bO)))
	}
	if !bytes.Equal(gE, bE) {
		parts = append(parts, fmt.Sprintf("stderr gnu=%q bashy=%q", trunc(gE), trunc(bE)))
	}
	return strings.Join(parts, " | ")
}

func trunc(b []byte) string {
	const max = 160
	if len(b) > max {
		return string(b[:max]) + "…"
	}
	return string(b)
}

func stdinReader(b []byte) io.Reader {
	if b == nil {
		return nil
	}
	return bytes.NewReader(b)
}

// builtinCases is a tiny starter set; real cases come from the three tiers.
func builtinCases(rc *tool.RunContext) []Case {
	big := rc.Path("corpus/lines-1e6.txt")
	return []Case{
		{Cmd: "wc", Argv: []string{"-l", big}},
		{Cmd: "sort", Argv: []string{"-n", big}},
		{Cmd: "head", Argv: []string{"-n", "5", big}},
		{Cmd: "cut", Argv: []string{"-d", " ", "-f", "1", big}},
	}
}
