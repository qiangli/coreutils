package spacetime

import (
	"runtime"
	"slices"
	"strings"
)

// coreProbeList is the ONE definition of the built-in probe table. It is a
// function rather than a literal inside DefaultProbes so the keying rules below
// and the probe set itself read the same list — a probe added here is covered by
// both, and by the ratchet test, without anyone remembering to update a second
// copy.
func coreProbeList() []Probe {
	return []Probe{
		{Name: "os", Eval: func() (string, error) { return runtime.GOOS, nil }},
		{Name: "arch", Eval: func() (string, error) { return runtime.GOARCH, nil }},
		{Name: "os.release", Eval: probeOSRelease},
		{Name: "libc", Eval: probeLibc},
		{Name: "container", Eval: probeContainer},
		{Name: "tty", Eval: probeTTY},
		{Name: "elevated", Eval: probeElevated},
		{Name: "time.hour", Eval: probeTimeHour, Volatile: true},
		{Name: "time.weekday", Eval: probeTimeWeekday, Volatile: true},
		{Name: "time.zone", Eval: probeTimeZone, Volatile: true},
		{Name: "time.attended", Eval: probeTimeAttended, Volatile: true},
		{Name: "place.id", Eval: probePlaceID, Volatile: true},
	}
}

// unkeyableNamespaces are probe prefixes whose values may be GATED on but must
// never enter a context key. Listed by prefix so a future `time.minute` is
// covered the moment it exists, not the moment someone remembers it.
var unkeyableNamespaces = []string{"time."}

// IsUnkeyableProbe reports whether a probe must be excluded from a context key.
//
// WHY, and why this is narrower than "volatile":
//
// A context key is the coordinate a skill's evidence is filed under, and
// evidence only accumulates when repeated runs land on the SAME coordinate. A
// probe is therefore unkeyable when its value churns WITHOUT the host's
// capability changing — the coordinate moves while the thing it describes does
// not, every run files under a fresh address, and the corpus fragments to n=1.
// Nothing errors when that happens; the store just quietly fills with
// singletons, which is exactly why this is enforced rather than advised.
//
// Wall-clock time is the clean case and the one §11.4 pins ("wall-clock time is
// never a probe"): the hour advances monotonically and unboundedly, and a host
// is no more or less capable at 21:00 than at 09:00.
//
// Volatility alone is NOT the test, and conflating the two would break working
// behaviour. `net.same_lan`, `mesh.paired`, `peer.online` and `place.id` are all
// volatile, and all legitimately keyable: whether a peer is reachable directly
// really is a different capability, so an entry that references them SHOULD
// re-key when the host roams. TestContextKeyDoesNotFragmentOnUnreferencedProbes
// in volatile_test.go pins that, and it stays true. Their cardinality is bounded
// by the network topology a host actually visits, not by a clock.
//
// The general rule, if this list ever grows: exclude a probe when its value
// changes on a dimension the skill's success does not depend on.
func IsUnkeyableProbe(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	for _, ns := range unkeyableNamespaces {
		if strings.HasPrefix(name, ns) {
			return true
		}
	}
	return false
}

// UnkeyableCoreProbes returns the built-in probe names excluded from context
// keys, sorted. Intended for diagnostics and for the ratchet test that asserts
// the set is non-empty — an empty set would mean the guard protects nothing.
func UnkeyableCoreProbes() []string {
	var out []string
	for _, p := range coreProbeList() {
		if IsUnkeyableProbe(p.Name) {
			out = append(out, p.Name)
		}
	}
	slices.Sort(out)
	return out
}

// KeyableProbes filters a probe-name list down to the facts safe to key on,
// preserving order and dropping duplicates.
func KeyableProbes(names []string) []string {
	out := make([]string, 0, len(names))
	for _, n := range names {
		if IsUnkeyableProbe(n) || slices.Contains(out, n) {
			continue
		}
		out = append(out, n)
	}
	return out
}
