package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	dhntskills "github.com/dhnt/dhnt/skills"
)

// P2 — run + attest. The dhnt spec's L3 says an executor binds
// primitive/predicate ids to real implementations; this file is bashy's
// L3 binding convention:
//
//   - a contract predicate id X resolves its concrete command from the
//     skill's SKILL.md metadata: `check-<X>` (the prose face carries the
//     per-executor argv; the canonical face stays portable). The starter
//     predicates keep friendly keys: gereeni→check-tests,
//     builida→check-build, exito→check-<name arg> (or `check`).
//     A predicate command reports the `read` effect.
//   - a step primitive id P resolves `step-<P>` and reports
//     `read`+`write`.
//   - the built-in env predicate (conitexuto) is bound over the skill's
//     key probes, so folded environment arms evaluate.
//
// Effects are reported by convention, not traced — so the static
// pre-flight audit below refuses to run when the declared cap cannot
// cover what the bindings will report, before any side effect lands.

// AttestRecord is one stored run receipt (JSONL, ring-1 store).
type AttestRecord struct {
	At         time.Time              `json:"at"`
	Name       string                 `json:"name"`
	Tier       string                 `json:"tier"`
	ContextKey string                 `json:"context_key"`
	Attest     dhntskills.Attestation `json:"attest"`
}

// preflightEffects is the static audit: the effects the bindings WILL
// report for this skill, checked against the declared cap before
// anything runs.
func preflightEffects(sk dhntskills.Skill) []dhntskills.Effect {
	var needed []dhntskills.Effect
	if len(sk.Contract) > 0 || anyBranch(sk.Steps) {
		needed = append(needed, dhntskills.EffRead)
	}
	if anyLeafStep(sk.Steps) {
		needed = append(needed, dhntskills.EffRead, dhntskills.EffWrite)
	}
	return needed
}

func anyLeafStep(steps []dhntskills.Step) bool {
	for i := range steps {
		if b := steps[i].Branch; b != nil {
			if anyLeafStep(b.Then) || anyLeafStep(b.Else) {
				return true
			}
			continue
		}
		return true
	}
	return false
}

func anyBranch(steps []dhntskills.Step) bool {
	for i := range steps {
		if b := steps[i].Branch; b != nil {
			return true
		}
	}
	return false
}

// bindEnv builds the executor Env for one skill: every predicate id in
// the contract (and branch conditions) and every step primitive id gets
// a binding that resolves its concrete command from the skill metadata
// and runs it through the in-process userland.
func bindEnv(sk dhntskills.Skill, meta map[string]string, dir string, log io.Writer, probes []dhntskills.EnvProbe) dhntskills.Env {
	env := dhntskills.Env{
		Primitives: map[string]dhntskills.PrimitiveFn{},
		Predicates: map[string]dhntskills.PredicateFn{},
	}

	runMeta := func(id, key string, effs []dhntskills.Effect) (bool, []dhntskills.Effect, error) {
		src, ok := meta[key]
		if !ok || strings.TrimSpace(src) == "" {
			return false, nil, fmt.Errorf("no metadata %q — add the concrete command for %s to SKILL.md metadata", key, id)
		}
		fmt.Fprintf(log, "skills: %s → %s\n", id, src)
		ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
		defer cancel()
		code, err := runShellCommand(ctx, dir, src, log, log)
		if err != nil {
			return false, nil, fmt.Errorf("%s (%s): %w", id, key, err)
		}
		fmt.Fprintf(log, "skills: %s exit=%d\n", id, code)
		return code == 0, effs, nil
	}

	readOnly := []dhntskills.Effect{dhntskills.EffRead}
	readWrite := []dhntskills.Effect{dhntskills.EffRead, dhntskills.EffWrite}

	bindPredicate := func(id string) {
		if _, done := env.Predicates[id]; done {
			return
		}
		key := "check-" + id
		switch id {
		case "gereeni":
			key = "check-tests"
		case "builida":
			key = "check-build"
		}
		env.Predicates[id] = func(args []dhntskills.Arg) (bool, []dhntskills.Effect, error) {
			k := key
			if id == "exito" {
				k = "check"
				if n := refArg(args, "name"); n != "" {
					k = "check-" + n
				}
			}
			return runMeta(id, k, readOnly)
		}
	}
	bindPrimitive := func(id string) {
		if _, done := env.Primitives[id]; done {
			return
		}
		env.Primitives[id] = func(args []dhntskills.Arg) ([]dhntskills.Effect, error) {
			ok, effs, err := runMeta(id, "step-"+id, readWrite)
			if err != nil {
				return nil, err
			}
			if !ok {
				return effs, fmt.Errorf("step %s: command failed", id)
			}
			return effs, nil
		}
	}

	var walk func(steps []dhntskills.Step)
	walk = func(steps []dhntskills.Step) {
		for i := range steps {
			if b := steps[i].Branch; b != nil {
				bindPredicate(b.Cond.Predicate)
				walk(b.Then)
				walk(b.Else)
				continue
			}
			bindPrimitive(steps[i].Primitive)
		}
	}
	walk(sk.Steps)
	for i := range sk.Contract {
		bindPredicate(sk.Contract[i].Predicate)
	}

	// Folded environment arms (`when conitexuto …`) evaluate against this
	// host's coordinate.
	return dhntskills.WithEnvPredicate(env, probes)
}

