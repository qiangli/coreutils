package meet

// Relay direct messages adapt the governed `bashy chat -m` primitive to the
// web conversation surface. They are deliberately not meetings: no agenda,
// chair, secretary, decisions, or minutes. Chat remains the execution/context
// owner; this file stores only the human-visible transcript Relay must replay.

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/qiangli/coreutils/pkg/chat"
)

type relayDM struct {
	Agent   string    `json:"agent"`
	Human   string    `json:"human"`
	Created time.Time `json:"created"`
	Updated time.Time `json:"updated"`
}

type relayDMEvent struct {
	ID      string    `json:"id"`
	Speaker string    `json:"speaker"`
	Role    string    `json:"role"`
	Text    string    `json:"text"`
	TS      time.Time `json:"ts"`
	Status  string    `json:"status,omitempty"`
}

type relayDMDetail struct {
	State  relayDM        `json:"state"`
	Events []relayDMEvent `json:"events"`
}

var relayDMLocks sync.Map // canonical agent -> *sync.Mutex

var invokeRelayDM = chat.Invoke

func relayDMLock(agent string) *sync.Mutex {
	v, _ := relayDMLocks.LoadOrStore(agent, &sync.Mutex{})
	return v.(*sync.Mutex)
}

func relayDMRoot() (string, error) {
	base, err := baseDir()
	if err != nil {
		return "", err
	}
	root := filepath.Join(base, "relay-dms")
	return root, os.MkdirAll(root, 0o700)
}

func relayDMDir(agent string) (string, error) {
	root, err := relayDMRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, slugify(agent)), nil
}

func ensureRelayDM(agent, human string) (relayDM, error) {
	agent = canonAgent(agent)
	if err := routableSeat(agent); err != nil {
		return relayDM{}, err
	}
	dir, err := relayDMDir(agent)
	if err != nil {
		return relayDM{}, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return relayDM{}, err
	}
	p := filepath.Join(dir, "state.json")
	var st relayDM
	if b, err := os.ReadFile(p); err == nil {
		if err := json.Unmarshal(b, &st); err != nil {
			return relayDM{}, fmt.Errorf("relay: read DM %s: %w", agent, err)
		}
	} else if !os.IsNotExist(err) {
		return relayDM{}, err
	}
	if st.Agent == "" {
		st = relayDM{Agent: agent, Human: human, Created: time.Now().UTC()}
	}
	if st.Human == "" {
		st.Human = human
	}
	st.Updated = time.Now().UTC()
	b, _ := json.MarshalIndent(st, "", "  ")
	if err := atomicWrite(p, append(b, '\n')); err != nil {
		return relayDM{}, err
	}
	return st, nil
}

