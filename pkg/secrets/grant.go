package secrets

import (
	"strings"
)

// The credential firewall denies by default: ScrubAgentEnv strips every
// vault-projected secret before a third-party agent CLI is exec'd, so an agent
// cannot walk off with the operator's whole keyring.
//
// But denying everything and granting nothing is not a security model, it is an
// outage. A metered model needs exactly one credential in order to answer, and
// the registry has always said which one — `api_key_ref: moonshot` on the model
// entry. Nothing read it. So the firewall stripped MOONSHOT_API_KEY, nothing put
// it back, and every kimi agent failed to authenticate on every run while the
// fleet recorded it as "medium reliability".
//
// (deepseek looked fine only by accident: aider happens to cache that key in its
// own config, so it survived a firewall it was never actually let through.)
//
// GrantAgentKey is the other half. Deny everything; grant precisely what the
// model declared it needs, and nothing else.

// keySuffixes are the shapes a provider credential takes, most specific first.
var keySuffixes = []string{"_API_KEY", "_TOKEN", "_KEY", ""}

// GrantAgentKey finds the one credential a model's api_key_ref names, in the
// parent's environment, and returns it as a NAME=value entry to add back to a
// scrubbed child environment.
//
// `moonshot` resolves MOONSHOT_API_KEY; `deepseek` resolves DEEPSEEK_API_KEY.
// Returns false when the ref names nothing this host has — which is a real
// answer, not a failure: the agent will fail to authenticate and say so, which
// is exactly what `agents verify --live` is for.
func GrantAgentKey(parentEnv []string, ref string) (string, bool) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", false
	}
	base := strings.ToUpper(strings.NewReplacer("-", "_", ".", "_", "/", "_").Replace(ref))

	have := map[string]string{}
	for _, kv := range parentEnv {
		if i := strings.IndexByte(kv, '='); i > 0 {
			have[kv[:i]] = kv
		}
	}
	for _, suf := range keySuffixes {
		if kv, ok := have[base+suf]; ok {
			return kv, true
		}
	}
	return "", false
}
