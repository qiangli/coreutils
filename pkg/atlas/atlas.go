// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

// Package atlas is the Command Atlas: the curated multi-axis catalog of the
// bashy/coreutils command surface. Beyond the classical class split
// (builtin / coreutils / verb) it records, per command, a functional group,
// the dhnt execution tier it operates in (userland/workspace/sandbox/sphere/
// cluster/cloud/account), and agentic capability flags (json, dry-run,
// destructive, …), plus a curated list of composite idioms (commands
// naturally used together).
//
// The atlas is an execution-assist substrate, not just presentation: it is
// imported by `bashy commands` (the views), the MCP server (list_tools
// metadata), and — per the roadmap — pkg/dag target preflight and the
// advisor. It is stdlib-only.
//
// Discipline: every assignment is a hand-set table entry; vocabularies are
// closed; coverage tests in this package and in bashy assert the tables stay
// exactly in sync with the live registries (tool.Names(), the shim lists).
// Shell builtins are NOT recorded here — the embedding shell owns that set
// and merges it in (bashy/internal/agentos/atlas.go). Declarative-registry
// CLIs (doctl, …) are also not hand-listed: their entries derive from
// external/registry's Entry.Tier via RegistryEntry.
//
// Design doc: bashy/docs/command-atlas.md.
package atlas

import (
	"fmt"
	"sort"
)

// Execution tiers (locked vocabulary — dhnt docs/execution-tiers.md), plus
// "account" for the Tessaro front door beside the stack.
const (
	TierUserland  = "userland"
	TierWorkspace = "workspace"
	TierSandbox   = "sandbox"
	TierSphere    = "sphere"
	TierCluster   = "cluster"
	TierCloud     = "cloud"
	TierAccount   = "account"
)

// Functional groups (closed vocabulary). "shell" is reserved for the
// builtins, which the embedding shell contributes.
const (
	GroupShell        = "shell"
	GroupFileutils    = "fileutils"
	GroupTextutils    = "textutils"
	GroupShellutils   = "shellutils"
	GroupCodeIntel    = "code-intel"
	GroupNet          = "net"
	GroupOrch         = "orchestration"
	GroupKnowledge    = "knowledge"
	GroupEngines      = "engines"
	GroupForge        = "forge"
	GroupToolchains   = "toolchains"
	GroupStorage      = "storage"
	GroupClusterCloud = "cluster-cloud"
	GroupPlatform     = "platform"
	GroupAccount      = "account"
)

// SDLC stages (closed vocabulary). The spine every front-door verb must place
// itself on: plan → code → test → deploy, plus "cross" for the verbs that serve
// every stage (knowledge, identity, diagnostics, the userland itself).
//
// This axis exists to answer ONE question, asked of every new verb before it
// ships: *which stage do you serve that nothing else already does?* bashy's
// agentic surface grew piecemeal until the Code stage had six overlapping verbs
// and the Test stage had none — a hole nobody could see because there was no
// axis on which to see it. A stage is therefore MANDATORY for a verb: addVerb
// panics without one, so a verb that cannot answer the question cannot start the
// binary. That is deliberately harsher than a test: a test can be defaulted
// around (this one was — see the git history of bashy's verbAtlasRecord, which
// invented a valid-looking group/tier for unclassified verbs and so silently
// defeated the very coverage test that was meant to catch them).
const (
	StagePlan   = "plan"   // decide what to build: sprint, meet, kb
	StageCode   = "code"   // build it: weave, chat, foreman
	StageTest   = "test"   // decide pass/fail: dag, check, verify
	StageDeploy = "deploy" // ship it: sdlc, cluster/cloud CLIs
	StageCross  = "cross"  // serves every stage: skills, secrets, doctor, the userland
)

// Agentic capability flags (closed vocabulary; curated, never inferred —
// absence means unknown, not no).
const (
	CapJSON             = "json"              // structured-output mode (--json or native)
	CapDryRun           = "dry-run"           // participates in the dry-run manifest
	CapDestructive      = "destructive"       // can irreversibly delete/overwrite data
	CapReadOnly         = "read-only"         // never mutates the filesystem
	CapCached           = "cached"            // keeps a persistent on-disk cache
	CapBudget           = "budget"            // token-budget-aware output
	CapNeedsNetwork     = "needs-network"     // requires network beyond first provision
	CapNeedsPairing     = "needs-pairing"     // requires a Tessaro-paired machine/token
	CapSelfProvisioning = "self-provisioning" // download → verify → cache → exec
	CapSpawnsProcesses  = "spawns-processes"  // executes external processes
	CapDaemon           = "daemon"            // starts/manages a long-running service
)

// Security effects (closed vocabulary; curated, never inferred). Unlike the
// capability flags — which describe what a command is FOR — effects describe
// what a command can DO to the machine, the data, or the outside world, from a
// security / privacy / governance lens. Every atlas entry declares at least one
// (EffPure is the explicit "no governed effect" declaration), so classification
// is mandatory: a command with no declared effect fails the coverage ratchet,
// it does not fail open.
//
// The first six mirror the dhnt skill-CNL effect lattice
// (coreutils/pkg/skills → github.com/dhnt/dhnt/skills); the last five are the
// finer distinctions a shell that an agent drives needs to reason about. A
// future policy engine projects this 11-atom set onto the dhnt 6 for skill-cap
// compatibility.
const (
	EffPure    = "pure"    // deterministic, no governed side effect (true, echo, seq)
	EffRead    = "read"    // reads filesystem / host state / input data (privacy surface)
	EffWrite   = "write"   // mutates the filesystem or host state
	EffDestroy = "destroy" // can IRREVERSIBLY lose data (rm, dd, shred)
	EffNet     = "net"     // opens a network connection (egress / exfiltration surface)
	EffExec    = "exec"    // spawns an external process that bashy no longer governs
	EffCred    = "cred"    // reads or writes credentials / secrets
	EffPriv    = "priv"    // changes privilege, ownership, or a security label
	EffRemote  = "remote"  // executes on ANOTHER host (crosses the machine boundary)
	EffPersist = "persist" // leaves something that OUTLIVES the session (cron, daemon, install)
	EffSpend   = "spend"   // incurs metered cost (paid inference, cloud resources)
)

