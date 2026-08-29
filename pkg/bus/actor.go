package bus

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	"github.com/qiangli/coreutils/pkg/room"
)

const identityRefusalTopic = "identity.refusal"

// CurrentSessionClaim is injected by the Bashy host. It returns the raw stable
// tool-session identifier inherited by this process for the named fleet agent.
// The value is hashed before comparison and is never persisted or rendered.
var CurrentSessionClaim func(agentName string) string

// HashSessionClaim returns the one-way representation stored in a room card.
// Hosts use this helper when registering a watcher or interactive session.
func HashSessionClaim(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(raw))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// ResolveAuthoredActor resolves the accountable sender of a manually authored
// coordination message. Unlike BoardIdentity (which also supports public board
// reads), this path treats a launcher-stamped agent principal as authoritative
// and requires an unattributed external harness to prove its live session claim.
func ResolveAuthoredActor(explicit string) (string, error) {
	requested := strings.TrimSpace(explicit)
	principal := strings.TrimSpace(os.Getenv("BASHY_PRINCIPAL"))
	if self, ok := agentNameFromPrincipal(principal); ok {
		if requested != "" {
			claimed := resolveBoardName(requested)
			if !strings.EqualFold(self, claimed) {
				return "", authoredActorRefusal(claimed, self,
					fmt.Sprintf("authenticated agent %q cannot author as %q", self, claimed))
			}
		}
		return resolveRegisteredActorClaim(self, self)
	}

	if DetectHarness != nil {
		if tool, detected := DetectHarness(); detected {
			if requested == "" {
				return "", fmt.Errorf("authored communication: %w: running under %s with no claimed agent identity", ErrUnattributed, tool)
			}
			claimed, registered := resolveAgentName(requested)
			if !registered {
				return "", fmt.Errorf("authored communication: --as %q is not a registered Bashy agent", requested)
			}
			return resolveRegisteredActorClaim(claimed, tool)
		}
	}

	// Public host-human behavior is unchanged. The OS login remains the trust
	// boundary for non-agent names. A registered agent name is still protected
	// by its live claim, even when the caller itself is a plain host process.
	actor, err := BoardIdentity(requested)
	if err != nil {
		return "", err
	}
	if registered, ok := resolveAgentName(actor); ok {
		return resolveRegisteredActorClaim(registered, loginName())
	}
	return actor, nil
}

func resolveRegisteredActorClaim(actor, claimant string) (string, error) {
	canonical, registered := resolveAgentName(actor)
	if !registered {
		return "", fmt.Errorf("authored communication: actor %q is not a registered Bashy agent", actor)
	}
	card, live, err := room.Find(canonical)
	if err != nil {
		return "", fmt.Errorf("authored communication: inspect session claim for %q: %w", canonical, err)
	}
	if live && card.SessionClaim != "" && CurrentSessionClaim != nil {
		current := HashSessionClaim(CurrentSessionClaim(canonical))
		if current != "" && subtle.ConstantTimeCompare([]byte(card.SessionClaim), []byte(current)) == 1 {
			return canonical, nil
		}
	}
	anchor := 0
	if live {
		anchor = card.OwnerPID
		if anchor <= 0 {
			anchor = card.PID
		}
	}
	if !live || anchor <= 0 || !processHasAncestor(anchor) {
		reason := fmt.Sprintf("%s has no matching live session claim for %q", claimant, canonical)
		return "", authoredActorRefusal(canonical, claimant, reason)
	}
	return canonical, nil
}

func agentNameFromPrincipal(principal string) (string, bool) {
	before, nick, ok := strings.Cut(principal, "agent/")
	if !ok || strings.TrimSpace(before) == "" || strings.TrimSpace(nick) == "" {
		return "", false
	}
	name := resolveBoardName(nick)
	return name, name != ""
}

// authoredActorRefusal warns the registered identity whose name was claimed.
// The rejected authored body is deliberately not accepted as an argument, so
// it cannot accidentally enter the warning timeline.
func authoredActorRefusal(claimed, actual, reason string) error {
	if owner, ok := resolveAgentName(claimed); ok {
		warning := fmt.Sprintf("Bashy refused an authored-message identity claim for %s by %s", owner, strings.TrimSpace(actual))
		if err := Publish(Notification{
			Principal: "bashy-identity-guard",
			Topic:     identityRefusalTopic,
			To:        owner,
			Body:      warning,
		}); err != nil {
			return fmt.Errorf("authored communication: %s; warning %q failed: %v", reason, owner, err)
		}
	}
	return fmt.Errorf("authored communication: %s", reason)
}
