package bus

// THE BOARD STORE — public by construction, and its own.
//
// `mb` first rode the bus's subscription + pending machinery, and that was the
// wrong substrate for a public board twice over.
//
// # It needed setup it should not need
//
// A post only reached an agent that HELD A SUBSCRIPTION, so the board depended
// on a reconcile step, and a name nobody had reconciled silently received
// nothing. A public board has no membership: you read it because it is there.
//
// # It resolved identity two different ways, and they disagreed
//
// The send side addressed the FLEET NAME (`codex-gpt5.6-sol`, what
// `bashy agents list` prints). The read side resolved `$BASHY_PRINCIPAL` →
// `$USER`, which is `dhnt:agent/Omar` for a bashy-launched agent and the login
// name for everything else. So a post addressed to an agent landed in a buffer
// that agent would never read, and every reader had to be told its own name
// with --as. Two spellings of one identity is the same defect as two live
// things sharing one address; it just fails quietly instead of loudly.
//
// # So the store is one public log plus a per-reader cursor
//
//	posts.jsonl    every post, append-only, world-readable on this host
//	seen/<reader>  one integer: the last sequence that reader has been shown
//
// That is the whole model. There is no per-recipient copy to fall out of sync,
// no membership to forget, and "read someone else's messages" is not a
// privilege escalation — it is the normal case, because the board is public.
// Addressing (`To`) is a HINT about who should act, never an access control.
//
// The bus keeps its subscriptions and pending buffers: they carry interrupts
// and topic-routed notifications, which genuinely are per-subscriber and
// genuinely are governed. The board is the public half and now owns its own
// storage.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// BoardSchema versions the on-disk post record.
const BoardSchema = "bashy-mb-v1"

// Post is one message on the board.
type Post struct {
	SchemaVersion string `json:"schema_version"`
	Seq           int64  `json:"seq"`
	At            string `json:"at"`
	From          string `json:"from"`
	// To is the single agent expected to act. Empty means the post is not
	// directed at one agent — see Audience. It is a hint about who should act,
	// never a permission: every reader can see every post.
	To string `json:"to,omitempty"`
	// Audience is a GROUP the post is for, stored as the SELECTOR rather than
	// expanded into one post per member.
	//
	// Expanding was the first implementation and it made the board grow with
	// the size of the audience: `--band 4` wrote eight identical posts, so
	// `--all` became unreadable and every reader's scan got longer even though
	// only one line concerned them. Storing the selector keeps the board's
	// length independent of how many agents a message reaches.
	//
	// It is resolved AT READ TIME, so relevance follows the role rather than
	// whoever held it when the post was written: "L4 agents should know this"
	// is a statement about the seat, and an agent promoted afterwards should
	// see it. The opposite is defensible; this is the choice.
	Audience *Audience `json:"audience,omitempty"`
	Topic    string    `json:"topic,omitempty"`
	Body     string    `json:"body"`
}

// Broadcast reports a post for everyone: no named recipient and no selector.
func (p Post) Broadcast() bool {
	return strings.TrimSpace(p.To) == "" && (p.Audience == nil || p.Audience.Empty())
}

// Directed reports a post naming ONE agent — the only kind that carries an
// obligation, and therefore the only kind never truncated from a default view.
func (p Post) Directed(reader string) bool {
	return strings.EqualFold(strings.TrimSpace(p.To), strings.TrimSpace(reader)) &&
		strings.TrimSpace(p.To) != ""
}

// Audiences describes a post's intended audience for display.
func (p Post) Audiences() string {
	if p.To != "" {
		return p.To
	}
	if p.Audience == nil || p.Audience.Empty() {
		return "all"
	}
	var parts []string
	if p.Audience.Band != 0 {
		parts = append(parts, "band "+strconv.Itoa(p.Audience.Band))
	}
	for _, kv := range [][2]string{
		{"tool", p.Audience.Tool}, {"provider", p.Audience.Provider},
		{"family", p.Audience.Family}, {"version", p.Audience.Version},
	} {
		if kv[1] != "" {
			parts = append(parts, kv[0]+" "+kv[1])
		}
	}
	return strings.Join(parts, " · ")
}

// ForReader reports whether a post concerns this reader: addressed to it,
// matching a selector it satisfies, or broadcast.
func (p Post) ForReader(reader string) bool {
	if p.Directed(reader) || p.Broadcast() {
		return true
	}
	if p.Audience != nil && !p.Audience.Empty() {
		return InAudience(*p.Audience, reader)
	}
	return false
}

// audienceCache memoizes selector resolution for the life of one command, so a
// board full of selector posts costs one catalog load per DISTINCT selector
// rather than one per post.
var audienceCache = map[Audience]map[string]bool{}

// InAudience reports whether reader is in the set a selector names.
//
// An unresolvable selector matches NOBODY rather than everybody. Erring toward
// silence costs one reader a message they can still find with --all; erring the
// other way turns every group post into a broadcast, which is precisely the
// clutter this design exists to prevent.
func InAudience(aud Audience, reader string) bool {
	if FleetSelect == nil {
		return false
	}
	members, ok := audienceCache[aud]
	if !ok {
		names, err := FleetSelect(aud)
		if err != nil {
			return false
		}
		members = make(map[string]bool, len(names))
		for _, n := range names {
			members[strings.ToLower(strings.TrimSpace(n))] = true
		}
		audienceCache[aud] = members
	}
	return members[strings.ToLower(strings.TrimSpace(reader))]
}

// BoardDir is the board's store. It is deliberately NOT under the room
// directory: the room holds the bus's private per-subscriber state, and mixing
// a public log into it is how the two got confused in the first place.
func BoardDir() string {
	if d := strings.TrimSpace(os.Getenv("BASHY_MB_DIR")); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".bashy", "mb")
}

