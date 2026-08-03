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
	// To is the agent expected to act. EMPTY MEANS EVERYONE — a broadcast.
	// It is a hint about audience, never a permission: every reader can see
	// every post.
	To    string `json:"to,omitempty"`
	Topic string `json:"topic,omitempty"`
	Body  string `json:"body"`
}

// Broadcast reports a post addressed to everyone.
func (p Post) Broadcast() bool { return strings.TrimSpace(p.To) == "" }

// ForReader reports whether a post is addressed to this reader (or to all).
func (p Post) ForReader(reader string) bool {
	return p.Broadcast() || strings.EqualFold(strings.TrimSpace(p.To), strings.TrimSpace(reader))
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

// Unseen returns the posts this reader has not been shown — those addressed to
// it or broadcast, after its cursor.
func Unseen(reader string) ([]Post, error) {
	all, err := Posts()
	if err != nil {
		return nil, err
	}
	at := SeenSeq(reader)
	var out []Post
	for _, p := range all {
		if p.Seq > at && p.ForReader(reader) {
			out = append(out, p)
		}
	}
	return out, nil
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
