package perfbenchcmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os/exec" // DEV-ONLY measurement exception — see the package doc.
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"github.com/qiangli/coreutils/tool"
)

// Arm is one thing-under-test. The four arms make the in-process-dispatch
// thesis visible: an external tool (cmdperf/hyperfine) can only ever produce
// the "bashy-cold" number, missing inproc + warm.
type Arm string

const (
	ArmGNU         Arm = "gnu"          // exec the real GNU coreutils 9.11 binary — the reference
	ArmBashyCold   Arm = "bashy-cold"   // exec `bashy <tool>` fresh each run — per-call spawn+init tax
	ArmBashyInproc Arm = "bashy-inproc" // call tool.Run in-process — the pure userland algorithm
	ArmBashyWarm   Arm = "bashy-warm"   // via a `bashy serve` warm session — the amortized fleet case
)

// Workload is one command invocation (a Tier-1 micro) or a shell pipeline/script
// (Tier-2/3), timed identically across arms. Exactly one of Argv / Script is set.
type Workload struct {
	Name   string   // stable id, e.g. "sort-1e6-numeric"
	Tier   int      // 1 micro · 2 pipeline · 3 script
	Argv   []string // T1: [tool, args...] — the tool name is Argv[0]
	Script string   // T2/T3: a shell script run through the arm's shell
}

// perfOptions is the parsed `perfbench run` config (a real flag parse is a TODO;
// these defaults mirror the spec: 3 warmup, 20 timed).
type perfOptions struct {
	Warmup    int
	Runs      int
	Arms      []Arm
	Tiers     []int
	Format    string // "json" | "md" | "csv"
	GNUPrefix string
	BashyBin  string
}

func defaultPerfOptions(rc *tool.RunContext) perfOptions {
	prefix := rc.Getenv("GNU_PREFIX")
	if prefix == "" {
		prefix = "/opt/gnu"
	}
	bashy := rc.Getenv("BASHY_BIN")
	if bashy == "" {
		bashy = "bashy"
	}
	return perfOptions{
		Warmup: 3, Runs: 20,
		Arms:      []Arm{ArmGNU, ArmBashyCold, ArmBashyInproc, ArmBashyWarm},
		Tiers:     []int{1, 2, 3},
		Format:    "md",
		GNUPrefix: prefix, BashyBin: bashy,
	}
}

// Sample is one timed execution.
type Sample struct {
	D    time.Duration
	Exit int
}

// Stat is the summary over the timed samples of one (workload, arm).
type Stat struct {
	Arm    Arm           `json:"arm"`
	N      int           `json:"n"`
	Min    time.Duration `json:"min_ns"`
	Mean   time.Duration `json:"mean_ns"`
	Median time.Duration `json:"median_ns"`
	Stddev time.Duration `json:"stddev_ns"`
	Ratio  float64       `json:"ratio_vs_gnu"` // Mean / gnu.Mean; 1.0 for the gnu arm
	Exit   int           `json:"exit"`
}

// Result is one workload's stats across every arm.
type Result struct {
	Workload string `json:"workload"`
	Tier     int    `json:"tier"`
	Arms     []Stat `json:"arms"`
}

func runPerf(rc *tool.RunContext, args []string) int {
	opt := defaultPerfOptions(rc)
	// Minimal flag parse: -n/--runs, -w/--warmup, --format.
	for i := 0; i < len(args); i++ {
		if i+1 >= len(args) {
			break
		}
		switch args[i] {
		case "-n", "--runs":
			if v, err := strconv.Atoi(args[i+1]); err == nil {
				opt.Runs = v
			}
			i++
		case "-w", "--warmup":
			if v, err := strconv.Atoi(args[i+1]); err == nil {
				opt.Warmup = v
			}
			i++
		case "--format":
			opt.Format = args[i+1]
			i++
		}
	}

	workloads := builtinWorkloads(rc) // TODO: also load mined workloads from the corpus manifest
	var results []Result
	for _, w := range workloads {
		if !containsInt(opt.Tiers, w.Tier) {
			continue
		}
		res := Result{Workload: w.Name, Tier: w.Tier}
		var gnuMean time.Duration
		for _, arm := range opt.Arms {
			samples := timeArm(rc, arm, w, opt)
			st := computeStat(arm, samples)
			if arm == ArmGNU {
				gnuMean = st.Mean
			}
			res.Arms = append(res.Arms, st)
		}
		// second pass: fill the ratio column now gnuMean is known
		for i := range res.Arms {
			if gnuMean > 0 {
				res.Arms[i].Ratio = float64(res.Arms[i].Mean) / float64(gnuMean)
			}
		}
		results = append(results, res)
	}
	return emit(rc, opt.Format, results)
}

