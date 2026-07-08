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
	Subclass string // verbs only: provisioner | managed-external | ""
	Caps     []string
	AliasOf  string // e.g. docker → podman, upgrade → self
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
	return Entry{
		Group:    GroupClusterCloud,
		Tier:     TierName(tier),
		Subclass: SubclassManagedExternal,
		Caps: []string{
			CapCached, CapNeedsNetwork, CapSelfProvisioning, CapSpawnsProcesses,
		},
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
		tools[n] = Entry{Group: group, Tier: TierUserland}
	}
}

func addVerb(name string, e Entry) {
	if _, dup := verbs[name]; dup {
		panic(fmt.Sprintf("atlas: duplicate verb entry %q", name))
	}
	if e.Tier == "" {
		e.Tier = TierUserland
	}
	verbs[name] = e
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
	addTools(GroupCodeIntel,
		"ast-query", "find-references", "list-symbols", "repo-map",
		"search-symbols", "graph-build", "graph-forget", "graph-hotspots",
		"graph-impact", "graph-link", "graph-neighbors", "graph-note",
		"graph-notes", "graph-observe", "graph-path", "graph-pitfalls",
		"graph-query", "graph-recall", "graph-stats",
	)
	addTools(GroupNet, "browser", "fetch")
	addTools(GroupOrch, "foreman")

	// Tool capabilities (evidence per flag: docs/command-atlas.md §2.3).
	capTools(CapJSON,
		"ast-query", "find-references", "list-symbols", "repo-map",
		"search-symbols", "graph-build", "graph-forget", "graph-hotspots",
		"graph-impact", "graph-link", "graph-neighbors", "graph-note",
		"graph-notes", "graph-observe", "graph-path", "graph-pitfalls",
		"graph-query", "graph-recall", "graph-stats",
		"browser", "fetch", "duration", "tz", "ntp", "sntp", "tokens",
		"foreman",
	)
	capTools(CapDryRun, "rm")
	capTools(CapDestructive, "rm", "dd", "shred", "truncate")
	capTools(CapReadOnly,
		"cat", "cmp", "comm", "df", "diff", "du", "grep", "head", "hexdump",
		"ls", "od", "readlink", "realpath", "stat", "strings", "tac", "tail",
		"tokens", "tree", "wc", "which",
		"ast-query", "find-references", "list-symbols", "repo-map",
		"search-symbols", "graph-query", "graph-neighbors", "graph-impact",
		"graph-path", "graph-hotspots", "graph-stats", "graph-notes",
		"graph-recall", "graph-pitfalls",
	)
	capTools(CapCached,
		"graph-build", "graph-query", "graph-neighbors", "graph-impact",
		"graph-path", "graph-hotspots", "graph-stats",
	)
	capTools(CapBudget, "tokens", "repo-map")
	capTools(CapNeedsNetwork, "fetch", "browser", "ntp", "sntp")
	capTools(CapSpawnsProcesses,
		"xargs", "timeout", "time", "watch", "nice", "nohup", "chroot",
		"runcon", "stdbuf", "at", "batch",
	)
	capTools(CapDaemon, "foreman")

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
	addVerb("weave", Entry{Group: GroupOrch, Tier: TierWorkspace, Caps: []string{CapJSON}})
	addVerb("sprint", Entry{Group: GroupOrch, Tier: TierWorkspace, Caps: []string{CapJSON}})
	addVerb("dag", Entry{Group: GroupOrch, Tier: TierWorkspace, Caps: []string{CapJSON}})
	addVerb("sdlc", Entry{Group: GroupOrch, Tier: TierWorkspace, Caps: []string{CapJSON}})
	addVerb("chat", Entry{Group: GroupOrch, Caps: []string{CapJSON, CapSpawnsProcesses}})
	addVerb("meet", Entry{Group: GroupOrch, Caps: []string{CapSpawnsProcesses}})
	addVerb("capability", Entry{Group: GroupOrch, Caps: []string{CapJSON}})
	addVerb("agent", Entry{Group: GroupOrch, Caps: []string{CapJSON}})
	addVerb("schedule", Entry{Group: GroupOrch, Caps: []string{CapJSON, CapSpawnsProcesses}})
	addVerb("act", managed(GroupOrch, TierSandbox))
	addVerb("act-runner", managed(GroupOrch, TierSandbox, CapDaemon))
	addVerb("mirror", Entry{Group: GroupOrch, Caps: []string{CapDaemon, CapSpawnsProcesses}})

	// knowledge
	addVerb("kb", Entry{Group: GroupKnowledge, Caps: []string{CapJSON}})
	addVerb("skills", Entry{Group: GroupKnowledge, Caps: []string{CapJSON}})

	// engines
	addVerb("podman", Entry{Group: GroupEngines, Tier: TierSandbox,
		Caps: []string{CapDaemon, CapSpawnsProcesses}})
	addVerb("docker", Entry{Group: GroupEngines, Tier: TierSandbox, AliasOf: "podman",
		Caps: []string{CapDaemon, CapSpawnsProcesses}})
	addVerb("ollama", Entry{Group: GroupEngines, Tier: TierSphere,
		Caps: []string{CapDaemon, CapNeedsNetwork, CapSpawnsProcesses}})
	addVerb("sphere", Entry{Group: GroupEngines, Tier: TierSphere,
		Caps: []string{CapNeedsNetwork, CapNeedsPairing, CapSpawnsProcesses}})

	// forge
	addVerb("git", managed(GroupForge, TierUserland, CapNeedsNetwork))
	addVerb("git-scm", provisioner(GroupForge, CapNeedsNetwork))
	addVerb("gh", managed(GroupForge, TierUserland, CapNeedsNetwork))
	addVerb("loom", managed(GroupForge, TierWorkspace, CapDaemon))

	// net
	addVerb("web", Entry{Group: GroupNet, Caps: []string{CapJSON}})
	addVerb("curl", provisioner(GroupNet, CapNeedsNetwork))

	// toolchains (self-provisioning, agent-mode shims)
	for _, n := range []string{
		"go", "cmake", "clang", "node", "npm", "npx", "pnpm", "yarn",
		"python", "pip", "uv", "mise", "cargo", "rustc", "rustup", "rust",
		"java", "javac", "mvn",
	} {
		e := provisioner(GroupToolchains)
		if n == "rust" {
			e.AliasOf = "rustc"
		}
		addVerb(n, e)
	}

	// storage
	addVerb("rclone", managed(GroupStorage, TierUserland, CapNeedsNetwork))
	addVerb("zot", managed(GroupStorage, TierUserland, CapDaemon))
	addVerb("seaweedfs", managed(GroupStorage, TierUserland, CapDaemon))
	addVerb("kopia", managed(GroupStorage, TierUserland, CapDaemon))

	// cluster
	addVerb("kubectl", managed(GroupClusterCloud, TierCluster, CapJSON, CapNeedsNetwork))
	addVerb("helm", managed(GroupClusterCloud, TierCluster, CapNeedsNetwork))

	// platform
	addVerb("commands", Entry{Group: GroupPlatform, Caps: []string{CapJSON, CapReadOnly}})
	addVerb("context", Entry{Group: GroupPlatform, Caps: []string{CapJSON, CapReadOnly}})
	addVerb("doctor", Entry{Group: GroupPlatform, Caps: []string{CapReadOnly}})
	addVerb("check", Entry{Group: GroupPlatform, Caps: []string{CapJSON, CapReadOnly}})
	addVerb("verify", Entry{Group: GroupPlatform, Caps: []string{CapSpawnsProcesses}})
	addVerb("self", Entry{Group: GroupPlatform, Caps: []string{CapCached, CapNeedsNetwork}})
	addVerb("bootstrap", Entry{Group: GroupPlatform, AliasOf: "self",
		Caps: []string{CapCached, CapNeedsNetwork}})
	addVerb("upgrade", Entry{Group: GroupPlatform, AliasOf: "self",
		Caps: []string{CapCached, CapNeedsNetwork}})
	addVerb("run", Entry{Group: GroupPlatform, Caps: []string{CapJSON, CapSpawnsProcesses}})
	addVerb("secrets", Entry{Group: GroupPlatform, Caps: []string{CapNeedsNetwork, CapNeedsPairing}})

	// account
	addVerb("tessaro", Entry{Group: GroupAccount, Tier: TierAccount,
		Caps: []string{CapNeedsNetwork, CapNeedsPairing, CapSpawnsProcesses}})
	addVerb("login", Entry{Group: GroupAccount, Tier: TierAccount,
		Caps: []string{CapNeedsNetwork, CapNeedsPairing, CapSpawnsProcesses}})

	// Deterministic ordering for every consumer.
	for n, e := range tools {
		sort.Strings(e.Caps)
		tools[n] = e
	}
	for n, e := range verbs {
		sort.Strings(e.Caps)
		verbs[n] = e
	}
}
