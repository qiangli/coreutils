package principal

import (
	"os"
	"strings"

	"github.com/qiangli/coreutils/pkg/fleet"
	"github.com/qiangli/coreutils/pkg/spacetime"
)

// Resolver answers "who or what is this name, and how do I reach it".
type Resolver struct {
	cat   *fleet.Catalog
	env   Env
	probe *spacetime.ProbeSet
	owner string
}

// NewResolver builds a resolver over a fleet catalog and the ambient host.
func NewResolver(cat *fleet.Catalog, env Env) *Resolver {
	return &Resolver{cat: cat, env: env, probe: spacetime.DefaultProbes(nil), owner: LocalOwner}
}

// Self reports the principal this process is running as.
//
// Resolution order, best evidence first:
//
//  1. $BASHY_PRINCIPAL — a minted nickname the launcher injected. Authoritative.
//  2. $BASHY_AGENT_ID / $WEAVE_AGENT — a launcher-set nickname, pre-URN.
//  3. an environment marker of a known harness — this yields a TOOL identity,
//     never a fabricated nickname, because a bare tool is not an agent.
//  4. nothing: an unattributed process.
//
// An agent does not prove it is 007. The launcher writes the name into the
// spawned process's environment, and the process can only sign with what it
// was given. Forging it means already controlling the launcher — that is,
// owning the machine. The OS boundary is the trust boundary; agent-side
// signing keys would add ceremony without moving it.
func (r *Resolver) Self() (Ref, bool) {
	if urn := os.Getenv("BASHY_PRINCIPAL"); urn != "" {
		if kind, name, _, err := ParseURN(urn); err == nil {
			return Ref{URN: urn, Kind: kind, Name: name, Episode: episode(), Host: r.hostID()}, true
		}
	}
	if nick := firstEnv("BASHY_AGENT_ID", "BASHY_AGENT", "WEAVE_AGENT"); nick != "" {
		return Ref{
			URN: URN(KindAgent, nick, r.owner), Kind: KindAgent, Name: nick,
			Episode: episode(), Host: r.hostID(),
		}, true
	}
	if tool, ok := r.DetectTool(); ok {
		return Ref{
			URN: URN(KindTool, tool, r.owner), Kind: KindTool, Name: tool,
			Episode: episode(), Host: r.hostID(),
		}, true
	}
	return Ref{Host: r.hostID(), Episode: episode()}, false
}

// DetectTool identifies the harness running this process from its own
// environment markers, which the fleet catalog declares per tool.
func (r *Resolver) DetectTool() (string, bool) { return r.cat.DetectTool() }

func episode() string { return firstEnv("BASHY_EPISODE", "WEAVE_EPISODE") }

// hostID is the name this machine answers to: the one a control plane knows
// it by when paired, else its own hostname.
func (r *Resolver) hostID() string {
	if r.env.PairedName != "" {
		return r.env.PairedName
	}
	return r.env.Hostname
}

// Resolve answers a short-form or typed query.
//
// A bare name is looked up in every kind. Matching more than one kind is
// ambiguity the caller must break with `kind:name`, not something to guess
// at. Matching one kind many times is impossible: names are unique within a
// kind, and CheckAliases enforces it.
func (r *Resolver) Resolve(query string) Answer {
	ans := Answer{SchemaVersion: SchemaVersion, Query: query}
	want, name := SplitQuery(query)

	try := func(k Kind, f func(string) (Resolution, bool)) {
		if want != "" && want != k {
			return
		}
		if res, ok := f(name); ok {
			ans.Matches = append(ans.Matches, res)
		}
	}
	try(KindAgent, r.resolveAgent)
	try(KindTool, r.resolveTool)
	try(KindModel, r.resolveModel)
	try(KindPerson, r.resolvePerson)
	try(KindHost, r.resolveHost)

	ans.Resolved = len(ans.Matches) > 0
	return ans
}

// --- per-kind ------------------------------------------------------------

func (r *Resolver) resolveAgent(name string) (Resolution, bool) {
	a, ok := r.cat.Agent(name)
	if !ok {
		return Resolution{}, false
	}
	res := Resolution{
		URN: URN(KindAgent, a.Name, r.owner), Kind: KindAgent, Name: a.Name,
		Owner: r.owner, Aliases: a.Aliases, Display: a.Display,
		Summary: a.MatrixKey(),
		Facts:   [][2]string{{"binding", a.MatrixKey()}},
	}
	if a.Description != "" {
		res.Facts = append(res.Facts, [2]string{"description", a.Description})
	}
	if a.Ledger != nil && a.Ledger.Reliability != "" {
		res.Facts = append(res.Facts, [2]string{"reliability", a.Ledger.Reliability})
	}

	chk := r.cat.VerifyAgent(a.Name, r.probe)
	_, tool, model, err := r.cat.Binding(a.Name)
	if err != nil {
		res.Contacts = []Contact{{
			Method: "cli", Source: "fleet", Confidence: Declared,
			Live: false, Why: err.Error(),
		}}
		return res, true
	}

	cli := Contact{
		// TargetFor, not Target: the id a model answers to is a property of the
		// TOOL asking (see Model.TargetFor). Rendering the global Target() here
		// handed ycode `moonshot/kimi-k3` — which the moonshot API 404s, so ycode
		// silently fell back to kimi-k2.5. The contact whois shows must be the id
		// that actually reaches the model, matching agentlaunch and verify.
		Method: "cli", Address: strings.Join(tool.Argv(model.TargetFor(tool.Name), "{prompt}"), " "),
		Source: "fleet", Confidence: Declared, Live: chk.OK, Cost: 10,
	}
	if !chk.OK {
		cli.Why = chk.Reason
	}
	chat := Contact{
		Method: "chat", Address: "bashy chat --agent " + a.Name,
		Source: "bashy", Confidence: Declared, Live: chk.OK, Cost: 20,
	}
	if !chk.OK {
		chat.Why = chk.Reason
	}
	res.Contacts = []Contact{cli, chat}
	rankContacts(res.Contacts)
	return res, true
}

