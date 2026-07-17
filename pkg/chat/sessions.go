package chat

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
)

// LiveSession is one governed agent session launched by `bashy chat`, recorded so
// the rest of the fleet can reach it.
//
// It is the connective tissue that makes a chat-launched agent ADDRESSABLE:
// steer / interrupt / attach / observe (and later coach / meet) all resolve an id
// to its control socket + capture log through this board. It is deliberately
// fleet-wide (~/.bashy/sessions/), not chat-private — the live-agent board a
// human, a supervisor, or another surface reads to find what is running.
type LiveSession struct {
	ID      string `json:"id"`
	Binding string `json:"binding"`
	Nick    string `json:"nick,omitempty"`
	Tool    string `json:"tool"`
	Model   string `json:"model,omitempty"`
	Band    int    `json:"band,omitempty"`
	CtlSock string `json:"ctl_sock"`
	LogPath string `json:"log_path,omitempty"`
	// PID is the foreground `bashy chat` process holding the session. Steering is
	// pid-independent (it goes to CtlSock); the pid is the liveness key — the
	// session is live exactly while this process is, so a crashed launcher leaves
	// a stale file that listSessions prunes rather than a lie on the board.
	PID     int    `json:"pid"`
	Cwd     string `json:"cwd,omitempty"`
	Started string `json:"started"`
}

func sessionsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".bashy", "sessions")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

var idSanitize = regexp.MustCompile(`[^a-zA-Z0-9]+`)

// sessionID is a short, human-typable handle: the label the caller would say
// (nick or tool) plus the launcher pid, which is unique per host. Readable enough
// to `chat steer claude-12345 "..."` without copy-pasting a hash.
func sessionID(l Launch) string {
	label := strings.TrimSpace(l.Nick)
	if label == "" || label == l.Binding() {
		label = l.ToolName
	}
	label = strings.Trim(idSanitize.ReplaceAllString(label, "-"), "-")
	if label == "" {
		label = "agent"
	}
	return fmt.Sprintf("%s-%d", label, os.Getpid())
}

func registerSession(s LiveSession) error {
	dir, err := sessionsDir()
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, s.ID+".json"), b, 0o600)
}

func deregisterSession(id string) {
	dir, err := sessionsDir()
	if err != nil {
		return
	}
	_ = os.Remove(filepath.Join(dir, id+".json"))
}

// listSessions returns the live governed sessions, newest first, pruning any whose
// launcher process is gone (a crash left the file behind). The prune keeps the
// board honest without a sweeper — reading it IS the reconciliation.
func listSessions() ([]LiveSession, error) {
	dir, err := sessionsDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []LiveSession
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		p := filepath.Join(dir, e.Name())
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var s LiveSession
		if json.Unmarshal(b, &s) != nil || s.ID == "" {
			continue
		}
		if !pidAlive(s.PID) {
			_ = os.Remove(p) // stale: the launcher is gone
			continue
		}
		out = append(out, s)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Started > out[j].Started })
	return out, nil
}

// findSession resolves an id to a live session. A bare tool/nick prefix matches
// when it is unambiguous, so an operator can type `claude` when only one claude
// session is up instead of the full `claude-12345`.
func findSession(id string) (LiveSession, error) {
	id = strings.TrimSpace(id)
	sessions, err := listSessions()
	if err != nil {
		return LiveSession{}, err
	}
	if id == "" {
		if len(sessions) == 1 {
			return sessions[0], nil
		}
		return LiveSession{}, fmt.Errorf("chat: name a session id (%d live) — `bashy chat sessions`", len(sessions))
	}
	for _, s := range sessions {
		if s.ID == id {
			return s, nil
		}
	}
	var pref []LiveSession
	for _, s := range sessions {
		if strings.HasPrefix(s.ID, id) || strings.EqualFold(s.Nick, id) {
			pref = append(pref, s)
		}
	}
	switch len(pref) {
	case 1:
		return pref[0], nil
	case 0:
		return LiveSession{}, fmt.Errorf("chat: no live session %q — `bashy chat sessions`", id)
	default:
		return LiveSession{}, fmt.Errorf("chat: %q matches %d live sessions — use the full id", id, len(pref))
	}
}

func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// Signal 0 probes existence without delivering anything. On platforms without
	// signals this errors, which reads as "not alive" — acceptable, since the
	// interactive session it guards needs a pty those platforms also lack.
	return p.Signal(syscall.Signal(0)) == nil
}
