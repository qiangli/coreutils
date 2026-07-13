// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package steward

import (
	"github.com/qiangli/coreutils/pkg/policy/coord"
	"github.com/qiangli/coreutils/pkg/principal"
)

// selfRef resolves who this process is, DELEGATING to the coord identity rule
// rather than inventing a second one.
//
// This matters more than it looks. coord.Self mints and exports BASHY_EPISODE when
// the ambient environment has none — the gap that made two human-launched agent
// sessions mutually invisible. If steward resolved identity its own way, the same
// agent could be "session-a1b2c3" to the claim registry and something else to the
// seat, and the singleton would be enforced against a name that nothing else uses.
// One host, one steward, one way of saying who you are.
func selfRef() principal.Ref { return coord.Self() }
