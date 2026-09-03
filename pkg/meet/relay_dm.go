package meet

// Relay direct messages adapt the governed `bashy chat -m` primitive to the
// web conversation surface. They are deliberately not meetings: no agenda,
// chair, secretary, decisions, or minutes. Chat remains the execution/context
// owner; this file stores only the human-visible transcript Relay must replay.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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

// dmRoomID resolves the ONE durable room two seats share.
//
// This used to be <meetdir>/relay-dms/<agent>/ — a private store nothing else
// read. That was the bug: `bashy meet dm` (the CLI) already writes to the
// permanent dm-<a>-<b> room, which the unified inbox reads as source "meet",
// while the web DM wrote here, which it does not. So one name had two mailboxes
// and only one of them was mail: a web DM was invisible to `bashy inbox --as X`,
// to reachability, and to any spawn that reads its inbox before answering.
//
// The fix is to delete a store rather than add an inbox source. A fourth source
// would have given one name two mailboxes ON PURPOSE.
func dmRoomID(agent, human string) (string, error) {
	name, err := directMessageRoomName(human, agent)
	if err != nil {
		return "", err
	}
	return name, nil
}

// relayDMDir is the room's own store, so the observe tail keeps working
// unchanged: a meet room already keeps its transcript at
// <meetdir>/<id>/transcript.jsonl, which is exactly what this returned before.
func relayDMDir(agent string) (string, error) {
	st, err := dmRoomFor(agent, "")
	if err != nil {
		return "", err
	}
	return storeDir(st.ID)
}

// dmRoomFor loads, or creates, the durable room for this agent and human.
func dmRoomFor(agent, human string) (*State, error) {
	agent = canonAgent(agent)
	if err := routableSeat(agent); err != nil {
		return nil, err
	}
	if strings.TrimSpace(human) == "" {
		human = humanName()
	}
	name, err := dmRoomID(agent, human)
	if err != nil {
		return nil, err
	}
	return EnsurePermanentRoom(name, CreateOptions{
		Topic:        "DM with " + agent,
		Participants: []string{agent},
		Human:        human,
		Initiator:    human,
		// A DM is a conversation, not a meeting: no chair runs the floor and
		// nobody files minutes about two people talking.
		Secretary: "",
	})
}

func ensureRelayDM(agent, human string) (relayDM, error) {
	st, err := dmRoomFor(agent, human)
	if err != nil {
		return relayDM{}, err
	}
	who := strings.TrimSpace(st.Human)
	if who == "" {
		who = human
	}
	return relayDM{Agent: canonAgent(agent), Human: who, Created: st.Created, Updated: nowFn()}, nil
}

func relayDMEvents(agent string) ([]relayDMEvent, error) {
	st, err := dmRoomFor(agent, "")
	if err != nil {
		return nil, err
	}
	events, err := readRoomTranscript(st.ID)
	if err != nil {
		return nil, err
	}
	out := make([]relayDMEvent, 0, len(events))
	for i, e := range events {
		role := "assistant"
		if e.Kind == "human" || strings.EqualFold(e.Speaker, st.Human) {
			role = "user"
		}
		status := ""
		if e.Status != "" {
			status = e.Status
		}
		out = append(out, relayDMEvent{
			ID: fmt.Sprintf("%d", i+1), Speaker: e.Speaker, Role: role,
			Text: e.Text, TS: e.TS, Status: status,
		})
	}
	return out, nil
}

// appendRelayDMEvent writes into the shared room.
//
// The addressee is set deliberately: a human's message names the AGENT, which
// is what makes it directed mail in that agent's unified inbox and what
// `meet dispatch` wakes on. The agent's reply names nobody, which is what stops
// a reply from waking anything and cascading.
func appendRelayDMEvent(agent string, event relayDMEvent) error {
	st, err := dmRoomFor(agent, "")
	if err != nil {
		return err
	}
	ts := event.TS
	if ts.IsZero() {
		ts = nowFn()
	}
	kind, to := "message", ""
	if event.Role == "user" {
		kind, to = "human", canonAgent(agent)
	}
	return AppendEvent(st.ID, Event{
		Round: st.Round, Speaker: event.Speaker, Role: string(RoleParticipant),
		Kind: kind, To: to, Text: event.Text, Status: event.Status, TS: ts,
	})
}

// listRelayDMs enumerates the durable dm-<a>-<b> rooms rather than a private
// directory, so the web list and `bashy meet list` agree about what exists.
func listRelayDMs() ([]relayDM, error) {
	sessions, err := listSessions()
	if err != nil {
		return nil, err
	}
	out := make([]relayDM, 0)
	for _, st := range sessions {
		if !st.Permanent || !strings.HasPrefix(st.Name, "dm-") {
			continue
		}
		agent := ""
		if len(st.Participants) > 0 {
			agent = canonAgent(st.Participants[0])
		}
		if agent == "" {
			continue
		}
		out = append(out, relayDM{
			Agent: agent, Human: st.Human, Created: st.Created, Updated: st.Created,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Agent < out[j].Agent })
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