// timeArm runs warmup passes (discarded) then Runs timed passes for one arm.
func timeArm(rc *tool.RunContext, arm Arm, w Workload, opt perfOptions) []Sample {
	fn := armExecutor(rc, arm, w, opt)
	for i := 0; i < opt.Warmup; i++ {
		fn() // discard — warm the page cache + amortize first-touch
	}
	out := make([]Sample, 0, opt.Runs)
	for i := 0; i < opt.Runs; i++ {
		t0 := time.Now()
		code := fn()
		out = append(out, Sample{D: time.Since(t0), Exit: code})
	}
	return out
}

// armExecutor returns a closure that runs one pass of w on the given arm and
// returns its exit code. Output is discarded (we time, not capture).
func armExecutor(rc *tool.RunContext, arm Arm, w Workload, opt perfOptions) func() int {
	switch arm {
	case ArmGNU:
		// GNU arm: exec the real coreutils/bash binary from $GNU_PREFIX.
		if w.Script != "" {
			return execClosure(rc, filepath.Join(opt.GNUPrefix, "bin", "bash"), []string{"-c", w.Script})
		}
		bin := filepath.Join(opt.GNUPrefix, "bin", w.Argv[0])
		return execClosure(rc, bin, w.Argv[1:])
	case ArmBashyCold:
		// bashy cold: exec `bashy <tool> args` / `bashy -c script` fresh each run.
		if w.Script != "" {
			return execClosure(rc, opt.BashyBin, []string{"-c", w.Script})
		}
		return execClosure(rc, opt.BashyBin, w.Argv)
	case ArmBashyInproc:
		// bashy in-process: call tool.Run directly — no spawn. Only meaningful for a
		// T1 single command (pipelines/scripts need the shell). Falls back to cold
		// for T2/T3 with a note.
		if w.Script != "" {
			return execClosure(rc, opt.BashyBin, []string{"-c", w.Script}) // TODO: in-process pipeline via the sh interp
		}
		t := tool.Lookup(w.Argv[0])
		if t == nil {
			return func() int { return 127 }
		}
		return func() int {
			sub := &tool.RunContext{
				Ctx: rc.Ctx, Dir: rc.Dir, Env: rc.Env, FS: rc.FS,
				Stdio: tool.Stdio{In: nil, Out: io.Discard, Err: io.Discard},
			}
			return t.Run(sub, w.Argv[1:])
		}
	case ArmBashyWarm:
		// TODO: dial the `bashy serve` warm-session socket and submit the request
		// (internal/agentos/session). Until wired, fall back to cold so the runner works.
		if w.Script != "" {
			return execClosure(rc, opt.BashyBin, []string{"-c", w.Script})
		}
		return execClosure(rc, opt.BashyBin, w.Argv)
	default:
		return func() int { return 127 }
	}
}

func execClosure(rc *tool.RunContext, bin string, args []string) func() int {
	return func() int {
		c := exec.Command(bin, args...)
		c.Dir = rc.Dir
		c.Env = rc.Env
		c.Stdin = nil
		c.Stdout, c.Stderr = io.Discard, io.Discard
		if err := c.Run(); err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				return ee.ExitCode()
			}
			return 127
		}
		return 0
	}
}

func computeStat(arm Arm, s []Sample) Stat {
	st := Stat{Arm: arm, N: len(s), Ratio: 1.0}
	if len(s) == 0 {
		return st
	}
	ds := make([]time.Duration, len(s))
	var sum time.Duration
	st.Min = s[0].D
	st.Exit = s[len(s)-1].Exit
	for i, x := range s {
		ds[i] = x.D
		sum += x.D
		if x.D < st.Min {
			st.Min = x.D
		}
	}
	st.Mean = sum / time.Duration(len(s))
	sort.Slice(ds, func(i, j int) bool { return ds[i] < ds[j] })
	st.Median = ds[len(ds)/2]
	// population stddev
	var acc float64
	for _, d := range ds {
		diff := float64(d - st.Mean)
		acc += diff * diff
	}
	st.Stddev = time.Duration(math.Sqrt(acc / float64(len(ds))))
	return st
}