func relayDMEvents(agent string) ([]relayDMEvent, error) {
	dir, err := relayDMDir(agent)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(filepath.Join(dir, "transcript.jsonl"))
	if os.IsNotExist(err) {
		return []relayDMEvent{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := make([]relayDMEvent, 0)
	s := bufio.NewScanner(f)
	for s.Scan() {
		var event relayDMEvent
		if err := json.Unmarshal(s.Bytes(), &event); err != nil {
			return nil, err
		}
		out = append(out, event)
	}
	return out, s.Err()
}

func appendRelayDMEvent(agent string, event relayDMEvent) error {
	dir, err := relayDMDir(agent)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	event.ID = fmt.Sprintf("%d", time.Now().UnixNano())
	if event.TS.IsZero() {
		event.TS = time.Now().UTC()
	}
	b, _ := json.Marshal(event)
	f, err := os.OpenFile(filepath.Join(dir, "transcript.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(b, '\n'))
	return err
}

func listRelayDMs() ([]relayDM, error) {
	root, err := relayDMRoot()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	out := make([]relayDM, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		b, err := os.ReadFile(filepath.Join(root, entry.Name(), "state.json"))
		if err != nil {
			continue
		}
		var st relayDM
		if json.Unmarshal(b, &st) == nil && st.Agent != "" {
			out = append(out, st)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Updated.After(out[j].Updated) })
	return out, nil
}

func handleRelayDMList(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var body struct{ Agent string }
		if err := decodeBody(r, &body); err != nil {
			apiErr(w, err)
			return
		}
		lock := relayDMLock(canonAgent(body.Agent))
		lock.Lock()
		st, err := ensureRelayDM(body.Agent, actorOf(r, ""))
		lock.Unlock()
		if err != nil {
			apiErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, st)
		return
	}
	items, err := listRelayDMs()
	if err != nil {
		apiErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func handleRelayDMGet(w http.ResponseWriter, r *http.Request) {
	agent := canonAgent(r.PathValue("agent"))
	lock := relayDMLock(agent)
	lock.Lock()
	defer lock.Unlock()
	st, err := ensureRelayDM(agent, actorOf(r, ""))
	if err != nil {
		apiErr(w, err)
		return
	}
	events, err := relayDMEvents(agent)
	if err != nil {
		apiErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, relayDMDetail{State: st, Events: events})
}

func handleRelayDMMessage(ctx context.Context) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		agent := canonAgent(r.PathValue("agent"))
		var body struct{ Text string }
		if err := decodeBody(r, &body); err != nil {
			apiErr(w, err)
			return
		}
		body.Text = strings.TrimSpace(body.Text)
		if body.Text == "" {
			apiErr(w, fmt.Errorf("relay: an empty message is not a contribution"))
			return
		}
		lock := relayDMLock(agent)
		lock.Lock()
		st, err := ensureRelayDM(agent, actorOf(r, ""))
		if err != nil {
			lock.Unlock()
			apiErr(w, err)
			return
		}
		if err := appendRelayDMEvent(agent, relayDMEvent{Speaker: st.Human, Role: "human", Text: body.Text}); err != nil {
			lock.Unlock()
			apiErr(w, err)
			return
		}
		lock.Unlock()
		go runRelayDMTurn(ctx, st, body.Text)
		writeJSON(w, http.StatusAccepted, map[string]string{"agent": agent, "status": "working"})
	}
}

func runRelayDMTurn(ctx context.Context, st relayDM, prompt string) {
	lock := relayDMLock(st.Agent)
	lock.Lock()
	defer lock.Unlock()
	turnCtx, cancel := context.WithTimeout(ctx, 20*time.Minute)
	defer cancel()
	res, err := invokeRelayDM(turnCtx, chat.Options{
		Agent: st.Agent, Instruction: prompt, Cwd: "", ReadOnly: true,
	}, nil)
	event := relayDMEvent{Speaker: st.Agent, Role: "assistant", Text: strings.TrimSpace(res.Output), Status: "ok"}
	if err != nil {
		event.Status = "error"
		if event.Text == "" {
			event.Text = err.Error()
		}
	}
	if event.Text == "" {
		event.Text = "(agent returned no text)"
	}
	_ = appendRelayDMEvent(st.Agent, event)
	_, _ = ensureRelayDM(st.Agent, st.Human)
}

func handleRelayDMObserve(w http.ResponseWriter, r *http.Request) {
	agent := canonAgent(r.URL.Query().Get("agent"))
	lock := relayDMLock(agent)
	lock.Lock()
	_, ensureErr := ensureRelayDM(agent, actorOf(r, ""))
	lock.Unlock()
	if ensureErr != nil {
		http.Error(w, ensureErr.Error(), http.StatusBadRequest)
		return
	}
	dir, _ := relayDMDir(agent)
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	rec := &lineTail{path: filepath.Join(dir, "transcript.jsonl")}
	write := func(kind string, data any) error { return conn.WriteJSON(wsFrame{Kind: kind, Data: data}) }
	if events, err := relayDMEvents(agent); err == nil {
		for _, event := range events {
			if write("dm-event", event) != nil {
				return
			}
		}
	}
	rec.skipToEnd()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-time.After(observePoll):
		}
		lines, err := rec.next()
		if err != nil {
			continue
		}
		for _, line := range lines {
			var event relayDMEvent
			if json.Unmarshal(line, &event) == nil {
				if write("dm-event", event) != nil {
					return
				}
			}
		}
	}
}
