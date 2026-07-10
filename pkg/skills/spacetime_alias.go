package skills

import (
	"time"

	"github.com/qiangli/coreutils/pkg/spacetime"
)

// The space-time coordinate engine — host probes, the `requires` grammar
// over them, and the ContextKey fingerprint — lives in pkg/spacetime so
// pkg/fleet and pkg/principal can gate contact methods on the same probes
// skills gates applicability on. These aliases keep the skills API and
// its `metadata.requires` contract unchanged.

type (
	Op        = spacetime.Op
	Clause    = spacetime.Clause
	Requires  = spacetime.Requires
	Verdict   = spacetime.Verdict
	Probe     = spacetime.Probe
	Resolver  = spacetime.Resolver
	ProbeSet  = spacetime.ProbeSet
	Cache     = spacetime.Cache
	FileCache = spacetime.FileCache
)

const (
	OpAnyOf   = spacetime.OpAnyOf
	OpAtLeast = spacetime.OpAtLeast
	OpBool    = spacetime.OpBool
)

// ErrNotApplicable marks a probe with no meaningful value on this host.
var ErrNotApplicable = spacetime.ErrNotApplicable

// ParseRequires parses a metadata.requires string.
func ParseRequires(s string) (Requires, error) { return spacetime.ParseRequires(s) }

// DefaultProbes returns the pinned probe set (core + tool/engine/mesh).
func DefaultProbes(cache Cache) *ProbeSet { return spacetime.DefaultProbes(cache) }

// NopCache caches nothing (tests, --refresh).
func NopCache() Cache { return spacetime.NopCache() }

// NewFileCache persists probe values in <dir>/probecache.json with a TTL.
func NewFileCache(dir string, ttl time.Duration) *FileCache {
	return spacetime.NewFileCache(dir, ttl)
}

// ContextKey fingerprints a probe snapshot (dhnt-runtime byte-compatible).
func ContextKey(vals map[string]string) string { return spacetime.ContextKey(vals) }