// emit writes the results in the requested format. JSON is the agent envelope;
// md is the cmdperf-style side-by-side table; csv is spreadsheet-friendly.
func emit(rc *tool.RunContext, format string, results []Result) int {
	switch format {
	case "json":
		enc := json.NewEncoder(rc.Out)
		enc.SetIndent("", "  ")
		if err := enc.Encode(map[string]any{"schema": "perfbench-v1", "results": results}); err != nil {
			fmt.Fprintln(rc.Err, "perfbench:", err)
			return 1
		}
	default: // "md"
		var b bytes.Buffer
		fmt.Fprintln(&b, "| workload | tier | arm | median | ratio×gnu |")
		fmt.Fprintln(&b, "|---|---|---|---|---|")
		for _, r := range results {
			// If the gnu arm itself failed (e.g. the reference binary isn't
			// installed — grep/find/sed/tar are NOT coreutils, they are separate
			// GNU packages), every ratio for this workload is meaningless.
			gnuFailed := false
			for _, a := range r.Arms {
				if a.Arm == ArmGNU && a.Exit != 0 {
					gnuFailed = true
				}
			}
			for _, a := range r.Arms {
				med, ratio := a.Median.String(), fmt.Sprintf("%.2f", a.Ratio)
				if a.Exit != 0 {
					// A nonzero exit means the binary was missing or errored — the
					// timing is a failed-exec artifact, not a measurement.
					med, ratio = fmt.Sprintf("FAIL(exit %d)", a.Exit), "—"
				} else if gnuFailed {
					ratio = "n/a"
				}
				fmt.Fprintf(&b, "| %s | T%d | %s | %s | %s |\n",
					r.Workload, r.Tier, a.Arm, med, ratio)
			}
		}
		rc.Out.Write(b.Bytes())
		// TODO: csv format.
	}
	return 0
}

// builtinWorkloads times each data-processing tool on the ~70 MB / 1e6-line
// corpus (where timing is meaningful — system-info/fs commands are ~instant and
// uninteresting). T1 = one command (Go in-process vs GNU C); T2 = a stdin/
// pipeline chain (the 0-fork in-process regime).
func builtinWorkloads(rc *tool.RunContext) []Workload {
	big := rc.Path("corpus/lines-1e6.txt") // 1e6 lines, ~70 MB
	tree := rc.Path("corpus/tree-wide")
	q := func(p string) string { return "'" + p + "'" } // paths have no spaces
	return []Workload{
		// ---- T1: single command on the big file ----
		{Name: "cat", Tier: 1, Argv: []string{"cat", big}},
		{Name: "wc", Tier: 1, Argv: []string{"wc", big}},
		{Name: "wc-l", Tier: 1, Argv: []string{"wc", "-l", big}},
		{Name: "head", Tier: 1, Argv: []string{"head", "-n", "500000", big}},
		{Name: "tail", Tier: 1, Argv: []string{"tail", "-n", "500000", big}},
		{Name: "sort", Tier: 1, Argv: []string{"sort", big}},
		{Name: "sort-n", Tier: 1, Argv: []string{"sort", "-n", big}},
		{Name: "cut", Tier: 1, Argv: []string{"cut", "-d", " ", "-f", "1", big}},
		{Name: "grep", Tier: 1, Argv: []string{"grep", "ERROR", big}},
		{Name: "sed", Tier: 1, Argv: []string{"sed", "s/tok/TOK/g", big}},
		{Name: "awk", Tier: 1, Argv: []string{"awk", "{n++} END{print n}", big}},
		{Name: "md5sum", Tier: 1, Argv: []string{"md5sum", big}},
		{Name: "sha256sum", Tier: 1, Argv: []string{"sha256sum", big}},
		{Name: "base64", Tier: 1, Argv: []string{"base64", big}},
		{Name: "tac", Tier: 1, Argv: []string{"tac", big}},
		// ---- T2: stdin/pipeline chains (times the whole chain to /dev/null) ----
		{Name: "tr(pipe)", Tier: 2, Script: "cat " + q(big) + " | tr a-z A-Z >/dev/null"},
		{Name: "uniq(pipe)", Tier: 2, Script: "sort " + q(big) + " | uniq -c >/dev/null"},
		{Name: "topN", Tier: 2, Script: "grep ERROR " + q(big) + " | sort | uniq -c | sort -rn | head >/dev/null"},
		{Name: "wordfreq", Tier: 2, Script: "cat " + q(big) + " | tr -s ' ' '\\n' | sort | uniq -c | sort -rn | head >/dev/null"},
		{Name: "find-count", Tier: 2, Script: "find " + q(tree) + " -type f | wc -l >/dev/null"},
	}
}

func containsInt(xs []int, v int) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}