// Subclass refines the verb class only.
const (
	SubclassProvisioner     = "provisioner"
	SubclassManagedExternal = "managed-external"
)

// Entry is one command's atlas record. The classical class (builtin /
// coreutils / verb) is not stored: it follows from which table (or the
// embedding shell's builtin set) the name resolves in.
type Entry struct {
	Group    string
	Tier     string
	Stage    string // SDLC stage (closed vocab); every VERB declares one
	Subclass string // verbs only: provisioner | managed-external | ""
	Caps     []string
	Effects  []string // security effects (closed vocab); every entry has ≥1
	AliasOf  string   // e.g. docker → podman, upgrade → self
}

// Idiom is one curated composite: commands naturally used together.
type Idiom struct {
	ID       string   `json:"id"`
	Commands []string `json:"commands"`
	Pattern  string   `json:"pattern"`
	Note     string   `json:"note"`
	Fused    string   `json:"fused,omitempty"` // shipped fused form, if any
	Tier     string   `json:"tier"`
}

var (
	tools = map[string]Entry{}
	verbs = map[string]Entry{}
)

// Groups returns the closed group vocabulary, sorted.
func Groups() []string {
	return []string{
		GroupAccount, GroupClusterCloud, GroupCodeIntel, GroupEngines,
		GroupFileutils, GroupForge, GroupKnowledge, GroupNet, GroupOrch,
		GroupPlatform, GroupShell, GroupShellutils, GroupStorage,
		GroupTextutils, GroupToolchains,
	}
}

// Tiers returns the tier vocabulary in stack order (foundation → payoff),
// with account last (beside the stack, not in it).
func Tiers() []string {
	return []string{
		TierUserland, TierWorkspace, TierSandbox, TierSphere,
		TierCluster, TierCloud, TierAccount,
	}
}

// Capabilities returns the closed capability vocabulary, sorted.
func Capabilities() []string {
	return []string{
		CapBudget, CapCached, CapDaemon, CapDestructive, CapDryRun,
		CapJSON, CapNeedsNetwork, CapNeedsPairing, CapReadOnly,
		CapSelfProvisioning, CapSpawnsProcesses,
	}
}

// Stages returns the closed SDLC-stage vocabulary, in pipeline order (not
// sorted: the order IS the spine, and reading it out of order loses the point).
func Stages() []string {
	return []string{StagePlan, StageCode, StageTest, StageDeploy, StageCross}
}

// Effects returns the closed security-effect vocabulary, sorted.
func Effects() []string {
	return []string{
		EffCred, EffDestroy, EffExec, EffNet, EffPersist, EffPriv,
		EffPure, EffRead, EffRemote, EffSpend, EffWrite,
	}
}

// Lookup returns the atlas entry for a command name: in-process tools first,
// then front-door verbs (mirroring dispatch precedence). Shell builtins and
// declarative-registry CLIs are the embedder's to merge (see RegistryEntry).
func Lookup(name string) (Entry, bool) {
	if e, ok := tools[name]; ok {
		return e, true
	}
	e, ok := verbs[name]
	return e, ok
}

// ToolNames returns the names of the in-process (coreutils-tool-class)
// entries, sorted. Coverage tests assert this set == tool.Names().
func ToolNames() []string { return sortedKeys(tools) }

// VerbNames returns the names of the front-door-verb-class entries, sorted.
// The declarative-registry CLIs are not included (derive via RegistryEntry).
func VerbNames() []string { return sortedKeys(verbs) }

// RegistryEntry returns the derived atlas entry for a declarative-registry
// CLI (external/registry), given its Entry.Tier int. Registry CLIs are never
// hand-listed in the atlas: new providers are registry data only.
func RegistryEntry(tier int) Entry {
	// Every managed external is downloaded (net) and then run as its own process
	// (exec). Only a tier-4+ CLI (sphere/cluster/cloud — doctl, …) drives a
	// control plane on ANOTHER host and so is also `remote`; a tier-2/3 local
	// tool like ripgrep is not.
	effects := []string{EffExec, EffNet}
	if tier >= 4 {
		effects = append(effects, EffRemote)
	}
	sort.Strings(effects)
	// SDLC stage follows the same tier split: a tier-4+ CLI drives a control
	// plane on another host — that is shipping (deploy). A local tier-2/3 tool
	// like ripgrep serves every stage (cross).
	stage := StageCross
	if tier >= 4 {
		stage = StageDeploy
	}
	return Entry{
		Group:    GroupClusterCloud,
		Tier:     TierName(tier),
		Stage:    stage,
		Subclass: SubclassManagedExternal,
		Caps: []string{
			CapCached, CapNeedsNetwork, CapSelfProvisioning, CapSpawnsProcesses,
		},
		Effects: effects,
	}
}

