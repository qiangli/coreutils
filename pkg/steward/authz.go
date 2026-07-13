// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package steward

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/qiangli/coreutils/pkg/principal"
)

// DefaultGrantTTL is how long an authorization stays usable. Short on purpose: a
// capability to seize the host's seat is not a thing to leave lying around, and the
// human who minted it is, by construction, right there.
const DefaultGrantTTL = 15 * time.Minute

// MaxGrantTTL caps --ttl. A capability that outlives the situation that justified it
// is a backdoor with a nice name.
const MaxGrantTTL = 24 * time.Hour

// Provenance says HOW an authorization was obtained — and, therefore, exactly how
// much it is worth. It is recorded verbatim in the journal, forever, because the
// difference between these two is the difference between a receipt and a promise.
type Provenance string

const (
	// ProvenanceOperatorAssertion — somebody with write access to this store ran
	// `steward authorize` and asserted, on an interactive terminal, that they are the
	// named actor.
	//
	// BE CLEAR ABOUT WHAT THIS IS NOT. It is not proof that a human was present. This
	// package cannot produce such proof: it runs as the user, on the user's machine,
	// with the user's filesystem, and ANYTHING else running as that user — including
	// the agent it is meant to restrain — can write this store's files directly. There
	// is no signature to check, no second party to ask, no secret the agent does not
	// also have. A design that claimed otherwise would be lying, and the lie would be
	// load-bearing in the worst place: the recovery path.
	//
	// So what IS it worth? It is a DURABLE, REPLAY-PROTECTED, AUDITABLE capability. It
	// forces the seizure of a seat to be a separate, deliberate, single-use act that
	// names an actor and a reason, that expires, that is bound to one epoch and one
	// grantee, and that is written into the permanent record where a human will see it.
	// It stops the ordinary failure — an agent that decides on its own initiative to
	// take over a steward it believes is stuck — and it makes the extraordinary one
	// (an agent forging its own authorization) leave fingerprints all over the journal.
	// That is a real control. It is just not a cryptographic one, and it is labelled
	// as an ASSERTION so nobody downstream mistakes it for one.
	ProvenanceOperatorAssertion Provenance = "operator-assertion"

	// ProvenanceExternalReceipt — the authorization is backed by an artifact produced
	// OUTSIDE this process: an approval comment, a signed ticket, a change record, a
	// pager acknowledgement. The bytes are copied into the store and pinned by digest,
	// so the receipt can be re-checked later and cannot be quietly swapped.
	//
	// This is still not a proof of humanity — this package cannot verify a signature it
	// has no root of trust for, and it does not pretend to. What it adds over an
	// assertion is that the evidence exists somewhere this agent does not control, and
	// the digest makes it checkable by someone who can go and look.
	//
	// It is REQUIRED for a NON-INTERACTIVE takeover. If nobody is at a terminal, an
	// assertion of human presence is worth precisely nothing, and the only honest thing
	// to demand is a receipt that a human can go and audit against its source.
	ProvenanceExternalReceipt Provenance = "external-receipt"
)

// ExternalReceipt pins an out-of-band approval artifact to exact bytes.
type ExternalReceipt struct {
	Issuer string `json:"issuer"`       // who issued it out of band — free text, NOT verified
	ID     string `json:"id,omitempty"` // its identifier over there (a PR number, a ticket)
	Path   string `json:"path"`         // store-relative copy of the bytes
	Digest string `json:"digest"`       // sha256 of those bytes
	Bytes  int64  `json:"bytes,omitempty"`
}