func (r *Resolver) resolveTool(name string) (Resolution, bool) {
	t, ok := r.cat.Tool(name)
	if !ok {
		return Resolution{}, false
	}
	res := Resolution{
		URN: URN(KindTool, t.Name, r.owner), Kind: KindTool, Name: t.Name,
		Owner: r.owner, Aliases: t.Aliases, Display: t.Display,
		Summary: t.Kind + " harness",
	}
	if t.CLI.Binary != "" {
		res.Facts = append(res.Facts, [2]string{"binary", t.CLI.Binary})
	}
	if t.Quirks != "" {
		res.Facts = append(res.Facts, [2]string{"quirks", t.Quirks})
	}

	chk := r.cat.VerifyTool(t.Name, r.probe)
	c := Contact{
		Method: "cli", Address: t.CLI.Launch.Exec,
		Source: "fleet", Confidence: Declared, Live: chk.OK, Cost: 10,
	}
	if !chk.OK {
		c.Why = chk.Reason
	} else if chk.Detail != "" {
		res.Facts = append(res.Facts, [2]string{"version", chk.Detail})
	}
	res.Contacts = []Contact{c}
	return res, true
}

func (r *Resolver) resolveModel(name string) (Resolution, bool) {
	m, ok := r.cat.Model(name)
	if !ok {
		return Resolution{}, false
	}
	res := Resolution{
		URN: URN(KindModel, m.Name, r.owner), Kind: KindModel, Name: m.Name,
		Owner: r.owner, Aliases: m.Aliases, Display: m.Display,
		Summary: m.Kind + " backend",
		Facts:   [][2]string{{"target", m.Target()}},
	}
	if m.Provider != "" {
		res.Facts = append(res.Facts, [2]string{"provider", m.Provider})
	}

	chk := r.cat.VerifyModel(m.Name, r.probe)
	var c Contact
	switch m.Kind {
	case fleet.ModelKindAPI:
		c = Contact{Method: "api", Address: firstNonEmpty(m.BaseURL, m.Provider), Cost: 20}
	case fleet.ModelKindLocal:
		c = Contact{Method: "gateway", Address: "pooled inference on a paired host", Cost: 5}
	default:
		c = Contact{Method: "seat", Address: m.Provider + " subscription", Cost: 10}
	}
	c.Source, c.Confidence, c.Live = "fleet", Declared, chk.OK
	if !chk.OK {
		c.Why = chk.Reason
	}
	res.Contacts = []Contact{c}
	return res, true
}

func (r *Resolver) resolvePerson(name string) (Resolution, bool) {
	p, ok := r.cat.Person(name)
	if !ok {
		return Resolution{}, false
	}
	res := Resolution{
		URN: URN(KindPerson, p.Handle, p.Email), Kind: KindPerson, Name: p.Handle,
		Owner: p.Email, Aliases: p.Aliases, Display: p.Display,
	}
	if p.Email != "" {
		res.Facts = append(res.Facts, [2]string{"email", p.Email})
	}
	for _, h := range p.Hosts {
		if u, known := p.OSUserFor(h); known {
			res.Facts = append(res.Facts, [2]string{"account on " + h, u})
		}
	}

	var cs []Contact
	if p.Email != "" {
		cs = append(cs, Contact{
			Method: "email", Address: "mailto:" + p.Email,
			Source: "fleet", Confidence: Declared, Live: true, Cost: 10,
		})
	}
	cs = append(cs, Contact{
		Method: "mention", Address: "@" + p.Handle,
		Source: "fleet", Confidence: Declared, Live: true, Cost: 1,
	})
	rankContacts(cs)
	res.Contacts = cs
	return res, true
}

func (r *Resolver) resolveHost(name string) (Resolution, bool) {
	alias, hasAlias := r.cat.Host(name)
	self := name != "" && (name == r.env.Hostname || name == r.env.PairedName)

	var cfg sshHostConfig
	if r.env.SSHConfig != "" {
		cfg = readSSHConfig(r.env.SSHConfig, name)
	}
	loc := observeLAN(r.env, name)

	// A host resolves when something vouches for its existence: an alias the
	// operator wrote, an ssh_config stanza they maintain, this machine
	// itself, an mDNS answer, or a plain name lookup. An arbitrary string is
	// not a host — otherwise every typo would resolve.
	if !hasAlias && !cfg.Exact && !self && !loc.sameLAN && !resolvable(r.env, name) {
		return Resolution{}, false
	}

	who, _ := r.cat.Person(r.env.LocalUser)
	res := Resolution{
		URN: URN(KindHost, name, r.owner), Kind: KindHost, Name: name,
		Owner: r.owner, Aliases: alias.Aliases, Display: alias.Display,
	}
	switch {
	case self:
		res.Summary = "this machine"
	case loc.sameLAN:
		res.Summary = "same network (observed via mdns)"
	default:
		res.Summary = "remote or unknown network"
	}
	if alias.Notes != "" {
		res.Facts = append(res.Facts, [2]string{"notes", alias.Notes})
	}
	res.Facts = append(res.Facts, [2]string{"same_network", boolWord(loc.sameLAN)})
	res.Facts = append(res.Facts, [2]string{"paired", boolWord(r.env.Paired)})

	res.Contacts = hostContacts(name, alias, cfg, loc, who, r.env)
	return res, true
}

func boolWord(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
