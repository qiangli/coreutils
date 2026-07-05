package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
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
func bindEnv(sk dhntskills.Skill, meta map[string]string, dir string, log io.Writer, probes []dhntskills.EnvProbe, prims map[string]dhntskills.PrimitiveFn) dhntskills.Env {
	env := dhntskills.Env{
		Primitives: map[string]dhntskills.PrimitiveFn{},
		Predicates: map[string]dhntskills.PredicateFn{},
	}
	for id, fn := range prims {
		env.Primitives[id] = fn // pre-bound (dag targets); walk() skips bound ids
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

// runPrep is everything a run (plain or adaptive) needs about one skill
// at this coordinate.
type runPrep struct {
	base   dhntskills.Skill  // the catalog skill's canonical face
	rootID string            // Identity(base) — the overlay key
	meta   map[string]string // SKILL.md metadata + learned bindings
	probes []dhntskills.EnvProbe
	ctxKey string
	tier   string
	prims  map[string]dhntskills.PrimitiveFn // pre-bound primitives (e.g. the dag target)
}

// prepareRun runs the shared gate: valid canonical face, applicable
// here, canonical parse, merged bindings, coordinate + tier.
func prepareRun(cfg *config, sk Skill, src Source, ps *ProbeSet) (runPrep, error) {
	if !sk.Dhnt.Valid() {
		if sk.HasDhnt {
			return runPrep{}, fmt.Errorf("skills: %q has an invalid canonical face: %s", sk.Name, sk.Dhnt.Err)
		}
		return runPrep{}, fmt.Errorf("skills: %q has no skill.dhnt — a prose-only skill is read by a model, not machine-run", sk.Name)
	}
	if v := verdictOf(sk, ps); !v.Applicable {
		return runPrep{}, fmt.Errorf("skills: %q is not applicable here (%s)", sk.Name, v.Failing)
	}
	canon, _ := src.File(sk.Name, "skill.dhnt")
	base, err := dhntskills.ParseDhnt(strings.TrimSpace(string(canon)))
	if err != nil {
		return runPrep{}, err
	}
	rootID, err := dhntskills.Identity(base)
	if err != nil {
		return runPrep{}, err
	}

	meta := map[string]string{}
	maps.Copy(meta, sk.Meta)
	// Learned bindings extend/override authored metadata.
	maps.Copy(meta, loadBindings(cfg.cfgDir, sk.Name))

	tier := "local"
	if len(cfg.statics) > 0 {
		keys := make([]string, 0, len(cfg.statics))
		for k := range cfg.statics {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		tier = keys[0] + "@" + cfg.statics[keys[0]]
	}
	return runPrep{
		base:   base,
		rootID: rootID,
		meta:   meta,
		probes: dhntProbes(ps, KeyProbes(sk)),
		ctxKey: ContextKey(ps.Snapshot(KeyProbes(sk))),
		tier:   tier,
	}, nil
}

// runEffective executes one concrete skill shape (base, overlay, or a
// repair candidate) under the shared conventions: pre-flight audit,
// bind, run, attest. A failed contract is a Valid=false record, not an
// error.
func runEffective(name string, effective dhntskills.Skill, p runPrep, dir string, log io.Writer) (AttestRecord, error) {
	if needed := preflightEffects(effective); !dhntskills.EffectsWithin(needed, effective.EffectCap) {
		var atoms []string
		for _, e := range needed {
			atoms = append(atoms, e.String())
		}
		return AttestRecord{}, fmt.Errorf(
			"skills: pre-flight: bindings report {%s} but the declared effect cap does not cover it — declare `efefecato … fini` (machine-run needs the cap rung)",
			strings.Join(atoms, " "))
	}
	env := bindEnv(effective, p.meta, dir, log, p.probes, p.prims)
	att, err := dhntskills.Run(effective, env, p.tier)
	if err != nil {
		return AttestRecord{}, err
	}
	return AttestRecord{
		At:         time.Now().UTC(),
		Name:       name,
		Tier:       p.tier,
		ContextKey: p.ctxKey,
		Attest:     att,
	}, nil
}

// runSkill is the plain run engine: gate → resolve the host's overlay
// version (a self-healed host transparently prefers it) → execute →
// attest → store.
func runSkill(cfg *config, sk Skill, src Source, ps *ProbeSet, dir string, log io.Writer) (AttestRecord, string, error) {
	p, err := prepareRun(cfg, sk, src, ps)
	if err != nil {
		return AttestRecord{}, "", err
	}
	effective := p.base
	if vs := cfg.versions(); vs != nil {
		if s, ok := dhntskills.ResolveLatest(vs, p.base); ok {
			fmt.Fprintf(log, "skills: %s: running the host's learned version (overlay of %s)\n", sk.Name, shortID(p.rootID))
			effective = s
		}
	}
	rec, err := runEffective(sk.Name, effective, p, dir, log)
	if err != nil {
		return AttestRecord{}, "", err
	}
	path := storeAttest(cfg, rec, log)
	return rec, path, nil
}

func storeAttest(cfg *config, rec AttestRecord, log io.Writer) string {
	if cfg.cfgDir == "" {
		return ""
	}
	p, err := appendAttest(cfg.cfgDir, rec)
	if err != nil {
		fmt.Fprintf(log, "skills: attest store: %v\n", err)
		return ""
	}
	return p
}

func shortID(id string) string {
	if len(id) > 13 {
		return id[:13]
	}
	return id
}