func postsPath() string { return filepath.Join(BoardDir(), "posts.jsonl") }

// PostMessage appends to the board.
//
// O_APPEND, so several agents post concurrently without a lock — the same
// discipline the room timeline and the graph contribution log use. The sequence
// is assigned from the current line count, which is exact under append-only.
func PostMessage(p Post) error {
	dir := BoardDir()
	if dir == "" {
		return fmt.Errorf("mb: no board directory")
	}
	if strings.TrimSpace(p.From) == "" {
		// A post with no author is unattributable, and an unattributable
		// message on a shared board is worse than none: nobody can ask the
		// sender what they meant.
		return fmt.Errorf("mb: a post needs a sender")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	existing, _ := Posts()
	p.SchemaVersion = BoardSchema
	p.Seq = int64(len(existing)) + 1
	if p.At == "" {
		p.At = time.Now().UTC().Format(time.RFC3339)
	}
	line, err := json.Marshal(p)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(postsPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(line, '\n'))
	return err
}

// Posts returns the whole board, oldest first.
//
// A malformed line is SKIPPED rather than fatal: this is an append-only log
// several processes write, and one torn record must not make the board
// unreadable. An absent board is empty and not an error.
func Posts() ([]Post, error) {
	f, err := os.Open(postsPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var out []Post
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var p Post
		if json.Unmarshal([]byte(line), &p) != nil {
			continue
		}
		out = append(out, p)
	}
	return out, sc.Err()
}

// Unseen returns what this reader has not been shown, split by obligation.
//
// directed posts NAME this reader, so somebody is waiting on it: they are
// returned in full and never truncated. Capping them is how an assignment gets
// dropped.
//
// other is everything else that concerns the reader — broadcasts and selector
// posts — trimmed to the newest limit, with older reporting how many were left
// out. A cap that does not say what it hid is a silent drop, and a reader who
// cannot tell "nothing else" from "twelve more" will act on the wrong one.
func Unseen(reader string, limit int) (directed, other []Post, older int, err error) {
	all, e := Posts()
	if e != nil {
		return nil, nil, 0, e
	}
	at := SeenSeq(reader)
	for _, p := range all {
		if p.Seq <= at || !p.ForReader(reader) {
			continue
		}
		if p.Directed(reader) {
			directed = append(directed, p)
			continue
		}
		other = append(other, p)
	}
	if limit > 0 && len(other) > limit {
		older = len(other) - limit
		other = other[len(other)-limit:]
	}
	return directed, other, older, nil
}

func seenPath(reader string) string {
	name := safeName.ReplaceAllString(strings.TrimSpace(reader), "_")
	name = strings.TrimLeft(name, ".")
	if name == "" {
		name = "anonymous"
	}
	return filepath.Join(BoardDir(), "seen", name)
}

// SeenSeq is the last sequence this reader has been shown. Zero when it has
// never read — so a first-time reader sees the whole board rather than nothing,
// which is what "public" means and the opposite of the private-inbox rule that
// opens a new mailbox at the head.
func SeenSeq(reader string) int64 {
	b, err := os.ReadFile(seenPath(reader))
	if err != nil {
		return 0
	}
	n, err := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// MarkSeen advances a reader's cursor. Never moves it backwards: re-reading an
// older view must not un-see what was already shown.
func MarkSeen(reader string, seq int64) error {
	if seq <= SeenSeq(reader) {
		return nil
	}
	p := seenPath(reader)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(strconv.FormatInt(seq, 10)+"\n"), 0o644)
}

// FleetResolveName maps any spelling of an agent — a nickname, a principal like
// `dhnt:agent/Omar`, a tool:model binding — to its canonical fleet name.
// Injected by the host, for the same reason FleetNames and FleetSelect are.
var FleetResolveName func(string) string

// BoardIdentity is WHO YOU ARE on the board, and it exists because the send and
// read sides used to disagree.
//
// Posts are addressed to the fleet name `bashy agents list` prints. But a
// reader's environment carries something else: a bashy-launched agent has
// BASHY_PRINCIPAL=dhnt:agent/<Nick>, and everything else falls back to $USER.
// Resolving those to the same name is what lets a bare `bashy mb` work, instead
// of every agent having to be told its own identity with --as.
//
// The ladder, most explicit first. Anything that does not resolve to a known
// agent is used AS ITSELF rather than guessed at — a human at a terminal is a
// legitimate board participant under their login name.
func BoardIdentity(as string) string {
	if s := strings.TrimSpace(as); s != "" {
		return resolveBoardName(s)
	}
	if v := strings.TrimSpace(os.Getenv("BASHY_PRINCIPAL")); v != "" {
		// `dhnt:agent/Omar` → `Omar` → the catalog's canonical name.
		if _, nick, ok := strings.Cut(v, "agent/"); ok {
			if n := resolveBoardName(nick); n != "" {
				return n
			}
		}
		if n := resolveBoardName(v); n != "" {
			return n
		}
	}
	for _, k := range []string{"USER", "LOGNAME"} {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return "anonymous"
}

func resolveBoardName(s string) string {
	if FleetResolveName != nil {
		if n := strings.TrimSpace(FleetResolveName(s)); n != "" {
			return n
		}
	}
	return s
}

// DefaultBoardLimit caps posts NOT addressed to a reader by name. Five is a
// screenful: enough that a busy board is still scannable at a turn boundary,
// small enough that it does not become the turn.
const DefaultBoardLimit = 5

// describe renders a selector for a confirmation line.
func (a Audience) describe() string {
	return Post{Audience: &a}.Audiences()
}
