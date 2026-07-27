package herald

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// GateExtensionURI identifies herald's evidence extension.
//
// A2A extensions are URI-identified, declared in the card's
// capabilities.extensions[], and negotiated per request via the A2A-Extensions
// header. Declaring one is how you add meaning to A2A without forking it.
const GateExtensionURI = "https://dhnt.io/ext/gate/v1"

// Why this extension exists.
//
// A2A's terminal state TASK_STATE_COMPLETED is SELF-REPORTED. The peer decides
// it succeeded and says so; the protocol gives a caller no way to disagree.
// That is the same defect the fleet measured directly in its three-harness A/B:
// ALL THREE HARNESSES EXITED 0 WHEN THEY FAILED. A success state reached by the
// absence of evidence is not a success state.
//
// So a herald task carries a GATE — a command that decides. The peer's
// COMPLETED is a claim; the gate is the verdict.
//
// The design rule that keeps this interoperable: DEGRADATION IS THE FEATURE.
// A peer that has never heard of the extension is not refused and is not
// trusted. herald simply runs the gate itself, locally, against whatever the
// peer returned. The guarantee therefore holds against ANY conformant A2A
// peer, which is what makes this worth proposing upstream rather than a
// private dialect.

// GateOutcome is the verdict on one delegated task.
type GateOutcome struct {
	// Ran reports whether a gate was executed at all. False means the task
	// was delegated with no gate — permitted, but the result is unverified
	// and callers must not report it as success.
	Ran bool `json:"ran"`
	// Passed is the verdict. Meaningless unless Ran.
	Passed bool `json:"passed"`
	// Where records who ran it: "peer" (the extension) or "local" (fallback).
	Where string `json:"where"`
	// Command is the gate as given.
	Command string `json:"command,omitempty"`
	// ExitCode is the gate's exit status when run locally.
	ExitCode int `json:"exit_code"`
	// Output is captured combined output, truncated for transport.
	Output string `json:"output,omitempty"`
	// PeerClaimed is what the peer said before the gate ran. Recorded
	// because the gap between claim and verdict is the interesting signal —
	// a peer that habitually claims COMPLETED on a failing gate is a peer
	// whose reliability ledger should say so.
	PeerClaimed string `json:"peer_claimed,omitempty"`
	// Elapsed is how long the gate took.
	Elapsed time.Duration `json:"elapsed_ns,omitempty"`
}

// Trusted reports whether the outcome may be treated as success.
//
// The whole point: an unrun gate is NOT success. Callers must branch on this,
// never on the peer's task state.
func (o GateOutcome) Trusted() bool { return o.Ran && o.Passed }

// Summary is a one-line human rendering.
func (o GateOutcome) Summary() string {
	switch {
	case !o.Ran:
		return "UNVERIFIED (no gate ran)"
	case o.Passed:
		return fmt.Sprintf("PASS (%s gate)", o.Where)
	default:
		return fmt.Sprintf("FAIL (%s gate, exit %d)", o.Where, o.ExitCode)
	}
}

// maxGateOutput caps captured output so a runaway gate cannot balloon a task
// record. The tail is kept: failures print last.
const maxGateOutput = 16 << 10

// RunLocalGate executes the gate in dir and returns the verdict.
//
// Shelling out here is correct and not a violation of the no-shell-out rule:
// that rule forbids a tool from spawning programs to implement ITS OWN
// behavior. A gate's entire documented purpose IS to run the operand command,
// exactly like timeout(1) or xargs(1) — the same carve-out those tools use.
func RunLocalGate(ctx context.Context, dir, gate string, peerClaimed string) GateOutcome {
	out := GateOutcome{Where: "local", Command: gate, PeerClaimed: peerClaimed}
	if strings.TrimSpace(gate) == "" {
		return out // Ran stays false: no gate, no verdict, no success.
	}
	if dir == "" {
		dir, _ = os.Getwd()
	}

	start := time.Now()
	cmd := exec.CommandContext(ctx, "sh", "-c", gate)
	cmd.Dir = dir
	// The gate must not inherit the operator's secrets: it is arbitrary
	// operator-supplied code being run to judge a REMOTE party's output.
	cmd.Env = gateEnv(dir)
	raw, err := cmd.CombinedOutput()
	out.Elapsed = time.Since(start)
	out.Ran = true
	out.Output = truncateTail(string(raw), maxGateOutput)

	if err == nil {
		out.Passed = true
		return out
	}
	var ee *exec.ExitError
	if errorsAs(err, &ee) {
		out.ExitCode = ee.ExitCode()
	} else {
		// The gate could not be executed at all (missing shell, bad dir).
		// That is NOT a pass, and it is not a peer failure either — it is an
		// unusable gate, which must be loud rather than silently permissive.
		out.ExitCode = -1
		out.Output = strings.TrimSpace(out.Output + "\nherald: gate could not run: " + err.Error())
	}
	return out
}

// gateEnv builds a minimal environment for gate execution. PATH and HOME are
// preserved because a gate is usually a build or test command; the vault
// variables are not.
func gateEnv(dir string) []string {
	keep := []string{"PATH", "HOME", "LANG", "TMPDIR", "GOCACHE", "GOMODCACHE", "GOPATH"}
	env := make([]string, 0, len(keep)+2)
	for _, k := range keep {
		if v, ok := os.LookupEnv(k); ok {
			env = append(env, k+"="+v)
		}
	}
	env = append(env, "LC_ALL=C", "HERALD_GATE_DIR="+dir)
	return env
}

// truncateTail keeps the last n bytes, which is where a failure explains
// itself.
func truncateTail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "…(truncated)…\n" + s[len(s)-n:]
}

// errorsAs is errors.As, wrapped so this file states its one dependency
// explicitly rather than importing errors for a single call.
func errorsAs(err error, target **exec.ExitError) bool {
	for err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			*target = ee
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}
