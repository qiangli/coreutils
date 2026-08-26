package commscli

// The atlas is this repo's manifest of the verbs a host mounts — the closest
// thing the coreutils side has to a record of "reachable from the CLI". The
// inbox/notify gap was invisible precisely along this axis: the constructors
// shipped, no atlas verb entry existed, and no test connected the two. These
// tests draw that line explicitly, in both directions.

import (
	"testing"

	"github.com/qiangli/coreutils/pkg/atlas"
)

// Every S80 verb this audit proves behaviorally must also be declared in the
// atlas. A verb that behaves but is not in the manifest is exactly one host
// refactor away from becoming unreachable without any test noticing.
func TestS80VerbsAreDeclaredInTheAtlas(t *testing.T) {
	for _, v := range []string{"whois", "agent", "mb", "bus", "weave"} {
		if _, ok := atlas.Lookup(v); !ok {
			t.Errorf("verb %q is audited as CLI-reachable but has no atlas entry — the manifest and the surface have drifted", v)
		}
	}
}

// KNOWN DEFECT, pinned (S80 follow-up). pkg/bus exports NewInboxCmd and
// NewNotifyCmd, and audit_test.go proves both behave from a built binary —
// but neither verb has an atlas entry, and the shipped bashy host does not
// mount them (`bashy inbox` → exit 127 on the host this audit was written
// on). They are reachable-in-principle, unreachable-in-fact: the precise
// state this audit exists to detect.
//
// This test pins the gap so it cannot be silently forgotten OR silently
// half-fixed: the day `inbox`/`notify` gain atlas entries (i.e. someone
// mounts them), it fails — telling the fixer to (1) add both verbs to the
// REQUIRED list in scripts/comms-cli-audit.sh, (2) run that script against
// the freshly built bashy, and (3) delete this test and move the two verbs
// into TestS80VerbsAreDeclaredInTheAtlas above.
func TestInboxAndNotifyReachabilityGapIsStillOpen(t *testing.T) {
	for _, v := range []string{"inbox", "notify"} {
		if _, ok := atlas.Lookup(v); ok {
			t.Errorf("%q now has an atlas entry — the pinned reachability gap has been (at least partly) closed. "+
				"Promote %q into TestS80VerbsAreDeclaredInTheAtlas, add it to the REQUIRED verbs in "+
				"scripts/comms-cli-audit.sh, verify against a freshly built bashy, and delete this test.", v, v)
		}
	}
}
