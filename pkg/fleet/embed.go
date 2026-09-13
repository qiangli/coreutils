package fleet

import "embed"

// baselineRoot is the prefix every embedded noun directory sits under.
const baselineRoot = "baseline"

// baselineFS is the compiled-in ring-0 fleet: the TOOLS bashy knows about
// with no configuration, no shared catalog, and no cloudbox — every launch
// contract, including the declared-but-not-measured ones. Every higher
// ring shadows it; nothing ever writes to it.
//
// It ships NO models and NO agents. A tool's launch contract is bashy's to
// know; which models an operator can reach and what they call them is not,
// and a compiled-in roster went stale the day it was cut. Those two nouns
// come only from the rings above: a shared dir on $BASHY_MODELS_PATH /
// $BASHY_AGENTS_PATH, an org overlay pulled by `sync`, or the local store.
// pkg/fleet/testdata/ring holds the roster the tests resolve against (see
// fleettest.Ring); it is not shipped content.
//
// The tools are the single source of truth for the launch contracts,
// capability priors, and env markers that used to be duplicated across
// pkg/chat, pkg/weave, pkg/capability, and pkg/skills.
//
//go:embed baseline/tools/*.yaml
var baselineFS embed.FS
