package principal

// Roles are the names the ADDRESSER already resolves — `bashy ping steward`
// posts to the seat's stable topic — and the resolver must answer about them
// the same way, or two commands that both resolve names give contradictory
// answers about the same name in the same second (measured on a live host
// 2026-08-25: `ping steward` delivered while `whois steward` said "names
// nothing"). See docs/agent-comms-synergy.md.
//
// pkg/principal cannot import pkg/bus (bus-adjacent packages import this one
// back), so the role table arrives through a registered source: pkg/bus
// bridges its HostRoles registry — the very table the addresser consults —
// into here from its own init(). One table, two front doors, no drift.

import "strings"

// HostRole is an addressable seat on this host. Label is what a person types
// ("steward", "conductor:22"); Topic is the stable board address it resolves
// to ("steward.<scope>"), which is what makes mail survive a handover.
type HostRole struct {
	Label string
	Topic string
}

// roleSources are the registered role tables. Composed, never replaced: the
// bridge from pkg/bus is one source, and a host may add its own.
var roleSources []func() []HostRole

// RegisterRoleSource adds a source of addressable roles. Sources compose;
// registering never replaces an earlier registration.
func RegisterRoleSource(src func() []HostRole) {
	if src == nil {
		return
	}
	roleSources = append(roleSources, src)
}

// hostRoles lists every addressable role the registered sources know.
func hostRoles() []HostRole {
	var out []HostRole
	for _, src := range roleSources {
		out = append(out, src()...)
	}
	return out
}

// resolveRole answers for a seat, by label or by its topic — the same two
// spellings the addresser accepts.
func (r *Resolver) resolveRole(name string) (Resolution, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Resolution{}, false
	}
	for _, hr := range hostRoles() {
		if !strings.EqualFold(hr.Label, name) && !strings.EqualFold(hr.Topic, name) {
			continue
		}
		return Resolution{
			URN: URN(KindRole, hr.Label, r.owner), Kind: KindRole, Name: hr.Label,
			Owner: r.owner, Source: SourceHost, Confidence: Declared,
			Summary: "addressable seat on this host — mail to it survives a handover",
			Facts:   [][2]string{{"topic", hr.Topic}},
			Contacts: []Contact{{
				Method: "mb", Address: "bashy ping " + hr.Label + " \"<message>\"",
				Source: SourceHost, Confidence: Declared, Live: true, Cost: 5,
			}},
		}, true
	}
	return Resolution{}, false
}
