package fleet

import "embed"

// baselineRoot is the prefix every embedded noun directory sits under.
const baselineRoot = "baseline"

// baselineFS is the compiled-in ring-0 fleet: the tools, models, and
// agents bashy knows about with no configuration, no shared catalog, and
// no cloudbox. Every higher ring shadows it; nothing ever writes to it.
//
// It is the single source of truth for the launch contracts, capability
// priors, and env markers that used to be duplicated across pkg/chat,
// pkg/weave, pkg/capability, and pkg/skills.
//
//go:embed baseline/tools/*.yaml baseline/models/*.yaml baseline/agents/*.yaml
var baselineFS embed.FS