// TierName maps the numeric execution tier (external/registry Entry.Tier,
// dhnt docs/execution-tiers.md) to the atlas tier name.
func TierName(t int) string {
	switch t {
	case 2:
		return TierWorkspace
	case 3:
		return TierSandbox
	case 4:
		return TierSphere
	case 5:
		return TierCluster
	case 6:
		return TierCloud
	default:
		return TierUserland
	}
}

// Idioms returns the curated composite list.
func Idioms() []Idiom {
	out := make([]Idiom, len(idioms))
	copy(out, idioms)
	return out
}

// idioms is the curated composite set. Growth rule: additions edit this
// table AND bashy/docs/command-atlas.md together; the coverage test asserts
// every referenced command resolves in the atlas (or is a known builtin).
var idioms = []Idiom{
	{ID: "count-matches", Commands: []string{"grep", "wc"},
		Pattern: "grep PAT F | wc -l", Fused: "grep -c PAT F",
		Note: "one process, one pipe fewer", Tier: TierUserland},
	{ID: "top-n", Commands: []string{"sort", "uniq", "head"},
		Pattern: "... | sort | uniq -c | sort -rn | head",
		Note:    "fusion candidate (bounded-heap top-N verb); no fused form shipped yet",
		Tier:    TierUserland},
	{ID: "find-exec", Commands: []string{"find", "xargs"},
		Pattern: "find ... -print0 | xargs -0 CMD",
		Note:    "the canonical scale-out; -print0/-0 for arbitrary names", Tier: TierUserland},
	{ID: "scoped-cd", Commands: []string{"cd"},
		Pattern: "(cd DIR && CMD)",
		Note:    "subshell keeps the cwd change scoped; avoid bare cd", Tier: TierUserland},
	{ID: "list-inspect", Commands: []string{"ls", "stat"},
		Pattern: "ls DIR; stat FILE",
		Note:    "enumerate, then inspect the interesting entry precisely", Tier: TierUserland},
	{ID: "tempfile-cleanup", Commands: []string{"mktemp", "rm", "trap"},
		Pattern: `t=$(mktemp) && trap 'rm -f "$t"' EXIT`,
		Note:    "leak-free scratch files", Tier: TierUserland},
	{ID: "archive", Commands: []string{"tar"},
		Pattern: "tar -czf out.tgz DIR",
		Note:    "tar+gzip in one call; avoid tar | gzip", Tier: TierUserland},
	{ID: "fetch-extract", Commands: []string{"fetch", "jq"},
		Pattern: "fetch --json URL | jq .field",
		Note:    "HTTP + structured extraction without a browser", Tier: TierUserland},
	{ID: "forge-loop", Commands: []string{"git", "gh", "act"},
		Pattern: "git push; gh pr create; act",
		Note:    "commit/push → PR → run the workflow locally before CI", Tier: TierUserland},
	{ID: "fleet-suite", Commands: []string{"weave", "sprint", "foreman", "dag"},
		Pattern: "sprint (plan) → weave (isolate/run) → foreman (steer) → dag (targets)",
		Note:    "the orchestration suite", Tier: TierWorkspace},
	{ID: "cluster-deploy", Commands: []string{"kubectl", "helm"},
		Pattern: "kubectl get ...; helm install ...",
		Note:    "inspect the cluster, install/upgrade via charts", Tier: TierCluster},
	{ID: "pair-first", Commands: []string{"login", "sphere"},
		Pattern: "login, then sphere/kubectl",
		Note:    "tiers 4-5 need a Tessaro-paired machine", Tier: TierAccount},
}

// --- table construction -----------------------------------------------------

func addTools(group string, names ...string) {
	for _, n := range names {
		if _, dup := tools[n]; dup {
			panic(fmt.Sprintf("atlas: duplicate tool entry %q", n))
		}
		// The userland serves every stage — `grep` is not a "test" command any
		// more than it is a "deploy" one. Only front-door VERBS take a position
		// on the spine.
		tools[n] = Entry{Group: group, Tier: TierUserland, Stage: StageCross}
	}
}

func addVerb(name string, e Entry) {
	if _, dup := verbs[name]; dup {
		panic(fmt.Sprintf("atlas: duplicate verb entry %q", name))
	}
	if e.Tier == "" {
		e.Tier = TierUserland
	}
	// A verb MUST place itself on the SDLC spine. This panics at init rather
	// than failing a test, because a test can be defaulted around and this one
	// was: bashy's verbAtlasRecord used to invent a valid-looking group/tier for
	// any verb missing an entry, so the coverage test that was supposed to catch
	// the omission passed happily instead. An unclassifiable verb should not be
	// able to start the binary.
	if !validStage(e.Stage) {
		panic(fmt.Sprintf("atlas: verb %q has no SDLC stage (one of %v). "+
			"Which stage does it serve that nothing else already does? If the honest "+
			"answer is 'none', it should not ship.", name, Stages()))
	}
	verbs[name] = e
}

func validStage(s string) bool {
	for _, v := range Stages() {
		if s == v {
			return true
		}
	}
	return false
}

// staged places an Entry built by the managed()/provisioner() helpers on the
// SDLC spine. Those helpers predate the stage axis and are shared by many verbs
// with different stages, so the stage is applied at the call site.
func staged(stage string, e Entry) Entry {
	e.Stage = stage
	return e
}

