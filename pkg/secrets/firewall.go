package secrets

import (
	"os"
	"strings"
)

// AllowAgentSecretsEnv opts a host into passing the vault's secrets through to
// spawned third-party agent CLIs. It is OFF by default, and turning it on is an
// explicit, auditable acceptance of risk — never the default.
//
// Why the default is deny: a coding-agent CLI processes untrusted content (repo
// files, fetched web pages, tool output) AND has its own network egress to an
// external LLM API. Handing it the whole decrypted vault via the inherited
// environment completes the "lethal trifecta" — a single prompt injection can
// exfiltrate every credential the operator holds (cloud keys, DB passwords,
// other providers' tokens), not just the one key the agent needs to run.
//
// This is a coarse P0 stopgap. The credential-firewall end state holds the
// secrets in bashy and injects them at an egress proxy per allowlisted host, so
// the agent never sees a raw key at all; until that lands, deny-by-default is
// the safe posture.
const AllowAgentSecretsEnv = "BASHY_ALLOW_AGENT_SECRETS"

// allowAgentSecrets reports whether the operator has explicitly opted into
// letting spawned agents inherit vault secrets.
func allowAgentSecrets() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(AllowAgentSecretsEnv))) {
	case "", "0", "false", "off", "no":
		return false
	default:
		return true
	}
}

// VaultEnvNames returns the set of environment-variable names the secrets vault
// projects into a shell — the LOCAL names declared in the binding template plus
// any names present in the on-disk render cache. NAMES ONLY, never values, and
// no network call: it is safe to invoke on every agent spawn.
//
// An absent or unreadable template/cache yields an empty set. The names are the
// authoritative "these variables carry vault secrets" signal, because they are
// exactly what `bashy secrets env` writes with `export NAME=...`.
func VaultEnvNames() map[string]struct{} {
	names := map[string]struct{}{}
	// (a) The declared binding template: LOCAL_NAME=@ref | literal. Only the @ref
	// bindings resolve to a VAULT SECRET; a bare literal (e.g. EDITOR=vim) is not
	// sensitive, so it is left in the child's environment.
	if bindings, err := readTemplate(defaultTemplatePath()); err == nil {
		for _, b := range bindings {
			if b.isRef && b.local != "" {
				names[b.local] = struct{}{}
			}
		}
	}
	// (b) The realized render cache (`export NAME='value'` lines): the set that
	// was actually projected into this shell, which can outlive a since-edited
	// template. Names only — parseEnvFile's values are discarded.
	if cp := cacheFile(); cp != "" {
		if f, err := os.Open(cp); err == nil {
			if items, perr := parseEnvFile(f); perr == nil {
				for _, it := range items {
					if it.Name != "" {
						names[it.Name] = struct{}{}
					}
				}
			}
			_ = f.Close()
		}
	}
	return names
}

// ScrubAgentEnv returns env with every vault-projected secret removed, so a
// spawned third-party agent does not inherit the operator's credentials by
// default. It is the one call an agent launcher makes before handing an
// environment to an exec'd agent CLI.
//
// When the operator has set AllowAgentSecretsEnv, or when no vault names are
// known, env is returned unchanged. Removal is by NAME (the vault's projected
// local names); a variable the vault does not project is never touched.
func ScrubAgentEnv(env []string) []string {
	if allowAgentSecrets() {
		return env
	}
	names := VaultEnvNames()
	if len(names) == 0 {
		return env
	}
	out := env[:0:0]
	for _, kv := range env {
		name := kv
		if i := strings.IndexByte(kv, '='); i >= 0 {
			name = kv[:i]
		}
		if _, secret := names[name]; secret {
			continue
		}
		out = append(out, kv)
	}
	return out
}
