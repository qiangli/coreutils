package llmbudget

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func (g *Gate) policy() (*Policy, error) {
	p := g.cfg.Policy
	if p == nil {
		path := g.cfg.PolicyPath
		if path == "" {
			path = os.Getenv("BASHY_LLM_BUDGET_POLICY")
		}
		if path == "" && g.cfg.StatePath != "" {
			path = filepath.Join(filepath.Dir(g.cfg.StatePath), "llm-budget-policy.json")
		}
		if path == "" {
			return &Policy{Version: 1}, nil
		}
		b, err := readBounded(path, 1<<20)
		if os.IsNotExist(err) {
			return &Policy{Version: 1}, nil
		}
		if err != nil {
			return nil, errors.New("llmbudget: policy unreadable")
		}
		p = new(Policy)
		dec := json.NewDecoder(strings.NewReader(string(b)))
		dec.DisallowUnknownFields()
		if err = dec.Decode(p); err != nil {
			return nil, errors.New("llmbudget: invalid policy")
		}
	}
	if p.Version != 1 {
		return nil, errors.New("llmbudget: unsupported policy version")
	}
	seen := map[string]bool{}
	for _, b := range p.Bindings {
		if b.Model == "" || b.Provider == "" || b.Account == "" || b.Pool == "" || !validLane(b.Lane) {
			return nil, errors.New("llmbudget: binding requires model, provider, account, pool and lane")
		}
		k := b.Model + "\x00" + b.Agent
		if seen[k] {
			return nil, errors.New("llmbudget: duplicate model/agent binding")
		}
		seen[k] = true
	}
	for _, c := range p.Constraints {
		for _, v := range []*int64{c.DailyTokens, c.WeeklyTokens, c.DailySpendMicroUSD, c.WeeklySpendMicroUSD} {
			if v != nil && *v < 0 {
				return nil, errors.New("llmbudget: negative limit")
			}
		}
		if c.Concurrency != nil && *c.Concurrency < 0 || c.HostSlots != nil && *c.HostSlots < 0 {
			return nil, errors.New("llmbudget: negative concurrency")
		}
	}
	seen = map[string]bool{}
	for _, s := range p.Sources {
		if s.ID == "" || seen[s.ID] || s.Provider == "" || s.Account == "" || s.Pool == "" || !validLane(s.Lane) {
			return nil, errors.New("llmbudget: invalid/duplicate source identity")
		}
		seen[s.ID] = true
	}
	return p, nil
}
func validLane(l Lane) bool { return l == LaneAPIKey || l == LaneSubscription || l == LaneLocal }
func bindingFor(p *Policy, model, agent string) (Binding, bool) {
	var fallback Binding
	found := false
	for _, b := range p.Bindings {
		if b.Model == model {
			b.AccountKnown = true
			if b.Agent == agent && agent != "" {
				return b, true
			}
			if b.Agent == "" {
				fallback = b
				found = true
			}
		}
	}
	return fallback, found
}
func poolKey(b Binding) string {
	v, _ := json.Marshal([]string{b.Provider, b.Account, b.Pool, string(b.Lane)})
	return string(v)
}
func requestBinding(r Request) Binding {
	return Binding{Provider: r.Provider, Account: r.Account, Pool: r.Pool, Lane: r.Lane, Model: r.Model, Agent: r.Agent, AccountKnown: r.Account != ""}
}
func matches(c Constraint, b Binding, host string) bool {
	return (c.Provider == "" || c.Provider == b.Provider) && (c.Account == "" || c.Account == b.Account) && (c.Pool == "" || c.Pool == b.Pool) && (c.Lane == "" || c.Lane == b.Lane) && (c.Host == "" || c.Host == host)
}