// Grant is a durable, single-use capability to take over the seat.
//
// Every field is a lock on how far it reaches:
//
//	ID          a 128-bit nonce. Consumption is recorded in the JOURNAL (the takeover
//	            entry names it), so replay refuses a second use even if the grant file
//	            is restored from a backup. The journal is the authority here too.
//	Action      what it authorizes. Only "takeover" exists today; a grant is not a
//	            general-purpose skeleton key.
//	Grantee     WHO may use it. A grant minted for one agent is not a coupon another
//	            can pick up.
//	Scope       the host/user store it was minted against. A capability does not
//	            travel between seats.
//	FromEpoch   the exact epoch it authorizes seizing FROM. If the seat moves on —
//	            someone else claims, releases, takes over — the grant is dead. It
//	            authorizes seizing THIS situation, not the seat in general.
//	ExpiresAt   when it stops working.
//
// And read Provenance for the one thing it does NOT do.
type Grant struct {
	SchemaVersion string           `json:"schema_version"`
	ID            string           `json:"id"`
	Action        string           `json:"action"`
	Grantee       principal.Ref    `json:"grantee"`
	Scope         string           `json:"scope"`
	FromEpoch     uint64           `json:"from_epoch"`
	Actor         string           `json:"actor"`
	Provenance    Provenance       `json:"provenance"`
	Receipt       *ExternalReceipt `json:"receipt,omitempty"`
	Reason        string           `json:"reason,omitempty"`
	IssuedAt      time.Time        `json:"issued_at"`
	ExpiresAt     time.Time        `json:"expires_at"`
}

// ActionTakeover is the only action a grant may authorize today.
const ActionTakeover = "takeover"

// GrantRequest is what a caller asks Authorize for.
type GrantRequest struct {
	// Grantee is the agent that will use the grant. Zero value means "whoever runs
	// takeover next on this host" — which is refused: an unbound capability is a
	// skeleton key.
	Grantee principal.Ref
	// Actor is the operator identity being ASSERTED (a human's name, an on-call
	// handle). Required, and recorded verbatim. It is a claim, not a credential.
	Actor  string
	Reason string
	TTL    time.Duration

	// Confirmed says the HOST obtained an interactive, typed confirmation from a
	// terminal before calling. Only meaningful for an operator-assertion grant, and
	// only as strong as the host's honesty — see Provenance.
	Confirmed bool

	// ReceiptPath / ReceiptIssuer / ReceiptID supply an external approval artifact.
	// Supplying one makes the grant an external-receipt grant.
	ReceiptPath   string
	ReceiptIssuer string
	ReceiptID     string
}

// TakeoverRequest is what Takeover is handed.
type TakeoverRequest struct {
	// GrantID names a grant in the store; GrantPath reads one from a file (for a
	// receipt minted elsewhere and copied in). Exactly one is required.
	GrantID   string
	GrantPath string

	// Interactive is the HOST's assertion that a terminal was attached. It is an
	// assertion, not a proof — a caller can set it — and it is recorded as such in the
	// journal. What it buys: a non-interactive takeover is REFUSED unless the grant
	// carries an external receipt, so the unattended path (a cron job, a runaway
	// agent loop, a CI runner) cannot lean on "a human said so" with nothing to show.
	Interactive bool
}

// ErrUnauthorized is returned when a takeover's authorization is missing, invalid,
// expired, replayed, or not strong enough for the circumstances.
type ErrUnauthorized struct{ Why string }

func (e *ErrUnauthorized) Error() string {
	return "steward: takeover refused — " + e.Why + ".\n" +
		"Seizing the seat is not an agent's call to make on its own: an agent that could decide to take over would " +
		"eventually decide to do it to a healthy steward. Mint a capability first:\n" +
		"  steward authorize --actor <who> --reason <why>            (interactive; recorded as an operator ASSERTION)\n" +
		"  steward authorize --actor <who> --receipt <file> --receipt-issuer <src>   (durable, digest-pinned; required when unattended)\n" +
		"then: steward takeover --grant <id>"
}

// newNonce mints a 128-bit grant id.
func newNonce() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("steward: cannot mint an authorization nonce: %w", err)
	}
	return "g-" + hex.EncodeToString(b), nil
}