// stageTools overrides the default StageCross for tool entries that are not
// really userland utilities. Today that is exactly one: `foreman`, which is an
// orchestration command registered through the tool registry (an import-cycle
// workaround), so addTools had filed it as cross-cutting alongside `cat` and
// `grep`. It drives an agent session — it belongs on the Code stage. The
// misfiling was invisible until there was an axis to see it on.
func stageTools(stage string, names ...string) {
	for _, n := range names {
		e, ok := tools[n]
		if !ok {
			panic(fmt.Sprintf("atlas: stage %q names unknown tool %q", stage, n))
		}
		if !validStage(stage) {
			panic(fmt.Sprintf("atlas: invalid stage %q for tool %q", stage, n))
		}
		e.Stage = stage
		tools[n] = e
	}
}

// capTools appends a capability to existing tool entries; unknown names panic
// so the tables self-check at init.
func capTools(capability string, names ...string) {
	for _, n := range names {
		e, ok := tools[n]
		if !ok {
			panic(fmt.Sprintf("atlas: cap %q names unknown tool %q", capability, n))
		}
		e.Caps = append(e.Caps, capability)
		tools[n] = e
	}
}

// eff appends a security effect to existing entries (tool OR verb); an unknown
// name panics so the classification self-checks at init and can never silently
// skip a command.
func eff(effect string, names ...string) {
	for _, n := range names {
		if e, ok := tools[n]; ok {
			e.Effects = append(e.Effects, effect)
			tools[n] = e
			continue
		}
		if e, ok := verbs[n]; ok {
			e.Effects = append(e.Effects, effect)
			verbs[n] = e
			continue
		}
		panic(fmt.Sprintf("atlas: effect %q names unknown command %q", effect, n))
	}
}

