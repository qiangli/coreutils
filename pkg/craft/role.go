package craft

// ROLES — what an argument MEANS, so knowledge can move between commands.
//
// The point of this file is TRANSFER, not recall. Recording "ssh with these
// flags worked" only ever helps re-running ssh. Recording "this host answers on
// port 2222 as user xuser" helps ssh, scp, sftp, rsync, git-over-ssh — anything
// that targets the same machine.
//
// That requires knowing what an argument IS, because the same fact renders
// differently everywhere:
//
//	ssh    -p 2222
//	scp    -P 2222          <- capital
//	sftp   -P 2222
//	rsync  -e 'ssh -p 2222'
//
// A human gets that wrong regularly and an agent gets it wrong more. Learning
// the port once and rendering it correctly per command is the whole payoff, and
// it is not reachable by matching argv shapes.
//
// # Realms: the prior that stops a wrong transfer
//
// "user" is not one concept. An ssh login and a PostgreSQL role are different
// namespaces that happen to share a word, and transferring between them
// produces a confident, wrong suggestion.
//
// So every command declares an auth REALM, and a fact only transfers within
// one. That is a hand-written prior — correct on day one with zero observations,
// which a learned model cannot be. Evidence can later refine it per
// (role, from, to) triple; it cannot bootstrap it.
//
// # Secrets are refused at extraction, not filtered downstream
//
// argv is exactly where passwords appear (`mysql -pSECRET`). A deny-list is
// declared per command and the value after such a flag is never read into a
// variable that could be stored or logged. Filtering later would mean the
// secret existed in the pipeline first.

import (
	"strings"
)

// Role is what an argument means. A closed vocabulary: an open one would let
// two commands invent different names for the same thing, which is precisely
// what prevents transfer.
type Role string

const (
	RoleUser     Role = "user"          // the login/account to authenticate as
	RolePort     Role = "port"          // the TCP port
	RoleIdentity Role = "identity_file" // a private key or credential FILE (a path, never a secret)
	RoleJump     Role = "jump_host"     // an intermediate host to route through
	RoleDatabase Role = "database"      // the database/namespace to select
)

// Realm is an authentication namespace. A fact transfers only within one.
type Realm string

const (
	RealmSSH      Realm = "ssh"      // ssh, scp, sftp, rsync — one credential world
	RealmPostgres Realm = "postgres" // psql — `-U` is a DB role, NOT a login
	RealmMySQL    Realm = "mysql"
	RealmHTTP     Realm = "http" // curl, wget
)

// CommandSpec is what bashy knows about one command's argument semantics.
type CommandSpec struct {
	Realm Realm
	// Flags maps a flag to the role of its VALUE (the next argv word).
	Flags map[string]Role
	// BoolFlags take no value; listing them prevents the following word being
	// mistaken for one — without this, `ssh -v host` would record the host as
	// the value of -v.
	BoolFlags map[string]bool
	// Render maps a role back to this command's flag, which is where the
	// ssh/scp capitalisation trap is actually solved.
	Render map[Role]string
	// SecretFlags are flags whose value must never be captured.
	SecretFlags map[string]bool
	// AttachedSecret marks flags that carry the secret in the SAME word
	// (`mysql -pSECRET`), which a next-word deny-list would miss entirely.
	AttachedSecret []string
	// HostPositional says the target host appears as a positional argument.
	HostPositional bool
	// HostHasPath marks the `host:path` operand form (scp, sftp, rsync). It is
	// the discriminator between a local file and a remote target: without it,
	// `scp file.txt remote-host:/tmp` records "file.txt" as the host, because a
	// bare filename parses as a hostname perfectly well.
	HostHasPath bool
}