// Authorize mints a durable authorization capability.
//
// It does NOT require the seat — that is the whole point. The agent that needs
// taking over is the one holding the seat, so a capability only its holder could
// mint would be useless exactly when it is needed.
//
// It DOES bind the grant to the current epoch, so what it authorizes is seizing the
// situation as it stands right now.
func (s *Store) Authorize(req GrantRequest, now time.Time) (Grant, error) {
	now = mustUTC(now)
	if strings.TrimSpace(req.Actor) == "" {
		return Grant{}, &ErrUnauthorized{Why: "an authorization must name the operator asserting it (--actor)"}
	}
	if req.Grantee.Name == "" && req.Grantee.Episode == "" {
		return Grant{}, &ErrUnauthorized{Why: "an authorization must name the agent it is for (an unbound capability is a skeleton key)"}
	}
	ttl := req.TTL
	if ttl <= 0 {
		ttl = DefaultGrantTTL
	}
	if ttl > MaxGrantTTL {
		return Grant{}, &ErrUnauthorized{Why: fmt.Sprintf("a capability to seize the seat may not live longer than %s (asked for %s)", MaxGrantTTL, ttl)}
	}

	var g Grant
	err := s.withLock(func() error {
		rep, err := s.Replay()
		if err != nil {
			return err
		}
		auth := deriveAuthority(rep)

		id, err := newNonce()
		if err != nil {
			return err
		}
		g = Grant{
			SchemaVersion: SchemaVersion,
			ID:            id,
			Action:        ActionTakeover,
			Grantee:       req.Grantee,
			Scope:         s.scope,
			FromEpoch:     auth.Epoch,
			Actor:         strings.TrimSpace(req.Actor),
			Reason:        req.Reason,
			IssuedAt:      now,
			ExpiresAt:     now.Add(ttl),
		}

		switch {
		case req.ReceiptPath != "":
			if strings.TrimSpace(req.ReceiptIssuer) == "" {
				return &ErrUnauthorized{Why: "an external receipt must say who issued it (--receipt-issuer): an artifact with no source is not auditable"}
			}
			rc, err := s.storeReceipt(req)
			if err != nil {
				return err
			}
			g.Provenance, g.Receipt = ProvenanceExternalReceipt, rc
		case req.Confirmed:
			g.Provenance = ProvenanceOperatorAssertion
		default:
			return &ErrUnauthorized{Why: "an operator-assertion authorization requires an interactive confirmation; " +
				"with no terminal attached, supply an external receipt instead (--receipt <file> --receipt-issuer <src>)"}
		}

		return writeJSONAtomic(filepath.Join(s.grantDir(), g.ID+".json"), g)
	})
	if err != nil {
		return Grant{}, err
	}
	return g, nil
}

// storeReceipt copies an approval artifact into the store and pins it by digest, so
// the bytes that justified a seizure survive the seizure and cannot be swapped
// afterwards.
func (s *Store) storeReceipt(req GrantRequest) (*ExternalReceipt, error) {
	f, err := os.Open(req.ReceiptPath)
	if err != nil {
		return nil, fmt.Errorf("steward: authorization receipt: %w", err)
	}
	defer f.Close()

	b, err := io.ReadAll(io.LimitReader(f, MaxReceiptBytes+1))
	if err != nil {
		return nil, fmt.Errorf("steward: authorization receipt: %w", err)
	}
	if int64(len(b)) > MaxReceiptBytes {
		return nil, &ErrUnauthorized{Why: fmt.Sprintf("the authorization receipt exceeds %d bytes", MaxReceiptBytes)}
	}
	if len(b) == 0 {
		return nil, &ErrUnauthorized{Why: "the authorization receipt is empty — an empty artifact attests to nothing"}
	}
	dig := digestOf(b)
	name := strings.TrimPrefix(dig, "sha256:") + ".bin"
	if err := writeBytesAtomic(filepath.Join(s.receiptDir(), name), b); err != nil {
		return nil, err
	}
	return &ExternalReceipt{
		Issuer: strings.TrimSpace(req.ReceiptIssuer),
		ID:     strings.TrimSpace(req.ReceiptID),
		Path:   filepath.ToSlash(filepath.Join("receipts", name)),
		Digest: dig,
		Bytes:  int64(len(b)),
	}, nil
}

// loadGrantFor resolves the grant a takeover names.
func (s *Store) loadGrantFor(req TakeoverRequest) (Grant, error) {
	switch {
	case req.GrantID != "" && req.GrantPath != "":
		return Grant{}, &ErrUnauthorized{Why: "name a grant by id OR by file, not both"}
	case req.GrantID != "":
		return s.LoadGrant(req.GrantID)
	case req.GrantPath != "":
		var g Grant
		found, err := readJSON(req.GrantPath, &g)
		if err != nil {
			return Grant{}, &ErrUnauthorized{Why: "the grant file is unreadable: " + err.Error()}
		}
		if !found {
			return Grant{}, &ErrUnauthorized{Why: "no grant file at " + req.GrantPath}
		}
		return g, nil
	default:
		return Grant{}, &ErrUnauthorized{Why: "no authorization was presented"}
	}
}