func refArg(args []dhntskills.Arg, name string) string {
	for _, a := range args {
		if a.Name == name && a.Value.Kind == dhntskills.ExprRef {
			return a.Value.Ref
		}
	}
	return ""
}

// dhntProbes projects the skill's key probes into dhnt EnvProbe form —
// the same coordinate vocabulary on both sides of the boundary.
func dhntProbes(ps *ProbeSet, names []string) []dhntskills.EnvProbe {
	snap := ps.Snapshot(names)
	keys := make([]string, 0, len(snap))
	for k := range snap {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]dhntskills.EnvProbe, 0, len(keys))
	for _, k := range keys {
		out = append(out, dhntskills.EnvProbe{Name: k, Value: snap[k]})
	}
	return out
}

// appendAttest stores a run receipt in the ring-1 store
// (<dir>/attest/<name>.jsonl).
func appendAttest(storeDir string, rec AttestRecord) (string, error) {
	dir := filepath.Join(storeDir, "attest")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, rec.Name+".jsonl")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if err := json.NewEncoder(f).Encode(rec); err != nil {
		return "", err
	}
	return path, nil
}

// runSkill is the run verb's engine: applicability gate → pre-flight
// effect audit → bind → execute → attest → store. It returns the record
// whether or not the attestation is Valid; err is non-nil only for
// refusals and execution faults (a failed contract is a Valid=false
// record, not an error).
func runSkill(cfg *config, sk Skill, src Source, ps *ProbeSet, dir string, log io.Writer) (AttestRecord, string, error) {
	if !sk.Dhnt.Valid() {
		if sk.HasDhnt {
			return AttestRecord{}, "", fmt.Errorf("skills: %q has an invalid canonical face: %s", sk.Name, sk.Dhnt.Err)
		}
		return AttestRecord{}, "", fmt.Errorf("skills: %q has no skill.dhnt — a prose-only skill is read by a model, not machine-run", sk.Name)
	}
	if v := verdictOf(sk, ps); !v.Applicable {
		return AttestRecord{}, "", fmt.Errorf("skills: %q is not applicable here (%s)", sk.Name, v.Failing)
	}
	canon, _ := src.File(sk.Name, "skill.dhnt")
	dsk, err := dhntskills.ParseDhnt(strings.TrimSpace(string(canon)))
	if err != nil {
		return AttestRecord{}, "", err
	}
	if needed := preflightEffects(dsk); !dhntskills.EffectsWithin(needed, dsk.EffectCap) {
		var atoms []string
		for _, e := range needed {
			atoms = append(atoms, e.String())
		}
		return AttestRecord{}, "", fmt.Errorf(
			"skills: pre-flight: bindings report {%s} but the declared effect cap does not cover it — declare `efefecato … fini` (machine-run needs the cap rung)",
			strings.Join(atoms, " "))
	}

	probes := dhntProbes(ps, KeyProbes(sk))
	env := bindEnv(dsk, sk.Meta, dir, log, probes)

	tier := "local"
	if len(cfg.statics) > 0 {
		keys := make([]string, 0, len(cfg.statics))
		for k := range cfg.statics {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		tier = keys[0] + "@" + cfg.statics[keys[0]]
	}

	att, err := dhntskills.Run(dsk, env, tier)
	if err != nil {
		return AttestRecord{}, "", err
	}
	rec := AttestRecord{
		At:         time.Now().UTC(),
		Name:       sk.Name,
		Tier:       tier,
		ContextKey: ContextKey(ps.Snapshot(KeyProbes(sk))),
		Attest:     att,
	}
	path := ""
	if cfg.cfgDir != "" {
		if p, err := appendAttest(cfg.cfgDir, rec); err == nil {
			path = p
		} else {
			fmt.Fprintf(log, "skills: attest store: %v\n", err)
		}
	}
	return rec, path, nil
}