func sortedKeys(m map[string]Entry) []string {
	out := make([]string, 0, len(m))
	for n := range m {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func init() {
	// In-process tools (tier userland by definition). The coverage test
	// asserts this set == tool.Names() with cmds/all + cmds/graph +
	// cmds/foreman registered.
	addTools(GroupFileutils,
		"basename", "chcon", "chgrp", "chmod", "chown", "clip", "cp", "dd",
		"df", "dir", "dircolors", "dirname", "du", "find", "install", "link",
		"ln", "ls", "mkdir", "mkfifo", "mknod", "mktemp", "mv", "readlink",
		"realpath", "rm", "rmdir", "shred", "stat", "sync", "tar", "touch",
		"tree", "truncate", "unlink", "vdir",
	)
	addTools(GroupTextutils,
		"awk", "b2sum", "base32", "base64", "basenc", "cat", "cksum", "cmp",
		"comm", "csplit", "cut", "diff", "expand", "fmt", "fold", "grep",
		"gunzip", "gzip", "head", "hexdump", "join", "jq", "md5sum", "more",
		"nl", "numfmt", "od", "paste", "pr", "ptx", "sed", "sha1sum",
		"sha224sum", "sha256sum", "sha384sum", "sha512sum", "shuf", "sort",
		"split", "strings", "sum", "tac", "tail", "tee", "tokens", "tr",
		"tsort", "unexpand", "uniq", "wc", "xargs", "zcat",
	)
	addTools(GroupShellutils,
		"arch", "at", "atq", "atrm", "batch", "cal", "chroot", "crontab",
		"date", "duration", "echo", "env", "expr", "factor", "false",
		"groups", "hostid", "hostname", "id", "logname", "ncal", "nice",
		"nohup", "nproc", "ntp", "pathchk", "pinky", "printenv", "pwd",
		"runcon", "seq", "sleep", "sntp", "stdbuf", "stty", "time",
		"timeout", "true", "tty", "tz", "uname", "uptime", "users", "watch",
		"which", "who", "whoami", "yes",
	)
	addTools(GroupCodeIntel, "ast", "graph")
	addTools(GroupNet, "browser", "fetch")
	addTools(GroupOrch, "foreman")

	// Tool capabilities (evidence per flag: docs/command-atlas.md §2.3).
	capTools(CapJSON,
		"ast", "graph",
		"browser", "fetch", "duration", "tz", "ntp", "sntp", "tokens",
		"foreman",
	)
	capTools(CapDryRun, "rm")
	capTools(CapDestructive, "rm", "dd", "shred", "truncate")
	capTools(CapReadOnly,
		"cat", "cmp", "comm", "df", "diff", "du", "grep", "head", "hexdump",
		"ls", "od", "readlink", "realpath", "stat", "strings", "tac", "tail",
		"tokens", "tree", "wc", "which",
		// `ast` (symbols/search/refs/map/query) is pure structural reads.
		"ast",
	)
	// The `graph` umbrella has write subcommands (note/link/observe/forget), so
	// it is not read-only; its structural reads keep a disk cache, so CapCached.
	capTools(CapCached, "graph")
	// `ast map` is the token-budgeted repo map.
	capTools(CapBudget, "tokens", "ast")
	capTools(CapNeedsNetwork, "fetch", "browser", "ntp", "sntp")
	capTools(CapSpawnsProcesses,
		"xargs", "timeout", "time", "watch", "nice", "nohup", "chroot",
		"runcon", "stdbuf", "at", "batch",
	)
	capTools(CapDaemon, "foreman")
	// foreman drives an agent session: it is a Code-stage orchestrator, not a
	// cross-cutting userland utility. It only lives in the tool table to dodge an
	// import cycle (see bashy agentos.go).
	stageTools(StageCode, "foreman")

	// Front-door verbs. Shell builtins and registry CLIs are merged by the
	// embedder (bashy); everything else lives here.
	managed := func(group, tier string, caps ...string) Entry {
		return Entry{Group: group, Tier: tier, Subclass: SubclassManagedExternal,
			Caps: append([]string{CapCached, CapSelfProvisioning, CapSpawnsProcesses}, caps...)}
	}
	provisioner := func(group string, caps ...string) Entry {
		return Entry{Group: group, Tier: TierUserland, Subclass: SubclassProvisioner,
			Caps: append([]string{CapCached, CapSelfProvisioning, CapSpawnsProcesses}, caps...)}
	}

	// orchestration
	addVerb("weave", Entry{Stage: StageCode, Group: GroupOrch, Tier: TierWorkspace, Caps: []string{CapJSON}})
	addVerb("sprint", Entry{Stage: StagePlan, Group: GroupOrch, Tier: TierWorkspace, Caps: []string{CapJSON}})
	addVerb("dag", Entry{Stage: StageCross, Group: GroupOrch, Tier: TierWorkspace, Caps: []string{CapJSON}})
	addVerb("sdlc", Entry{Stage: StageDeploy, Group: GroupOrch, Tier: TierWorkspace, Caps: []string{CapJSON}})
	// invoke: ONE agent, ONCE, on one instruction — the primitive that unifies the
	// heterogeneous agent CLIs. Renamed from `chat` 2026-07-12: chat does not chat.
	// Its own synopsis always read "invoke an agent with a single unattended
	// instruction" — no conversation, no session. The name misled agents into
	// thinking it was a session, which is what `foreman` is.
	addVerb("invoke", Entry{Stage: StageCode, Group: GroupOrch, Caps: []string{CapJSON, CapSpawnsProcesses}})
	addVerb("chat", Entry{Stage: StageCode, Group: GroupOrch, AliasOf: "invoke", Caps: []string{CapJSON, CapSpawnsProcesses}})
	addVerb("meet", Entry{Stage: StagePlan, Group: GroupOrch, Caps: []string{CapSpawnsProcesses}})
	addVerb("supervise", Entry{Stage: StageCode, Group: GroupOrch, Caps: []string{CapSpawnsProcesses}})
	addVerb("capability", Entry{Stage: StageCross, Group: GroupOrch, Caps: []string{CapJSON}})
	// handoff/resume: pause a live session and pass the work on -- to another
	// agentic tool, a scheduler, or tomorrow. CROSS, because you hand off work
	// at any stage: a half-finished plan, a half-finished refactor, a half-run
	// test campaign, a half-done deploy.
	addVerb("handoff", Entry{Stage: StageCross, Group: GroupOrch, Caps: []string{CapJSON}})
	addVerb("resume", Entry{Stage: StageCross, Group: GroupOrch, Caps: []string{CapJSON}})
	addVerb("agent", Entry{Stage: StageCode, Group: GroupOrch, Caps: []string{CapJSON}})

	// the fleet registry: what this host runs with
	addVerb("tools", Entry{Stage: StageCross, Group: GroupOrch, Caps: []string{CapJSON}})
	addVerb("models", Entry{Stage: StageCross, Group: GroupOrch, Caps: []string{CapJSON}})
	addVerb("agents", Entry{Stage: StageCross, Group: GroupOrch, Caps: []string{CapJSON}})
	addVerb("people", Entry{Stage: StageCross, Group: GroupOrch, Caps: []string{CapJSON}})
	addVerb("whois", Entry{Stage: StageCross, Group: GroupOrch, Caps: []string{CapJSON}})
	addVerb("schedule", Entry{Stage: StageCross, Group: GroupOrch, Caps: []string{CapJSON, CapSpawnsProcesses}})
	addVerb("act", staged(StageTest, managed(GroupOrch, TierSandbox)))
	addVerb("act-runner", staged(StageTest, managed(GroupOrch, TierSandbox, CapDaemon)))
	addVerb("mirror", Entry{Stage: StageCross, Group: GroupOrch, Caps: []string{CapDaemon, CapSpawnsProcesses}})

	// knowledge
	addVerb("kb", Entry{Stage: StageCross, Group: GroupKnowledge, Caps: []string{CapJSON}})
	// lexicon: what do this project's words mean HERE? It PROJECTS the atlas and the
	// fleet registry into the channels agents read -- it introduces NO new source of
	// truth, which is the test it has to keep passing. The moment it starts STORING
	// vocabulary rather than projecting it, it has become the hand-written glossary
	// that the whole data-catalog industry exists because it failed.
	addVerb("lexicon", Entry{Stage: StageCross, Group: GroupKnowledge, Caps: []string{CapJSON, CapReadOnly}})
	// claim: who is working in this project. CROSS -- you collide at any stage:
	// planning, coding, testing, deploying. Two agents writing one project is how
	// an untested change reaches main.
	addVerb("claim", Entry{Stage: StageCross, Group: GroupOrch, Caps: []string{CapJSON}})
	// steward: WHO ANSWERS FOR THIS HOST, and what actually happened on it. Exactly one
	// seat per host/user, held under a monotonic fencing epoch, over an append-only
	// hash-chained journal that outlives whoever holds it — board, status, log, history
	// and checkpoints are read-only projections of that one record.
	//
	// CROSS, and for the same reason claim is: authority is not a stage. A steward
	// crashes mid-plan, mid-refactor, mid-deploy, and the successor's first question is
	// the same in every case — who was in charge, what did they do, and which of their
	// claims did anybody actually check?
	//
	// Distinct from claim and from handoff, which is why it is a third verb rather than
	// a flag on either: claim says who is working in a PROJECT, handoff moves a working
	// TREE, and steward holds a MANDATE. Claiming the seat restores no diff and touches
	// no repository — work is a diff, a seat is not.
	addVerb("steward", Entry{Stage: StageCross, Group: GroupOrch, Caps: []string{CapJSON}})
	addVerb("skills", Entry{Stage: StageCross, Group: GroupKnowledge, Caps: []string{CapJSON}})

	// engines
	addVerb("podman", Entry{Stage: StageCross, Group: GroupEngines, Tier: TierSandbox,
		Caps: []string{CapDaemon, CapSpawnsProcesses}})
	addVerb("docker", Entry{Stage: StageCross, Group: GroupEngines, Tier: TierSandbox, AliasOf: "podman",
		Caps: []string{CapDaemon, CapSpawnsProcesses}})
	addVerb("ollama", Entry{Stage: StageCross, Group: GroupEngines, Tier: TierSphere,
		Caps: []string{CapDaemon, CapNeedsNetwork, CapSpawnsProcesses}})
	addVerb("sphere", Entry{Stage: StageDeploy, Group: GroupEngines, Tier: TierSphere,
		Caps: []string{CapNeedsNetwork, CapNeedsPairing, CapSpawnsProcesses}})

	// forge
	addVerb("git", staged(StageCross, managed(GroupForge, TierUserland, CapNeedsNetwork)))
	addVerb("git-scm", staged(StageCross, provisioner(GroupForge, CapNeedsNetwork)))
	addVerb("gh", staged(StageCross, managed(GroupForge, TierUserland, CapNeedsNetwork)))
	addVerb("loom", staged(StageCross, managed(GroupForge, TierWorkspace, CapDaemon)))

	// net
	addVerb("web", Entry{Stage: StageCross, Group: GroupNet, Caps: []string{CapJSON}})
	addVerb("curl", staged(StageCross, provisioner(GroupNet, CapNeedsNetwork)))

	// toolchains (self-provisioning, agent-mode shims)
	for _, n := range []string{
		"go", "cmake", "clang", "node", "npm", "npx", "pnpm", "yarn",
		"python", "pip", "uv", "mise", "cargo", "rustc", "rustup", "rust",
	} {
		// A compiler/package-manager is a CODE-stage tool: it is how the thing
		// gets built. (`bashy go` also runs tests, but so does every compiler —
		// the stage is what the verb is FOR, not every use it can be put to.)
		e := staged(StageCode, provisioner(GroupToolchains))
		if n == "rust" {
			e.AliasOf = "rustc"
		}
		addVerb(n, e)
	}

	// storage
	addVerb("rclone", staged(StageCross, managed(GroupStorage, TierUserland, CapNeedsNetwork)))
	addVerb("zot", staged(StageCross, managed(GroupStorage, TierUserland, CapDaemon)))
	addVerb("seaweedfs", staged(StageCross, managed(GroupStorage, TierUserland, CapDaemon)))
	addVerb("kopia", staged(StageCross, managed(GroupStorage, TierUserland, CapDaemon)))

	// cluster
	addVerb("kubectl", staged(StageDeploy, managed(GroupClusterCloud, TierCluster, CapJSON, CapNeedsNetwork)))
	addVerb("helm", staged(StageDeploy, managed(GroupClusterCloud, TierCluster, CapNeedsNetwork)))

	// platform
	addVerb("commands", Entry{Stage: StageCross, Group: GroupPlatform, Caps: []string{CapJSON, CapReadOnly}})
	addVerb("context", Entry{Stage: StageCross, Group: GroupPlatform, Caps: []string{CapJSON, CapReadOnly}})
	addVerb("doctor", Entry{Stage: StageCross, Group: GroupPlatform, Caps: []string{CapReadOnly}})
	addVerb("otel", Entry{Stage: StageCross, Group: GroupPlatform, Caps: []string{CapJSON, CapReadOnly, CapNeedsNetwork}})
	addVerb("audit", Entry{Stage: StageCross, Group: GroupPlatform, Caps: []string{CapJSON, CapReadOnly}})
	addVerb("check", Entry{Stage: StageTest, Group: GroupPlatform, Caps: []string{CapJSON, CapReadOnly}})
	// gate: THE Test verb. Before it, the Test stage was EMPTY -- not because
	// nobody tested, but because the gate (the command that decides pass/fail)
	// was spelled four incompatible ways across four packages: weave's
	// suite-gate file, sdlc's healthcheck: key, supervise's :: string, and a
	// dag target that happens to fail. All four mean the same thing; they only
	// disagreed about where the command lives. This is the one place it lives.
	// The Plan stage's intake verb: the durable, committed register of bugs,
	// features and requirements — filed BEFORE anyone starts work. Plan had only
	// sprint (a conductor's live board) and meet (deliberation); neither can hold
	// an untriaged thought, so those lived as bullets in docs/TODO.md.
	addVerb("issue", Entry{Stage: StagePlan, Group: GroupOrch, Tier: TierWorkspace, Caps: []string{CapJSON}})
	// todo is level 1 of the tracking hierarchy: issue is per-repo committed, sprint
	// is cross-repo, todo is the per-host/user personal list (~/.bashy/todo/<owner>/,
	// NOT committed) — the steward's/human's/fixer's own running list of what they are
	// doing across every thread. Userland tier: one host, no repo, no forge, no cloud.
	addVerb("todo", Entry{Stage: StagePlan, Group: GroupOrch, Tier: TierUserland, Caps: []string{CapJSON}})
	// judge is gate's SEMANTIC twin: gate asks "does it PASS" (mechanical,
	// reproducible); judge asks "is it GOOD" (an LLM opinion, advisory unless
	// --gate). Together they finally encode "sandbox-green is not mergeable".
	addVerb("pair", Entry{Stage: StageTest, Group: GroupOrch, Tier: TierWorkspace, Caps: []string{CapJSON, CapSpawnsProcesses}})
	addVerb("judge", Entry{Stage: StageTest, Group: GroupOrch, Tier: TierWorkspace, Caps: []string{CapJSON, CapSpawnsProcesses}})
	addVerb("gate", Entry{Stage: StageTest, Group: GroupPlatform, Caps: []string{CapJSON, CapSpawnsProcesses}})
	// conform: BASHY'S OWN fidelity batteries (bash-5.3 compat / POSIX conformance /
	// VSC-PCTS compliance / benchmark). Renamed from `verify` 2026-07-12: it had
	// claimed the most general word in the vocabulary for the narrowest possible
	// thing — verifying BASHY ITSELF. A project that ADOPTS bashy would reach for
	// `bashy verify` to ask "does MY code pass?" and get bash's conformance suites.
	// The general pass/fail question is `bashy gate`.
	addVerb("conform", Entry{Stage: StageTest, Group: GroupPlatform, Caps: []string{CapSpawnsProcesses}})
	addVerb("verify", Entry{Stage: StageTest, Group: GroupPlatform, AliasOf: "conform", Caps: []string{CapSpawnsProcesses}})
	addVerb("self", Entry{Stage: StageCross, Group: GroupPlatform, Caps: []string{CapCached, CapNeedsNetwork}})
	addVerb("bootstrap", Entry{Stage: StageCross, Group: GroupPlatform, AliasOf: "self",
		Caps: []string{CapCached, CapNeedsNetwork}})
	addVerb("upgrade", Entry{Stage: StageCross, Group: GroupPlatform, AliasOf: "self",
		Caps: []string{CapCached, CapNeedsNetwork}})
	addVerb("run", Entry{Stage: StageCross, Group: GroupPlatform, Caps: []string{CapJSON, CapSpawnsProcesses}})
	addVerb("secrets", Entry{Stage: StageCross, Group: GroupPlatform, Caps: []string{CapNeedsNetwork, CapNeedsPairing}})

	// account
	addVerb("tessaro", Entry{Stage: StageCross, Group: GroupAccount, Tier: TierAccount,
		Caps: []string{CapNeedsNetwork, CapNeedsPairing, CapSpawnsProcesses}})
	addVerb("login", Entry{Stage: StageCross, Group: GroupAccount, Tier: TierAccount,
		Caps: []string{CapNeedsNetwork, CapNeedsPairing, CapSpawnsProcesses}})

	// --- security-effect classification ------------------------------------
	//
	// What each command can DO, from a security/privacy/governance lens. Runs
	// last, over BOTH tables, because eff() resolves a name in either. The
	// coverage ratchet requires ≥1 effect on every entry, so a new command that
	// is added without a line here fails the build by name — classification is
	// mandatory, never fail-open. A command legitimately lists several atoms.

	// pure — deterministic, touches nothing governed.
	eff(EffPure,
		"basename", "dirname", "dircolors", "cal", "ncal", "duration", "echo",
		"expr", "factor", "false", "true", "numfmt", "seq", "sleep", "yes",
		"sync",
	)

	// read — reads filesystem, host state, or input data (the privacy surface).
	eff(EffRead,
		// fileutils/inspection
		"df", "du", "ls", "dir", "vdir", "stat", "readlink", "realpath", "tree",
		"find", "clip",
		// textutils (transform/read input)
		"awk", "cat", "cmp", "comm", "csplit", "cut", "diff", "expand", "fmt",
		"fold", "grep", "gzip", "gunzip", "head", "hexdump", "join", "jq",
		"more", "nl", "od", "paste", "pr", "ptx", "sed", "shuf", "sort",
		"split", "strings", "tac", "tail", "tee", "tokens", "tr", "tsort",
		"unexpand", "uniq", "wc", "xargs", "zcat",
		"b2sum", "cksum", "md5sum", "sha1sum", "sha224sum", "sha256sum",
		"sha384sum", "sha512sum", "sum", "base32", "base64", "basenc",
		// handoff READS the working tree (the diff + untracked files it captures);
		// resume READS the record. Both are a privacy surface: a handoff record
		// carries real source, so treat it like the working tree it came from.
		"handoff", "resume",
		// host info
		"arch", "groups", "hostid", "hostname", "id", "logname", "nproc",
		"pathchk", "pinky", "pwd", "tty", "tz", "uname", "uptime", "users",
		"which", "who", "whoami", "atq", "date", "env", "printenv", "ntp",
		"sntp",
		// code-intel / net
		"ast", "graph", "browser", "fetch",
		// verbs that read stores / remote state
		"capability", "agent", "tools", "models", "agents", "people", "whois",
		"kb", "skills", "lexicon", "claim", "git", "web", "rclone", "kopia", "commands", "context",
		"doctor", "otel", "audit", "check", "sprint",
		// steward READS the host's authority record (status/board/log/history/reconcile)
		// and WRITES it (below). A privacy surface: the journal is a durable account of
		// what agents did on this machine, and its transcripts can carry real
		// conversation.
		"steward",
		// issue READS the committed register (`list`/`show`) and WRITES it (below).
		"issue",
		// todo READS the host-scoped personal task list (`list`/`show`) and WRITES it
		// (below). A privacy surface: ~/.bashy/todo/ is a durable account of what the
		// steward/user is doing across every thread.
		"todo",
	)

	// write — mutates the filesystem or host state (short of irreversible loss).
	eff(EffWrite,
		// issue writes the project's COMMITTED issue register (.bashy/issues/) —
		// source, not scratch, so a write here lands in the repo's history.
		"issue",
		// todo writes the host-scoped personal task list (~/.bashy/todo/<owner>/) —
		// home, not a repo; NOT committed. Level 1 of the tracking hierarchy.
		"todo",
		"clip", "cp", "install", "link", "ln", "mkdir", "mkfifo", "mknod",
		"mktemp", "mv", "rmdir", "tar", "touch",
		"awk", "csplit", "gzip", "gunzip", "sed", "split", "tee", "graph",
		"stty", "atrm", "crontab",
		// handoff WRITES a portable record; resume WRITES the captured working
		// tree back into a checkout. Both also read (below). Neither execs: v1
		// dispatch PRINTS the command to launch a successor rather than spawning
		// one behind the user's back.
		"handoff", "resume",
		// verbs
		"weave", "sprint", "dag", "sdlc", "supervise", "capability", "agent",
		"tools", "models", "agents", "people", "kb", "skills", "lexicon", "claim", "mirror", "git",
		"git-scm", "gh", "curl", "helm", "self", "bootstrap", "upgrade",
		"rclone",
		// steward APPENDS to the host's journal and rewrites the seat/grant files. It is
		// write, not destroy: the one thing that removes bytes (`steward repair`) refuses
		// anything but a torn final append, and quarantines the exact bytes it discards
		// BEFORE truncating, so nothing it does is irreversible.
		"steward",
	)

	// destroy — can IRREVERSIBLY lose data.
	eff(EffDestroy, "dd", "rm", "shred", "truncate", "unlink")

	// net — opens a network connection (the egress / exfiltration surface).
	eff(EffNet,
		"ntp", "sntp", "browser", "fetch",
		"sdlc", "chat", "invoke", "meet", "pair", "judge", "tools", "models", "agents", "act",
		"act-runner", "mirror", "podman", "docker", "ollama", "sphere", "git",
		"git-scm", "gh", "loom", "web", "curl", "rclone", "zot", "seaweedfs",
		"kopia", "kubectl", "helm", "self", "bootstrap", "upgrade", "secrets",
		"otel", "tessaro", "login",
	)

	// exec — spawns a process bashy no longer governs (the coreutils userland,
	// the advisor, and the audit hook do not reach across an execve).
	eff(EffExec,
		"find", "awk", "xargs", "at", "batch", "chroot", "nice", "nohup",
		"runcon", "stdbuf", "time", "timeout", "watch", "env", "foreman",
		"weave", "dag", "sdlc", "chat", "invoke", "meet", "pair", "judge", "supervise", "schedule", "act",
		"act-runner", "skills", "podman", "docker", "ollama", "sphere",
		"git-scm", "loom", "curl", "zot", "seaweedfs", "kopia", "kubectl",
		"verify", "conform", "gate", "run", "tessaro", "login",
	)

	// cred — reads or writes credentials / secrets. `env`/`printenv` are here
	// because they emit the whole environment, secrets included — the reason the
	// context-redaction allowlist must also cover them.
	eff(EffCred, "env", "printenv", "git", "git-scm", "gh", "secrets", "tessaro", "login")

	// priv — changes privilege, ownership, or a security label.
	eff(EffPriv, "chcon", "chgrp", "chmod", "chown", "install", "mknod", "chroot", "runcon")

	// remote — executes on ANOTHER host (crosses the machine boundary). `dag`
	// pipes a Host:-tagged target body to a remote `bash -s`; sphere runs pooled
	// compute on peers; mirror/rclone push to a remote endpoint.
	eff(EffRemote, "dag", "mirror", "sphere", "rclone", "kubectl", "helm")

	// persist — leaves something that OUTLIVES the session: a cron entry, a
	// daemon, an installed/upgraded binary.
	eff(EffPersist,
		"at", "batch", "crontab", "nohup", "foreman",
		"schedule", "act-runner", "mirror", "podman", "docker", "ollama",
		"loom", "zot", "seaweedfs", "kopia", "self", "bootstrap", "upgrade",
	)

	// spend — incurs metered cost: paid inference the agent drives, pooled
	// compute, or cloud resources.
	// judge SPENDS: every reviewer is a metered inference call, and a --panel 3
	// costs three of them. An agent must be able to see that before it fans out.
	eff(EffSpend, "chat", "invoke", "meet", "pair", "judge", "supervise", "sdlc", "weave", "foreman", "sphere", "ollama")

	// The toolchain provisioners each download over the network and then run
	// arbitrary code (a compiler / package manager / interpreter — npm and pip
	// run install scripts), so they are net+exec+write as a class.
	for _, n := range []string{
		"go", "cmake", "clang", "node", "npm", "npx", "pnpm", "yarn",
		"python", "pip", "uv", "mise", "cargo", "rustc", "rustup", "rust",
	} {
		eff(EffNet, n)
		eff(EffExec, n)
		eff(EffWrite, n)
	}

	// Deterministic ordering for every consumer.
	for n, e := range tools {
		sort.Strings(e.Caps)
		sort.Strings(e.Effects)
		tools[n] = e
	}
	for n, e := range verbs {
		sort.Strings(e.Caps)
		sort.Strings(e.Effects)
		verbs[n] = e
	}
}
