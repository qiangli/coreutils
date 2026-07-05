package skills

// The tasks face: a skill folder MAY carry tasks.md — a dag task file
// (### headings = targets with Requires:/Sources:/Effects: metadata).
// `skills run NAME --target T` executes one target through the dag
// engine. The line that matters here is executability, not markdown
// dialect: SKILL.md stays model-facing prose; tasks.md is the
// machine-executable face; skill.dhnt is the verification spine. For a
// contracted skill the target runs AS the steps phase — a synthetic
// step bound to the dag engine — so the contract, the effect cap, the
// pre-flight audit, and the attestation all apply unchanged.

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	dhntskills "github.com/dhnt/dhnt/skills"
	"github.com/qiangli/coreutils/pkg/dag"
)

// dagPrimitive is the synthetic dhnt primitive id a --target run binds
// (canonical dhnt: da-ga).
const dagPrimitive = "daga"

// materializeTasks returns an on-disk path for the skill's tasks.md (the
// real file for directory-backed skills, a temp file for embedded ones)
// plus the target inventory — parsed by the dag engine's own parser, so
// the names match exactly what execution will accept. cleanup is always
// safe to call.
func materializeTasks(sk Skill, src Source) (path string, targets []string, cleanup func(), err error) {
	cleanup = func() {}
	data, ok := src.File(sk.Name, "tasks.md")
	if !ok {
		return "", nil, cleanup, fmt.Errorf("skills: %q has no tasks.md face (targets run the dag file bundled with a skill)", sk.Name)
	}
	if sk.Dir != "" {
		path = filepath.Join(sk.Dir, "tasks.md")
	} else {
		f, err := os.CreateTemp("", "bashy-skill-"+sk.Name+"-*.md")
		if err != nil {
			return "", nil, cleanup, err
		}
		if _, err := f.Write(data); err != nil {
			f.Close()
			return "", nil, cleanup, err
		}
		if err := f.Close(); err != nil {
			return "", nil, cleanup, err
		}
		path = f.Name()
		cleanup = func() { _ = os.Remove(f.Name()) }
	}
	doc, err := dag.ParseFile(path)
	if err != nil {
		return "", nil, cleanup, fmt.Errorf("skills: %q tasks.md: %w", sk.Name, err)
	}
	return path, doc.Order, cleanup, nil
}

// runDagTarget executes one target of a task file through the dag
// engine (the same engine as `bashy dag`), streaming output to log.
func runDagTarget(tasksPath, target string, log io.Writer) error {
	cmd := dag.NewDagCmd()
	cmd.SetArgs([]string{tasksPath, target})
	cmd.SetOut(log)
	cmd.SetErr(log)
	if err := cmd.Execute(); err != nil {
		return fmt.Errorf("skills: dag target %q: exit %d: %w", target, dag.ExitCodeOf(err), err)
	}
	return nil
}

// runTargetSkill is the --target engine. Contracted skills run the
// target as the steps phase (attested, cap-audited); uncontracted ones
// run it plainly (rung "tasks": executable, not yet verifiable).
func runTargetSkill(cfg *config, sk Skill, src Source, ps *ProbeSet, target string, log io.Writer) (AttestRecord, bool, error) {
	if v := verdictOf(sk, ps); !v.Applicable {
		return AttestRecord{}, false, fmt.Errorf("skills: %q is not applicable here (%s)", sk.Name, v.Failing)
	}
	tasksPath, targets, cleanup, err := materializeTasks(sk, src)
	defer cleanup()
	if err != nil {
		return AttestRecord{}, false, err
	}
	known := false
	for _, t := range targets {
		if t == target {
			known = true
			break
		}
	}
	if !known {
		return AttestRecord{}, false, fmt.Errorf("skills: %q has no target %q (targets: %v)", sk.Name, target, targets)
	}

	if !sk.Dhnt.Valid() {
		// Executable but uncontracted: run the target, report plainly.
		return AttestRecord{}, false, runDagTarget(tasksPath, target, log)
	}

	// Contracted: the dag target IS the steps phase. Pre-flight, cap,
	// contract, and attestation apply exactly as for a normal run.
	p, err := prepareRun(cfg, sk, src, ps)
	if err != nil {
		return AttestRecord{}, false, err
	}
	p.prims = map[string]dhntskills.PrimitiveFn{
		dagPrimitive: func([]dhntskills.Arg) ([]dhntskills.Effect, error) {
			if err := runDagTarget(tasksPath, target, log); err != nil {
				return []dhntskills.Effect{dhntskills.EffRead, dhntskills.EffWrite}, err
			}
			return []dhntskills.Effect{dhntskills.EffRead, dhntskills.EffWrite}, nil
		},
	}
	effective := dhntskills.Skill{
		Name:      p.base.Name,
		Caps:      p.base.Caps,
		EffectCap: p.base.EffectCap,
		Contract:  p.base.Contract,
		// Step name must be canonical dhnt too — the human-facing target
		// name travels in the receipt's outcome, not the canonical form.
		Steps: []dhntskills.Step{{Name: dagPrimitive, Primitive: dagPrimitive}},
	}
	rec, err := runEffective(sk.Name, effective, p, mustGetwd(), log)
	if err != nil {
		return AttestRecord{}, false, err
	}
	storeAttest(cfg, rec, log)
	return rec, true, nil
}