// commandSpecs is the per-command table. Small on purpose: fifteen commands
// cover nearly every connection an operator makes, and each entry is the one
// place per-command knowledge genuinely earns its cost.
var commandSpecs = map[string]CommandSpec{
	"ssh": {
		Realm:          RealmSSH,
		Flags:          map[string]Role{"-p": RolePort, "-l": RoleUser, "-i": RoleIdentity, "-J": RoleJump},
		BoolFlags:      map[string]bool{"-v": true, "-vv": true, "-vvv": true, "-q": true, "-t": true, "-T": true, "-A": true, "-N": true, "-f": true, "-4": true, "-6": true, "-C": true},
		Render:         map[Role]string{RolePort: "-p", RoleUser: "-l", RoleIdentity: "-i", RoleJump: "-J"},
		HostPositional: true,
	},
	"scp": {
		HostHasPath: true,
		Realm:       RealmSSH,
		// -P, capital. The single most-forgotten flag difference in this family,
		// and the clearest thing this table buys.
		Flags:          map[string]Role{"-P": RolePort, "-i": RoleIdentity, "-J": RoleJump},
		BoolFlags:      map[string]bool{"-r": true, "-v": true, "-q": true, "-C": true, "-p": true, "-3": true},
		Render:         map[Role]string{RolePort: "-P", RoleIdentity: "-i", RoleJump: "-J"},
		HostPositional: true,
	},
	"sftp": {
		HostHasPath:    true,
		Realm:          RealmSSH,
		Flags:          map[string]Role{"-P": RolePort, "-i": RoleIdentity, "-J": RoleJump},
		BoolFlags:      map[string]bool{"-r": true, "-v": true, "-q": true},
		Render:         map[Role]string{RolePort: "-P", RoleIdentity: "-i", RoleJump: "-J"},
		HostPositional: true,
	},
	"rsync": {
		HostHasPath: true,
		Realm:       RealmSSH,
		Flags:       map[string]Role{},
		BoolFlags:   map[string]bool{"-a": true, "-v": true, "-z": true, "-r": true, "-P": true, "-n": true},
		// rsync carries the port INSIDE its transport string, which is why the
		// render map is a direction of its own rather than a mirror of Flags.
		Render:         map[Role]string{},
		HostPositional: true,
	},
	"psql": {
		// A DIFFERENT realm. `-U` is a database role; transferring an ssh login
		// here would be a confident wrong answer.
		Realm:          RealmPostgres,
		Flags:          map[string]Role{"-p": RolePort, "-U": RoleUser, "-h": RoleJump, "-d": RoleDatabase},
		BoolFlags:      map[string]bool{"-l": true, "-q": true, "-t": true},
		Render:         map[Role]string{RolePort: "-p", RoleUser: "-U", RoleDatabase: "-d"},
		SecretFlags:    map[string]bool{"-W": true},
		HostPositional: false,
	},
	"mysql": {
		Realm:     RealmMySQL,
		Flags:     map[string]Role{"-P": RolePort, "-u": RoleUser, "-h": RoleJump, "-D": RoleDatabase},
		BoolFlags: map[string]bool{"-e": false},
		Render:    map[Role]string{RolePort: "-P", RoleUser: "-u", RoleDatabase: "-D"},
		// `mysql -pSECRET` attaches the password to the flag itself.
		AttachedSecret: []string{"-p", "--password="},
		HostPositional: false,
	},
	"curl": {
		Realm:          RealmHTTP,
		Flags:          map[string]Role{},
		SecretFlags:    map[string]bool{"-u": true, "--user": true, "-H": true, "--header": true},
		Render:         map[Role]string{},
		HostPositional: false,
	},
}

// SpecFor returns what is known about a command's arguments.
func SpecFor(binary string) (CommandSpec, bool) {
	s, ok := commandSpecs[strings.TrimSpace(binary)]
	return s, ok
}

// Extraction is what one invocation taught.
type Extraction struct {
	// Entity is what the facts are ABOUT — the target host. Zero when the
	// command named no entity, in which case the facts have nothing to bind to
	// and are dropped.
	Entity Entity
	Realm  Realm
	Roles  map[Role]string
	// Redacted counts values refused because their flag carries a secret.
	// Surfaced so a caller can see the deny-list firing rather than assume it.
	Redacted int
}