// LoadGrant reads a stored grant by id.
func (s *Store) LoadGrant(id string) (Grant, error) {
	if strings.ContainsAny(id, `/\.`) {
		return Grant{}, &ErrUnauthorized{Why: fmt.Sprintf("%q is not a grant id", id)}
	}
	var g Grant
	found, err := readJSON(filepath.Join(s.grantDir(), id+".json"), &g)
	if err != nil {
		return Grant{}, &ErrUnauthorized{Why: "the grant is unreadable: " + err.Error()}
	}
	if !found {
		return Grant{}, &ErrUnauthorized{Why: fmt.Sprintf("no such authorization %q", id)}
	}
	return g, nil
}

// grantConsumed reports whether a grant id has ALREADY been used, according to the
// journal.
//
// The journal is the authority for consumption, not the grant file. A file can be
// deleted, restored from a backup, or copied out and back; the hash-chained record
// of the takeover that used it cannot. This is what makes a grant genuinely
// single-use rather than single-use-if-nobody-touches-the-filesystem.
func grantConsumed(rep *Replay, id string) bool {
	for _, e := range rep.Entries {
		if e.Kind == KindSeatTakeover && e.Authz != nil && e.Authz.GrantID == id {
			return true
		}
	}
	return false
}

// verifyGrant is the full check, run under the store lock against a fresh replay.
func (s *Store) verifyGrant(rep *Replay, g Grant, holder principal.Ref, auth Authority, req TakeoverRequest, now time.Time) error {
	switch {
	case g.SchemaVersion != SchemaVersion:
		return &ErrUnauthorized{Why: fmt.Sprintf("the authorization has schema %q, not %q", g.SchemaVersion, SchemaVersion)}
	case g.ID == "":
		return &ErrUnauthorized{Why: "the authorization carries no nonce, so its single use cannot be tracked"}
	case g.Action != ActionTakeover:
		return &ErrUnauthorized{Why: fmt.Sprintf("the authorization is for %q, not %q", g.Action, ActionTakeover)}
	case g.Scope != s.scope:
		return &ErrUnauthorized{Why: fmt.Sprintf("the authorization was minted for seat %q, not this one (%q) — a capability does not travel between hosts", g.Scope, s.scope)}
	case grantConsumed(rep, g.ID):
		return &ErrUnauthorized{Why: fmt.Sprintf("authorization %s has already been used (the journal records the takeover that consumed it) — a capability is single-use", g.ID)}
	case !SameHolder(g.Grantee, holder):
		return &ErrUnauthorized{Why: fmt.Sprintf("authorization %s was minted for %s, not for %s", g.ID, holderName(g.Grantee), holderName(holder))}
	case g.FromEpoch != auth.Epoch:
		return &ErrUnauthorized{Why: fmt.Sprintf("authorization %s authorizes seizing epoch %d, but the seat is at epoch %d — "+
			"the situation it was minted for is over, and it does not authorize whatever replaced it", g.ID, g.FromEpoch, auth.Epoch)}
	case g.IssuedAt.IsZero() || g.ExpiresAt.IsZero():
		return &ErrUnauthorized{Why: "the authorization has no validity window"}
	case now.Before(g.IssuedAt.UTC().Add(-clockSkew)):
		return &ErrUnauthorized{Why: fmt.Sprintf("authorization %s is not valid yet (issued %s) — a capability dated into the future is a forgery", g.ID, g.IssuedAt.UTC().Format(time.RFC3339))}
	case !now.Before(g.ExpiresAt.UTC()):
		return &ErrUnauthorized{Why: fmt.Sprintf("authorization %s expired at %s", g.ID, g.ExpiresAt.UTC().Format(time.RFC3339))}
	case strings.TrimSpace(g.Actor) == "":
		return &ErrUnauthorized{Why: "the authorization names no operator"}
	}

	switch g.Provenance {
	case ProvenanceExternalReceipt:
		if err := s.verifyReceipt(g.Receipt); err != nil {
			return err
		}
	case ProvenanceOperatorAssertion:
		if !req.Interactive {
			return &ErrUnauthorized{Why: fmt.Sprintf("authorization %s is an operator ASSERTION, and this takeover is unattended. "+
				"An assertion that a human approved is worth nothing with no human present to make it; an unattended seizure "+
				"needs a receipt somebody can go and audit (--receipt at authorize time)", g.ID)}
		}
	default:
		return &ErrUnauthorized{Why: fmt.Sprintf("the authorization has an unknown provenance %q", g.Provenance)}
	}
	return nil
}

