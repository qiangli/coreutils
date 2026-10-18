package tool

import "fmt"

// PlannedEffect is a conditional operation, not a promise of syscall success.
// Composite operations name their scope in Op; Path retains the applet's spelling.
type PlannedEffect struct {
	Op   string `json:"op"`
	Path string `json:"path"`
}

// DryRun reports whether this invocation is recording a local effect plan.
func (rc *RunContext) DryRun() bool { return rc.dryRun && !CertProfile(rc) }

// Effect gates a mutation block. Callers MUST return/continue without performing
// that block when false. A block may include dependent mutations (e.g. backup,
// copy, attributes). No pretend files or successful syscalls are manufactured.
// The normal/cert path returns true without output or filesystem work.
func (rc *RunContext) Effect(op, path string) bool {
	if !rc.DryRun() {
		return true
	}
	rc.Effects = append(rc.Effects, PlannedEffect{Op: op, Path: path})
	if rc.effectErr == nil {
		_, rc.effectErr = fmt.Fprintf(rc.Out, "planned (local-preview, conditional): %s %q\n", op, path)
		if rc.effectErr != nil {
			fmt.Fprintf(rc.Err, "dry-run: write error: %v\n", rc.effectErr)
		}
	}
	return false
}