// Extract reads roles and a target entity out of an invocation.
//
// It reads only what a command DECLARES it means. An unknown binary yields
// nothing rather than a guess: inventing roles from flag shapes is how `-p`
// becomes "port" for a command where it means "preserve".
func Extract(argv []string) (Extraction, bool) {
	if len(argv) == 0 {
		return Extraction{}, false
	}
	spec, ok := SpecFor(argv[0])
	if !ok {
		return Extraction{}, false
	}

	out := Extraction{Realm: spec.Realm, Roles: map[Role]string{}}
	var positionals []string

	for i := 1; i < len(argv); i++ {
		a := argv[i]

		// Attached secrets first: `-pSECRET` is one word, so a next-word check
		// would never see it.
		if hasAttachedSecret(spec, a) {
			out.Redacted++
			continue
		}
		if spec.SecretFlags[a] {
			out.Redacted++
			i++ // consume the value without reading it
			continue
		}
		if role, isRole := spec.Flags[a]; isRole {
			if i+1 < len(argv) {
				out.Roles[role] = argv[i+1]
				i++
			}
			continue
		}
		if spec.BoolFlags[a] {
			continue
		}
		if strings.HasPrefix(a, "-") {
			continue // an unknown flag is skipped, never guessed at
		}
		positionals = append(positionals, a)
	}

	if spec.HostPositional {
		// For the host:path form, the REMOTE operand is the one carrying a
		// colon; everything else is a local file.
		if spec.HostHasPath {
			var remote []string
			for _, p := range positionals {
				if strings.Contains(p, ":") {
					remote = append(remote, p)
				}
			}
			positionals = remote
		}
		for _, p := range positionals {
			host, user := splitTarget(p)
			if host == "" {
				continue
			}
			out.Entity = Entity{Kind: EntityHost, Name: host}
			// `user@host` states the login inline, and it should not be lost
			// just because it was not given as a flag.
			if user != "" {
				if _, already := out.Roles[RoleUser]; !already {
					out.Roles[RoleUser] = user
				}
			}
			break
		}
	} else if h, hasHost := out.Roles[RoleJump]; hasHost {
		// For commands where the host arrives as a flag (-h), that flag IS the
		// entity rather than a jump host.
		out.Entity = Entity{Kind: EntityHost, Name: h}
		delete(out.Roles, RoleJump)
	}

	return out, out.Entity.Valid() && len(out.Roles) > 0
}

// splitTarget pulls host and optional user out of a target operand, handling
// `user@host`, `host:/path`, and `user@host:/path`.
func splitTarget(p string) (host, user string) {
	// A local path is not a host. Checking the separator rather than the
	// filesystem keeps this a pure function.
	if strings.HasPrefix(p, "/") || strings.HasPrefix(p, ".") {
		return "", ""
	}
	if i := strings.Index(p, ":"); i >= 0 {
		p = p[:i]
	}
	if i := strings.Index(p, "@"); i >= 0 {
		user, p = p[:i], p[i+1:]
	}
	if p == "" || strings.Contains(p, "/") {
		return "", user
	}
	return p, user
}

func hasAttachedSecret(spec CommandSpec, arg string) bool {
	for _, p := range spec.AttachedSecret {
		if len(arg) > len(p) && strings.HasPrefix(arg, p) {
			return true
		}
	}
	return false
}

// Transfers reports whether a role learned from one command may be suggested
// for another.
//
// The realm check is the prior, and it is what makes the system correct before
// it has any evidence: an ssh login and a psql role are different namespaces
// that share a word, and suggesting one for the other is a confident wrong
// answer. Observation can later refine this per (role, from, to); it cannot
// bootstrap it, because the first wrong suggestion has already been made by
// then.
//
// This answers the SEMANTIC question only — whether the fact is about the same
// thing in both commands. Whether the target can express it as a flag is a
// separate question, answered by RenderRole, and conflating the two was a real
// bug: scp accepts a login perfectly well, just as `user@host` rather than
// `-l`, so a realm check that demanded a flag reported "does not transfer" for
// a fact that plainly does.
func Transfers(role Role, from, to string) bool {
	a, okA := SpecFor(from)
	b, okB := SpecFor(to)
	if !okA || !okB {
		return false
	}
	return a.Realm == b.Realm
}

// RenderRole formats a known fact as the target command's flag.
//
// This is where the ssh/scp capitalisation trap is actually paid off: the same
// port renders `-p` for ssh and `-P` for scp, and a caller never has to know.
// A role the target cannot express returns false rather than a plausible guess.
func RenderRole(binary string, role Role, value string) (string, bool) {
	spec, ok := SpecFor(binary)
	if !ok {
		return "", false
	}
	flag, ok := spec.Render[role]
	if !ok || strings.TrimSpace(value) == "" {
		return "", false
	}
	return flag + " " + value, true
}

// FactsFrom converts an extraction into entity-bound facts.
//
// The role is the key, so the fact is stated in transferable terms rather than
// in one command's flag spelling — which is the difference between knowledge
// that moves and knowledge that does not.
func FactsFrom(x Extraction, source string) []Fact {
	if !x.Entity.Valid() {
		return nil
	}
	out := make([]Fact, 0, len(x.Roles))
	for role, value := range x.Roles {
		out = append(out, Fact{
			Entity: x.Entity,
			Key:    string(role),
			Value:  value,
			Source: source,
		})
	}
	return out
}