// verifyReceipt re-hashes the stored approval artifact. A receipt whose bytes no
// longer match its digest is worse than no receipt: it is one somebody edited.
func (s *Store) verifyReceipt(rc *ExternalReceipt) error {
	if rc == nil {
		return &ErrUnauthorized{Why: "the authorization claims an external receipt but carries none"}
	}
	if rc.Digest == "" || rc.Path == "" {
		return &ErrUnauthorized{Why: "the external receipt is not pinned to any bytes"}
	}
	b, err := os.ReadFile(filepath.Join(s.dir, filepath.FromSlash(rc.Path)))
	if err != nil {
		return &ErrUnauthorized{Why: "the external receipt's bytes are gone (" + rc.Path + ") — the artifact that justified this seizure must be there to be audited"}
	}
	if got := digestOf(b); got != rc.Digest {
		return &ErrUnauthorized{Why: "the external receipt's bytes no longer match its digest — the artifact was altered"}
	}
	return nil
}

// markGrantConsumed is a courtesy: it moves the spent grant out of the way so
// `steward grants` does not show it as available. It is BEST-EFFORT and is allowed
// to fail silently, because the journal — not this file — is what makes the grant
// single-use (see grantConsumed). Failing a completed takeover because we could not
// tidy up a cache would be strictly worse than an untidy cache.
func (s *Store) markGrantConsumed(g Grant, epoch uint64, now time.Time) {
	_ = writeJSONAtomic(filepath.Join(s.grantDir(), g.ID+".consumed.json"), struct {
		Grant
		ConsumedAt    time.Time `json:"consumed_at"`
		ConsumedEpoch uint64    `json:"consumed_epoch"`
	}{Grant: g, ConsumedAt: now, ConsumedEpoch: epoch})
	_ = os.Remove(filepath.Join(s.grantDir(), g.ID+".json"))
}

// GrantStatus is a grant as `steward grants` reports it: the capability, plus
// whether it can still be used and why not.
type GrantStatus struct {
	Grant    Grant  `json:"grant"`
	Usable   bool   `json:"usable"`
	Consumed bool   `json:"consumed"`
	Reason   string `json:"reason,omitempty"`
}

// ListGrants reports every grant in the store and its true status, checked against
// the journal.
func (s *Store) ListGrants(now time.Time) ([]GrantStatus, error) {
	now = mustUTC(now)
	rep, err := s.Replay()
	if err != nil {
		return nil, err
	}
	auth := deriveAuthority(rep)

	des, err := os.ReadDir(s.grantDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []GrantStatus
	for _, de := range des {
		if de.IsDir() || !strings.HasSuffix(de.Name(), ".json") {
			continue
		}
		var g Grant
		if found, err := readJSON(filepath.Join(s.grantDir(), de.Name()), &g); err != nil || !found {
			continue
		}
		st := GrantStatus{Grant: g, Consumed: grantConsumed(rep, g.ID)}
		switch {
		case st.Consumed:
			st.Reason = "already used"
		case !now.Before(g.ExpiresAt.UTC()):
			st.Reason = "expired"
		case g.FromEpoch != auth.Epoch:
			st.Reason = fmt.Sprintf("minted for epoch %d, seat is at %d", g.FromEpoch, auth.Epoch)
		case g.Scope != s.scope:
			st.Reason = "minted for another seat"
		default:
			st.Usable = true
		}
		out = append(out, st)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Grant.IssuedAt.After(out[j].Grant.IssuedAt) })
	return out, nil
}
